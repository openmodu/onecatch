package desktop

import (
	"context"
	workflows "github.com/openmodu/onecatch/internal/domain/workflows"
	localdata "github.com/openmodu/onecatch/internal/repo/store/local"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
	"path/filepath"
	"testing"
	"time"
)

func TestSetRunLoopPersistsAndRejectsActiveRun(t *testing.T) {
	ctx := context.Background()
	store, err := localdata.OpenStore(filepath.Join(t.TempDir(), ".onecatch"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	def := workflows.Definition{ID: "loop", Name: "Loop", Steps: []workflows.Step{{ID: "work", Name: "Work", Runtime: "codex"}}}
	def, err = store.Repos.Workflows.SaveDefinition(ctx, def)
	if err != nil {
		t.Fatal(err)
	}
	run, err := workflows.NewRun(def, "run", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	run.TaskID = "task"
	if err := store.Repos.Workflows.SaveRun(ctx, run, def); err != nil {
		t.Fatal(err)
	}
	service := &Service{store: store, active: map[string]context.CancelFunc{}}
	updated, err := service.SetRunLoop(ctx, run.ID, true)
	if err != nil || !updated.LoopEnabled || updated.LoopRound != 1 {
		t.Fatalf("enable: %+v %v", updated, err)
	}
	persisted, err := store.Repos.Workflows.GetRun(ctx, run.ID)
	if err != nil || !persisted.LoopEnabled {
		t.Fatal("lost loop mode")
	}
	service.active[run.ID] = func() {}
	if _, err := service.SetRunLoop(ctx, run.ID, false); err == nil {
		t.Fatal("changed active run mode")
	}
	delete(service.active, run.ID)
	updated, err = service.SetRunLoop(ctx, run.ID, false)
	if err != nil || updated.LoopEnabled {
		t.Fatalf("disable: %+v %v", updated, err)
	}
}

func TestComposerLoopChoiceAppliesBeforeDispatchWithoutChangingDefault(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		t.Run(map[bool]string{true: "on", false: "off"}[enabled], func(t *testing.T) {
			engine := &fifoEngine{started: make(chan agentrun.Request, 4), release: make(chan struct{})}
			service, _ := newLocalTestApp(t, engine)
			ctx := context.Background()
			definition, err := service.SaveDefinition(ctx, workflows.Definition{ID: "composer_loop", Name: "Composer loop", LoopEnabled: !enabled, Steps: []workflows.Step{{ID: "work", Name: "Work", Runtime: "codex"}}})
			if err != nil {
				t.Fatal(err)
			}
			workspace, err := service.AddWorkspace(ctx, AddWorkspaceInput{Path: t.TempDir()})
			if err != nil {
				t.Fatal(err)
			}
			task, err := service.CreateTask(ctx, CreateTaskInput{WorkspaceID: workspace.ID, Title: "Loop setting", Prompt: "Complete the task", WorkflowID: definition.ID, Harness: "codex", Sandbox: "read-only", WorktreeMode: "project"})
			if err != nil {
				t.Fatal(err)
			}
			run, err := service.StartRunLoopConfirmed(ctx, task.ID, "", enabled)
			if err != nil || run.LoopEnabled != enabled {
				t.Fatalf("startup choice: %+v %v", run, err)
			}
			select {
			case <-engine.started:
			case <-time.After(3 * time.Second):
				t.Fatal("workflow did not start")
			}
			detail, err := service.GetRunDetail(ctx, run.ID)
			if err != nil || detail.Run.LoopEnabled != enabled || detail.Workflow.LoopEnabled != enabled {
				t.Fatal("running workflow did not use composer's choice")
			}
			stored, err := service.GetDefinition(ctx, definition.ID)
			if err != nil || stored.LoopEnabled == enabled {
				t.Fatal("composer changed the shared workflow default")
			}
		})
	}
}
