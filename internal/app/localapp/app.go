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
	domainsettings "github.com/openmodu/oneshot/internal/domain/settings"
	domaintasks "github.com/openmodu/oneshot/internal/domain/tasks"
	domainworkflows "github.com/openmodu/oneshot/internal/domain/workflows"
	domainworkspaces "github.com/openmodu/oneshot/internal/domain/workspaces"
	"github.com/openmodu/oneshot/internal/gitinspect"
	settingsrepo "github.com/openmodu/oneshot/internal/repo/settings"
	repoworkflows "github.com/openmodu/oneshot/internal/repo/workflows"
	"github.com/openmodu/oneshot/internal/runstream"
	workflowuc "github.com/openmodu/oneshot/internal/usecase/workflows"
	"github.com/openmodu/oneshot/internal/worker"
	"github.com/openmodu/oneshot/internal/workspacelock"
	"github.com/openmodu/oneshot/pkg/localfile"
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
	WorkspaceID     string   `json:"workspaceId"`
	Title           string   `json:"title"`
	Prompt          string   `json:"prompt"`
	WorkflowID      string   `json:"workflowId"`
	AttachmentPaths []string `json:"attachmentPaths,omitempty"`
}

type SearchTasksInput struct {
	Keyword string `json:"keyword,omitempty"`
	Limit   int    `json:"limit,omitempty"`
}

type TaskSearchItem struct {
	Task      domaintasks.Task           `json:"task"`
	Workspace domainworkspaces.Workspace `json:"workspace"`
	LatestRun *domainworkflows.Run       `json:"latestRun,omitempty"`
}

type TaskSearchPage struct {
	Items []TaskSearchItem `json:"items"`
	Total int              `json:"total"`
}

type InstructionInput struct {
	Content         string   `json:"content"`
	AttachmentPaths []string `json:"attachmentPaths,omitempty"`
}

type ListRunsInput struct {
	WorkspaceID string `json:"workspaceId"`
	Status      string `json:"status,omitempty"`
	Keyword     string `json:"keyword,omitempty"`
	Cursor      string `json:"cursor,omitempty"`
	Limit       int    `json:"limit,omitempty"`
}

type RunListItem struct {
	Run  domainworkflows.Run `json:"run"`
	Task domaintasks.Task    `json:"task"`
}

type RunListPage struct {
	Items      []RunListItem `json:"items"`
	NextCursor string        `json:"nextCursor,omitempty"`
	Total      int           `json:"total"`
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
	StreamID  string `json:"streamId,omitempty"`
	Revision  uint64 `json:"revision,omitempty"`
	Streaming bool   `json:"streaming,omitempty"`
	Text      string `json:"text,omitempty"`
	Failed    bool   `json:"failed,omitempty"`
	At        string `json:"at"`
}

type RunDetail struct {
	Run           domainworkflows.Run           `json:"run"`
	Task          domaintasks.Task              `json:"task"`
	Workspace     domainworkspaces.Workspace    `json:"workspace"`
	Workflow      domainworkflows.Definition    `json:"workflow"`
	StepRuns      []domainworkflows.StepRun     `json:"stepRuns"`
	Events        []WorkflowEventView           `json:"events"`
	RuntimeEvents []RuntimeEventView            `json:"runtimeEvents"`
	Instructions  []domainworkflows.Instruction `json:"instructions"`
	Active        bool                          `json:"active"`
	LastError     string                        `json:"lastError,omitempty"`
}

type App struct {
	store          *localdata.Store
	orchestrator   *workflowuc.Usecase
	runtimes       *RuntimeRegistry
	git            *gitinspect.Inspector
	rootCtx        context.Context
	rootCancel     context.CancelFunc
	mu             sync.RWMutex
	active         map[string]context.CancelFunc
	lastErrors     map[string]string
	wg             sync.WaitGroup
	workers        *worker.Registry
	workerClient   *worker.Client
	settings       settingsrepo.SettingsRepo
	cleanupMu      sync.Mutex
	cleanupPlans   map[string]cleanupPlan
	confirmations  map[string]runConfirmation
	settingsReload func(domainsettings.Settings) error
	queueMu        sync.Mutex
	runStreams     *runstream.Hub
}

