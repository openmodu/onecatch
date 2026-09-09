// Package mobile provides the remote-worker-only application service used by
// the iOS and Android clients. It exposes remote Worker resources without
// granting the mobile process access to local runtimes or worktrees.
package mobile

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	domainworkspaces "github.com/openmodu/onecatch/internal/domain/workspaces"
	"github.com/openmodu/onecatch/internal/service/worker"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
	"github.com/openmodu/onecatch/pkg/localfile"
)

const RunEventName = "mobile:run"

type WorkerStatus struct {
	Worker              worker.Info   `json:"worker"`
	Health              worker.Health `json:"health"`
	CheckedAt           time.Time     `json:"checkedAt"`
	LatencyMilliseconds int64         `json:"latencyMilliseconds"`
}

type StartRunInput struct {
	WorkerID        string `json:"workerId"`
	WorkspaceID     string `json:"workspaceId"`
	ConversationID  string `json:"conversationId,omitempty"`
	Runtime         string `json:"runtime"`
	Prompt          string `json:"prompt"`
	Model           string `json:"model,omitempty"`
	ReasoningEffort string `json:"reasoningEffort,omitempty"`
	ServiceTier     string `json:"serviceTier,omitempty"`
	Provider        string `json:"provider,omitempty"`
	ResumeSessionID string `json:"resumeSessionId,omitempty"`
	TimeoutSeconds  int    `json:"timeoutSeconds,omitempty"`
}

type PermissionDecisionInput struct {
	RunID     string `json:"runId"`
	RequestID string `json:"requestId"`
	Decision  string `json:"decision"`
}

type RunView = worker.SharedRun

type RunFrame struct {
	RunID  string           `json:"runId"`
	Status string           `json:"status,omitempty"`
	Event  *agentrun.Event  `json:"event,omitempty"`
	Result *agentrun.Result `json:"result,omitempty"`
	Error  string           `json:"error,omitempty"`
	At     time.Time        `json:"at"`
}

type runState struct {
	view   RunView
	config worker.Config
	cancel context.CancelFunc
	// window is the transcript index the page last handed to the UI starts
	// at. A local run keeps its whole transcript in view; this is what the
	// phone has actually been given.
	window int
}

// transcriptWindow bounds what opening a conversation loads. A long session's
// transcript is unbounded, and every entry costs a bridge round trip and a
// mounted component on the phone, so the newest page arrives first and earlier
// ones are fetched only when the reader asks for them.
const transcriptWindow = 200

type Service struct {
	registry *worker.Registry
	client   *worker.Client
	runsPath string

	syncMu  sync.Mutex
	mu      sync.RWMutex
	runs    map[string]*runState
	emitter func(RunFrame)
}

func NewService(root string) (*Service, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, errors.New("mobile data root is required")
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, err
	}
	service := &Service{
		registry: worker.NewRegistry(filepath.Join(root, "workers.json")),
		client:   worker.NewClient(),
		runsPath: filepath.Join(root, "runs.json"),
		runs:     make(map[string]*runState),
	}
	if err := service.loadRuns(); err != nil {
		return nil, err
	}
	return service, nil
}

func (s *Service) loadRuns() error {
	var views []RunView
	if err := localfile.ReadJSON(s.runsPath, &views); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	changed := false
	for _, view := range views {
		if strings.TrimSpace(view.ID) == "" {
			continue
		}
		if view.Status == "running" && !view.Shared {
			finishedAt := time.Now().UTC()
			view.Status = "failed"
			view.FinishedAt = &finishedAt
			view.Error = "the app closed before this run returned a terminal result"
			changed = true
		}
		if strings.TrimSpace(view.ConversationID) == "" {
			view.ConversationID = view.ID
			changed = true
		}
		s.runs[view.ID] = &runState{view: view}
	}
	if changed {
		return s.persistRunsLocked()
	}
	return nil
}

func (s *Service) SetEmitter(emitter func(RunFrame)) {
	s.mu.Lock()
	s.emitter = emitter
	s.mu.Unlock()
}

func (s *Service) ListWorkers(ctx context.Context) ([]worker.Info, error) {
	return s.registry.List(ctx)
}

func (s *Service) PairWorker(ctx context.Context, baseURL, code string) (worker.Info, error) {
	paired, err := s.client.Pair(ctx, baseURL, code)
	if err != nil {
		return worker.Info{}, err
	}
	return s.registry.Save(ctx, worker.Input{
		ID: paired.WorkerID, Name: paired.Name, BaseURL: baseURL, Token: paired.Token,
		ServerCertificateSHA256: paired.ServerCertificateSHA256, Enabled: true,
	})
}

