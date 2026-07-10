// Package localapp exposes the desktop-facing application service for local
// Agent workflow orchestration. It owns background run cancellation handles
// while domain state remains durable in repositories.
package localapp

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/openmodu/oneshot/internal/agentrun"
	localdata "github.com/openmodu/oneshot/internal/data/local"
	domaintasks "github.com/openmodu/oneshot/internal/domain/tasks"
	domainworkflows "github.com/openmodu/oneshot/internal/domain/workflows"
	domainworkspaces "github.com/openmodu/oneshot/internal/domain/workspaces"
	"github.com/openmodu/oneshot/internal/gitinspect"
	repoworkflows "github.com/openmodu/oneshot/internal/repo/workflows"
	workflowuc "github.com/openmodu/oneshot/internal/usecase/workflows"
	"github.com/openmodu/oneshot/internal/workspacelock"
)

type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e Error) Error() string { return e.Code + ": " + e.Message }

func coded(code, message string) error { return Error{Code: code, Message: message} }

type AddWorkspaceInput struct {
	Path           string `json:"path"`
	Name           string `json:"name,omitempty"`
	DefaultSandbox string `json:"defaultSandbox,omitempty"`
}

type WorkspaceStatus struct {
	Workspace domainworkspaces.Workspace   `json:"workspace"`
	Git       domainworkspaces.GitSnapshot `json:"git"`
}

type CreateTaskInput struct {
	WorkspaceID string `json:"workspaceId"`
	Title       string `json:"title"`
	Prompt      string `json:"prompt"`
	WorkflowID  string `json:"workflowId"`
}

type WorkflowEventView struct {
	RunID   string `json:"runId"`
	Seq     int64  `json:"seq"`
	Type    string `json:"type"`
	StepID  string `json:"stepId,omitempty"`
	Payload string `json:"payload"`
	At      string `json:"at"`
}

type RuntimeEventView struct {
	StepRunID string `json:"stepRunId"`
	Seq       int64  `json:"seq"`
	Kind      string `json:"kind"`
	Text      string `json:"text,omitempty"`
	At        string `json:"at"`
}

type RunDetail struct {
	Run           domainworkflows.Run        `json:"run"`
	Task          domaintasks.Task           `json:"task"`
	Workspace     domainworkspaces.Workspace `json:"workspace"`
	Workflow      domainworkflows.Definition `json:"workflow"`
	StepRuns      []domainworkflows.StepRun  `json:"stepRuns"`
	Events        []WorkflowEventView        `json:"events"`
	RuntimeEvents []RuntimeEventView         `json:"runtimeEvents"`
	Active        bool                       `json:"active"`
	LastError     string                     `json:"lastError,omitempty"`
}

type App struct {
	store        *localdata.Store
	orchestrator *workflowuc.Usecase
	runtimes     *RuntimeRegistry
	git          *gitinspect.Inspector
	rootCtx      context.Context
	rootCancel   context.CancelFunc
	mu           sync.RWMutex
	active       map[string]context.CancelFunc
	lastErrors   map[string]string
	wg           sync.WaitGroup
}

func New(store *localdata.Store, orchestrator *workflowuc.Usecase, runtimes *RuntimeRegistry, git *gitinspect.Inspector) *App {
	ctx, cancel := context.WithCancel(context.Background())
	return &App{
		store: store, orchestrator: orchestrator, runtimes: runtimes, git: git,
		rootCtx: ctx, rootCancel: cancel, active: make(map[string]context.CancelFunc), lastErrors: make(map[string]string),
	}
}

func (a *App) Close() error {
	a.rootCancel()
	a.mu.Lock()
	for _, cancel := range a.active {
		cancel()
	}
	a.mu.Unlock()
	a.wg.Wait()
	return a.store.Close()
}

func (a *App) DataRoot() string { return a.store.Data.Paths.Root }

func (a *App) ListRuntimes() []RuntimeInfo                      { return a.runtimes.List() }
func (a *App) CheckRuntime(runtime string) (RuntimeInfo, error) { return a.runtimes.Check(runtime) }
func (a *App) UpdateRuntimeConfig(input RuntimeConfigInput) (RuntimeInfo, error) {
	return a.runtimes.Update(input)
}

