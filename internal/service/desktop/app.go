// Package desktop exposes the desktop-facing service for local Agent workflow
// orchestration. It composes repositories and use cases while keeping transport
// concerns such as Wails outside this package.
package desktop

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	pathpkg "path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	domainsettings "github.com/openmodu/onecatch/internal/domain/settings"
	domaintasks "github.com/openmodu/onecatch/internal/domain/tasks"
	domainworkflows "github.com/openmodu/onecatch/internal/domain/workflows"
	domainworkspaces "github.com/openmodu/onecatch/internal/domain/workspaces"
	"github.com/openmodu/onecatch/internal/remotefs"
	"github.com/openmodu/onecatch/internal/repo/git"
	settingsrepo "github.com/openmodu/onecatch/internal/repo/settings"
	localdata "github.com/openmodu/onecatch/internal/repo/store/local"
	repoworkflows "github.com/openmodu/onecatch/internal/repo/workflows"
	"github.com/openmodu/onecatch/internal/repo/workspacelock"
	"github.com/openmodu/onecatch/internal/service/desktop/runstate"
	"github.com/openmodu/onecatch/internal/service/desktop/runstream"
	"github.com/openmodu/onecatch/internal/service/worker"
	"github.com/openmodu/onecatch/internal/sshcredentials"
	"github.com/openmodu/onecatch/internal/sshendpoint"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
	agentrunseam "github.com/openmodu/onecatch/internal/usecase/agentrun/seam"
	workflowuc "github.com/openmodu/onecatch/internal/usecase/workflows"
	"github.com/openmodu/onecatch/pkg/localfile"
)

const directAgentWorkflowID = "single_agent"

type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e Error) Error() string { return e.Code + ": " + e.Message }

func coded(code, message string) error { return Error{Code: code, Message: message} }

type AddWorkspaceInput struct {
	Path           string                     `json:"path"`
	Name           string                     `json:"name,omitempty"`
	DefaultSandbox string                     `json:"defaultSandbox,omitempty"`
	RemoteFS       *domainworkspaces.RemoteFS `json:"remoteFs,omitempty"`
	Password       string                     `json:"password,omitempty"`
}