func (s *Service) DeleteWorker(ctx context.Context, id string) error {
	return s.registry.Delete(ctx, strings.TrimSpace(id))
}

func (s *Service) CheckWorker(ctx context.Context, id string) (WorkerStatus, error) {
	config, err := s.enabledWorker(ctx, id)
	if err != nil {
		return WorkerStatus{}, err
	}
	startedAt := time.Now()
	health, err := s.client.Health(ctx, config)
	if err != nil {
		return WorkerStatus{}, err
	}
	return WorkerStatus{
		Worker:              workerInfo(config),
		Health:              health,
		CheckedAt:           time.Now().UTC(),
		LatencyMilliseconds: time.Since(startedAt).Milliseconds(),
	}, nil
}

func (s *Service) ListWorkspaces(ctx context.Context, workerID string) ([]worker.WorkspaceMapping, error) {
	config, err := s.enabledWorker(ctx, workerID)
	if err != nil {
		return nil, err
	}
	return s.client.ListWorkspaces(ctx, config)
}

func (s *Service) PrepareWorkspace(ctx context.Context, workerID, workspaceID string, input worker.WorkspacePrepareRequest) (worker.WorkspacePrepareResult, error) {
	config, err := s.enabledWorker(ctx, workerID)
	if err != nil {
		return worker.WorkspacePrepareResult{}, err
	}
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return worker.WorkspacePrepareResult{}, worker.RemoteError{Code: "mobile_workspace_invalid", Message: "workspace id is required"}
	}
	return s.client.PrepareWorkspace(ctx, config, workspaceID, input)
}

func (s *Service) RemoveWorkspace(ctx context.Context, workerID, workspaceID string, deleteFiles bool) error {
	config, err := s.enabledWorker(ctx, workerID)
	if err != nil {
		return err
	}
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return worker.RemoteError{Code: "mobile_workspace_invalid", Message: "workspace id is required"}
	}
	return s.client.RemoveWorkspace(ctx, config, workspaceID, deleteFiles)
}

func (s *Service) WorkspaceGitStatus(ctx context.Context, workerID, workspaceID string) (domainworkspaces.GitSnapshot, error) {
	config, err := s.enabledWorker(ctx, workerID)
	if err != nil {
		return domainworkspaces.GitSnapshot{}, err
	}
	return s.client.GitStatus(ctx, config, strings.TrimSpace(workspaceID))
}