func (a *App) AddWorkspace(ctx context.Context, input AddWorkspaceInput) (domainworkspaces.Workspace, error) {
	path := strings.TrimSpace(input.Path)
	if path == "" {
		return domainworkspaces.Workspace{}, coded("workspace_invalid", "path is required")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return domainworkspaces.Workspace{}, coded("workspace_invalid", "path cannot be resolved")
	}
	if resolved, resolveErr := filepath.EvalSymlinks(abs); resolveErr == nil {
		abs = resolved
	}
	info, err := os.Stat(abs)
	if err != nil || !info.IsDir() {
		return domainworkspaces.Workspace{}, coded("workspace_not_found", "directory does not exist")
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = filepath.Base(abs)
	}
	sandbox := strings.TrimSpace(input.DefaultSandbox)
	if sandbox == "" {
		sandbox = string(agentrun.SandboxWorkspaceWrite)
	}
	now := time.Now().UTC()
	workspace := domainworkspaces.Workspace{ID: workspaceID(abs), Name: name, Path: filepath.Clean(abs), DefaultSandbox: sandbox, CreatedAt: now, LastOpenedAt: now}
	if current, getErr := a.store.Repos.Tasks.GetWorkspace(ctx, workspace.ID); getErr == nil {
		workspace.CreatedAt = current.CreatedAt
	}
	if err := a.store.Repos.Tasks.SaveWorkspace(ctx, workspace); err != nil {
		return domainworkspaces.Workspace{}, err
	}
	return workspace, nil
}

func (a *App) ListWorkspaces(ctx context.Context) ([]domainworkspaces.Workspace, error) {
	return a.store.Repos.Tasks.ListWorkspaces(ctx)
}

func (a *App) GetWorkspace(ctx context.Context, id string) (domainworkspaces.Workspace, error) {
	workspace, err := a.store.Repos.Tasks.GetWorkspace(ctx, id)
	if err != nil {
		return workspace, coded("workspace_not_found", "workspace was not found")
	}
	return workspace, nil
}

func (a *App) GetWorkspaceStatus(ctx context.Context, id string) (WorkspaceStatus, error) {
	workspace, err := a.GetWorkspace(ctx, id)
	if err != nil {
		return WorkspaceStatus{}, err
	}
	snapshot, err := a.git.Inspect(ctx, workspace.Path)
	if err != nil {
		return WorkspaceStatus{}, err
	}
	return WorkspaceStatus{Workspace: workspace, Git: snapshot}, nil
}

func (a *App) ValidateDefinition(input domainworkflows.Definition) []domainworkflows.ValidationIssue {
	err := domainworkflows.Validate(input)
	var issues domainworkflows.ValidationErrors
	if errors.As(err, &issues) {
		return []domainworkflows.ValidationIssue(issues)
	}
	return []domainworkflows.ValidationIssue{}
}

func (a *App) SaveDefinition(ctx context.Context, input domainworkflows.Definition) (domainworkflows.Definition, error) {
	definition, err := a.store.Repos.Workflows.SaveDefinition(ctx, input)
	if err != nil {
		var issues domainworkflows.ValidationErrors
		if errors.As(err, &issues) {
			return domainworkflows.Definition{}, coded("workflow_invalid_definition", err.Error())
		}
		return domainworkflows.Definition{}, err
	}
	return definition, nil
}

func (a *App) ListDefinitions(ctx context.Context) ([]domainworkflows.Definition, error) {
	return a.store.Repos.Workflows.ListDefinitions(ctx)
}

func (a *App) GetDefinition(ctx context.Context, id string) (domainworkflows.Definition, error) {
	definition, err := a.store.Repos.Workflows.GetDefinition(ctx, id)
	if err != nil {
		return definition, coded("workflow_not_found", "workflow was not found")
	}
	return definition, nil
}

func (a *App) EnsureBuiltinDefinitions(ctx context.Context) error {
	for _, definition := range builtinDefinitions() {
		if _, err := a.store.Repos.Workflows.GetDefinition(ctx, definition.ID); err == nil {
			continue
		}
		if _, err := a.store.Repos.Workflows.SaveDefinition(ctx, definition); err != nil {
			return err
		}
	}
	return nil
}

// RecoverInterruptedRuns repairs the narrow crash window between persisting a
// running Run and owning its Agent process. A live lock means another Oneshot
// process is still executing that workspace, so the Run is not changed.
func (a *App) RecoverInterruptedRuns(ctx context.Context) error {
	runs, err := a.store.Repos.Workflows.ListRunsByTask(ctx, "")
	if err != nil {
		return err
	}
	for _, run := range runs {
		if run.Status != domainworkflows.RunRunning {
			continue
		}
		if _, err := a.orchestrator.RecoverRun(ctx, run.ID); errors.Is(err, workspacelock.ErrLocked) {
			continue
		} else if err != nil {
			return mapError(err)
		}
	}
	return nil
}

