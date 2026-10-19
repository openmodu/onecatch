// Package workflows orchestrates local Agent workflow runs. It composes the
// pure workflow state machine, local repositories, runtime adapters, workspace
// locking and read-only git inspection.
package workflows

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	domaintasks "github.com/openmodu/onecatch/internal/domain/tasks"
	domainworkflows "github.com/openmodu/onecatch/internal/domain/workflows"
	domainworkspaces "github.com/openmodu/onecatch/internal/domain/workspaces"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
	"github.com/openmodu/onecatch/internal/usecase/agentrun/seam"
)

const (
	directAgentWorkflowID         = "single_agent"
	PauseReasonInterrupted        = "interrupted"
	PauseReasonRuntimeUnavailable = "runtime_unavailable"
	PauseReasonWorkspaceLocked    = "workspace_locked"
)

// ErrRetiredRun rejects resuming a multi-step run recorded by the previous
// signal-based engine: its snapshot has no edges, so every step would run at
// once. Single-step runs (plain conversations) carry over unchanged.
var ErrRetiredRun = errors.New("this run was recorded by the previous workflow engine and cannot be resumed; start a new task")

// Definitions saved by this engine are normalized, so every step has a kind.
func retiredDefinition(definition domainworkflows.Definition) bool {
	if len(definition.Steps) < 2 {
		return false
	}
	for _, step := range definition.Steps {
		if step.Kind != "" {
			return false
		}
	}
	return true
}

type TaskRepository interface {
	GetTask(context.Context, string) (domaintasks.Task, error)
	SaveTask(context.Context, domaintasks.Task) error
	UpdateTaskStatus(context.Context, string, domaintasks.Status, time.Time) (domaintasks.Task, error)
	GetWorkspace(context.Context, string) (domainworkspaces.Workspace, error)
}

type WorkflowRepository interface {
	GetDefinition(context.Context, string) (domainworkflows.Definition, error)
	SaveRun(context.Context, domainworkflows.Run, domainworkflows.Definition) error
	GetRun(context.Context, string) (domainworkflows.Run, error)
	GetRunDefinition(context.Context, string) (domainworkflows.Definition, error)
	UpdateRunDefinition(context.Context, string, domainworkflows.Definition) error
	UpdateRun(context.Context, domainworkflows.Run, int64) (domainworkflows.Run, error)
	SaveStepRun(context.Context, domainworkflows.StepRun) error
	ListStepRuns(context.Context, string) ([]domainworkflows.StepRun, error)
	AppendEvent(context.Context, domainworkflows.WorkflowEvent) (domainworkflows.WorkflowEvent, error)
	AppendRuntimeEvent(context.Context, string, string, json.RawMessage) (domainworkflows.RuntimeEvent, error)
	ClaimInstructions(context.Context, string) ([]domainworkflows.Instruction, error)
	WriteRunSummary(context.Context, string, string) error
}

type Engine interface {
	Available(agentrun.Runtime) bool
	Run(context.Context, agentrun.Request, agentrun.Sink) (agentrun.Result, error)
}

type RemoteExecutor interface {
	RunRemote(context.Context, string, string, agentrun.Request, agentrun.Sink) (agentrun.Result, error)
}

type WorkspaceLocker interface {
	Acquire(context.Context, string, string, string) (func() error, error)
}

type GitInspector interface {
	Inspect(context.Context, string) (domainworkspaces.GitSnapshot, error)
}

// RuntimeEventFrame is the transport-neutral event emitted while a workflow
// step is running. Service and transport layers decide how to deliver it.
type RuntimeEventFrame struct {
	RunID     string `json:"runId"`
	StepRunID string `json:"stepRunId"`
	Seq       int64  `json:"seq,omitempty"`
	agentrun.Event
}

type RuntimeEventPublisher interface {
	Publish(RuntimeEventFrame)
}

type IDGenerator func(prefix string) string

type RunResolution struct {
	MaxConcurrency        int
	InterruptGraceSeconds int
	RuntimeSettings       map[string]domainworkflows.ResolvedRuntimeSettings
}