// StartRun uses the host's task service when available. Legacy standalone
// workers retain the read-only clone protocol, since a phone cannot apply
// their writable patches to a coordinator worktree.
func (s *Service) StartRun(ctx context.Context, input StartRunInput) (RunView, error) {
	input.WorkerID = strings.TrimSpace(input.WorkerID)
	input.WorkspaceID = strings.TrimSpace(input.WorkspaceID)
	input.Runtime = strings.TrimSpace(input.Runtime)
	input.Prompt = strings.TrimSpace(input.Prompt)
	if input.WorkerID == "" || input.WorkspaceID == "" || input.Prompt == "" {
		return RunView{}, worker.RemoteError{Code: "mobile_run_invalid", Message: "worker, workspace and prompt are required"}
	}
	runtimeName := agentrun.Runtime(input.Runtime)
	if !runtimeName.Valid() {
		return RunView{}, worker.RemoteError{Code: "mobile_runtime_invalid", Message: "runtime must be codex, claude or modu"}
	}
	if input.TimeoutSeconds < 0 || time.Duration(input.TimeoutSeconds)*time.Second > worker.MaxRunDuration {
		return RunView{}, worker.RemoteError{Code: "mobile_timeout_invalid", Message: "timeout must be omitted or between 1 second and 24 hours"}
	}
	config, err := s.enabledWorker(ctx, input.WorkerID)
	if err != nil {
		return RunView{}, err
	}
	health, err := s.client.Health(ctx, config)
	if err != nil {
		return RunView{}, err
	}
	if !health.Runtimes[input.Runtime] {
		return RunView{}, worker.RemoteError{Code: "worker_runtime_unavailable", Message: "runtime is unavailable on this worker"}
	}
	sharedWorkspace := false
	if health.Capabilities["sharedRuns"] {
		mappings, err := s.client.ListWorkspaces(ctx, config)
		if err != nil {
			return RunView{}, err
		}
		for _, mapping := range mappings {
			if mapping.ID == input.WorkspaceID {
				sharedWorkspace = mapping.Shared
				break
			}
		}
	}
	if sharedWorkspace {
		s.syncMu.Lock()
		defer s.syncMu.Unlock()
		view, err := s.client.StartSharedRun(ctx, config, worker.SharedRunInput{WorkspaceID: input.WorkspaceID,
			ConversationID: input.ConversationID, Runtime: input.Runtime, Prompt: input.Prompt,
			Model: input.Model, ReasoningEffort: input.ReasoningEffort, ServiceTier: input.ServiceTier})
		if err != nil {
			return RunView{}, err
		}
		view.WorkerID, view.Shared = config.ID, true
		s.cacheSharedRun(config, view)
		return view, nil
	}
	snapshot, err := s.client.GitStatus(ctx, config, input.WorkspaceID)
	if err != nil {
		return RunView{}, err
	}
	if !snapshot.IsRepo || snapshot.Head == "" {
		return RunView{}, worker.RemoteError{Code: "mobile_workspace_git_required", Message: "the mapped workspace must be a Git repository with a commit"}
	}
	if strings.TrimSpace(snapshot.Status) != "" || len(snapshot.Files) > 0 {
		return RunView{}, worker.RemoteError{Code: "mobile_workspace_dirty", Message: "the mapped workspace must be clean before a mobile run"}
	}
	runID, err := newRunID()
	if err != nil {
		return RunView{}, err
	}
	conversationID := strings.TrimSpace(input.ConversationID)
	if conversationID == "" {
		conversationID = runID
	}
	runCtx, cancel := context.WithCancel(context.Background())
	state := &runState{
		config: config,
		cancel: cancel,
		view: RunView{
			ID: runID, ConversationID: conversationID, WorkerID: input.WorkerID, WorkspaceID: input.WorkspaceID,
			Runtime: runtimeName, Prompt: input.Prompt, Status: "running",
			Events: []agentrun.Event{}, StartedAt: time.Now().UTC(),
		},
	}
	s.mu.Lock()
	s.runs[runID] = state
	if err := s.persistRunsLocked(); err != nil {
		delete(s.runs, runID)
		s.mu.Unlock()
		cancel()
		return RunView{}, err
	}
	s.mu.Unlock()
	s.emit(RunFrame{RunID: runID, Status: "running", At: state.view.StartedAt})
	view := copyRunView(state.view)
	go s.execute(runCtx, state, input, snapshot.Head)
	return view, nil
}

func (s *Service) execute(ctx context.Context, state *runState, input StartRunInput, baseRevision string) {
	result, err := s.client.Execute(ctx, state.config, worker.ExecuteRequest{
		RunID: state.view.ID, WorkspaceID: state.view.WorkspaceID,
		Runtime: state.view.Runtime, Model: strings.TrimSpace(input.Model),
		ReasoningEffort: strings.TrimSpace(input.ReasoningEffort),
		ServiceTier:     strings.TrimSpace(input.ServiceTier), Provider: strings.TrimSpace(input.Provider),
		Sandbox: agentrun.SandboxReadOnly, Prompt: state.view.Prompt,
		ResumeSessionID: strings.TrimSpace(input.ResumeSessionID), TimeoutSeconds: input.TimeoutSeconds,
		InterruptGraceSeconds: 8, BaseRevision: baseRevision,
	}, func(event agentrun.Event) {
		s.mu.Lock()
		state.view.Events = append(state.view.Events, event)
		if len(state.view.Events) > 2000 {
			state.view.Events = append([]agentrun.Event{}, state.view.Events[len(state.view.Events)-2000:]...)
		}
		s.mu.Unlock()
		e := event
		s.emit(RunFrame{RunID: state.view.ID, Event: &e, At: time.Now().UTC()})
	})

	finishedAt := time.Now().UTC()
	status := "succeeded"
	errorMessage := ""
	if err != nil {
		status = "failed"
		errorMessage = err.Error()
	} else if !result.Succeeded {
		status = "failed"
	}
	s.mu.Lock()
	state.view.Status = status
	state.view.FinishedAt = &finishedAt
	state.view.Result = &result
	state.view.Error = errorMessage
	state.cancel = nil
	_ = s.persistRunsLocked()
	s.mu.Unlock()
	s.emit(RunFrame{RunID: state.view.ID, Status: status, Result: &result, Error: errorMessage, At: finishedAt})
}

