package desktop

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	domaintasks "github.com/openmodu/onecatch/internal/domain/tasks"
	domainworkflows "github.com/openmodu/onecatch/internal/domain/workflows"
	"github.com/openmodu/onecatch/internal/repo/git"
	localdata "github.com/openmodu/onecatch/internal/repo/store/local"
	"github.com/openmodu/onecatch/internal/repo/workspacelock"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
	workflowuc "github.com/openmodu/onecatch/internal/usecase/workflows"
)

func runtimeEvent(t *testing.T, seq int64, event agentrun.Event) domainworkflows.RuntimeEvent {
	t.Helper()
	payload, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	return domainworkflows.RuntimeEvent{Seq: seq, At: event.At, Payload: payload}
}

func TestValidateResumeHarnessLocksDirectAgentConversation(t *testing.T) {
	definition := domainworkflows.Definition{Steps: []domainworkflows.Step{{Runtime: "codex"}}}
	task := domaintasks.Task{WorkflowID: directAgentWorkflowID, Harness: "codex"}
	if err := validateResumeHarness(task, definition, "codex"); err != nil {
		t.Fatalf("same Agent was rejected: %v", err)
	}
	if err := validateResumeHarness(task, definition, "claude"); errorCode(err) != "runtime_locked" {
		t.Fatalf("changed Agent error = %v", err)
	}
	task.WorkflowID = "review_loop"
	if err := validateResumeHarness(task, definition, "claude"); err != nil {
		t.Fatalf("workflow runtime override was rejected: %v", err)
	}
}

func TestFoldRuntimeEventViewsCollapsesDurableStream(t *testing.T) {
	at := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)
	base := agentrun.Event{Kind: agentrun.KindMessage, StreamID: "message-1", At: at}
	start := base
	start.Phase = agentrun.StreamStart
	first := base
	first.Phase, first.Revision, first.Text = agentrun.StreamDelta, 1, "hel"
	second := base
	second.Phase, second.Revision, second.Text = agentrun.StreamDelta, 2, "lo"
	end := base
	end.Phase, end.Revision, end.Text = agentrun.StreamEnd, 3, "hello!"
	atomic := agentrun.Event{Kind: agentrun.KindUsage, At: at}

	views := foldRuntimeEventViews("step-1", []domainworkflows.RuntimeEvent{
		runtimeEvent(t, 1, start), runtimeEvent(t, 2, first), runtimeEvent(t, 3, second), runtimeEvent(t, 4, end), runtimeEvent(t, 5, atomic),
	})
	if len(views) != 2 {
		t.Fatalf("views = %+v", views)
	}
	if views[0].Seq != 1 || views[0].Text != "hello!" || views[0].Revision != 3 || views[0].Streaming {
		t.Fatalf("stream view = %+v", views[0])
	}
	if views[1].Kind != string(agentrun.KindUsage) || views[1].Seq != 5 {
		t.Fatalf("atomic view = %+v", views[1])
	}
}

func TestEnrichStepRunUsageRecoversHistoricalProviderDetails(t *testing.T) {
	tests := []struct {
		name string
		step domainworkflows.StepRun
		raw  string
		want agentrun.Usage
		kind agentrun.EventKind
	}{
		{
			name: "Codex cached and reasoning subsets",
			step: domainworkflows.StepRun{InputTokens: 1200, OutputTokens: 42},
			raw:  `{"type":"turn.completed","usage":{"input_tokens":1200,"cached_input_tokens":900,"output_tokens":42,"reasoning_output_tokens":12}}`,
			want: agentrun.Usage{InputTokens: 1200, CachedInputTokens: 900, OutputTokens: 42, ReasoningOutputTokens: 12},
			kind: agentrun.KindUsage,
		},
		{
			name: "Claude input total includes cache creation and reads",
			step: domainworkflows.StepRun{InputTokens: 7, OutputTokens: 9},
			raw:  `{"type":"result","usage":{"input_tokens":7,"cache_creation_input_tokens":13,"cache_read_input_tokens":80,"output_tokens":9}}`,
			want: agentrun.Usage{InputTokens: 100, CachedInputTokens: 80, CacheCreationInputTokens: 13, OutputTokens: 9},
			kind: agentrun.KindResult,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			at := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)
			items := []domainworkflows.RuntimeEvent{runtimeEvent(t, 1, agentrun.Event{Kind: test.kind, Raw: test.raw, At: at})}
			enrichStepRunUsage(&test.step, items)
			if test.step.InputTokens != test.want.InputTokens || test.step.CachedInputTokens != test.want.CachedInputTokens || test.step.CacheCreationInputTokens != test.want.CacheCreationInputTokens || test.step.OutputTokens != test.want.OutputTokens || test.step.ReasoningOutputTokens != test.want.ReasoningOutputTokens {
				t.Fatalf("step = %+v, want usage %+v", test.step, test.want)
			}
		})
	}
}