// ResumeProfile shapes a resume. FromStepID reruns that step and everything
// after it. Harness overrides the runtime of StepID (default CurrentStepID);
// a runtime change starts a fresh session for that step while the rest of the
// run keeps its sessions and results. ModelDefinition supplies the latest
// template model choices, resolved and validated by the caller; an explicit
// step profile takes precedence over those choices.
type ResumeProfile struct {
	ModelDefinition *domainworkflows.Definition
	FromStepID      string
	StepID          string
	Harness         string
	Model           string
	RuntimeSettings domainworkflows.ResolvedRuntimeSettings
}

type Usecase struct {
	resolveWorkspace         func(context.Context, domaintasks.Task) (domainworkspaces.Workspace, error)
	resolveDefinition        func(context.Context, domaintasks.Task) (domainworkflows.Definition, error)
	resolveExecutionSettings func(context.Context, domaintasks.Task, domainworkflows.Definition) (domainworkflows.Definition, RunResolution, error)
	tasks                    TaskRepository
	workflows                WorkflowRepository
	engine                   Engine
	locker                   WorkspaceLocker
	git                      GitInspector
	now                      func() time.Time
	newID                    IDGenerator
	remote                   RemoteExecutor
	runtimePublisher         RuntimeEventPublisher
	maxConcurrency           atomic.Int64
}

func (s *Usecase) SetRemoteExecutor(remote RemoteExecutor) { s.remote = remote }

// localStep reports whether a step runs on this machine rather than a remote
// worker. An empty WorkerID and the sentinel "local" both mean local.
func localStep(step domainworkflows.Step) bool {
	return step.WorkerID == "" || step.WorkerID == "local"
}

// dispatchStep runs one step and streams its events to sink, choosing local or
// remote execution from step.WorkerID. It is the single decision point both the
// serial and DAG executors share, so a workflow behaves the same whichever mode
// runs it. Both paths share the workflow step timeout; the remote executor also
// forwards the remaining deadline so the worker can enforce it independently.
func (s *Usecase) dispatchStep(ctx context.Context, definition domainworkflows.Definition, step domainworkflows.Step, workspaceID string, request agentrun.Request, sink agentrun.Sink) (agentrun.Result, error) {
	stepCtx, cancel := context.WithTimeout(ctx, time.Duration(definition.Policy.StepTimeoutSeconds)*time.Second)
	defer cancel()
	if !localStep(step) {
		if strings.HasPrefix(workspaceID, "worktree:") || strings.HasPrefix(workspaceID, "task:") {
			return agentrun.Result{}, fmt.Errorf("worktree_local_only: remote worker execution is not supported")
		}
		if request.Remote != nil {
			return agentrun.Result{}, fmt.Errorf("remote_fs_local_agent_required: remote FS workspaces require a local workflow node")
		}
		if s.remote == nil {
			return agentrun.Result{}, fmt.Errorf("worker_unavailable: remote executor is not configured")
		}
		return s.remote.RunRemote(stepCtx, step.WorkerID, workspaceID, request, sink)
	}
	if !request.Runtime.Valid() || !s.engine.Available(request.Runtime) {
		return agentrun.Result{}, fmt.Errorf("runtime_unavailable: runtime %q is unavailable", step.Runtime)
	}
	return s.engine.Run(stepCtx, request, sink)
}
func (s *Usecase) SetRuntimeEventPublisher(publisher RuntimeEventPublisher) {
	s.runtimePublisher = publisher
}
func (s *Usecase) SetMaxConcurrency(value int) {
	if value < 1 {
		value = 1
	}
	s.maxConcurrency.Store(int64(value))
}

func NewUsecase(tasks TaskRepository, workflows WorkflowRepository, engine Engine, locker WorkspaceLocker, git GitInspector) *Usecase {
	usecase := &Usecase{
		tasks:     tasks,
		workflows: workflows,
		engine:    engine,
		locker:    locker,
		git:       git,
		now:       time.Now,
		newID:     randomID,
	}
	usecase.SetMaxConcurrency(4)
	return usecase
}

// StartTask creates and persists a running Run and its private workflow
// snapshot. Execution is deliberately separate so desktop callers receive the
// durable run ID before a potentially long Agent process starts.
func (s *Usecase) StartTask(ctx context.Context, taskID string) (domainworkflows.Run, error) {
	task, _, definition, err := s.loadTaskContext(ctx, taskID)
	if err != nil {
		return domainworkflows.Run{}, err
	}
	return s.startTask(ctx, task, definition, RunResolution{})
}