func (s *Service) GetRun(id string) (RunView, error) {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	s.mu.RLock()
	state := s.runs[strings.TrimSpace(id)]
	if state == nil {
		s.mu.RUnlock()
		return RunView{}, worker.RemoteError{Code: "mobile_run_not_found", Message: "run was not found on this device"}
	}
	view, held := copyRunView(state.view), len(state.view.Events)-state.window
	s.mu.RUnlock()
	if !view.Shared {
		return s.localRunPage(state, len(view.Events)-max(transcriptWindow, held)), nil
	}
	config, err := s.enabledWorker(context.Background(), view.WorkerID)
	if err != nil {
		return view, err
	}
	// Refreshing keeps whatever history the reader has already loaded, so a
	// poll cannot yank a conversation back to its newest page mid-scroll.
	limit := max(transcriptWindow, len(view.Events))
	remote, err := s.client.GetSharedRun(context.Background(), config, view.ID, worker.TranscriptWindow{Limit: limit})
	if err != nil {
		return view, err
	}
	remote.WorkerID, remote.Shared = config.ID, true
	remote = mergeTranscriptWindow(view, remote)
	s.cacheSharedRun(config, remote)
	return remote, nil
}

// mergeTranscriptWindow keeps history the reader has already paged back to
// when a refresh answers with a shorter window than the one on screen — an
// older host that ignores the page size, or one that caps it. Without this a
// poll a few seconds later silently throws their scrollback away.
func mergeTranscriptWindow(cached, fresh RunView) RunView {
	keep := fresh.EventsOffset - cached.EventsOffset
	if len(cached.Events) == 0 || keep <= 0 || keep > len(cached.Events) {
		return fresh
	}
	fresh.Events = append(append([]agentrun.Event{}, cached.Events[:keep]...), fresh.Events...)
	fresh.EventsOffset = cached.EventsOffset
	return fresh
}

// LoadEarlierRun extends a conversation one page further back.
func (s *Service) LoadEarlierRun(id string) (RunView, error) {
	s.mu.RLock()
	state := s.runs[strings.TrimSpace(id)]
	if state == nil {
		s.mu.RUnlock()
		return RunView{}, worker.RemoteError{Code: "mobile_run_not_found", Message: "run was not found on this device"}
	}
	view := copyRunView(state.view)
	s.mu.RUnlock()
	if !view.Shared {
		return s.localRunPage(state, state.window-transcriptWindow), nil
	}
	if view.EventsOffset <= 0 {
		return view, nil
	}
	config, err := s.enabledWorker(context.Background(), view.WorkerID)
	if err != nil {
		return view, err
	}
	earlier, err := s.client.GetSharedRun(context.Background(), config, view.ID,
		worker.TranscriptWindow{Limit: transcriptWindow, Before: view.EventsOffset})
	if err != nil {
		return view, err
	}
	earlier.WorkerID, earlier.Shared = config.ID, true
	earlier.Events = append(earlier.Events, view.Events...)
	if earlier.EventsTotal < view.EventsTotal {
		earlier.EventsTotal = view.EventsTotal
	}
	s.cacheSharedRun(config, earlier)
	return earlier, nil
}

// localRunPage windows a run this phone executed itself. Its transcript is
// held whole, so paging back is a slice rather than a request.
func (s *Service) localRunPage(state *runState, start int) RunView {
	s.mu.Lock()
	defer s.mu.Unlock()
	total := len(state.view.Events)
	start = min(max(start, 0), total)
	state.window = start
	view := copyRunView(state.view)
	view.EventsTotal = total
	view.EventsOffset = start
	view.Events = view.Events[start:]
	return view
}

func (s *Service) ListRuns() []RunView {
	s.syncSharedRuns(context.Background())
	s.mu.RLock()
	items := s.runViewsLocked()
	s.mu.RUnlock()
	return items
}