func (a *App) CreateTask(ctx context.Context, input CreateTaskInput) (domaintasks.Task, error) {
	if _, err := a.GetWorkspace(ctx, strings.TrimSpace(input.WorkspaceID)); err != nil {
		return domaintasks.Task{}, err
	}
	if _, err := a.GetDefinition(ctx, strings.TrimSpace(input.WorkflowID)); err != nil {
		return domaintasks.Task{}, err
	}
	now := time.Now().UTC()
	task := domaintasks.Task{ID: randomID("task"), WorkspaceID: strings.TrimSpace(input.WorkspaceID), Title: strings.TrimSpace(input.Title), Prompt: strings.TrimSpace(input.Prompt), WorkflowID: strings.TrimSpace(input.WorkflowID), Status: domaintasks.StatusReady, CreatedAt: now, UpdatedAt: now}
	if err := a.store.Repos.Tasks.SaveTask(ctx, task); err != nil {
		return domaintasks.Task{}, coded("task_invalid", err.Error())
	}
	return task, nil
}

func (a *App) ListTasks(ctx context.Context, workspaceID string) ([]domaintasks.Task, error) {
	return a.store.Repos.Tasks.ListTasks(ctx, workspaceID)
}

func (a *App) StartRun(ctx context.Context, taskID string) (domainworkflows.Run, error) {
	run, err := a.orchestrator.StartTask(ctx, taskID)
	if err != nil {
		return run, mapError(err)
	}
	a.dispatch(run.ID, func(runCtx context.Context) (domainworkflows.Run, error) {
		return a.orchestrator.ExecuteRun(runCtx, run.ID)
	})
	return run, nil
}

func (a *App) ResumeRun(ctx context.Context, runID, instruction string) (domainworkflows.Run, error) {
	run, err := a.store.Repos.Workflows.GetRun(ctx, runID)
	if err != nil {
		return run, coded("run_not_found", "run was not found")
	}
	if run.Status != domainworkflows.RunPaused {
		return run, coded("run_invalid_state", "only paused runs can be resumed")
	}
	if a.isActive(runID) {
		return run, coded("run_invalid_state", "run is still stopping")
	}
	a.dispatch(run.ID, func(runCtx context.Context) (domainworkflows.Run, error) {
		return a.orchestrator.ResumeRun(runCtx, run.ID, instruction)
	})
	return run, nil
}

func (a *App) InterruptRun(ctx context.Context, runID string) (domainworkflows.Run, error) {
	a.mu.RLock()
	cancel := a.active[runID]
	a.mu.RUnlock()
	if cancel == nil {
		return domainworkflows.Run{}, coded("run_invalid_state", "run is not actively executing")
	}
	cancel()
	return a.store.Repos.Workflows.GetRun(ctx, runID)
}

func (a *App) CancelRun(ctx context.Context, runID string) (domainworkflows.Run, error) {
	if a.isActive(runID) {
		return domainworkflows.Run{}, coded("run_invalid_state", "interrupt the active run before cancelling it")
	}
	run, err := a.orchestrator.CancelRun(ctx, runID)
	return run, mapError(err)
}

func (a *App) ListRunsByTask(ctx context.Context, taskID string) ([]domainworkflows.Run, error) {
	return a.store.Repos.Workflows.ListRunsByTask(ctx, taskID)
}

func (a *App) ListRunEvents(ctx context.Context, runID string, afterSeq int64) ([]WorkflowEventView, error) {
	events, err := a.store.Repos.Workflows.ListEvents(ctx, runID, afterSeq, 1_000)
	if err != nil {
		return nil, err
	}
	out := make([]WorkflowEventView, 0, len(events))
	for _, event := range events {
		out = append(out, WorkflowEventView{RunID: event.RunID, Seq: event.Seq, Type: event.Type, StepID: event.StepID, Payload: string(event.Payload), At: event.At.Format(time.RFC3339Nano)})
	}
	return out, nil
}

func (a *App) GetRunDetail(ctx context.Context, runID string) (RunDetail, error) {
	run, err := a.store.Repos.Workflows.GetRun(ctx, runID)
	if err != nil {
		return RunDetail{}, coded("run_not_found", "run was not found")
	}
	workflow, err := a.store.Repos.Workflows.GetRunDefinition(ctx, runID)
	if err != nil {
		return RunDetail{}, err
	}
	task, err := a.store.Repos.Tasks.GetTask(ctx, run.TaskID)
	if err != nil {
		return RunDetail{}, err
	}
	workspace, err := a.store.Repos.Tasks.GetWorkspace(ctx, task.WorkspaceID)
	if err != nil {
		return RunDetail{}, err
	}
	stepRuns, err := a.store.Repos.Workflows.ListStepRuns(ctx, runID)
	if err != nil {
		return RunDetail{}, err
	}
	events, err := a.store.Repos.Workflows.ListEvents(ctx, runID, 0, 10_000)
	if err != nil {
		return RunDetail{}, err
	}
	detail := RunDetail{Run: run, Task: task, Workspace: workspace, Workflow: workflow, StepRuns: stepRuns, Events: make([]WorkflowEventView, 0, len(events)), RuntimeEvents: []RuntimeEventView{}, Active: a.isActive(runID), LastError: a.lastError(runID)}
	for _, event := range events {
		detail.Events = append(detail.Events, WorkflowEventView{RunID: event.RunID, Seq: event.Seq, Type: event.Type, StepID: event.StepID, Payload: string(event.Payload), At: event.At.Format(time.RFC3339Nano)})
	}
	for _, stepRun := range stepRuns {
		items, listErr := a.store.Repos.Workflows.ListRuntimeEvents(ctx, runID, stepRun.ID, 0, 10_000)
		if listErr != nil {
			return RunDetail{}, listErr
		}
		for _, item := range items {
			var event agentrun.Event
			_ = json.Unmarshal(item.Payload, &event)
			detail.RuntimeEvents = append(detail.RuntimeEvents, RuntimeEventView{StepRunID: stepRun.ID, Seq: item.Seq, Kind: string(event.Kind), Text: event.Text, At: item.At.Format(time.RFC3339Nano)})
		}
	}
	return detail, nil
}