func (s *Usecase) StartTaskResolved(ctx context.Context, taskID string, definition domainworkflows.Definition, resolution RunResolution) (domainworkflows.Run, error) {
	task, _, stored, err := s.loadTaskContext(ctx, taskID)
	if err != nil {
		return domainworkflows.Run{}, err
	}
	if definition.ID != stored.ID {
		return domainworkflows.Run{}, fmt.Errorf("workflow definition does not match task")
	}
	if err := domainworkflows.Validate(definition); err != nil {
		return domainworkflows.Run{}, err
	}
	return s.startTask(ctx, task, definition, resolution)
}

func (s *Usecase) startTask(ctx context.Context, task domaintasks.Task, definition domainworkflows.Definition, resolution RunResolution) (domainworkflows.Run, error) {
	run, err := domainworkflows.NewRun(definition, s.newID("run"), s.now())
	if err != nil {
		return domainworkflows.Run{}, err
	}
	run.TaskID = task.ID
	run.MaxConcurrency = resolution.MaxConcurrency
	run.InterruptGraceSeconds = resolution.InterruptGraceSeconds
	run.RuntimeSettings = resolution.RuntimeSettings
	run, err = domainworkflows.Start(definition, run, s.now())
	if err != nil {
		return domainworkflows.Run{}, err
	}
	if err := s.workflows.SaveRun(ctx, run, definition); err != nil {
		return run, err
	}
	if err := s.setTaskStatus(ctx, &task, domaintasks.StatusRunning); err != nil {
		return run, err
	}
	if err := s.appendEvent(ctx, run.ID, "run.started", "", map[string]any{"taskId": task.ID, "workflowId": definition.ID}); err != nil {
		return run, err
	}
	return run, nil
}

// ExecuteTask is the synchronous compatibility entry point used by tests and
// non-UI callers.
func (s *Usecase) ExecuteTask(ctx context.Context, taskID string) (domainworkflows.Run, error) {
	run, err := s.StartTask(ctx, taskID)
	if err != nil {
		return run, err
	}
	return s.ExecuteRun(ctx, run.ID)
}

// ExecuteRun drives a previously prepared running Run.
func (s *Usecase) ExecuteRun(ctx context.Context, runID string) (domainworkflows.Run, error) {
	run, err := s.workflows.GetRun(ctx, runID)
	if err != nil {
		return domainworkflows.Run{}, err
	}
	definition, err := s.workflows.GetRunDefinition(ctx, runID)
	if err != nil {
		return run, err
	}
	if run.Status != domainworkflows.RunRunning {
		return run, fmt.Errorf("%w: execute from %s", domainworkflows.ErrInvalidRunState, run.Status)
	}
	task, err := s.tasks.GetTask(ctx, run.TaskID)
	if err != nil {
		return run, err
	}
	workspace, err := s.taskWorkspace(ctx, task)
	if err != nil {
		return run, err
	}
	return s.drive(ctx, task, workspace, definition, run)
}

// RecoverRun turns a durable running Run with no live workspace owner into a
// resumable paused Run. If another process still owns the workspace lock it is
// left untouched.
func (s *Usecase) RecoverRun(ctx context.Context, runID string) (domainworkflows.Run, error) {
	run, err := s.workflows.GetRun(ctx, runID)
	if err != nil {
		return domainworkflows.Run{}, err
	}
	if run.Status != domainworkflows.RunRunning {
		return run, nil
	}
	definition, err := s.workflows.GetRunDefinition(ctx, runID)
	if err != nil {
		return run, err
	}
	task, err := s.tasks.GetTask(ctx, run.TaskID)
	if err != nil {
		return run, err
	}
	message := "desktop exited before the run completed"
	for stepID, node := range run.Nodes {
		if node.Status == domainworkflows.NodeRunning {
			node.Status = domainworkflows.NodePending
			node.Error = message
			node.FinishedAt = s.now()
			run.Nodes[stepID] = node
		}
	}
	paused, err := domainworkflows.Pause(definition, run, PauseReasonInterrupted, message, s.now())
	if err != nil {
		return run, err
	}
	paused, err = s.workflows.UpdateRun(ctx, paused, run.Revision)
	if err != nil {
		return run, err
	}
	if err := s.setTaskStatus(ctx, &task, domaintasks.StatusPaused); err != nil {
		return paused, err
	}
	if err := s.appendEvent(ctx, run.ID, "run.paused", run.CurrentStepID, map[string]any{"reason": PauseReasonInterrupted, "error": message, "recovered": true}); err != nil {
		return paused, err
	}
	return paused, nil
}