// ListRunSummaries keeps the periodic phone sync cheap. A full run can carry
// thousands of transcript events; sending every unchanged transcript through
// the WKWebView bridge every few seconds makes scrolling hitch. The list and
// navigation only need run metadata. Conversation bodies are loaded with
// GetRun when opened.
func (s *Service) ListRunSummaries() []RunView {
	s.syncSharedRuns(context.Background())
	s.mu.RLock()
	items := make([]RunView, 0, len(s.runs))
	for _, state := range s.runs {
		view := state.view
		view.Events, view.EventsOffset, view.EventsTotal = nil, 0, 0
		if state.view.Result != nil {
			result := *state.view.Result
			view.Result = &result
		}
		if state.view.FinishedAt != nil {
			finishedAt := *state.view.FinishedAt
			view.FinishedAt = &finishedAt
		}
		items = append(items, view)
	}
	s.mu.RUnlock()
	sort.Slice(items, func(i, j int) bool { return items[i].StartedAt.After(items[j].StartedAt) })
	return items
}

func (s *Service) runViewsLocked() []RunView {
	items := make([]RunView, 0, len(s.runs))
	for _, state := range s.runs {
		items = append(items, copyRunView(state.view))
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].StartedAt.After(items[j].StartedAt)
	})
	return items
}

func (s *Service) persistRunsLocked() error {
	return localfile.WriteJSONAtomic(s.runsPath, s.runViewsLocked())
}

func (s *Service) InterruptRun(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	s.mu.RLock()
	state := s.runs[id]
	if state == nil {
		s.mu.RUnlock()
		return worker.RemoteError{Code: "mobile_run_not_found", Message: "run was not found on this device"}
	}
	config, shared, workerID := state.config, state.view.Shared, state.view.WorkerID
	s.mu.RUnlock()
	if shared {
		var err error
		config, err = s.enabledWorker(ctx, workerID)
		if err != nil {
			return err
		}
		return s.client.InterruptSharedRun(ctx, config, id)
	}
	return s.client.Interrupt(ctx, config, id)
}

func (s *Service) RespondPermission(ctx context.Context, input PermissionDecisionInput) error {
	input.RunID = strings.TrimSpace(input.RunID)
	input.RequestID = strings.TrimSpace(input.RequestID)
	input.Decision = strings.TrimSpace(input.Decision)
	s.mu.RLock()
	state := s.runs[input.RunID]
	if state == nil {
		s.mu.RUnlock()
		return worker.RemoteError{Code: "mobile_run_not_found", Message: "run was not found on this device"}
	}
	config, shared, workerID := state.config, state.view.Shared, state.view.WorkerID
	s.mu.RUnlock()
	if shared {
		var err error
		config, err = s.enabledWorker(ctx, workerID)
		if err != nil {
			return err
		}
		return s.client.RespondSharedPermission(ctx, config, input.RunID, input.RequestID, input.Decision)
	}
	return s.client.RespondPermission(ctx, config, input.RunID, input.RequestID, input.Decision)
}

func (s *Service) Close() {
	s.mu.Lock()
	for _, state := range s.runs {
		if state.cancel != nil {
			state.cancel()
		}
	}
	s.mu.Unlock()
}

func (s *Service) enabledWorker(ctx context.Context, id string) (worker.Config, error) {
	config, err := s.registry.Get(ctx, strings.TrimSpace(id))
	if err != nil || !config.Enabled {
		return worker.Config{}, worker.RemoteError{Code: "worker_not_found", Message: "worker is missing or disabled"}
	}
	return config, nil
}

func (s *Service) emit(frame RunFrame) {
	s.mu.RLock()
	emitter := s.emitter
	s.mu.RUnlock()
	if emitter != nil {
		emitter(frame)
	}
}

func copyRunView(value RunView) RunView {
	copyValue := value
	copyValue.Events = append([]agentrun.Event{}, value.Events...)
	if value.Result != nil {
		result := *value.Result
		copyValue.Result = &result
	}
	if value.FinishedAt != nil {
		finishedAt := *value.FinishedAt
		copyValue.FinishedAt = &finishedAt
	}
	return copyValue
}

func workerInfo(config worker.Config) worker.Info {
	return worker.Info{
		ID: config.ID, Name: config.Name, BaseURL: config.BaseURL,
		CAFile: config.CAFile, ClientCertFile: config.ClientCertFile, ClientKeyFile: config.ClientKeyFile,
		ServerName: config.ServerName, ServerCertificateSHA256: config.ServerCertificateSHA256,
		Enabled: config.Enabled, HasToken: config.Token != "", CreatedAt: config.CreatedAt, UpdatedAt: config.UpdatedAt,
	}
}

func newRunID() (string, error) {
	value := make([]byte, 12)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("create mobile run id: %w", err)
	}
	return "mobile_" + hex.EncodeToString(value), nil
}