func (a *App) dispatch(runID string, execute func(context.Context) (domainworkflows.Run, error)) {
	runCtx, cancel := context.WithCancel(a.rootCtx)
	a.mu.Lock()
	delete(a.lastErrors, runID)
	a.active[runID] = cancel
	a.mu.Unlock()
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		_, err := execute(runCtx)
		a.mu.Lock()
		delete(a.active, runID)
		if err != nil && !errors.Is(err, context.Canceled) {
			a.lastErrors[runID] = mapError(err).Error()
		}
		a.mu.Unlock()
	}()
}

func (a *App) isActive(runID string) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.active[runID] != nil
}
func (a *App) lastError(runID string) string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.lastErrors[runID]
}

func workspaceID(path string) string {
	slug := regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(strings.ToLower(filepath.Base(path)), "-")
	slug = strings.Trim(slug, "-")
	if slug == "" {
		slug = "workspace"
	}
	sum := sha256.Sum256([]byte(filepath.Clean(path)))
	return fmt.Sprintf("%s-%x", slug, sum[:6])
}

func randomID(prefix string) string {
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	}
	return prefix + "_" + hex.EncodeToString(bytes)
}

func mapError(err error) error {
	if err == nil {
		return nil
	}
	var appErr Error
	if errors.As(err, &appErr) {
		return err
	}
	if errors.Is(err, domainworkflows.ErrInvalidRunState) {
		return coded("run_invalid_state", err.Error())
	}
	if errors.Is(err, repoworkflows.ErrStateConflict) {
		return coded("workflow_state_conflict", err.Error())
	}
	if errors.Is(err, repoworkflows.ErrRunNotFound) {
		return coded("run_not_found", err.Error())
	}
	if errors.Is(err, workspacelock.ErrLocked) {
		return coded("workspace_locked", err.Error())
	}
	if strings.Contains(err.Error(), "runtime") && strings.Contains(err.Error(), "unavailable") {
		return coded("runtime_unavailable", err.Error())
	}
	return err
}

func builtinDefinitions() []domainworkflows.Definition {
	return []domainworkflows.Definition{
		{ID: "single_agent", Name: "单 Agent 完成", Description: "由一个 Codex Agent 执行任务，需要时暂停给人处理。", EntryStepID: "execute", Steps: []domainworkflows.Step{{ID: "execute", Name: "执行", Runtime: "codex", Sandbox: "workspace-write", RolePrompt: "你是负责独立完成任务的本地 Agent。", Instruction: "完成任务、验证结果，并按 outcome contract 汇报。", Transitions: map[string]string{"completed": domainworkflows.TargetDone, "need_human": domainworkflows.TargetPause}}}},
		{ID: "implement_review", Name: "实现与审查 Loop", Description: "Codex 实现，Claude Code 审查；有问题就回到实现步骤。", EntryStepID: "implement", Steps: []domainworkflows.Step{
			{ID: "implement", Name: "实现", Runtime: "codex", Sandbox: "workspace-write", RolePrompt: "你是负责落地代码和测试的实现者。", Instruction: "实现目标、运行验证，并把变更交给审查者。", Transitions: map[string]string{"ready_for_review": "review", "need_human": domainworkflows.TargetPause}},
			{ID: "review", Name: "审查", Runtime: "claude", Sandbox: "read-only", RolePrompt: "你是独立、严格的代码审查者。", Instruction: "检查需求、实现、风险和测试；有问题要求修改。", Transitions: map[string]string{"changes_requested": "implement", "approved": domainworkflows.TargetDone, "need_human": domainworkflows.TargetPause}},
		}},
	}
}