// ResumeRun resumes the current step from the Run's private workflow snapshot.
// instruction is injected once into the first resumed step prompt.
func (s *Usecase) ResumeRun(ctx context.Context, runID, instruction string) (domainworkflows.Run, error) {
	return s.ResumeRunWithProfile(ctx, runID, instruction, ResumeProfile{})
}

// ResumeRunWithProfile resumes a run after applying an optional runtime
// profile to the selected step in its private workflow snapshot.
func (s *Usecase) ResumeRunWithProfile(ctx context.Context, runID, instruction string, profile ResumeProfile) (domainworkflows.Run, error) {
	run, err := s.workflows.GetRun(ctx, runID)
	if err != nil {
		return domainworkflows.Run{}, err
	}
	definition, err := s.workflows.GetRunDefinition(ctx, runID)
	if err != nil {
		return run, err
	}
	task, err := s.tasks.GetTask(ctx, run.TaskID)
	if err != nil {
		return run, err
	}
	latest := profile.ModelDefinition
	if latest == nil && definition.ID != directAgentWorkflowID {
		resolve := s.resolveDefinition
		if resolve == nil {
			resolve = func(ctx context.Context, task domaintasks.Task) (domainworkflows.Definition, error) {
				return s.workflows.GetDefinition(ctx, task.WorkflowID)
			}
		}
		// A removed template remains resumable from its durable snapshot.
		if current, loadErr := resolve(ctx, task); loadErr == nil {
			latest = &current
		}
	}
	if latest != nil {
		updated := RefreshRunDefinition(definition, *latest)
		changedSession := false
		for _, old := range definition.Steps {
			step, exists := domainworkflows.FindStep(updated, old.ID)
			if !exists || step.Runtime != old.Runtime || step.WorkerID != old.WorkerID || step.Kind != old.Kind {
				delete(run.Sessions, old.ID)
				changedSession = true
			}
		}
		if changedSession {
			saved, err := s.workflows.UpdateRun(ctx, run, run.Revision)
			if err != nil {
				return run, err
			}
			run = saved
		}
		if err := s.workflows.UpdateRunDefinition(ctx, run.ID, updated); err != nil {
			return run, err
		}
		definition = updated
	}
	if strings.TrimSpace(profile.Harness) != "" {
		definition, run, err = s.applyResumeProfile(ctx, definition, run, profile)
		if err != nil {
			return run, err
		}
	}
	workspace, err := s.taskWorkspace(ctx, task)
	if err != nil {
		return run, err
	}
	if retiredDefinition(definition) {
		return run, ErrRetiredRun
	}
	fromStepID := strings.TrimSpace(profile.FromStepID)
	resumeBase := run
	if run.LoopEnabled && run.Status == domainworkflows.RunCompleted && fromStepID == "" {
		resumeBase, err = domainworkflows.NextLoopRound(definition, run, s.now())
		if err != nil {
			return run, err
		}
		// Resume delivers the human instruction to the newly pending roots.
		resumeBase.Status = domainworkflows.RunPaused
	}
	resumed, err := domainworkflows.Resume(definition, resumeBase, fromStepID, instruction, s.now())
	if err != nil {
		return run, err
	}
	resumed, err = s.workflows.UpdateRun(ctx, resumed, run.Revision)
	if err != nil {
		return run, err
	}
	if err := s.setTaskStatus(ctx, &task, domaintasks.StatusRunning); err != nil {
		return resumed, err
	}
	trimmedInstruction := strings.TrimSpace(instruction)
	if err := s.appendEvent(ctx, run.ID, "run.resumed", fromStepID, map[string]any{"hasInstruction": trimmedInstruction != "", "instruction": trimmedInstruction, "fromStepId": fromStepID}); err != nil {
		return resumed, err
	}
	preserveStep := ""
	if profile.Harness != "" {
		preserveStep = profile.StepID
		if preserveStep == "" {
			preserveStep = resumed.CurrentStepID
		}
	}
	return s.drive(ctx, task, workspace, definition, resumed, preserveStep)
}