func New(store *localdata.Store, orchestrator *workflowuc.Usecase, runtimes *RuntimeRegistry, git *gitinspect.Inspector) *App {
	ctx, cancel := context.WithCancel(context.Background())
	app := &App{
		store: store, orchestrator: orchestrator, runtimes: runtimes, git: git,
		rootCtx: ctx, rootCancel: cancel, active: make(map[string]context.CancelFunc), lastErrors: make(map[string]string),
		workers: worker.NewRegistry(filepath.Join(store.Data.Paths.Root, "workers.json")), workerClient: worker.NewClient(),
		settings:      settingsrepo.NewSettingsRepo(store.Data.Paths.Root),
		cleanupPlans:  make(map[string]cleanupPlan),
		confirmations: make(map[string]runConfirmation),
	}
	orchestrator.SetRemoteExecutor(remoteExecutor{registry: app.workers, client: app.workerClient})
	return app
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

func (a *App) SetRunStreamHub(hub *runstream.Hub) {
	a.runStreams = hub
	a.orchestrator.SetRunStreamPublisher(hub)
}

func (a *App) GetRunStreamSnapshot(runID string) []runstream.Frame {
	if a.runStreams == nil {
		return []runstream.Frame{}
	}
	return a.runStreams.Snapshot(runID)
}

func (a *App) ListRuntimes() []RuntimeInfo                      { return a.runtimes.List() }
func (a *App) CheckRuntime(runtime string) (RuntimeInfo, error) { return a.runtimes.Check(runtime) }
func (a *App) UpdateRuntimeConfig(input RuntimeConfigInput) (RuntimeInfo, error) {
	current, err := a.settings.Get(context.Background())
	if err != nil {
		return RuntimeInfo{}, mapSettingsError(err)
	}
	runtime := current.Runtimes[input.Runtime]
	runtime.Binary = strings.TrimSpace(input.Binary)
	current.Runtimes[input.Runtime] = runtime
	_, err = a.UpdateRuntimeSettings(context.Background(), current.Runtimes, current.Revision)
	if err != nil {
		return RuntimeInfo{}, err
	}
	return a.runtimes.Check(input.Runtime)
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
		settings, settingsErr := a.settings.Get(ctx)
		if settingsErr != nil {
			return domainworkspaces.Workspace{}, mapSettingsError(settingsErr)
		}
		sandbox = settings.Execution.DefaultSandbox
	}
	if sandbox == string(agentrun.SandboxFull) {
		settings, settingsErr := a.settings.Get(ctx)
		if settingsErr != nil {
			return domainworkspaces.Workspace{}, mapSettingsError(settingsErr)
		}
		if !settings.Security.AllowFullSandbox {
			return domainworkspaces.Workspace{}, coded("security_full_sandbox_disabled", "Full access sandbox is disabled in Settings")
		}
	}
	now := time.Now().UTC()
	workspace := domainworkspaces.Workspace{ID: workspaceID(abs), Name: name, Path: filepath.Clean(abs), DefaultSandbox: sandbox, CreatedAt: now, LastOpenedAt: now}
	if current, getErr := a.store.Repos.Tasks.GetWorkspace(ctx, workspace.ID); getErr == nil {
		workspace.CreatedAt = current.CreatedAt
		workspace.Pinned = current.Pinned
	}
	if err := a.store.Repos.Tasks.SaveWorkspace(ctx, workspace); err != nil {
		return domainworkspaces.Workspace{}, err
	}
	return workspace, nil
}

func (a *App) ListWorkspaces(ctx context.Context) ([]domainworkspaces.Workspace, error) {
	return a.store.Repos.Tasks.ListWorkspaces(ctx)
}

func (a *App) OpenWorkspace(ctx context.Context, id string) (domainworkspaces.Workspace, error) {
	workspace, err := a.GetWorkspace(ctx, strings.TrimSpace(id))
	if err != nil {
		return domainworkspaces.Workspace{}, err
	}
	workspace.LastOpenedAt = time.Now().UTC()
	if err := a.store.Repos.Tasks.SaveWorkspace(ctx, workspace); err != nil {
		return domainworkspaces.Workspace{}, err
	}
	return workspace, nil
}

