package worker

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/openmodu/oneshot/internal/agentrun"
	domainworkspaces "github.com/openmodu/oneshot/internal/domain/workspaces"
)

type Engine interface {
	Available(agentrun.Runtime) bool
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
	id         string
	name       string
	token      string
	workspaces map[string]string
	locks      map[string]*sync.RWMutex
	engine     Engine
	git        GitInspector
	slots      chan struct{}

	mu       sync.Mutex
	running  map[string]*runState
	patches  map[string]pendingPatch
	acked    map[string]string
	ackOrder []string
}

type runState struct {
	cancel      context.CancelFunc
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

// NewServer builds a worker HTTP server. maxConcurrency <= 0 uses
// defaultMaxConcurrency.
func NewServer(id, name, token string, workspaces map[string]string, engine Engine, maxConcurrency int) *Server {
	if maxConcurrency <= 0 {
		maxConcurrency = defaultMaxConcurrency
	}
	locks := make(map[string]*sync.RWMutex, len(workspaces))
	for workspaceID := range workspaces {
		locks[workspaceID] = &sync.RWMutex{}
	}
	return &Server{
		id:         id,
		name:       name,
		token:      token,
		workspaces: workspaces,
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
	mux.HandleFunc("GET /v1/health", s.authorize(s.health))
	mux.HandleFunc("POST /v1/execute", s.authorize(s.execute))
	mux.HandleFunc("POST /v1/runs/{runID}/interrupt", s.authorize(s.interrupt))
	mux.HandleFunc("POST /v1/runs/{runID}/permissions/{requestID}", s.authorize(s.respondPermission))
	mux.HandleFunc("POST /v1/runs/{runID}/patch/ack", s.authorize(s.ackPatch))
	mux.HandleFunc("GET /v1/workspaces/{workspaceID}/git", s.authorize(s.workspaceGit))
	return mux
}

func (s *Server) workspaceGit(writer http.ResponseWriter, request *http.Request) {
	if s.git == nil {
		writeError(writer, http.StatusNotImplemented, "worker_git_unsupported", "git inspection is not enabled on this worker")
		return
	}
	workspace, ok := s.workspaces[request.PathValue("workspaceID")]
	if !ok {
		writeError(writer, http.StatusConflict, "worker_workspace_unmapped", "workspace is not mapped on this worker")
		return
	}
	snapshot, err := s.git.Inspect(request.Context(), workspace)
	if err != nil {
		writeError(writer, http.StatusInternalServerError, "worker_git_failed", "git inspection failed")
		return
	}
	writeJSON(writer, http.StatusOK, snapshot)
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
	writeJSON(writer, http.StatusOK, Health{
		WorkerID: s.id, Name: s.name, ProtocolVersion: 2,
		Runtimes:     map[string]bool{"codex": s.engine.Available(agentrun.RuntimeCodex), "claude": s.engine.Available(agentrun.RuntimeClaude), "modu": s.engine.Available(agentrun.RuntimeModu)},
		Capabilities: map[string]bool{"interactivePermissions": true, "workspaceSync": true},
	})
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
	workspace, ok := s.workspaces[input.WorkspaceID]
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
	state, tracked := s.track(input.RunID, cancel)
	if !tracked {
		writeError(writer, http.StatusConflict, "worker_run_exists", "run id is already in flight")
		return
	}
	defer s.untrack(input.RunID)

	workspaceLock := s.locks[input.WorkspaceID]
	if input.Sandbox == agentrun.SandboxReadOnly {
		workspaceLock.RLock()
		defer workspaceLock.RUnlock()
	} else {
		workspaceLock.Lock()
		defer workspaceLock.Unlock()
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
	if input.Runtime == agentrun.RuntimeClaude {
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
	lock := s.locks[pending.workspaceID]
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

func (s *Server) track(runID string, cancel context.CancelFunc) (*runState, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running[runID] != nil {
		return nil, false
	}
	state := &runState{cancel: cancel, permissions: make(map[string]*pendingPermission)}
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