func (s *Usecase) applyResumeProfile(ctx context.Context, definition domainworkflows.Definition, run domainworkflows.Run, profile ResumeProfile) (domainworkflows.Definition, domainworkflows.Run, error) {
	stepID := strings.TrimSpace(profile.StepID)
	if stepID == "" {
		stepID = run.CurrentStepID
	}
	stepIndex := -1
	for index := range definition.Steps {
		if definition.Steps[index].ID == stepID {
			stepIndex = index
			break
		}
	}
	if stepIndex < 0 {
		return definition, run, fmt.Errorf("workflow step %q not found", stepID)
	}
	harness := strings.TrimSpace(profile.Harness)
	if runtime := agentrun.Runtime(harness); !runtime.Valid() {
		return definition, run, fmt.Errorf("runtime %q is invalid", harness)
	}

	definition.Steps = append([]domainworkflows.Step(nil), definition.Steps...)
	step := &definition.Steps[stepIndex]
	settings := profile.RuntimeSettings
	settings.EnvironmentAllowlist = append([]string(nil), settings.EnvironmentAllowlist...)
	currentSettings := run.RuntimeSettings[harness]
	changed := step.Runtime != harness || step.Model != profile.Model || step.ReasoningEffort != settings.ReasoningEffort || !resolvedRuntimeSettingsEqual(currentSettings, settings)
	step.Runtime = harness
	step.Model = strings.TrimSpace(profile.Model)
	step.ReasoningEffort = strings.TrimSpace(settings.ReasoningEffort)
	if err := domainworkflows.Validate(definition); err != nil {
		return definition, run, err
	}
	if err := s.workflows.UpdateRunDefinition(ctx, run.ID, definition); err != nil {
		return definition, run, err
	}

	if run.RuntimeSettings == nil {
		run.RuntimeSettings = make(map[string]domainworkflows.ResolvedRuntimeSettings)
	} else {
		cloned := make(map[string]domainworkflows.ResolvedRuntimeSettings, len(run.RuntimeSettings)+1)
		for key, value := range run.RuntimeSettings {
			value.EnvironmentAllowlist = append([]string(nil), value.EnvironmentAllowlist...)
			cloned[key] = value
		}
		run.RuntimeSettings = cloned
	}
	run.RuntimeSettings[harness] = settings
	if changed && run.Sessions != nil {
		delete(run.Sessions, stepID)
	}
	updated, err := s.workflows.UpdateRun(ctx, run, run.Revision)
	if err != nil {
		return definition, run, err
	}
	return definition, updated, nil
}

func resolvedRuntimeSettingsEqual(left, right domainworkflows.ResolvedRuntimeSettings) bool {
	if left.Provider != right.Provider || left.ReasoningEffort != right.ReasoningEffort || left.ServiceTier != right.ServiceTier || len(left.EnvironmentAllowlist) != len(right.EnvironmentAllowlist) {
		return false
	}
	for index := range left.EnvironmentAllowlist {
		if left.EnvironmentAllowlist[index] != right.EnvironmentAllowlist[index] {
			return false
		}
	}
	return true
}

// CancelRun marks a ready or paused run and its task as cancelled. Active runs
// are interrupted first by the application service, which causes drive to
// persist the paused state before this method is called.
func (s *Usecase) CancelRun(ctx context.Context, runID string) (domainworkflows.Run, error) {
	run, err := s.workflows.GetRun(ctx, runID)
	if err != nil {
		return domainworkflows.Run{}, err
	}
	definition, err := s.workflows.GetRunDefinition(ctx, runID)
	if err != nil {
		return run, err
	}
	cancelled, err := domainworkflows.Cancel(definition, run, s.now())
	if err != nil {
		return run, err
	}
	cancelled, err = s.workflows.UpdateRun(ctx, cancelled, run.Revision)
	if err != nil {
		return run, err
	}
	task, err := s.tasks.GetTask(ctx, run.TaskID)
	if err != nil {
		return cancelled, err
	}
	if err := s.setTaskStatus(ctx, &task, domaintasks.StatusCancelled); err != nil {
		return cancelled, err
	}
	if err := s.appendEvent(ctx, run.ID, "run.cancelled", run.CurrentStepID, map[string]any{}); err != nil {
		return cancelled, err
	}
	return cancelled, nil
}