func (a *App) SetWorkspacePinned(ctx context.Context, id string, pinned bool) (domainworkspaces.Workspace, error) {
	workspace, err := a.GetWorkspace(ctx, strings.TrimSpace(id))
	if err != nil {
		return domainworkspaces.Workspace{}, err
	}
	workspace.Pinned = pinned
	if err := a.store.Repos.Tasks.SaveWorkspace(ctx, workspace); err != nil {
		return domainworkspaces.Workspace{}, err
	}
	return workspace, nil
}

func (a *App) RemoveWorkspace(ctx context.Context, id string) error {
	workspace, err := a.GetWorkspace(ctx, strings.TrimSpace(id))
	if err != nil {
		return err
	}
	workspace.Hidden = true
	workspace.Pinned = false
	if err := a.store.Repos.Tasks.SaveWorkspace(ctx, workspace); err != nil {
		return err
	}
	return nil
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
	input, err := a.prepareDefinition(ctx, input)
	if err != nil {
		return domainworkflows.Definition{}, err
	}
	definition, err := a.store.Repos.Workflows.SaveDefinition(ctx, input)
	if err != nil {
		return domainworkflows.Definition{}, mapDefinitionError(err)
	}
	return definition, nil
}

func (a *App) CreateDefinition(ctx context.Context, input domainworkflows.Definition) (domainworkflows.Definition, error) {
	if _, err := a.store.Repos.Workflows.GetDefinition(ctx, input.ID); err == nil {
		return domainworkflows.Definition{}, coded("workflow_already_exists", "a workflow with this ID already exists")
	} else if !errors.Is(err, repoworkflows.ErrDefinitionNotFound) {
		return domainworkflows.Definition{}, err
	}
	return a.SaveDefinition(ctx, input)
}

func (a *App) UpdateDefinition(ctx context.Context, currentID string, input domainworkflows.Definition) (domainworkflows.Definition, error) {
	input, err := a.prepareDefinition(ctx, input)
	if err != nil {
		return domainworkflows.Definition{}, err
	}
	var previous domainworkflows.Definition
	var taskReferences []domaintasks.Task
	if currentID != input.ID {
		previous, err = a.store.Repos.Workflows.GetDefinition(ctx, currentID)
		if err != nil {
			return domainworkflows.Definition{}, mapDefinitionError(err)
		}
		taskReferences, err = a.store.Repos.Tasks.ListTasks(ctx, "")
		if err != nil {
			return domainworkflows.Definition{}, err
		}
	}
	definition, err := a.store.Repos.Workflows.UpdateDefinition(ctx, currentID, input)
	if err != nil {
		return domainworkflows.Definition{}, mapDefinitionError(err)
	}
	if currentID != definition.ID {
		var migrated []domaintasks.Task
		for _, task := range taskReferences {
			if task.WorkflowID != currentID {
				continue
			}
			original := task
			task.WorkflowID = definition.ID
			if err := a.store.Repos.Tasks.SaveTask(ctx, task); err != nil {
				rollbackErrors := []error{err}
				if _, rollbackErr := a.store.Repos.Workflows.UpdateDefinition(ctx, definition.ID, previous); rollbackErr != nil {
					rollbackErrors = append(rollbackErrors, rollbackErr)
				}
				for _, migratedTask := range migrated {
					if rollbackErr := a.store.Repos.Tasks.SaveTask(ctx, migratedTask); rollbackErr != nil {
						rollbackErrors = append(rollbackErrors, rollbackErr)
					}
				}
				return domainworkflows.Definition{}, fmt.Errorf("migrate workflow task references: %w", errors.Join(rollbackErrors...))
			}
			migrated = append(migrated, original)
		}
	}
	return definition, nil
}

func (a *App) DeleteDefinition(ctx context.Context, id string) error {
	if err := a.store.Repos.Workflows.DeleteDefinition(ctx, id); err != nil {
		return mapDefinitionError(err)
	}
	return nil
}