type UpdateWorkspaceInput struct {
	ID             string                     `json:"id"`
	Path           string                     `json:"path"`
	Name           string                     `json:"name,omitempty"`
	DefaultSandbox string                     `json:"defaultSandbox,omitempty"`
	RemoteFS       *domainworkspaces.RemoteFS `json:"remoteFs,omitempty"`
	Password       string                     `json:"password,omitempty"`
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
	Sandbox         string   `json:"sandbox,omitempty"`
	Harness         string   `json:"harness,omitempty"`
	Model           string   `json:"model,omitempty"`
	ReasoningEffort string   `json:"reasoningEffort,omitempty"`
	ServiceTier     string   `json:"serviceTier,omitempty"`
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

type ResumeRunInput struct {
	Instruction     string `json:"instruction"`
	StepID          string `json:"stepId,omitempty"`
	Harness         string `json:"harness,omitempty"`
	Model           string `json:"model,omitempty"`
	ReasoningEffort string `json:"reasoningEffort,omitempty"`
	ServiceTier     string `json:"serviceTier,omitempty"`
}

type PermissionDecisionInput struct {
	RunID     string `json:"runId"`
	RequestID string `json:"requestId"`
	Decision  string `json:"decision"`
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
	StepRunID          string                      `json:"stepRunId"`
	Seq                int64                       `json:"seq"`
	Kind               string                      `json:"kind"`
	StreamID           string                      `json:"streamId,omitempty"`
	Revision           uint64                      `json:"revision,omitempty"`
	Streaming          bool                        `json:"streaming,omitempty"`
	Text               string                      `json:"text,omitempty"`
	Failed             bool                        `json:"failed,omitempty"`
	Permission         *agentrun.PermissionRequest `json:"permission,omitempty"`
	PermissionDecision string                      `json:"permissionDecision,omitempty"`
	At                 string                      `json:"at"`
}

type RunDetail struct {
	Run           domainworkflows.Run        `json:"run"`
	Task          domaintasks.Task           `json:"task"`
	Workspace     domainworkspaces.Workspace `json:"workspace"`
	Workflow      domainworkflows.Definition `json:"workflow"`
	StepRuns      []domainworkflows.StepRun  `json:"stepRuns"`
	Events        []WorkflowEventView        `json:"events"`
	RuntimeEvents []RuntimeEventView         `json:"runtimeEvents"`
	// RuntimeEventsTotal is how many entries the transcript has in full, which
	// may exceed the window carried in RuntimeEvents.
	RuntimeEventsTotal int                           `json:"runtimeEventsTotal"`
	Instructions       []domainworkflows.Instruction `json:"instructions"`
	Active             bool                          `json:"active"`
	LastError          string                        `json:"lastError,omitempty"`
}

// runtimeTranscriptWindow bounds what opening a run ships to the UI. A long
// session's transcript is unbounded, and every entry becomes a mounted
// component, so the newest slice is sent first and the rest is fetched only if
// the user scrolls back for it.
const runtimeTranscriptWindow = 400

type Service struct {
	store             *localdata.Store
	orchestrator      *workflowuc.Usecase
	runtimes          *RuntimeRegistry
	git               *gitrepo.Inspector
	rootCtx           context.Context
	rootCancel        context.CancelFunc
	mu                sync.RWMutex
	active            map[string]context.CancelFunc
	lastErrors        map[string]string
	wg                sync.WaitGroup
	workers           *worker.Registry
	workerClient      *worker.Client
	remotePermissions *remotePermissionRegistry
	settings          settingsrepo.SettingsRepo
	cleanupMu         sync.Mutex
	cleanupPlans      map[string]cleanupPlan
	confirmations     map[string]runConfirmation
	settingsReload    func(domainsettings.Settings) error
	queueMu           sync.Mutex
	followUpMu        sync.Mutex
	titleMu           sync.Mutex
	pendingTitles     map[string]pendingTaskTitle
	runStreams        *runstream.Hub
	runStates         *runstate.Hub
	remoteFSProbe     func(context.Context, domainworkspaces.RemoteFS) (string, error)
	remoteCredentials sshcredentials.Store
	remoteGitExecutor func(domainworkspaces.RemoteFS) agentrunseam.Executor
}

func NewService(store *localdata.Store, orchestrator *workflowuc.Usecase, runtimes *RuntimeRegistry, git *gitrepo.Inspector) *Service {
	ctx, cancel := context.WithCancel(context.Background())
	app := &Service{
		store: store, orchestrator: orchestrator, runtimes: runtimes, git: git,
		rootCtx: ctx, rootCancel: cancel, active: make(map[string]context.CancelFunc), lastErrors: make(map[string]string),
		workers: worker.NewRegistry(filepath.Join(store.Data.Paths.Root, "workers.json")), workerClient: worker.NewClient(),
		settings:          settingsrepo.NewSettingsRepo(store.Data.Paths.Root),
		cleanupPlans:      make(map[string]cleanupPlan),
		confirmations:     make(map[string]runConfirmation),
		pendingTitles:     make(map[string]pendingTaskTitle),
		remoteFSProbe:     canonicalRemoteFSRoot,
		remoteCredentials: sshcredentials.KeyringStore{},
		remoteGitExecutor: newRemoteGitExecutor,
	}
	app.remotePermissions = newRemotePermissionRegistry(app.workerClient)
	orchestrator.SetRemoteExecutor(&remoteExecutor{registry: app.workers, client: app.workerClient, permissions: app.remotePermissions, preparations: newRemotePreparationRegistry()})
	return app
}

func (a *Service) Close() error {
	a.rootCancel()
	a.mu.Lock()
	for _, cancel := range a.active {
		cancel()
	}
	a.mu.Unlock()
	a.wg.Wait()
	_ = a.runtimes.Close()
	return a.store.Close()
}

func (a *Service) DataRoot() string { return a.store.Data.Paths.Root }

func (a *Service) SetRunStreamHub(hub *runstream.Hub) {
	a.runStreams = hub
	a.orchestrator.SetRuntimeEventPublisher(hub)
}

// SetRunStateHub installs the push channel for the bounded half of a run's
// state. The hub only ever calls back into RunStateView, which reads run, step
// runs and instructions — never the transcript — so a push stays cheap however
// long the run has been going.
func (a *Service) SetRunStateHub(hub *runstate.Hub) {
	a.runStates = hub
	if hub == nil {
		return
	}
	hub.SetResolver(a.RunStateView)
}

// RunStateView assembles the pushed slice of a run. It returns false when the
// run has been deleted between the notification and this read.
func (a *Service) RunStateView(runID string) (runstate.View, bool) {
	ctx, cancel := context.WithTimeout(a.rootCtx, 5*time.Second)
	defer cancel()
	run, err := a.store.Repos.Workflows.GetRun(ctx, runID)
	if err != nil {
		return runstate.View{}, false
	}
	stepRuns, err := a.store.Repos.Workflows.ListStepRuns(ctx, runID)
	if err != nil {
		return runstate.View{}, false
	}
	instructions, err := a.store.Repos.Workflows.ListInstructions(ctx, runID)
	if err != nil {
		return runstate.View{}, false
	}
	return runstate.View{
		RunID:        runID,
		Run:          run,
		StepRuns:     stepRuns,
		Instructions: instructions,
		Active:       a.isActive(runID),
	}, true
}

func (a *Service) GetRunStreamSnapshot(runID string) []runstream.Frame {
	if a.runStreams == nil {
		return []runstream.Frame{}
	}
	return a.runStreams.Snapshot(runID)
}

func (a *Service) ListRuntimes() []RuntimeInfo { return a.runtimes.List() }

// ListSkills returns the selected runtime's effective user-invocable Skill
// catalog. OneCatch exposes every runtime through the same `$name` UI syntax;
// the runner retains ownership of discovery and native invocation details.
func (a *Service) ListSkills(ctx context.Context, runtime, cwd string) ([]agentrun.Skill, error) {
	settings, err := a.settings.Get(ctx)
	if err != nil {
		return nil, mapSettingsError(err)
	}
	runtimeID := agentrun.Runtime(strings.TrimSpace(runtime))
	runtimeSettings, ok := settings.Runtimes[string(runtimeID)]
	if !ok {
		return nil, coded("runtime_not_found", "Unknown runtime")
	}
	if !runtimeSettings.Enabled {
		return nil, coded("runtime_disabled", string(runtimeID)+" is disabled")
	}
	cwd = strings.TrimSpace(cwd)
	if cwd == "" {
		cwd, _ = os.UserHomeDir()
	}
	listCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	return a.runtimes.ListSkills(listCtx, runtimeID, cwd, allowedEnvironment(runtimeSettings.EnvironmentAllowlist))
}

// CheckRuntime answers "is this runtime available right now", so unlike
// ListRuntimes it must not serve a cached probe.
func (a *Service) CheckRuntime(runtime string) (RuntimeInfo, error) {
	a.runtimes.invalidateRuntimeStatus(runtime)
	return a.runtimes.Check(runtime)
}
func (a *Service) UpdateRuntimeConfig(input RuntimeConfigInput) (RuntimeInfo, error) {
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

func (a *Service) AddWorkspace(ctx context.Context, input AddWorkspaceInput) (domainworkspaces.Workspace, error) {
	return a.saveWorkspace(ctx, input, "")
}

func (a *Service) UpdateWorkspace(ctx context.Context, input UpdateWorkspaceInput) (domainworkspaces.Workspace, error) {
	id := strings.TrimSpace(input.ID)
	if id == "" {
		return domainworkspaces.Workspace{}, coded("workspace_invalid", "workspace id is required")
	}
	return a.saveWorkspace(ctx, AddWorkspaceInput{
		Path: input.Path, Name: input.Name, DefaultSandbox: input.DefaultSandbox,
		RemoteFS: input.RemoteFS, Password: input.Password,
	}, id)
}

func (a *Service) saveWorkspace(ctx context.Context, input AddWorkspaceInput, updateID string) (domainworkspaces.Workspace, error) {
	var updating *domainworkspaces.Workspace
	if updateID != "" {
		current, err := a.GetWorkspace(ctx, updateID)
		if err != nil {
			return domainworkspaces.Workspace{}, err
		}
		updating = &current
	}
	workspacePath := strings.TrimSpace(input.Path)
	identity := workspacePath
	var remoteFS *domainworkspaces.RemoteFS
	var stagedCredentialID string
	credentialCommitted := false
	credentials := a.remoteCredentials
	if credentials == nil {
		credentials = sshcredentials.KeyringStore{}
	}
	defer func() {
		if stagedCredentialID != "" && !credentialCommitted {
			_ = credentials.Delete(stagedCredentialID)
		}
	}()
	if input.RemoteFS != nil {
		settings, settingsErr := a.settings.Get(ctx)
		if settingsErr != nil {
			return domainworkspaces.Workspace{}, mapSettingsError(settingsErr)
		}
		if !settings.HasRemoteFSHarness() {
			return domainworkspaces.Workspace{}, coded("remote_fs_no_harness", "enable at least one remote FS capable Agent in Settings")
		}
		remote := domainworkspaces.RemoteFS{
			Host:     strings.TrimSpace(input.RemoteFS.Host),
			Root:     pathpkg.Clean(strings.TrimSpace(input.RemoteFS.Root)),
			Username: strings.TrimSpace(input.RemoteFS.Username),
		}
		if input.Password == "" && updating != nil && updating.RemoteFS != nil && strings.TrimSpace(updating.RemoteFS.Username) == remote.Username {
			remote.CredentialID = updating.RemoteFS.CredentialID
		}
		for _, option := range input.RemoteFS.SSHOptions {
			if option = strings.TrimSpace(option); option != "" {
				remote.SSHOptions = append(remote.SSHOptions, option)
			}
		}
		if remote.Host == "" {
			return domainworkspaces.Workspace{}, coded("remote_fs_invalid", "SSH host is required")
		}
		endpoint, err := sshendpoint.Parse(remote.Host)
		if err != nil {
			return domainworkspaces.Workspace{}, coded("remote_fs_invalid", err.Error())
		}
		remote.Host = endpoint.String()
		if !pathpkg.IsAbs(remote.Root) {
			return domainworkspaces.Workspace{}, coded("remote_fs_invalid", "remote root must be absolute")
		}
		if strings.ContainsAny(remote.Username, "\x00\r\n") {
			return domainworkspaces.Workspace{}, coded("remote_fs_invalid", "SSH username contains invalid characters")
		}
		if input.Password != "" {
			if remote.Username == "" {
				return domainworkspaces.Workspace{}, coded("remote_fs_credentials_invalid", "SSH username is required for password authentication")
			}
			if len(input.Password) > 2048 || strings.ContainsAny(input.Password, "\x00\r\n") {
				return domainworkspaces.Workspace{}, coded("remote_fs_credentials_invalid", "SSH password contains unsupported characters or is too long")
			}
			credentialID, err := sshcredentials.NewID()
			if err != nil {
				return domainworkspaces.Workspace{}, coded("remote_fs_credentials_unavailable", err.Error())
			}
			if err := credentials.Set(credentialID, input.Password); err != nil {
				return domainworkspaces.Workspace{}, coded("remote_fs_credentials_unavailable", err.Error())
			}
			remote.CredentialID = credentialID
			stagedCredentialID = credentialID
		}
		probe := a.remoteFSProbe
		if probe == nil {
			probe = canonicalRemoteFSRoot
		}
		canonical, err := probe(ctx, remote)
		if err != nil {
			return domainworkspaces.Workspace{}, coded("remote_fs_unavailable", err.Error())
		}
		remote.Root = pathpkg.Clean(canonical)
		remoteFS = &remote
		workspacePath = remote.Root
		identity = "ssh://" + remote.Host + remote.Root
		if remote.Username != "" {
			identity = "ssh://" + remote.Username + "@" + remote.Host + remote.Root
		}
	}
	if workspacePath == "" {
		return domainworkspaces.Workspace{}, coded("workspace_invalid", "path is required")
	}
	if remoteFS == nil {
		abs, err := filepath.Abs(workspacePath)
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
		workspacePath = filepath.Clean(abs)
		identity = workspacePath
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = pathpkg.Base(filepath.ToSlash(workspacePath))
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
		if remoteFS != nil {
			return domainworkspaces.Workspace{}, coded("remote_fs_full_sandbox_unsupported", "remote FS workspaces support read-only or workspace-write sandboxing")
		}
		settings, settingsErr := a.settings.Get(ctx)
		if settingsErr != nil {
			return domainworkspaces.Workspace{}, mapSettingsError(settingsErr)
		}
		if !settings.Security.AllowFullSandbox {
			return domainworkspaces.Workspace{}, coded("security_full_sandbox_disabled", "Full access sandbox is disabled in Settings")
		}
	}
	now := time.Now().UTC()
	id := workspaceID(identity)
	if updateID != "" {
		id = updateID
	}
	workspace := domainworkspaces.Workspace{ID: id, Name: name, Path: workspacePath, RemoteFS: remoteFS, DefaultSandbox: sandbox, CreatedAt: now, LastOpenedAt: now}
	oldCredentialID := ""
	if current, getErr := a.store.Repos.Tasks.GetWorkspace(ctx, workspace.ID); getErr == nil {
		workspace.CreatedAt = current.CreatedAt
		workspace.LastOpenedAt = current.LastOpenedAt
		workspace.Pinned = current.Pinned
		if current.RemoteFS != nil {
			oldCredentialID = current.RemoteFS.CredentialID
		}
	}
	if err := a.store.Repos.Tasks.SaveWorkspace(ctx, workspace); err != nil {
		return domainworkspaces.Workspace{}, err
	}
	credentialCommitted = true
	newCredentialID := ""
	if remoteFS != nil {
		newCredentialID = remoteFS.CredentialID
	}
	if oldCredentialID != "" && oldCredentialID != newCredentialID {
		_ = credentials.Delete(oldCredentialID)
	}
	return workspace, nil
}

func canonicalRemoteFSRoot(ctx context.Context, remote domainworkspaces.RemoteFS) (string, error) {
	backend, err := remotefs.NewSFTPBackend(ctx, remotefs.SFTPConfig{
		Host: remote.Host, Root: remote.Root, Username: remote.Username, CredentialID: remote.CredentialID, SSHOptions: remote.SSHOptions,
	})
	if err != nil {
		return "", err
	}
	defer backend.Close()
	return backend.RealPath(".")
}

func (a *Service) ListWorkspaces(ctx context.Context) ([]domainworkspaces.Workspace, error) {
	return a.store.Repos.Tasks.ListWorkspaces(ctx)
}

func (a *Service) OpenWorkspace(ctx context.Context, id string) (domainworkspaces.Workspace, error) {
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

func (a *Service) SetWorkspacePinned(ctx context.Context, id string, pinned bool) (domainworkspaces.Workspace, error) {
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

func (a *Service) RemoveWorkspace(ctx context.Context, id string) error {
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

func (a *Service) GetWorkspace(ctx context.Context, id string) (domainworkspaces.Workspace, error) {
	workspace, err := a.store.Repos.Tasks.GetWorkspace(ctx, id)
	if err != nil {
		return workspace, coded("workspace_not_found", "workspace was not found")
	}
	return workspace, nil
}

func (a *Service) GetWorkspaceStatus(ctx context.Context, id string) (WorkspaceStatus, error) {
	workspace, err := a.GetWorkspace(ctx, id)
	if err != nil {
		return WorkspaceStatus{}, err
	}
	if workspace.RemoteFS != nil {
		probe := a.remoteFSProbe
		if probe == nil {
			probe = canonicalRemoteFSRoot
		}
		probeCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
		defer cancel()
		if _, err := probe(probeCtx, *workspace.RemoteFS); err != nil {
			return WorkspaceStatus{}, coded("remote_fs_unavailable", err.Error())
		}
		return WorkspaceStatus{Workspace: workspace}, nil
	}
	snapshot, err := a.git.Inspect(ctx, workspace.Path)
	if err != nil {
		return WorkspaceStatus{}, err
	}
	return WorkspaceStatus{Workspace: workspace, Git: snapshot}, nil
}

func (a *Service) ValidateDefinition(input domainworkflows.Definition) []domainworkflows.ValidationIssue {
	err := domainworkflows.Validate(input)
	var issues domainworkflows.ValidationErrors
	if errors.As(err, &issues) {
		return []domainworkflows.ValidationIssue(issues)
	}
	return []domainworkflows.ValidationIssue{}
}

func (a *Service) SaveDefinition(ctx context.Context, input domainworkflows.Definition) (domainworkflows.Definition, error) {
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

func (a *Service) CreateDefinition(ctx context.Context, input domainworkflows.Definition) (domainworkflows.Definition, error) {
	if _, err := a.store.Repos.Workflows.GetDefinition(ctx, input.ID); err == nil {
		return domainworkflows.Definition{}, coded("workflow_already_exists", "a workflow with this ID already exists")
	} else if !errors.Is(err, repoworkflows.ErrDefinitionNotFound) {
		return domainworkflows.Definition{}, err
	}
	return a.SaveDefinition(ctx, input)
}

func (a *Service) UpdateDefinition(ctx context.Context, currentID string, input domainworkflows.Definition) (domainworkflows.Definition, error) {
	if strings.TrimSpace(currentID) == directAgentWorkflowID {
		return domainworkflows.Definition{}, coded("workflow_builtin_readonly", "the direct Agent definition is managed by OneCatch")
	}
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

func (a *Service) DeleteDefinition(ctx context.Context, id string) error {
	if strings.TrimSpace(id) == directAgentWorkflowID {
		return coded("workflow_builtin_readonly", "the direct Agent definition is managed by OneCatch")
	}
	if err := a.store.Repos.Workflows.DeleteDefinition(ctx, id); err != nil {
		return mapDefinitionError(err)
	}
	return nil
}

func (a *Service) prepareDefinition(ctx context.Context, input domainworkflows.Definition) (domainworkflows.Definition, error) {
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

func (a *Service) ListDefinitions(ctx context.Context) ([]domainworkflows.Definition, error) {
	return a.store.Repos.Workflows.ListDefinitions(ctx)
}

func (a *Service) GetDefinition(ctx context.Context, id string) (domainworkflows.Definition, error) {
	id = strings.TrimSpace(id)
	if id == directAgentWorkflowID {
		if err := a.ensureDirectAgentDefinition(ctx); err != nil {
			return domainworkflows.Definition{}, mapDefinitionError(err)
		}
	}
	definition, err := a.store.Repos.Workflows.GetDefinition(ctx, id)
	if err != nil {
		return definition, coded("workflow_not_found", "workflow was not found")
	}
	return definition, nil
}

func (a *Service) EnsureBuiltinDefinitions(ctx context.Context) error {
	// The direct Agent target is application infrastructure, not a user
	// workflow. Repair it on every launch even when an older build allowed the
	// backing definition to be deleted after the seed marker was written.
	if err := a.ensureDirectAgentDefinition(ctx); err != nil {
		return err
	}
	marker := filepath.Join(a.store.Data.Paths.Workflows, ".builtins-v1")
	if _, err := os.Stat(marker); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	for _, definition := range builtinDefinitions() {
		if definition.ID == directAgentWorkflowID {
			continue
		}
		if _, err := a.store.Repos.Workflows.GetDefinition(ctx, definition.ID); err == nil {
			continue
		}
		if _, err := a.store.Repos.Workflows.SaveDefinition(ctx, definition); err != nil {
			return err
		}
	}
	return localfile.WriteTextAtomic(marker, "seeded\n")
}

func (a *Service) ensureDirectAgentDefinition(ctx context.Context) error {
	if _, err := a.store.Repos.Workflows.GetDefinition(ctx, directAgentWorkflowID); err == nil {
		return nil
	} else if !errors.Is(err, repoworkflows.ErrDefinitionNotFound) {
		return err
	}
	_, err := a.store.Repos.Workflows.SaveDefinition(ctx, builtinDefinitions()[0])
	if errors.Is(err, repoworkflows.ErrDefinitionExists) {
		return nil
	}
	return err
}

// RecoverInterruptedRuns repairs the narrow crash window between persisting a
// running Run and owning its Agent process. A live lock means another OneCatch
// process is still executing that workspace, so the Run is not changed.
func (a *Service) RecoverInterruptedRuns(ctx context.Context) error {
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

func (a *Service) CreateTask(ctx context.Context, input CreateTaskInput) (domaintasks.Task, error) {
	workspace, err := a.GetWorkspace(ctx, strings.TrimSpace(input.WorkspaceID))
	if err != nil {
		return domaintasks.Task{}, err
	}
	definition, err := a.GetDefinition(ctx, strings.TrimSpace(input.WorkflowID))
	if err != nil {
		return domaintasks.Task{}, err
	}
	settings, err := a.settings.Get(ctx)
	if err != nil {
		return domaintasks.Task{}, mapSettingsError(err)
	}
	validationDefinition := definition
	validationDefinition.Steps = append([]domainworkflows.Step(nil), definition.Steps...)
	if strings.TrimSpace(input.Harness) != "" && validationDefinition.ID == directAgentWorkflowID && len(validationDefinition.Steps) == 1 {
		validationDefinition.Steps[0].Runtime = strings.TrimSpace(input.Harness)
	}
	if err := validateHarnessSettings(validationDefinition, settings, workspace.RemoteFS != nil); err != nil {
		return domaintasks.Task{}, err
	}
	title := strings.TrimSpace(input.Title)
	refineTitle := title == ""
	if refineTitle {
		title = taskTitleFromPrompt(input.Prompt, "新建任务")
	}
	now := time.Now().UTC()
	task := domaintasks.Task{ID: randomID("task"), WorkspaceID: strings.TrimSpace(input.WorkspaceID), Title: title, Prompt: strings.TrimSpace(input.Prompt), WorkflowID: strings.TrimSpace(input.WorkflowID), Sandbox: strings.TrimSpace(input.Sandbox), Harness: strings.TrimSpace(input.Harness), Model: strings.TrimSpace(input.Model), ReasoningEffort: strings.TrimSpace(input.ReasoningEffort), ServiceTier: strings.TrimSpace(input.ServiceTier), Status: domaintasks.StatusReady, ExecutionMode: domaintasks.ExecutionImmediate, CreatedAt: now, UpdatedAt: now}
	attachments, err := a.persistAttachments(ctx, task, input.AttachmentPaths)
	if err != nil {
		return domaintasks.Task{}, err
	}
	task.Attachments = attachments
	if err := a.store.Repos.Tasks.SaveTask(ctx, task); err != nil {
		return domaintasks.Task{}, coded("task_invalid", err.Error())
	}
	if refineTitle {
		titleWorkspace := workspace.Path
		if workspace.RemoteFS != nil {
			// Title generation is explicitly tool-free. It still needs a valid
			// local cwd for the harness process, so never hand it the remote path.
			titleWorkspace = os.TempDir()
		}
		a.queueTaskTitleRefinement(task.ID, title, titleWorkspace, definition, input)
	}
	return task, nil
}

func (a *Service) ListTasks(ctx context.Context, workspaceID string) ([]domaintasks.Task, error) {
	return a.store.Repos.Tasks.ListTasks(ctx, workspaceID)
}

func (a *Service) SearchTasks(ctx context.Context, input SearchTasksInput) (TaskSearchPage, error) {
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

func (a *Service) StartRun(ctx context.Context, taskID string) (domainworkflows.Run, error) {
	return a.startRun(ctx, taskID, "")
}

func (a *Service) StartRunConfirmed(ctx context.Context, taskID, confirmationToken string) (domainworkflows.Run, error) {
	return a.startRun(ctx, taskID, confirmationToken)
}

func (a *Service) startRun(ctx context.Context, taskID, confirmationToken string) (domainworkflows.Run, error) {
	if err := a.validateTaskSecurity(ctx, taskID, confirmationToken); err != nil {
		return domainworkflows.Run{}, err
	}
	return a.startRunAuthorized(ctx, taskID)
}

func (a *Service) ResumeRun(ctx context.Context, runID, instruction string) (domainworkflows.Run, error) {
	return a.ResumeRunConfigured(ctx, runID, ResumeRunInput{Instruction: instruction})
}

func validateResumeHarness(task domaintasks.Task, definition domainworkflows.Definition, requested string) error {
	requested = strings.TrimSpace(requested)
	if requested == "" || task.WorkflowID != directAgentWorkflowID {
		return nil
	}
	locked := strings.TrimSpace(task.Harness)
	if locked == "" && len(definition.Steps) > 0 {
		locked = strings.TrimSpace(definition.Steps[0].Runtime)
	}
	if locked != "" && requested != locked {
		return coded("runtime_locked", "the Agent selected for this conversation cannot be changed")
	}
	return nil
}

func (a *Service) ResumeRunConfigured(ctx context.Context, runID string, input ResumeRunInput) (domainworkflows.Run, error) {
	run, err := a.store.Repos.Workflows.GetRun(ctx, runID)
	if err != nil {
		return run, coded("run_not_found", "run was not found")
	}
	if run.Status != domainworkflows.RunPaused && run.Status != domainworkflows.RunCompleted {
		return run, coded("run_invalid_state", "only paused or completed runs can be resumed")
	}
	if a.isActive(runID) {
		return run, coded("run_invalid_state", "run is still stopping")
	}
	definition, err := a.store.Repos.Workflows.GetRunDefinition(ctx, runID)
	if err != nil {
		return run, mapError(err)
	}
	task, err := a.store.Repos.Tasks.GetTask(ctx, run.TaskID)
	if err != nil {
		return run, coded("task_not_found", "task was not found")
	}
	if err := validateResumeHarness(task, definition, input.Harness); err != nil {
		return run, err
	}
	settings, err := a.settings.Get(ctx)
	if err != nil {
		return run, mapSettingsError(err)
	}
	if err := validateDefinitionSettings(definition, settings); err != nil {
		return run, err
	}
	workspace, err := a.store.Repos.Tasks.GetWorkspace(ctx, task.WorkspaceID)
	if err != nil {
		return run, coded("workspace_not_found", "workspace was not found")
	}
	if err := validateHarnessSettings(definition, settings, workspace.RemoteFS != nil); err != nil {
		return run, err
	}
	profile := workflowuc.ResumeProfile{}
	if harness := strings.TrimSpace(input.Harness); harness != "" {
		runtime := agentrun.Runtime(harness)
		if !runtime.Valid() {
			return run, coded("runtime_unknown", "selected runtime is unknown")
		}
		if !a.runtimes.Available(runtime) {
			return run, coded("runtime_unavailable", "selected runtime is unavailable")
		}
		resolved := resolvedRuntimeSetting(harness, settings.Runtimes[harness])
		if reasoningEffort := strings.TrimSpace(input.ReasoningEffort); reasoningEffort != "" {
			resolved.ReasoningEffort = reasoningEffort
		}
		if serviceTier := strings.TrimSpace(input.ServiceTier); serviceTier != "" {
			resolved.ServiceTier = serviceTier
		}
		model := strings.TrimSpace(input.Model)
		if model == "" {
			model = settings.Runtimes[harness].DefaultModel
		}
		profile = workflowuc.ResumeProfile{StepID: strings.TrimSpace(input.StepID), Harness: harness, Model: model, RuntimeSettings: resolved}
	}
	a.dispatch(run.ID, func(runCtx context.Context) (domainworkflows.Run, error) {
		return a.orchestrator.ResumeRunWithProfile(runCtx, run.ID, input.Instruction, profile)
	})
	return run, nil
}

func (a *Service) InterruptRun(ctx context.Context, runID string) (domainworkflows.Run, error) {
	a.mu.RLock()
	cancel := a.active[runID]
	a.mu.RUnlock()
	if cancel == nil {
		return domainworkflows.Run{}, coded("run_invalid_state", "run is not actively executing")
	}
	cancel()
	return a.store.Repos.Workflows.GetRun(ctx, runID)
}

func (a *Service) CancelRun(ctx context.Context, runID string) (domainworkflows.Run, error) {
	if a.isActive(runID) {
		return domainworkflows.Run{}, coded("run_invalid_state", "interrupt the active run before cancelling it")
	}
	run, err := a.orchestrator.CancelRun(ctx, runID)
	if err == nil {
		go a.reconcileQueueForRun(runID)
	}
	return run, mapError(err)
}

func (a *Service) ListRunsByTask(ctx context.Context, taskID string) ([]domainworkflows.Run, error) {
	return a.store.Repos.Workflows.ListRunsByTask(ctx, taskID)
}

func (a *Service) ListRuns(ctx context.Context, input ListRunsInput) (RunListPage, error) {
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

func (a *Service) ListRunEvents(ctx context.Context, runID string, afterSeq int64) ([]WorkflowEventView, error) {
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

func (a *Service) RespondPermission(input PermissionDecisionInput) error {
	runID, requestID, decision := strings.TrimSpace(input.RunID), strings.TrimSpace(input.RequestID), strings.TrimSpace(input.Decision)
	if found, err := a.remotePermissions.resolve(runID, requestID, decision); found {
		return err
	}
	return a.runtimes.ResolvePermission(runID, requestID, decision)
}

func (a *Service) GetRunDetail(ctx context.Context, runID string) (RunDetail, error) {
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
		views, usage := foldRuntimeEventViews(stepRun.ID, items, needsUsageBackfill(detail.StepRuns[stepIndex]))
		applyUsageBackfill(&detail.StepRuns[stepIndex], usage)
		detail.RuntimeEvents = append(detail.RuntimeEvents, views...)
	}
	detail.RuntimeEventsTotal = len(detail.RuntimeEvents)
	if len(detail.RuntimeEvents) > runtimeTranscriptWindow {
		// Keep the newest end of the transcript. The frontend renders every
		// entry it is given through a Markdown pipeline, so a long history is
		// paid for in mounted components even though the user is looking at the
		// bottom of it; RuntimeEventsTotal tells it to offer the rest.
		detail.RuntimeEvents = detail.RuntimeEvents[len(detail.RuntimeEvents)-runtimeTranscriptWindow:]
	}
	return detail, nil
}

// GetRunTranscript returns the whole folded transcript, without the window
// GetRunDetail applies. It backs the frontend's "load earlier" control.
func (a *Service) GetRunTranscript(ctx context.Context, runID string) ([]RuntimeEventView, error) {
	stepRuns, err := a.store.Repos.Workflows.ListStepRuns(ctx, runID)
	if err != nil {
		return nil, err
	}
	out := []RuntimeEventView{}
	for _, stepRun := range stepRuns {
		items, listErr := a.store.Repos.Workflows.ListRuntimeEvents(ctx, runID, stepRun.ID, 0, 10_000)
		if listErr != nil {
			return nil, listErr
		}
		views, _ := foldRuntimeEventViews(stepRun.ID, items, false)
		out = append(out, views...)
	}
	return out, nil
}

// needsUsageBackfill reports whether a step run predates the detailed usage
// fields. A run finished by the current code writes them at completion, so the
// derivation below is a compatibility layer for old data — and skipping it lets
// the fold pass ignore Raw entirely for everything written since.
func needsUsageBackfill(stepRun domainworkflows.StepRun) bool {
	return stepRun.InputTokens == 0 && stepRun.OutputTokens == 0 &&
		stepRun.CachedInputTokens == 0 && stepRun.CacheCreationInputTokens == 0 &&
		stepRun.ReasoningOutputTokens == 0
}

// applyUsageBackfill fills in a step run's token breakdown from what the fold
// pass recovered. It only ever raises a value, so a partially populated step run
// keeps whatever it already recorded.
func applyUsageBackfill(stepRun *domainworkflows.StepRun, usage agentrun.Usage) {
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
		return grokUsageFromRuntimeRaw(raw)
	}
}

// grokUsageFromRuntimeRaw recovers runs written before the ACP adapter read
// usage from session/prompt responses. Those records have no structured usage
// event, but the original response is still preserved on the result event.
func grokUsageFromRuntimeRaw(raw string) (agentrun.Usage, bool) {
	var envelope struct {
		Result struct {
			Meta struct {
				Usage struct {
					InputTokens              int `json:"inputTokens"`
					OutputTokens             int `json:"outputTokens"`
					CachedReadTokens         int `json:"cachedReadTokens"`
					CachedInputTokens        int `json:"cachedInputTokens"`
					CacheCreationTokens      int `json:"cacheCreationTokens"`
					CacheCreationInputTokens int `json:"cacheCreationInputTokens"`
					ReasoningTokens          int `json:"reasoningTokens"`
					ReasoningOutputTokens    int `json:"reasoningOutputTokens"`
				} `json:"usage"`
			} `json:"_meta"`
		} `json:"result"`
	}
	if json.Unmarshal([]byte(raw), &envelope) != nil {
		return agentrun.Usage{}, false
	}
	fields := envelope.Result.Meta.Usage
	usage := agentrun.Usage{
		InputTokens:              fields.InputTokens,
		CachedInputTokens:        max(fields.CachedReadTokens, fields.CachedInputTokens),
		CacheCreationInputTokens: max(fields.CacheCreationTokens, fields.CacheCreationInputTokens),
		OutputTokens:             fields.OutputTokens,
		ReasoningOutputTokens:    max(fields.ReasoningTokens, fields.ReasoningOutputTokens),
	}
	return usage, usage != (agentrun.Usage{})
}

// foldRuntimeEventViews collapses a step's stored events into the transcript
// the UI renders. When withUsage is set it also reports the last usage it saw,
// in the same pass: a stored payload is several kilobytes of provider JSON, and
// decoding the whole file once per thing we want out of it made opening a run
// cost three passes over the same megabytes.
func foldRuntimeEventViews(stepRunID string, items []domainworkflows.RuntimeEvent, withUsage bool) ([]RuntimeEventView, agentrun.Usage) {
	views := make([]RuntimeEventView, 0, len(items))
	streamIndexes := make(map[string]int)
	var usage agentrun.Usage
	for _, item := range items {
		var event agentrun.Event
		if json.Unmarshal(item.Payload, &event) != nil {
			continue
		}
		if withUsage {
			if event.Usage != nil {
				usage = *event.Usage
			} else if event.Raw != "" {
				if found, ok := usageFromRuntimeRaw(event.Raw); ok {
					usage = found
				}
			}
		}
		at := item.At
		if !event.At.IsZero() {
			at = event.At
		}
		view := RuntimeEventView{StepRunID: stepRunID, Seq: item.Seq, Kind: string(event.Kind), StreamID: event.StreamID, Revision: event.Revision, Text: event.Text, Failed: event.Failed, Permission: event.Permission, PermissionDecision: event.PermissionDecision, At: at.Format(time.RFC3339Nano)}
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
	return views, usage
}

func (a *Service) dispatch(runID string, execute func(context.Context) (domainworkflows.Run, error)) {
	runCtx, cancel := context.WithCancel(a.rootCtx)
	a.mu.Lock()
	delete(a.lastErrors, runID)
	a.active[runID] = cancel
	a.mu.Unlock()
	a.markRunStateDirty(runID)
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
		a.resumePendingFollowUp(runID)
		// active is repository-adjacent state (isActive reads it directly) but
		// lives outside the WorkflowRepository the notifier decorates, so its
		// transitions need their own push. Without this, a paused run's Resume
		// button stays disabled for the run's InterruptGrace window (10s by
		// default) after the underlying goroutine actually stops — the run
		// already reports status "paused", but active only flips to false here,
		// and nothing else re-reads it until the window regains focus. From the
		// user's side that reads as "the button needs two clicks": the first
		// lands on a still-disabled button before this fires.
		a.markRunStateDirty(runID)
		a.reconcileQueueForRun(runID)
		a.refineTaskTitleAfterRun(runID)
	}()
}

func (a *Service) markRunStateDirty(runID string) {
	if a.runStates != nil {
		a.runStates.MarkDirty(runID)
	}
}

func (a *Service) isActive(runID string) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.active[runID] != nil
}
func (a *Service) lastError(runID string) string {
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
		{ID: "single_agent", Name: "单 Agent 完成", Description: "由选定的 coding agent 持续执行任务，需要时暂停给人处理。", EntryStepID: "execute", Steps: []domainworkflows.Step{{ID: "execute", Name: "执行", Runtime: "codex", Sandbox: "workspace-write", RolePrompt: "你是负责独立完成任务的本地 Agent。", Instruction: "完成任务、验证结果，并按 outcome contract 汇报。", Transitions: map[string]string{"completed": domainworkflows.TargetDone, "need_human": domainworkflows.TargetPause}}}},
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