func resolvedEnvironmentAllowlist(run domainworkflows.Run, runtime string) []string {
	if run.RuntimeSettings == nil {
		return nil
	}
	settings, ok := run.RuntimeSettings[runtime]
	if !ok {
		return nil
	}
	return append([]string{}, settings.EnvironmentAllowlist...)
}

func resolvedRuntimeProvider(run domainworkflows.Run, runtime string) string {
	if run.RuntimeSettings == nil {
		return ""
	}
	return run.RuntimeSettings[runtime].Provider
}

func resolvedReasoningEffort(run domainworkflows.Run, runtime string) string {
	if run.RuntimeSettings == nil {
		return ""
	}
	return run.RuntimeSettings[runtime].ReasoningEffort
}

func resolvedServiceTier(run domainworkflows.Run, runtime string) string {
	if run.RuntimeSettings == nil {
		return ""
	}
	return run.RuntimeSettings[runtime].ServiceTier
}

func resolvedMaxContextWindow(run domainworkflows.Run, runtime string) bool {
	if run.RuntimeSettings == nil {
		return false
	}
	return run.RuntimeSettings[runtime].MaxContextWindow
}

func (s *Usecase) loadTaskContext(ctx context.Context, taskID string) (domaintasks.Task, domainworkspaces.Workspace, domainworkflows.Definition, error) {
	task, err := s.tasks.GetTask(ctx, taskID)
	if err != nil {
		return domaintasks.Task{}, domainworkspaces.Workspace{}, domainworkflows.Definition{}, err
	}
	workspace, err := s.taskWorkspace(ctx, task)
	if err != nil {
		return domaintasks.Task{}, domainworkspaces.Workspace{}, domainworkflows.Definition{}, err
	}
	resolve := s.resolveDefinition
	if resolve == nil {
		resolve = func(ctx context.Context, task domaintasks.Task) (domainworkflows.Definition, error) {
			return s.workflows.GetDefinition(ctx, task.WorkflowID)
		}
	}
	definition, err := resolve(ctx, task)
	if err != nil {
		return domaintasks.Task{}, domainworkspaces.Workspace{}, domainworkflows.Definition{}, err
	}
	return task, workspace, definition, nil
}

// SetDefinitionResolver lets the application resolve workflows that are not
// stored definitions, such as a project's workflow files.
func (s *Usecase) SetDefinitionResolver(resolve func(context.Context, domaintasks.Task) (domainworkflows.Definition, error)) {
	s.resolveDefinition = resolve
}

func (s *Usecase) nextAttempt(ctx context.Context, runID, stepID string) (int, int, error) {
	items, err := s.workflows.ListStepRuns(ctx, runID)
	if err != nil {
		return 0, 0, err
	}
	maxAttempt, contextTokens := 0, 0
	for _, item := range items {
		if item.StepID == stepID && item.Attempt > maxAttempt {
			maxAttempt = item.Attempt
			contextTokens = item.ContextTokens
		}
	}
	return maxAttempt + 1, contextTokens, nil
}

func (s *Usecase) setTaskStatus(ctx context.Context, task *domaintasks.Task, status domaintasks.Status) error {
	updated, err := s.tasks.UpdateTaskStatus(ctx, task.ID, status, s.now())
	if err != nil {
		return err
	}
	*task = updated
	return nil
}

func (s *Usecase) appendEvent(ctx context.Context, runID, eventType, stepID string, payload any) error {
	_, err := s.workflows.AppendEvent(ctx, domainworkflows.WorkflowEvent{RunID: runID, Type: eventType, StepID: stepID, Payload: mustJSON(payload), At: s.now()})
	return err
}