type completingEngine struct{}

func (completingEngine) Available(agentrun.Runtime) bool { return true }
func (completingEngine) Run(context.Context, agentrun.Request, agentrun.Sink) (agentrun.Result, error) {
	return agentrun.Result{Succeeded: true, SessionID: "session_1", FinalMessage: `{"signal":"completed","content":"done"}`}, nil
}

type fifoEngine struct {
	started chan agentrun.Request
	release chan struct{}
}

func (e *fifoEngine) Available(agentrun.Runtime) bool { return true }
func (e *fifoEngine) Run(ctx context.Context, request agentrun.Request, _ agentrun.Sink) (agentrun.Result, error) {
	e.started <- request
	select {
	case <-ctx.Done():
		return agentrun.Result{}, ctx.Err()
	case <-e.release:
		return agentrun.Result{Succeeded: true, SessionID: "session_fifo", FinalMessage: `{"signal":"completed","content":"done"}`}, nil
	}
}

func newLocalTestApp(t *testing.T, engine workflowuc.Engine) (*Service, *localdata.Store) {
	t.Helper()
	root := t.TempDir()
	store, err := localdata.OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	runtimes, err := NewRuntimeRegistry(root)
	if err != nil {
		t.Fatal(err)
	}
	git := gitrepo.New("")
	orchestrator := workflowuc.NewUsecase(store.Repos.Tasks, store.Repos.Workflows, engine, workspacelock.New(store.Data.Paths.Locks), git)
	app := NewService(store, orchestrator, runtimes, git)
	t.Cleanup(func() { _ = app.Close() })
	if err := app.EnsureBuiltinDefinitions(context.Background()); err != nil {
		t.Fatal(err)
	}
	return app, store
}

