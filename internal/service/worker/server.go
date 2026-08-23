package worker

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	domainharnesses "github.com/openmodu/onecatch/internal/domain/harnesses"
	domainworkspaces "github.com/openmodu/onecatch/internal/domain/workspaces"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
)

type Engine interface {
	Available(agentrun.Runtime) bool
	// SupportsInteractivePermissions decides whether this run gets a
	// PermissionHandler, so the worker never installs one for a harness that
	// will not ask.
	SupportsInteractivePermissions(agentrun.Runtime, agentrun.Sandbox) bool
	Run(ctx context.Context, request agentrun.Request, sink agentrun.Sink) (agentrun.Result, error)
}

// GitInspector reads the operational git state of a workspace directory. It is
// optional: a worker without one simply does not serve the git endpoint.
type GitInspector interface {
	Inspect(ctx context.Context, workspace string) (domainworkspaces.GitSnapshot, error)
}

// defaultMaxConcurrency caps simultaneous runs when the operator does not set
// one. A remote worker is usually a single dev machine; a small cap keeps a
// burst of dispatches from spawning unbounded long-lived agent processes.
const defaultMaxConcurrency = 4

var runIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

type Server struct {
	id                string
	name              string
	token             string
	workspaces        map[string]string
	workspacesMu      sync.RWMutex
	workspaceRegistry *WorkspaceRegistry
	locks             map[string]*sync.RWMutex
	engine            Engine
	git               GitInspector
	slots             chan struct{}
	pairing           *pairingState

	mu       sync.Mutex
	running  map[string]*runState
	patches  map[string]pendingPatch
	acked    map[string]string
	ackOrder []string
}

type runState struct {
	cancel      context.CancelFunc
	workspaceID string
	permissions map[string]*pendingPermission
}

type pendingPermission struct {
	request  agentrun.PermissionRequest
	response chan agentrun.PermissionDecision
}

type pendingPatch struct {
	workspaceID string
	workspace   string
	patch       WorkspacePatch
}

// SetGitInspector enables the read-only git status endpoint. Without it, that
// endpoint reports the capability as unavailable.
func (s *Server) SetGitInspector(git GitInspector) { s.git = git }

// EnablePairing exposes a short-lived, one-time bootstrap endpoint. Pairing is
// normally used over HTTPS; allowInsecure is only appropriate for explicitly
// trusted loopback or private-network HTTP.
func (s *Server) EnablePairing(code string, expiresAt time.Time, allowInsecure bool) {
	s.pairing = newPairingState(code, expiresAt, allowInsecure)
}

func (s *Server) SetWorkspaceRegistry(ctx context.Context, registry *WorkspaceRegistry) error {
	items, err := registry.List(ctx)
	if err != nil {
		return err
	}
	s.workspacesMu.Lock()
	defer s.workspacesMu.Unlock()
	s.workspaceRegistry = registry
	for _, item := range items {
		s.workspaces[item.ID] = item.Path
		if s.locks[item.ID] == nil {
			s.locks[item.ID] = &sync.RWMutex{}
		}
	}
	for id, path := range s.workspaces {
		if _, err := registry.Save(ctx, id, path); err != nil {
			return err
		}
	}
	return nil
}