func (s *Usecase) recordGit(ctx context.Context, runID, stepID, phase string, workspace domainworkspaces.Workspace) error {
	if s.git == nil || workspace.RemoteFS != nil {
		return nil
	}
	snapshot, err := s.git.Inspect(ctx, workspace.Path)
	if err != nil {
		return s.appendEvent(ctx, runID, "git.inspect_failed", stepID, map[string]any{"phase": phase, "error": err.Error()})
	}
	return s.appendEvent(ctx, runID, "git.snapshot", stepID, map[string]any{"phase": phase, "snapshot": snapshot})
}

func workspaceRemoteTarget(workspace domainworkspaces.Workspace) *seam.Target {
	if workspace.RemoteFS == nil {
		return nil
	}
	return &seam.Target{
		Host:         workspace.RemoteFS.Host,
		Root:         workspace.RemoteFS.Root,
		Username:     workspace.RemoteFS.Username,
		CredentialID: workspace.RemoteFS.CredentialID,
		SSHOptions:   append([]string{}, workspace.RemoteFS.SSHOptions...),
	}
}

func finishStepRun(stepRun *domainworkflows.StepRun, result agentrun.Result, finishedAt time.Time) {
	stepRun.FinishedAt = finishedAt
	stepRun.InputTokens = result.Usage.InputTokens
	stepRun.CachedInputTokens = result.Usage.CachedInputTokens
	stepRun.CacheCreationInputTokens = result.Usage.CacheCreationInputTokens
	stepRun.OutputTokens = result.Usage.OutputTokens
	stepRun.ReasoningOutputTokens = result.Usage.ReasoningOutputTokens
	// A runtime that never reported a window leaves the previous attempt's
	// figure in place rather than blanking a gauge the user was reading.
	if result.Context.Window > 0 {
		stepRun.ContextWindow = result.Context.Window
	}
	if result.Context.Tokens > 0 {
		stepRun.ContextTokens = result.Context.Tokens
	}
	if !stepRun.StartedAt.IsZero() && finishedAt.After(stepRun.StartedAt) {
		stepRun.DurationMS = finishedAt.Sub(stepRun.StartedAt).Milliseconds()
	}
}

func allowedSandbox(requested, maximum string) agentrun.Sandbox {
	if maximum == "" {
		maximum = string(agentrun.SandboxWorkspaceWrite)
	}
	if requested == "" {
		requested = maximum
	}
	rank := map[string]int{"read-only": 0, "workspace-write": 1, "full": 2}
	requestedRank, ok := rank[requested]
	if !ok {
		requested, requestedRank = "workspace-write", 1
	}
	maximumRank, ok := rank[maximum]
	if !ok {
		maximum, maximumRank = "workspace-write", 1
	}
	if requestedRank > maximumRank {
		return agentrun.Sandbox(maximum)
	}
	return agentrun.Sandbox(requested)
}

func failureMessage(err error, finalMessage string) string {
	if err != nil {
		return err.Error()
	}
	if strings.TrimSpace(finalMessage) != "" {
		return finalMessage
	}
	return "agent did not complete successfully"
}

func mustJSON(value any) json.RawMessage {
	payload, err := json.Marshal(value)
	if err != nil {
		return json.RawMessage(`{"error":"encode event payload"}`)
	}
	return payload
}

func randomID(prefix string) string {
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	}
	return prefix + "_" + hex.EncodeToString(bytes)
}

// SetWorkspaceResolver supplies the same task directory resolution used by UI
// operations. It must be configured before any runs start.
func (s *Usecase) SetWorkspaceResolver(resolve func(context.Context, domaintasks.Task) (domainworkspaces.Workspace, error)) {
	s.resolveWorkspace = resolve
}
func (s *Usecase) taskWorkspace(ctx context.Context, task domaintasks.Task) (domainworkspaces.Workspace, error) {
	if s.resolveWorkspace != nil {
		return s.resolveWorkspace(ctx, task)
	}
	if task.Worktree != nil {
		return domainworkspaces.Workspace{}, fmt.Errorf("worktree resolver is unavailable")
	}
	return s.tasks.GetWorkspace(ctx, task.WorkspaceID)
}

func workspaceLockID(workspace domainworkspaces.Workspace) string {
	if workspace.LockID != "" {
		return workspace.LockID
	}
	return workspace.ID
}
