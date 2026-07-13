package localapp

import (
	"context"
	"testing"
	"time"

	"github.com/openmodu/oneshot/internal/agentrun"
	localdata "github.com/openmodu/oneshot/internal/data/local"
	"github.com/openmodu/oneshot/internal/gitinspect"
	workflowuc "github.com/openmodu/oneshot/internal/usecase/workflows"
	"github.com/openmodu/oneshot/internal/workspacelock"
)

type completingEngine struct{}

func (completingEngine) Available(agentrun.Runtime) bool { return true }
func (completingEngine) Run(context.Context, agentrun.Request, agentrun.Sink) (agentrun.Result, error) {
	return agentrun.Result{Succeeded: true, SessionID: "session_1", FinalMessage: `{"signal":"completed","content":"done"}`}, nil
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
	git := gitinspect.New("")
	orchestrator := workflowuc.NewUsecase(store.Repos.Tasks, store.Repos.Workflows, completingEngine{}, workspacelock.New(store.Data.Paths.Locks), git)
	app := New(store, orchestrator, runtimes, git)
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
	app := &App{}
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
	git := gitinspect.New("")
	orchestrator := workflowuc.NewUsecase(store.Repos.Tasks, store.Repos.Workflows, completingEngine{}, workspacelock.New(store.Data.Paths.Locks), git)
	app := New(store, orchestrator, runtimes, git)
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