// NewServer builds a worker HTTP server. maxConcurrency <= 0 uses
// defaultMaxConcurrency.
func NewServer(id, name, token string, workspaces map[string]string, engine Engine, maxConcurrency int) *Server {
	if maxConcurrency <= 0 {
		maxConcurrency = defaultMaxConcurrency
	}
	mapped := make(map[string]string, len(workspaces))
	locks := make(map[string]*sync.RWMutex, len(workspaces))
	for workspaceID, path := range workspaces {
		mapped[workspaceID] = path
		locks[workspaceID] = &sync.RWMutex{}
	}
	return &Server{
		id:         id,
		name:       name,
		token:      token,
		workspaces: mapped,
		locks:      locks,
		engine:     engine,
		slots:      make(chan struct{}, maxConcurrency),
		running:    make(map[string]*runState),
		patches:    make(map[string]pendingPatch),
		acked:      make(map[string]string),
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/pair", s.pair)
	mux.HandleFunc("GET /v1/health", s.authorize(s.health))
	mux.HandleFunc("POST /v1/execute", s.authorize(s.execute))
	mux.HandleFunc("POST /v1/runs/{runID}/interrupt", s.authorize(s.interrupt))
	mux.HandleFunc("POST /v1/runs/{runID}/permissions/{requestID}", s.authorize(s.respondPermission))
	mux.HandleFunc("POST /v1/runs/{runID}/patch/ack", s.authorize(s.ackPatch))
	mux.HandleFunc("GET /v1/workspaces", s.authorize(s.listWorkspaces))
	mux.HandleFunc("PUT /v1/workspaces/{workspaceID}", s.authorize(s.prepareWorkspace))
	mux.HandleFunc("DELETE /v1/workspaces/{workspaceID}", s.authorize(s.removeWorkspace))
	mux.HandleFunc("GET /v1/workspaces/{workspaceID}/git", s.authorize(s.workspaceGit))
	return mux
}

func (s *Server) pair(writer http.ResponseWriter, request *http.Request) {
	if s.pairing == nil {
		writeError(writer, http.StatusNotFound, "worker_pairing_unavailable", "worker pairing is not enabled")
		return
	}
	if request.TLS == nil && !s.pairing.allowInsecure {
		writeError(writer, http.StatusUpgradeRequired, "worker_pairing_requires_tls", "worker pairing requires HTTPS")
		return
	}
	var input PairRequest
	decoder := json.NewDecoder(io.LimitReader(request.Body, 1<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil || !s.pairing.consume(input.Code, time.Now()) {
		writeError(writer, http.StatusUnauthorized, "worker_pairing_invalid", "pairing code is invalid or expired")
		return
	}
	health := s.healthValue()
	writeJSON(writer, http.StatusOK, PairResult{
		WorkerID: health.WorkerID, Name: health.Name, Token: s.token,
		ProtocolVersion: health.ProtocolVersion, Runtimes: health.Runtimes, Capabilities: health.Capabilities,
	})
}

func (s *Server) workspaceGit(writer http.ResponseWriter, request *http.Request) {
	if s.git == nil {
		writeError(writer, http.StatusNotImplemented, "worker_git_unsupported", "git inspection is not enabled on this worker")
		return
	}
	workspace, workspaceLock, ok := s.workspaceState(request.PathValue("workspaceID"))
	if !ok {
		writeError(writer, http.StatusConflict, "worker_workspace_unmapped", "workspace is not mapped on this worker")
		return
	}
	workspaceLock.RLock()
	defer workspaceLock.RUnlock()
	snapshot, err := s.git.Inspect(request.Context(), workspace)
	if err != nil {
		writeError(writer, http.StatusInternalServerError, "worker_git_failed", "git inspection failed")
		return
	}
	writeJSON(writer, http.StatusOK, snapshot)
}

func (s *Server) listWorkspaces(writer http.ResponseWriter, request *http.Request) {
	stored := map[string]WorkspaceMapping{}
	if s.workspaceRegistry != nil {
		items, err := s.workspaceRegistry.List(request.Context())
		if err != nil {
			writeError(writer, http.StatusInternalServerError, "worker_workspace_list_failed", "could not read workspace mappings")
			return
		}
		for _, item := range items {
			stored[item.ID] = item
		}
	}
	s.workspacesMu.RLock()
	items := make([]WorkspaceMapping, 0, len(s.workspaces))
	for id, path := range s.workspaces {
		mapping := stored[id]
		mapping.ID = id
		mapping.Path = path
		if strings.TrimSpace(mapping.Name) == "" {
			mapping.Name = id
		}
		if s.workspaceRegistry != nil {
			mapping.Managed = s.workspaceRegistry.IsManagedPath(id, path)
		}
		items = append(items, mapping)
	}
	s.workspacesMu.RUnlock()
	for index := range items {
		if items[index].RemoteURL != "" && items[index].Revision != "" {
			continue
		}
		_, workspaceLock, ok := s.workspaceState(items[index].ID)
		if !ok {
			continue
		}
		workspaceLock.RLock()
		remoteURL, revision := workspaceIdentity(request.Context(), items[index].Path)
		workspaceLock.RUnlock()
		if items[index].RemoteURL == "" {
			items[index].RemoteURL = remoteURL
		}
		if items[index].Revision == "" {
			items[index].Revision = revision
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	writeJSON(writer, http.StatusOK, items)
}

func (s *Server) prepareWorkspace(writer http.ResponseWriter, request *http.Request) {
	if s.workspaceRegistry == nil {
		writeError(writer, http.StatusNotImplemented, "worker_workspace_management_unsupported", "persistent workspace management is not enabled on this worker")
		return
	}
	workspaceID := request.PathValue("workspaceID")
	if !runIDPattern.MatchString(workspaceID) {
		writeError(writer, http.StatusBadRequest, "worker_workspace_prepare_invalid", "workspace id is invalid")
		return
	}
	if s.workspaceInUse(workspaceID) {
		writeError(writer, http.StatusConflict, "worker_workspace_busy", "the workspace has an active run or an unacknowledged patch")
		return
	}
	workspaceLock := s.ensureWorkspaceLock(workspaceID)
	workspaceLock.Lock()
	defer workspaceLock.Unlock()
	if s.workspaceInUse(workspaceID) {
		writeError(writer, http.StatusConflict, "worker_workspace_busy", "the workspace has an active run or an unacknowledged patch")
		return
	}
	var input WorkspacePrepareRequest
	decoder := json.NewDecoder(io.LimitReader(request.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeError(writer, http.StatusBadRequest, "worker_workspace_prepare_invalid", "invalid workspace preparation request")
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	input.Path = strings.TrimSpace(input.Path)
	input.RemoteURL = strings.TrimSpace(input.RemoteURL)
	input.Revision = strings.TrimSpace(input.Revision)
	workspacePath, _, mapped := s.workspaceState(workspaceID)
	if input.Path != "" {
		if !filepath.IsAbs(input.Path) {
			writeError(writer, http.StatusBadRequest, "worker_workspace_prepare_invalid", "workspace path must be absolute")
			return
		}
		workspacePath = filepath.Clean(input.Path)
	} else if !mapped {
		workspacePath = s.workspaceRegistry.DefaultPath(workspaceID)
	}
	if !s.workspacePathAvailable(workspaceID, workspacePath) {
		writeError(writer, http.StatusConflict, "worker_workspace_path_mapped", "the workspace path is already mapped with another id")
		return
	}
	remoteURL := input.RemoteURL
	revision := input.Revision
	if remoteURL != "" {
		if revision == "" {
			revision = "HEAD"
		}
		if remoteErr := prepareGitWorkspace(request.Context(), workspacePath, remoteURL, revision); remoteErr != nil {
			writeError(writer, http.StatusConflict, remoteErr.Code, remoteErr.Message)
			return
		}
		resolvedRemote, resolvedRevision, remoteErr := prepareExistingWorkspace(request.Context(), workspacePath, "")
		if remoteErr != nil {
			writeError(writer, http.StatusConflict, remoteErr.Code, remoteErr.Message)
			return
		}
		remoteURL, revision = resolvedRemote, resolvedRevision
	} else {
		resolvedRemote, resolvedRevision, remoteErr := prepareExistingWorkspace(request.Context(), workspacePath, revision)
		if remoteErr != nil {
			writeError(writer, http.StatusConflict, remoteErr.Code, remoteErr.Message)
			return
		}
		remoteURL, revision = resolvedRemote, resolvedRevision
	}
	mapping, err := s.workspaceRegistry.SaveMapping(request.Context(), WorkspaceMapping{
		ID: workspaceID, Name: input.Name, Path: workspacePath, RemoteURL: remoteURL, Revision: revision,
	})
	if err != nil {
		writeError(writer, http.StatusInternalServerError, "worker_workspace_persist_failed", "could not persist the workspace mapping")
		return
	}
	s.workspacesMu.Lock()
	s.workspaces[workspaceID] = mapping.Path
	if s.locks[workspaceID] == nil {
		s.locks[workspaceID] = &sync.RWMutex{}
	}
	s.workspacesMu.Unlock()
	var snapshot domainworkspaces.GitSnapshot
	if s.git != nil {
		snapshot, err = s.git.Inspect(request.Context(), mapping.Path)
		if err != nil {
			writeError(writer, http.StatusInternalServerError, "worker_git_failed", "git inspection failed after preparing the workspace")
			return
		}
	}
	writeJSON(writer, http.StatusOK, WorkspacePrepareResult{Mapping: mapping, Git: snapshot})
}

func (s *Server) removeWorkspace(writer http.ResponseWriter, request *http.Request) {
	if s.workspaceRegistry == nil {
		writeError(writer, http.StatusNotImplemented, "worker_workspace_management_unsupported", "persistent workspace management is not enabled on this worker")
		return
	}
	workspaceID := request.PathValue("workspaceID")
	if !runIDPattern.MatchString(workspaceID) {
		writeError(writer, http.StatusBadRequest, "worker_workspace_remove_invalid", "workspace id is invalid")
		return
	}
	workspacePath, workspaceLock, ok := s.workspaceState(workspaceID)
	if !ok {
		writeError(writer, http.StatusNotFound, "worker_workspace_unmapped", "workspace is not mapped on this worker")
		return
	}
	workspaceLock.Lock()
	defer workspaceLock.Unlock()
	if s.workspaceInUse(workspaceID) {
		writeError(writer, http.StatusConflict, "worker_workspace_busy", "the workspace has an active run or an unacknowledged patch")
		return
	}
	deleteFiles := request.URL.Query().Get("deleteFiles") == "true"
	if deleteFiles {
		if !s.workspaceRegistry.IsManagedPath(workspaceID, workspacePath) {
			writeError(writer, http.StatusBadRequest, "worker_workspace_delete_forbidden", "only Worker-managed clones can be deleted")
			return
		}
		if _, _, remoteErr := prepareExistingWorkspace(request.Context(), workspacePath, ""); remoteErr != nil {
			writeError(writer, http.StatusConflict, remoteErr.Code, remoteErr.Message)
			return
		}
		if err := os.RemoveAll(workspacePath); err != nil {
			writeError(writer, http.StatusInternalServerError, "worker_workspace_delete_failed", "could not delete the managed workspace clone")
			return
		}
	}
	if err := s.workspaceRegistry.Delete(request.Context(), workspaceID); err != nil {
		writeError(writer, http.StatusInternalServerError, "worker_workspace_persist_failed", "could not remove the workspace mapping")
		return
	}
	s.workspacesMu.Lock()
	delete(s.workspaces, workspaceID)
	delete(s.locks, workspaceID)
	s.workspacesMu.Unlock()
	writeJSON(writer, http.StatusOK, map[string]any{"id": workspaceID, "deletedFiles": deleteFiles})
}

func (s *Server) workspaceState(id string) (string, *sync.RWMutex, bool) {
	s.workspacesMu.RLock()
	defer s.workspacesMu.RUnlock()
	path, ok := s.workspaces[id]
	return path, s.locks[id], ok
}

func (s *Server) ensureWorkspaceLock(id string) *sync.RWMutex {
	s.workspacesMu.Lock()
	defer s.workspacesMu.Unlock()
	if s.locks[id] == nil {
		s.locks[id] = &sync.RWMutex{}
	}
	return s.locks[id]
}

func (s *Server) workspaceInUse(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, state := range s.running {
		if state.workspaceID == id {
			return true
		}
	}
	for _, patch := range s.patches {
		if patch.workspaceID == id {
			return true
		}
	}
	return false
}

func (s *Server) workspaceMatches(id, path string) bool {
	s.workspacesMu.RLock()
	defer s.workspacesMu.RUnlock()
	return s.workspaces[id] == path
}

func (s *Server) workspacePathAvailable(id, path string) bool {
	s.workspacesMu.RLock()
	defer s.workspacesMu.RUnlock()
	wanted := filepath.Clean(path)
	for currentID, currentPath := range s.workspaces {
		if currentID != id && pathsOverlap(filepath.Clean(currentPath), wanted) {
			return false
		}
	}
	return true
}

func pathsOverlap(left, right string) bool {
	for _, pair := range [][2]string{{left, right}, {right, left}} {
		relative, err := filepath.Rel(pair[0], pair[1])
		if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func (s *Server) authorize(next http.HandlerFunc) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		provided := strings.TrimPrefix(request.Header.Get("Authorization"), "Bearer ")
		if provided == "" || len(provided) != len(s.token) || subtle.ConstantTimeCompare([]byte(provided), []byte(s.token)) != 1 {
			writeError(writer, http.StatusUnauthorized, "worker_unauthorized", "invalid worker token")
			return
		}
		next(writer, request)
	}
}

func (s *Server) health(writer http.ResponseWriter, _ *http.Request) {
	writeJSON(writer, http.StatusOK, s.healthValue())
}

func (s *Server) healthValue() Health {
	return Health{
		WorkerID: s.id, Name: s.name, ProtocolVersion: 3,
		Runtimes:     workerRuntimeAvailability(s.engine),
		Capabilities: map[string]bool{"interactivePermissions": true, "workspaceSync": true, "workspaceManagement": s.workspaceRegistry != nil, "pairing": s.pairing != nil},
	}
}

// execute streams the run as NDJSON: one Frame per line. Pre-flight failures
// (bad request, unknown workspace/runtime, no free slot) are ordinary HTTP
// errors sent before the stream opens. Once the run starts, the response is a
// committed 200 stream, so a mid-run failure arrives as a terminal Error frame
// rather than an HTTP status.
func (s *Server) execute(writer http.ResponseWriter, request *http.Request) {
	var input ExecuteRequest
	decoder := json.NewDecoder(io.LimitReader(request.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeError(writer, http.StatusBadRequest, "worker_invalid_request", "invalid execute request")
		return
	}
	if !runIDPattern.MatchString(input.RunID) {
		writeError(writer, http.StatusBadRequest, "worker_invalid_request", "run id is missing or malformed")
		return
	}
	workspace, workspaceLock, ok := s.workspaceState(input.WorkspaceID)
	if !ok {
		writeError(writer, http.StatusConflict, "worker_workspace_unmapped", "workspace is not mapped on this worker")
		return
	}
	if !s.engine.Available(input.Runtime) {
		writeError(writer, http.StatusConflict, "worker_runtime_unavailable", "runtime is unavailable on this worker")
		return
	}
	if input.Sandbox == agentrun.SandboxFull {
		writeError(writer, http.StatusConflict, "worker_full_sandbox_unsupported", "remote workers can synchronize workspace-write changes only")
		return
	}
	runTimeout := 35 * time.Minute
	if input.TimeoutSeconds < 0 || time.Duration(input.TimeoutSeconds)*time.Second > MaxRunDuration {
		writeError(writer, http.StatusBadRequest, "worker_invalid_request", "timeout must be omitted or between 1 second and 24 hours")
		return
	}
	if input.TimeoutSeconds > 0 {
		runTimeout = time.Duration(input.TimeoutSeconds) * time.Second
	}
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	default:
		writeError(writer, http.StatusTooManyRequests, "worker_busy", "worker is at capacity")
		return
	}

	flusher, ok := writer.(http.Flusher)
	if !ok {
		writeError(writer, http.StatusInternalServerError, "worker_execution_failed", "streaming is unsupported")
		return
	}

	// Register a cancellable context so an interrupt call can stop this run.
	runCtx, cancel := context.WithTimeout(request.Context(), runTimeout)
	defer cancel()
	state, tracked := s.track(input.RunID, input.WorkspaceID, cancel)
	if !tracked {
		writeError(writer, http.StatusConflict, "worker_run_exists", "run id is already in flight")
		return
	}
	defer s.untrack(input.RunID)

	if input.Sandbox == agentrun.SandboxReadOnly {
		workspaceLock.RLock()
		defer workspaceLock.RUnlock()
		if !s.workspaceMatches(input.WorkspaceID, workspace) {
			writeError(writer, http.StatusConflict, "worker_workspace_unmapped", "workspace is no longer mapped on this worker")
			return
		}
		if input.BaseRevision != "" {
			if err := validateWorkspaceBaseline(request.Context(), workspace, input.BaseRevision); err != nil {
				writeError(writer, http.StatusConflict, err.Code, err.Message)
				return
			}
		}
	} else {
		workspaceLock.Lock()
		defer workspaceLock.Unlock()
		if !s.workspaceMatches(input.WorkspaceID, workspace) {
			writeError(writer, http.StatusConflict, "worker_workspace_unmapped", "workspace is no longer mapped on this worker")
			return
		}
		if !input.SyncChanges || input.BaseRevision == "" {
			writeError(writer, http.StatusConflict, "worker_write_sync_required", "writable remote runs require workspace synchronization")
			return
		}
		if err := validateWorkspaceBaseline(request.Context(), workspace, input.BaseRevision); err != nil {
			writeError(writer, http.StatusConflict, err.Code, err.Message)
			return
		}
	}

	writer.Header().Set("Content-Type", "application/x-ndjson")
	writer.WriteHeader(http.StatusOK)
	flusher.Flush()

	encoder := json.NewEncoder(writer)
	var writeErr error
	send := func(frame Frame) {
		if writeErr != nil {
			return
		}
		if writeErr = encoder.Encode(frame); writeErr == nil {
			flusher.Flush()
		}
	}

	runRequest := agentrun.Request{
		Runtime:                 input.Runtime,
		Workspace:               workspace,
		Prompt:                  input.Prompt,
		Model:                   input.Model,
		ReasoningEffort:         input.ReasoningEffort,
		ServiceTier:             input.ServiceTier,
		Provider:                input.Provider,
		Sandbox:                 input.Sandbox,
		ResumeSessionID:         input.ResumeSessionID,
		Environment:             workerEnvironment(input.EnvironmentAllowlist),
		EnvironmentAllowlist:    append([]string{}, input.EnvironmentAllowlist...),
		InterruptGrace:          time.Duration(input.InterruptGraceSeconds) * time.Second,
		RuntimeDefaultsResolved: true,
	}
	if s.engine.SupportsInteractivePermissions(input.Runtime, input.Sandbox) {
		runRequest.PermissionHandler = func(ctx context.Context, permission agentrun.PermissionRequest) (agentrun.PermissionDecision, error) {
			return s.awaitPermission(ctx, state, permission)
		}
	}
	result, runErr := s.engine.Run(runCtx, runRequest, func(event agentrun.Event) {
		if event.Kind == agentrun.KindPermissionRequest && event.Permission != nil {
			s.registerPermission(state, *event.Permission)
		}
		e := event
		send(Frame{Event: &e})
	})
	if input.Sandbox != agentrun.SandboxReadOnly {
		patchCtx, cancelPatch := context.WithTimeout(context.Background(), 2*time.Minute)
		patch, patchErr := buildWorkspacePatch(patchCtx, workspace, input.BaseRevision)
		cancelPatch()
		if patchErr != nil {
			send(Frame{Error: patchErr})
			return
		}
		if patch != nil {
			s.mu.Lock()
			s.patches[input.RunID] = pendingPatch{workspaceID: input.WorkspaceID, workspace: workspace, patch: *patch}
			s.mu.Unlock()
			send(Frame{Patch: patch})
		}
	}

	switch {
	case request.Context().Err() != nil:
		// The client hung up; nothing left to tell it.
		return
	case runErr != nil && errors.Is(runErr, context.Canceled):
		// Graceful interrupt: hand back whatever the agent produced before it stopped.
		send(Frame{Result: &result})
	case runErr != nil:
		send(Frame{Error: &RemoteError{Code: "worker_execution_failed", Message: "remote agent execution failed"}})
	default:
		send(Frame{Result: &result})
	}
}

func (s *Server) ackPatch(writer http.ResponseWriter, request *http.Request) {
	var input PatchAckRequest
	decoder := json.NewDecoder(io.LimitReader(request.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil || input.Digest == "" {
		writeError(writer, http.StatusBadRequest, "worker_patch_ack_invalid", "invalid patch acknowledgement")
		return
	}
	runID := request.PathValue("runID")
	s.mu.Lock()
	pending, ok := s.patches[runID]
	ackedDigest := s.acked[runID]
	s.mu.Unlock()
	if !ok && ackedDigest == input.Digest {
		writer.WriteHeader(http.StatusNoContent)
		return
	}
	if !ok || pending.patch.Digest != input.Digest {
		writeError(writer, http.StatusNotFound, "worker_patch_not_found", "no matching patch is awaiting acknowledgement")
		return
	}
	_, lock, ok := s.workspaceState(pending.workspaceID)
	if !ok {
		writeError(writer, http.StatusConflict, "worker_workspace_unmapped", "workspace is not mapped on this worker")
		return
	}
	lock.Lock()
	defer lock.Unlock()
	current, err := buildWorkspacePatch(request.Context(), pending.workspace, pending.patch.BaseRevision)
	if err != nil {
		writeError(writer, http.StatusConflict, err.Code, err.Message)
		return
	}
	if current == nil || current.Digest != pending.patch.Digest {
		writeError(writer, http.StatusConflict, "worker_patch_changed", "remote workspace changed after patch delivery; changes were preserved")
		return
	}
	if cleanupErr := cleanWorkspace(request.Context(), pending.workspace, pending.patch); cleanupErr != nil {
		writeError(writer, http.StatusInternalServerError, cleanupErr.Code, cleanupErr.Message)
		return
	}
	s.mu.Lock()
	if stored, exists := s.patches[runID]; exists && stored.patch.Digest == input.Digest {
		delete(s.patches, runID)
		s.acked[runID] = input.Digest
		s.ackOrder = append(s.ackOrder, runID)
		if len(s.ackOrder) > 1024 {
			expired := s.ackOrder[0]
			s.ackOrder = s.ackOrder[1:]
			delete(s.acked, expired)
		}
	}
	s.mu.Unlock()
	writer.WriteHeader(http.StatusNoContent)
}

func workerEnvironment(allowlist []string) []string {
	if allowlist == nil {
		return nil
	}
	keys := append([]string{"PATH", "HOME", "TMPDIR", "USER", "LANG", "LC_ALL"}, allowlist...)
	seen := make(map[string]struct{}, len(keys))
	environment := make([]string, 0, len(keys))
	for _, key := range keys {
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		if value, ok := os.LookupEnv(key); ok {
			environment = append(environment, key+"="+value)
		}
	}
	return environment
}

func (s *Server) interrupt(writer http.ResponseWriter, request *http.Request) {
	runID := request.PathValue("runID")
	s.mu.Lock()
	state := s.running[runID]
	s.mu.Unlock()
	if state == nil {
		writeError(writer, http.StatusNotFound, "worker_run_not_found", "no such in-flight run")
		return
	}
	state.cancel()
	writer.WriteHeader(http.StatusNoContent)
}

func (s *Server) respondPermission(writer http.ResponseWriter, request *http.Request) {
	var input PermissionResponse
	decoder := json.NewDecoder(io.LimitReader(request.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeError(writer, http.StatusBadRequest, "permission_decision_invalid", "invalid permission response")
		return
	}
	runID, requestID := request.PathValue("runID"), request.PathValue("requestID")
	s.mu.Lock()
	state := s.running[runID]
	var pending *pendingPermission
	if state != nil {
		pending = state.permissions[requestID]
	}
	if pending == nil {
		s.mu.Unlock()
		writeError(writer, http.StatusNotFound, "permission_not_pending", "permission request is no longer pending")
		return
	}
	decision, err := permissionDecision(input.Decision, pending.request)
	if err != nil {
		s.mu.Unlock()
		writeError(writer, http.StatusBadRequest, err.Code, err.Message)
		return
	}
	select {
	case pending.response <- decision:
		s.mu.Unlock()
		writer.WriteHeader(http.StatusNoContent)
	default:
		s.mu.Unlock()
		writeError(writer, http.StatusConflict, "permission_not_pending", "permission request was already answered")
	}
}

func permissionDecision(decision string, permission agentrun.PermissionRequest) (agentrun.PermissionDecision, *RemoteError) {
	switch decision {
	case "allow_once":
		return agentrun.PermissionDecision{Behavior: "allow", DecisionClassification: "user_temporary"}, nil
	case "allow_always":
		if permission.SuppressAlwaysAllow || len(permission.Suggestions) == 0 {
			return agentrun.PermissionDecision{}, &RemoteError{Code: "permission_persistent_unavailable", Message: "this request cannot be permanently allowed"}
		}
		return agentrun.PermissionDecision{Behavior: "allow", UpdatedPermissions: permission.Suggestions, DecisionClassification: "user_permanent"}, nil
	case "deny":
		return agentrun.PermissionDecision{Behavior: "deny", Message: "Permission denied by user", DecisionClassification: "user_reject"}, nil
	default:
		return agentrun.PermissionDecision{}, &RemoteError{Code: "permission_decision_invalid", Message: "permission decision must be allow_once, allow_always or deny"}
	}
}

func (s *Server) registerPermission(state *runState, permission agentrun.PermissionRequest) *pendingPermission {
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing := state.permissions[permission.ID]; existing != nil {
		return existing
	}
	pending := &pendingPermission{request: permission, response: make(chan agentrun.PermissionDecision, 1)}
	state.permissions[permission.ID] = pending
	return pending
}

func (s *Server) awaitPermission(ctx context.Context, state *runState, permission agentrun.PermissionRequest) (agentrun.PermissionDecision, error) {
	pending := s.registerPermission(state, permission)
	defer func() {
		s.mu.Lock()
		if state.permissions[permission.ID] == pending {
			delete(state.permissions, permission.ID)
		}
		s.mu.Unlock()
	}()
	select {
	case decision := <-pending.response:
		return decision, nil
	case <-ctx.Done():
		return agentrun.PermissionDecision{}, ctx.Err()
	}
}

func (s *Server) track(runID, workspaceID string, cancel context.CancelFunc) (*runState, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running[runID] != nil {
		return nil, false
	}
	state := &runState{cancel: cancel, workspaceID: workspaceID, permissions: make(map[string]*pendingPermission)}
	s.running[runID] = state
	return state, true
}

func (s *Server) untrack(runID string) {
	s.mu.Lock()
	state := s.running[runID]
	if state != nil {
		delete(s.running, runID)
	}
	s.mu.Unlock()
}

func writeError(writer http.ResponseWriter, status int, code, message string) {
	writeJSON(writer, status, RemoteError{Code: code, Message: message})
}
func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

// workerRuntimeAvailability reports which harnesses this worker can actually
// run, so the host never dispatches a step to a worker that lacks the CLI. It
// covers the whole catalog, so a harness added to the product is reported here
// without an edit.
func workerRuntimeAvailability(engine Engine) map[string]bool {
	catalog := domainharnesses.Catalog()
	available := make(map[string]bool, len(catalog))
	for _, harness := range catalog {
		available[harness.ID] = engine.Available(agentrun.Runtime(harness.ID))
	}
	return available
}