func TestDirectAgentWorkflowIsProtectedAndSelfHealing(t *testing.T) {
	app, store := newLocalTestApp(t, completingEngine{})
	ctx := context.Background()
	updated := builtinDefinitions()[0]
	updated.Name = "My Single Agent"
	if _, err := app.UpdateDefinition(ctx, directAgentWorkflowID, updated); errorCode(err) != "workflow_builtin_readonly" {
		t.Fatalf("UpdateDefinition() error = %v", err)
	}
	if err := app.DeleteDefinition(ctx, directAgentWorkflowID); errorCode(err) != "workflow_builtin_readonly" {
		t.Fatalf("DeleteDefinition() error = %v", err)
	}

	// Simulate an older build that allowed the backing record to be deleted
	// after the builtins marker had already been written.
	if err := store.Repos.Workflows.DeleteDefinition(ctx, directAgentWorkflowID); err != nil {
		t.Fatal(err)
	}
	if err := app.EnsureBuiltinDefinitions(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Repos.Workflows.GetDefinition(ctx, directAgentWorkflowID); err != nil {
		t.Fatalf("EnsureBuiltinDefinitions() did not repair direct Agent definition: %v", err)
	}
	if err := store.Repos.Workflows.DeleteDefinition(ctx, directAgentWorkflowID); err != nil {
		t.Fatal(err)
	}
	definition, err := app.GetDefinition(ctx, directAgentWorkflowID)
	if err != nil {
		t.Fatalf("GetDefinition() did not self-heal direct Agent definition: %v", err)
	}
	if definition.ID != directAgentWorkflowID || len(definition.Steps) != 1 {
		t.Fatalf("repaired direct Agent definition = %+v", definition)
	}
}

func TestSearchTasksReturnsMatchesAcrossVisibleWorkspaces(t *testing.T) {
	app, _ := newLocalTestApp(t, completingEngine{})
	ctx := context.Background()
	alpha, err := app.AddWorkspace(ctx, AddWorkspaceInput{Path: t.TempDir(), Name: "Alpha Project"})
	if err != nil {
		t.Fatal(err)
	}
	beta, err := app.AddWorkspace(ctx, AddWorkspaceInput{Path: t.TempDir(), Name: "Beta Project"})
	if err != nil {
		t.Fatal(err)
	}
	first, err := app.CreateTask(ctx, CreateTaskInput{WorkspaceID: alpha.ID, WorkflowID: "single_agent", Title: "first task", Prompt: "search the workspace name"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.StartRun(ctx, first.ID); err != nil {
		t.Fatal(err)
	}
	second, err := app.CreateTask(ctx, CreateTaskInput{WorkspaceID: beta.ID, WorkflowID: "single_agent", Title: "second task", Prompt: "find me globally"})
	if err != nil {
		t.Fatal(err)
	}

	all, err := app.SearchTasks(ctx, SearchTasksInput{Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if all.Total != 2 || len(all.Items) != 1 {
		t.Fatalf("limited search = %+v", all)
	}
	byTitle, err := app.SearchTasks(ctx, SearchTasksInput{Keyword: "SECOND", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(byTitle.Items) != 1 || byTitle.Items[0].Task.ID != second.ID || byTitle.Items[0].Workspace.ID != beta.ID {
		t.Fatalf("title search = %+v", byTitle)
	}
	byWorkspace, err := app.SearchTasks(ctx, SearchTasksInput{Keyword: "alpha project", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(byWorkspace.Items) != 1 || byWorkspace.Items[0].Task.ID != first.ID || byWorkspace.Items[0].LatestRun == nil {
		t.Fatalf("workspace search = %+v", byWorkspace)
	}
}

func TestDesktopApplicationServiceCreatesAndExecutesRun(t *testing.T) {
	root := t.TempDir()
	store, err := localdata.OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	runtimes, err := NewRuntimeRegistry(root)
	if err != nil {
		t.Fatal(err)
	}
	git := gitrepo.New("")
	orchestrator := workflowuc.NewUsecase(store.Repos.Tasks, store.Repos.Workflows, completingEngine{}, workspacelock.New(store.Data.Paths.Locks), git)
	app := NewService(store, orchestrator, runtimes, git)
	defer app.Close()
	ctx := context.Background()

	if err := app.EnsureBuiltinDefinitions(ctx); err != nil {
		t.Fatal(err)
	}
	workspace, err := app.AddWorkspace(ctx, AddWorkspaceInput{Path: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	task, err := app.CreateTask(ctx, CreateTaskInput{WorkspaceID: workspace.ID, WorkflowID: "single_agent", Title: "local run", Prompt: "finish it"})
	if err != nil {
		t.Fatal(err)
	}
	run, err := app.StartRun(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if run.ID == "" || run.Status != "running" {
		t.Fatalf("prepared run = %+v", run)
	}

	deadline := time.Now().Add(3 * time.Second)
	for {
		detail, detailErr := app.GetRunDetail(ctx, run.ID)
		if detailErr != nil {
			t.Fatal(detailErr)
		}
		if !detail.Active {
			if detail.Run.Status != "completed" || len(detail.StepRuns) != 1 || len(detail.Events) == 0 {
				t.Fatalf("completed detail = %+v", detail)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("background run did not finish")
		}
		time.Sleep(10 * time.Millisecond)
	}

	interruptedTask, err := app.CreateTask(ctx, CreateTaskInput{WorkspaceID: workspace.ID, WorkflowID: "single_agent", Title: "interrupted run", Prompt: "recover me"})
	if err != nil {
		t.Fatal(err)
	}
	interruptedRun, err := orchestrator.StartTask(ctx, interruptedTask.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := app.RecoverInterruptedRuns(ctx); err != nil {
		t.Fatal(err)
	}
	recovered, err := store.Repos.Workflows.GetRun(ctx, interruptedRun.ID)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Status != "paused" || recovered.PauseReason != workflowuc.PauseReasonInterrupted {
		t.Fatalf("recovered run = %+v", recovered)
	}
}

func TestValidateDefinitionReturnsAllIssues(t *testing.T) {
	app := &Service{}
	issues := app.ValidateDefinition(builtinDefinitions()[0])
	if len(issues) != 0 {
		t.Fatalf("builtin issues = %+v", issues)
	}
	invalid := builtinDefinitions()[0]
	invalid.EntryStepID = "missing"
	issues = app.ValidateDefinition(invalid)
	if len(issues) == 0 {
		t.Fatal("invalid workflow returned no issues")
	}
}

func TestNormalizeCommitMessageProducesOneConventionalSubject(t *testing.T) {
	tests := map[string]string{
		"```\nFix the workbench.\n```":       "chore: fix the workbench",
		"Add queue support.\nThis is a body": "chore: add queue support",
		"feat(tasks): add queue":             "feat(tasks): add queue",
		"":                                   "chore: update workspace",
	}
	for input, want := range tests {
		if got := normalizeCommitMessage(input); got != want {
			t.Errorf("normalizeCommitMessage(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestWorkspacePreferencesAndRunListStayWorkspaceScoped(t *testing.T) {
	root := t.TempDir()
	store, err := localdata.OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	runtimes, err := NewRuntimeRegistry(root)
	if err != nil {
		t.Fatal(err)
	}
	git := gitrepo.New("")
	orchestrator := workflowuc.NewUsecase(store.Repos.Tasks, store.Repos.Workflows, completingEngine{}, workspacelock.New(store.Data.Paths.Locks), git)
	app := NewService(store, orchestrator, runtimes, git)
	defer app.Close()
	ctx := context.Background()
	if err := app.EnsureBuiltinDefinitions(ctx); err != nil {
		t.Fatal(err)
	}
	workspace, err := app.AddWorkspace(ctx, AddWorkspaceInput{Path: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	other, err := app.AddWorkspace(ctx, AddWorkspaceInput{Path: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	pinned, err := app.SetWorkspacePinned(ctx, workspace.ID, true)
	if err != nil || !pinned.Pinned {
		t.Fatalf("SetWorkspacePinned() = %+v, %v", pinned, err)
	}
	beforeOpen := pinned.LastOpenedAt
	time.Sleep(time.Millisecond)
	opened, err := app.OpenWorkspace(ctx, workspace.ID)
	if err != nil || !opened.LastOpenedAt.After(beforeOpen) {
		t.Fatalf("OpenWorkspace() = %+v, %v", opened, err)
	}
	task, err := app.CreateTask(ctx, CreateTaskInput{WorkspaceID: workspace.ID, WorkflowID: "single_agent", Title: "Searchable task", Prompt: "finish"})
	if err != nil {
		t.Fatal(err)
	}
	run, err := orchestrator.StartTask(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.CreateTask(ctx, CreateTaskInput{WorkspaceID: other.ID, WorkflowID: "single_agent", Title: "Other task", Prompt: "ignore"}); err != nil {
		t.Fatal(err)
	}
	page, err := app.ListRuns(ctx, ListRunsInput{WorkspaceID: workspace.ID, Keyword: "searchable", Limit: 50})
	if err != nil || len(page.Items) != 1 || page.Items[0].Run.ID != run.ID || page.Items[0].Task.ID != task.ID || page.Total != 1 {
		t.Fatalf("ListRuns() = %+v, %v", page, err)
	}
	if _, err := app.ListRuns(ctx, ListRunsInput{WorkspaceID: workspace.ID, Status: "unknown"}); err == nil {
		t.Fatal("ListRuns() accepted an invalid status")
	}
	if err := app.RemoveWorkspace(ctx, other.ID); err != nil {
		t.Fatal(err)
	}
	workspaces, err := app.ListWorkspaces(ctx)
	if err != nil || len(workspaces) != 1 || workspaces[0].ID != workspace.ID {
		t.Fatalf("ListWorkspaces() after remove = %+v, %v", workspaces, err)
	}
	if _, err := app.GetWorkspace(ctx, other.ID); err != nil {
		t.Fatalf("hidden workspace no longer available to history: %v", err)
	}
}

func TestWorkspaceQueueActivatesTasksInFIFOOrder(t *testing.T) {
	ctx := context.Background()
	engine := &fifoEngine{started: make(chan agentrun.Request, 2), release: make(chan struct{}, 2)}
	app, _ := newLocalTestApp(t, engine)
	workspace, err := app.AddWorkspace(ctx, AddWorkspaceInput{Path: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	create := func(title string) domaintasks.Task {
		task, createErr := app.CreateTask(ctx, CreateTaskInput{WorkspaceID: workspace.ID, WorkflowID: "single_agent", Title: title, Prompt: "complete " + title})
		if createErr != nil {
			t.Fatal(createErr)
		}
		return task
	}
	first := create("first")
	second := create("second")
	if _, err := app.EnqueueTask(ctx, first.ID, ""); err != nil {
		t.Fatal(err)
	}
	select {
	case request := <-engine.started:
		if !strings.Contains(request.Prompt, "complete first") {
			t.Fatalf("first request prompt = %q", request.Prompt)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("first queued task did not start")
	}
	if _, err := app.EnqueueTask(ctx, second.ID, ""); err != nil {
		t.Fatal(err)
	}
	snapshot, err := app.QueueSnapshot(ctx, workspace.ID)
	if err != nil || len(snapshot) != 2 || snapshot[0].ID != first.ID || snapshot[1].ID != second.ID || snapshot[0].Queue.State != domaintasks.QueueActive || snapshot[1].Queue.State != domaintasks.QueueWaiting {
		t.Fatalf("QueueSnapshot() = %+v, %v", snapshot, err)
	}
	engine.release <- struct{}{}
	select {
	case request := <-engine.started:
		if !strings.Contains(request.Prompt, "complete second") {
			t.Fatalf("second request prompt = %q", request.Prompt)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("second queued task did not start after first completed")
	}
	storedFirst, err := app.store.Repos.Tasks.GetTask(ctx, first.ID)
	if err != nil || storedFirst.Status != domaintasks.StatusCompleted || storedFirst.Queue.State != domaintasks.QueueSuperseded {
		t.Fatalf("first task after release = %+v, %v", storedFirst, err)
	}
	engine.release <- struct{}{}
	deadline := time.Now().Add(2 * time.Second)
	for {
		snapshot, err = app.QueueSnapshot(ctx, workspace.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(snapshot) == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("queue did not drain: %+v", snapshot)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestTaskAttachmentsRenameAndSoftDelete(t *testing.T) {
	ctx := context.Background()
	app, _ := newLocalTestApp(t, completingEngine{})
	workspacePath := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspacePath, ".git", "info"), 0o700); err != nil {
		t.Fatal(err)
	}
	workspace, err := app.AddWorkspace(ctx, AddWorkspaceInput{Path: workspacePath})
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "产品参考 图.png")
	wantContent := []byte("reference image bytes")
	if err := os.WriteFile(source, wantContent, 0o600); err != nil {
		t.Fatal(err)
	}
	task, err := app.CreateTask(ctx, CreateTaskInput{WorkspaceID: workspace.ID, WorkflowID: "single_agent", Title: "with attachment", Prompt: "inspect it", AttachmentPaths: []string{source}})
	if err != nil {
		t.Fatal(err)
	}
	if len(task.Attachments) != 1 || task.Attachments[0].StoredPath == source || !strings.Contains(task.Attachments[0].StoredPath, filepath.Join(".onecatch", "attachments", task.ID)) {
		t.Fatalf("attachments = %+v", task.Attachments)
	}
	gotContent, err := os.ReadFile(task.Attachments[0].StoredPath)
	if err != nil || string(gotContent) != string(wantContent) {
		t.Fatalf("stored attachment = %q, %v", gotContent, err)
	}
	exclude, err := os.ReadFile(filepath.Join(workspacePath, ".git", "info", "exclude"))
	if err != nil || !strings.Contains(string(exclude), ".onecatch/") {
		t.Fatalf("git exclude = %q, %v", exclude, err)
	}
	renamed, err := app.RenameTask(ctx, task.ID, "renamed task")
	if err != nil || renamed.Title != "renamed task" {
		t.Fatalf("RenameTask() = %+v, %v", renamed, err)
	}
	if err := app.DeleteTask(ctx, task.ID); err != nil {
		t.Fatal(err)
	}
	items, err := app.ListTasks(ctx, workspace.ID)
	if err != nil || len(items) != 0 {
		t.Fatalf("ListTasks() after delete = %+v, %v", items, err)
	}
	if _, err := os.Stat(filepath.Dir(task.Attachments[0].StoredPath)); !os.IsNotExist(err) {
		t.Fatalf("attachment directory still exists: %v", err)
	}
}

func TestInterruptAndInsertResumesWithPriorityInstruction(t *testing.T) {
	ctx := context.Background()
	engine := &fifoEngine{started: make(chan agentrun.Request, 2), release: make(chan struct{}, 1)}
	app, _ := newLocalTestApp(t, engine)
	workspace, err := app.AddWorkspace(ctx, AddWorkspaceInput{Path: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	task, err := app.CreateTask(ctx, CreateTaskInput{WorkspaceID: workspace.ID, WorkflowID: "single_agent", Title: "insert", Prompt: "start work"})
	if err != nil {
		t.Fatal(err)
	}
	run, err := app.StartRun(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-engine.started:
	case <-time.After(2 * time.Second):
		t.Fatal("initial run did not start")
	}
	ordinary, err := app.EnqueueInstruction(ctx, run.ID, InstructionInput{Content: "ordinary follow-up"})
	if err != nil {
		t.Fatal(err)
	}
	if err := app.RemoveInstruction(ctx, run.ID, ordinary.ID); err != nil {
		t.Fatal(err)
	}
	inserted, err := app.InterruptAndInsert(ctx, run.ID, InstructionInput{Content: "urgent correction"})
	if err != nil || !inserted.Priority {
		t.Fatalf("InterruptAndInsert() = %+v, %v", inserted, err)
	}
	select {
	case request := <-engine.started:
		if !strings.Contains(request.Prompt, "urgent correction") || strings.Contains(request.Prompt, "ordinary follow-up") {
			t.Fatalf("resumed request prompt = %q", request.Prompt)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("interrupted run did not resume")
	}
	engine.release <- struct{}{}
	deadline := time.Now().Add(2 * time.Second)
	for {
		detail, detailErr := app.GetRunDetail(ctx, run.ID)
		if detailErr != nil {
			t.Fatal(detailErr)
		}
		if !detail.Active && detail.Run.Status == "completed" {
			if len(detail.Instructions) != 1 || detail.Instructions[0].ID != inserted.ID || detail.Instructions[0].Status != "applied" {
				t.Fatalf("instructions after completion = %+v", detail.Instructions)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("resumed run did not complete: %+v", detail.Run)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