func (a *App) prepareDefinition(ctx context.Context, input domainworkflows.Definition) (domainworkflows.Definition, error) {
	settings, err := a.settings.Get(ctx)
	if err != nil {
		return domainworkflows.Definition{}, mapSettingsError(err)
	}
	if input.Policy.MaxTransitions == 0 {
		input.Policy.MaxTransitions = settings.Execution.MaxTransitions
	}
	if input.Policy.MaxConsecutiveFailures == 0 {
		input.Policy.MaxConsecutiveFailures = settings.Execution.MaxConsecutiveFailures
	}
	if input.Policy.StepTimeoutSeconds == 0 {
		input.Policy.StepTimeoutSeconds = settings.Execution.StepTimeoutSeconds
	}
	if err := validateDefinitionSettings(input, settings); err != nil {
		return domainworkflows.Definition{}, err
	}
	return input, nil
}

func mapDefinitionError(err error) error {
	var issues domainworkflows.ValidationErrors
	if errors.As(err, &issues) {
		return coded("workflow_invalid_definition", err.Error())
	}
	if errors.Is(err, repoworkflows.ErrDefinitionNotFound) {
		return coded("workflow_not_found", "workflow was not found")
	}
	if errors.Is(err, repoworkflows.ErrDefinitionExists) {
		return coded("workflow_already_exists", "a workflow with this ID already exists")
	}
	return err
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
	marker := filepath.Join(a.store.Data.Paths.Workflows, ".builtins-v1")
	if _, err := os.Stat(marker); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	for _, definition := range builtinDefinitions() {
		if _, err := a.store.Repos.Workflows.GetDefinition(ctx, definition.ID); err == nil {
			continue
		}
		if _, err := a.store.Repos.Workflows.SaveDefinition(ctx, definition); err != nil {
			return err
		}
	}
	return localfile.WriteTextAtomic(marker, "seeded\n")
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
	workspaces, err := a.store.Repos.Tasks.ListWorkspaces(ctx)
	if err != nil {
		return err
	}
	for _, workspace := range workspaces {
		a.reconcileWorkspaceQueue(workspace.ID)
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
	task := domaintasks.Task{ID: randomID("task"), WorkspaceID: strings.TrimSpace(input.WorkspaceID), Title: strings.TrimSpace(input.Title), Prompt: strings.TrimSpace(input.Prompt), WorkflowID: strings.TrimSpace(input.WorkflowID), Status: domaintasks.StatusReady, ExecutionMode: domaintasks.ExecutionImmediate, CreatedAt: now, UpdatedAt: now}
	attachments, err := a.persistAttachments(ctx, task, input.AttachmentPaths)
	if err != nil {
		return domaintasks.Task{}, err
	}
	task.Attachments = attachments
	if err := a.store.Repos.Tasks.SaveTask(ctx, task); err != nil {
		return domaintasks.Task{}, coded("task_invalid", err.Error())
	}
	return task, nil
}

func (a *App) ListTasks(ctx context.Context, workspaceID string) ([]domaintasks.Task, error) {
	return a.store.Repos.Tasks.ListTasks(ctx, workspaceID)
}

func (a *App) SearchTasks(ctx context.Context, input SearchTasksInput) (TaskSearchPage, error) {
	workspaces, err := a.store.Repos.Tasks.ListWorkspaces(ctx)
	if err != nil {
		return TaskSearchPage{}, err
	}
	workspaceByID := make(map[string]domainworkspaces.Workspace, len(workspaces))
	for _, workspace := range workspaces {
		workspaceByID[workspace.ID] = workspace
	}
	tasks, err := a.store.Repos.Tasks.ListTasks(ctx, "")
	if err != nil {
		return TaskSearchPage{}, err
	}
	keyword := strings.ToLower(strings.TrimSpace(input.Keyword))
	matches := make([]domaintasks.Task, 0, len(tasks))
	for _, task := range tasks {
		workspace, visible := workspaceByID[task.WorkspaceID]
		if !visible {
			continue
		}
		if keyword != "" {
			haystack := strings.ToLower(strings.Join([]string{task.Title, task.ID, task.Prompt, workspace.Name, workspace.Path}, "\n"))
			if !strings.Contains(haystack, keyword) {
				continue
			}
		}
		matches = append(matches, task)
	}
	total := len(matches)
	limit := input.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	if len(matches) > limit {
		matches = matches[:limit]
	}
	items := make([]TaskSearchItem, 0, len(matches))
	for _, task := range matches {
		runs, listErr := a.store.Repos.Workflows.ListRunsByTask(ctx, task.ID)
		if listErr != nil {
			return TaskSearchPage{}, listErr
		}
		item := TaskSearchItem{Task: task, Workspace: workspaceByID[task.WorkspaceID]}
		if len(runs) > 0 {
			latest := runs[0]
			item.LatestRun = &latest
		}
		items = append(items, item)
	}
	return TaskSearchPage{Items: items, Total: total}, nil
}

func (a *App) StartRun(ctx context.Context, taskID string) (domainworkflows.Run, error) {
	return a.startRun(ctx, taskID, "")
}

func (a *App) StartRunConfirmed(ctx context.Context, taskID, confirmationToken string) (domainworkflows.Run, error) {
	return a.startRun(ctx, taskID, confirmationToken)
}

func (a *App) startRun(ctx context.Context, taskID, confirmationToken string) (domainworkflows.Run, error) {
	if err := a.validateTaskSecurity(ctx, taskID, confirmationToken); err != nil {
		return domainworkflows.Run{}, err
	}
	return a.startRunAuthorized(ctx, taskID)
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
	definition, err := a.store.Repos.Workflows.GetRunDefinition(ctx, runID)
	if err != nil {
		return run, mapError(err)
	}
	settings, err := a.settings.Get(ctx)
	if err != nil {
		return run, mapSettingsError(err)
	}
	if err := validateDefinitionSettings(definition, settings); err != nil {
		return run, err
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
	if err == nil {
		go a.reconcileQueueForRun(runID)
	}
	return run, mapError(err)
}

func (a *App) ListRunsByTask(ctx context.Context, taskID string) ([]domainworkflows.Run, error) {
	return a.store.Repos.Workflows.ListRunsByTask(ctx, taskID)
}

func (a *App) ListRuns(ctx context.Context, input ListRunsInput) (RunListPage, error) {
	workspaceID := strings.TrimSpace(input.WorkspaceID)
	if _, err := a.GetWorkspace(ctx, workspaceID); err != nil {
		return RunListPage{}, err
	}
	status := domainworkflows.RunStatus(strings.TrimSpace(input.Status))
	if status != "" {
		switch status {
		case domainworkflows.RunReady, domainworkflows.RunRunning, domainworkflows.RunPaused, domainworkflows.RunCompleted, domainworkflows.RunFailed, domainworkflows.RunCancelled:
		default:
			return RunListPage{}, coded("run_query_invalid_status", "run status filter is invalid")
		}
	}
	tasks, err := a.store.Repos.Tasks.ListTasks(ctx, workspaceID)
	if err != nil {
		return RunListPage{}, err
	}
	keyword := strings.ToLower(strings.TrimSpace(input.Keyword))
	taskIDs := make([]string, 0, len(tasks))
	titleTaskIDs := make([]string, 0)
	taskByID := make(map[string]domaintasks.Task, len(tasks))
	for _, task := range tasks {
		taskIDs = append(taskIDs, task.ID)
		taskByID[task.ID] = task
		if keyword != "" && (strings.Contains(strings.ToLower(task.Title), keyword) || strings.Contains(strings.ToLower(task.ID), keyword)) {
			titleTaskIDs = append(titleTaskIDs, task.ID)
		}
	}
	page, err := a.store.Repos.Workflows.ListRuns(ctx, domainworkflows.RunListQuery{TaskIDs: taskIDs, TitleTaskIDs: titleTaskIDs, Status: status, Keyword: keyword, Cursor: strings.TrimSpace(input.Cursor), Limit: input.Limit})
	if errors.Is(err, repoworkflows.ErrInvalidRunCursor) {
		return RunListPage{}, coded("run_query_invalid_cursor", "run cursor is invalid")
	}
	if err != nil {
		return RunListPage{}, err
	}
	items := make([]RunListItem, 0, len(page.Items))
	for _, run := range page.Items {
		task, ok := taskByID[run.TaskID]
		if !ok {
			continue
		}
		items = append(items, RunListItem{Run: run, Task: task})
	}
	return RunListPage{Items: items, NextCursor: page.NextCursor, Total: page.Total}, nil
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
	instructions, err := a.store.Repos.Workflows.ListInstructions(ctx, runID)
	if err != nil {
		return RunDetail{}, err
	}
	detail := RunDetail{Run: run, Task: task, Workspace: workspace, Workflow: workflow, StepRuns: stepRuns, Events: make([]WorkflowEventView, 0, len(events)), RuntimeEvents: []RuntimeEventView{}, Instructions: instructions, Active: a.isActive(runID), LastError: a.lastError(runID)}
	for _, event := range events {
		detail.Events = append(detail.Events, WorkflowEventView{RunID: event.RunID, Seq: event.Seq, Type: event.Type, StepID: event.StepID, Payload: string(event.Payload), At: event.At.Format(time.RFC3339Nano)})
	}
	for stepIndex, stepRun := range stepRuns {
		items, listErr := a.store.Repos.Workflows.ListRuntimeEvents(ctx, runID, stepRun.ID, 0, 10_000)
		if listErr != nil {
			return RunDetail{}, listErr
		}
		enrichStepRunUsage(&detail.StepRuns[stepIndex], items)
		detail.RuntimeEvents = append(detail.RuntimeEvents, foldRuntimeEventViews(stepRun.ID, items)...)
	}
	return detail, nil
}

// enrichStepRunUsage derives the cache breakdown for runs written before the
// detailed fields were added. Runtime event Raw values already retain the
// provider's terminal usage object, so this is a read-only compatibility layer
// rather than a risky migration of users' JSONL history.
func enrichStepRunUsage(stepRun *domainworkflows.StepRun, items []domainworkflows.RuntimeEvent) {
	for index := len(items) - 1; index >= 0; index-- {
		var event agentrun.Event
		if json.Unmarshal(items[index].Payload, &event) != nil || event.Raw == "" {
			continue
		}
		usage, ok := usageFromRuntimeRaw(event.Raw)
		if !ok {
			continue
		}
		if usage.InputTokens > stepRun.InputTokens {
			stepRun.InputTokens = usage.InputTokens
		}
		if usage.OutputTokens > stepRun.OutputTokens {
			stepRun.OutputTokens = usage.OutputTokens
		}
		if usage.CachedInputTokens > stepRun.CachedInputTokens {
			stepRun.CachedInputTokens = usage.CachedInputTokens
		}
		if usage.CacheCreationInputTokens > stepRun.CacheCreationInputTokens {
			stepRun.CacheCreationInputTokens = usage.CacheCreationInputTokens
		}
		if usage.ReasoningOutputTokens > stepRun.ReasoningOutputTokens {
			stepRun.ReasoningOutputTokens = usage.ReasoningOutputTokens
		}
		return
	}
}

func usageFromRuntimeRaw(raw string) (agentrun.Usage, bool) {
	var envelope struct {
		Type  string `json:"type"`
		Usage struct {
			InputTokens              int `json:"input_tokens"`
			CachedInputTokens        int `json:"cached_input_tokens"`
			CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
			CacheReadInputTokens     int `json:"cache_read_input_tokens"`
			OutputTokens             int `json:"output_tokens"`
			ReasoningOutputTokens    int `json:"reasoning_output_tokens"`
		} `json:"usage"`
	}
	if json.Unmarshal([]byte(raw), &envelope) != nil {
		return agentrun.Usage{}, false
	}
	switch envelope.Type {
	case "turn.completed":
		return agentrun.Usage{
			InputTokens:           envelope.Usage.InputTokens,
			CachedInputTokens:     envelope.Usage.CachedInputTokens,
			OutputTokens:          envelope.Usage.OutputTokens,
			ReasoningOutputTokens: envelope.Usage.ReasoningOutputTokens,
		}, true
	case "result":
		return agentrun.Usage{
			InputTokens:              envelope.Usage.InputTokens + envelope.Usage.CacheCreationInputTokens + envelope.Usage.CacheReadInputTokens,
			CachedInputTokens:        envelope.Usage.CacheReadInputTokens,
			CacheCreationInputTokens: envelope.Usage.CacheCreationInputTokens,
			OutputTokens:             envelope.Usage.OutputTokens,
		}, true
	default:
		return agentrun.Usage{}, false
	}
}

func foldRuntimeEventViews(stepRunID string, items []domainworkflows.RuntimeEvent) []RuntimeEventView {
	views := make([]RuntimeEventView, 0, len(items))
	streamIndexes := make(map[string]int)
	for _, item := range items {
		var event agentrun.Event
		if json.Unmarshal(item.Payload, &event) != nil {
			continue
		}
		at := item.At
		if !event.At.IsZero() {
			at = event.At
		}
		view := RuntimeEventView{StepRunID: stepRunID, Seq: item.Seq, Kind: string(event.Kind), StreamID: event.StreamID, Revision: event.Revision, Text: event.Text, Failed: event.Failed, At: at.Format(time.RFC3339Nano)}
		if event.StreamID == "" || event.Phase == "" {
			views = append(views, view)
			continue
		}
		index, exists := streamIndexes[event.StreamID]
		if !exists {
			view.Streaming = event.Phase != agentrun.StreamEnd
			if event.Phase == agentrun.StreamStart {
				view.Text = ""
			}
			streamIndexes[event.StreamID] = len(views)
			views = append(views, view)
			continue
		}
		current := views[index]
		switch event.Phase {
		case agentrun.StreamDelta:
			current.Text += event.Text
		case agentrun.StreamSnapshot, agentrun.StreamEnd:
			current.Text = event.Text
		}
		current.Kind = string(event.Kind)
		current.Revision = event.Revision
		current.Streaming = event.Phase != agentrun.StreamEnd
		current.Failed = current.Failed || event.Failed
		current.At = view.At
		views[index] = current
	}
	return views
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
		defer func() {
			if a.runStreams != nil {
				a.runStreams.ClearRun(runID)
			}
		}()
		_, err := execute(runCtx)
		a.mu.Lock()
		delete(a.active, runID)
		if err != nil && !errors.Is(err, context.Canceled) {
			a.lastErrors[runID] = mapError(err).Error()
		}
		a.mu.Unlock()
		a.reconcileQueueForRun(runID)
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
		{ID: "parallel_review", Name: "并行审查 DAG", Description: "安全与测试节点并行只读分析，等待全部完成后由汇总节点落地修改。", Mode: domainworkflows.ModeDAG, EntryStepID: "security", Layout: domainworkflows.Layout{Nodes: map[string]domainworkflows.Point{"security": {X: 80, Y: 90}, "tests": {X: 80, Y: 280}, "synthesis": {X: 420, Y: 185}}}, Steps: []domainworkflows.Step{
			{ID: "security", Name: "安全审查", Runtime: "codex", WorkerID: "local", Sandbox: "read-only", RolePrompt: "你是安全审查 Agent。", Instruction: "独立检查安全风险并给出可操作建议。", Transitions: map[string]string{"completed": domainworkflows.TargetDone, "need_human": domainworkflows.TargetPause}},
			{ID: "tests", Name: "测试审查", Runtime: "claude", WorkerID: "local", Sandbox: "read-only", RolePrompt: "你是测试与可靠性审查 Agent。", Instruction: "独立检查测试覆盖、并发和回归风险。", Transitions: map[string]string{"completed": domainworkflows.TargetDone, "need_human": domainworkflows.TargetPause}},
			{ID: "synthesis", Name: "汇总落地", Runtime: "codex", WorkerID: "local", Sandbox: "workspace-write", DependsOn: []string{"security", "tests"}, RolePrompt: "你是负责汇总并落地修改的 Agent。", Instruction: "结合所有直接依赖结果实施修改并验证。", Transitions: map[string]string{"completed": domainworkflows.TargetDone, "need_human": domainworkflows.TargetPause}},
		}},
	}
}
