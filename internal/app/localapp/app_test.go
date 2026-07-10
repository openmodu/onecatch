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
