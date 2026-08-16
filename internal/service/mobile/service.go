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

	domainworkspaces "github.com/openmodu/oneshot/internal/domain/workspaces"
	"github.com/openmodu/oneshot/internal/service/worker"
	"github.com/openmodu/oneshot/internal/usecase/agentrun"
	"github.com/openmodu/oneshot/pkg/localfile"
)

const RunEventName = "mobile:run"

const maxStoredRuns = 100

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

type RunView struct {
	ID             string           `json:"id"`
	ConversationID string           `json:"conversationId"`
	WorkerID       string           `json:"workerId"`
	WorkspaceID    string           `json:"workspaceId"`
	Runtime        agentrun.Runtime `json:"runtime"`
	Prompt         string           `json:"prompt"`
	Status         string           `json:"status"`
	Events         []agentrun.Event `json:"events"`
	Result         *agentrun.Result `json:"result,omitempty"`
	Error          string           `json:"error,omitempty"`
	StartedAt      time.Time        `json:"startedAt"`
	FinishedAt     *time.Time       `json:"finishedAt,omitempty"`
}

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
}

type Service struct {
	registry *worker.Registry
	client   *worker.Client
	runsPath string

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
		if view.Status == "running" {
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

// StartRun starts an analysis-only run on a mapped remote workspace. Writable
// runs are intentionally absent: the worker protocol cleans a writable clone
// only after its patch has been applied to a coordinator workspace, and a
// sandboxed phone has no such workspace to receive that patch.
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
	s.mu.RLock()
	state := s.runs[strings.TrimSpace(id)]
	if state == nil {
		s.mu.RUnlock()
		return RunView{}, worker.RemoteError{Code: "mobile_run_not_found", Message: "run was not found on this device"}
	}
	view := copyRunView(state.view)
	s.mu.RUnlock()
	return view, nil
}

func (s *Service) ListRuns() []RunView {
	s.mu.RLock()
	items := s.runViewsLocked()
	s.mu.RUnlock()
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
	if len(items) > maxStoredRuns {
		items = items[:maxStoredRuns]
	}
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
	config := state.config
	s.mu.RUnlock()
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
	config := state.config
	s.mu.RUnlock()
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
