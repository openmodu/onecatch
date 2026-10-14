package desktop

import (
	"context"
	"testing"
	"time"

	workflows "github.com/openmodu/onecatch/internal/domain/workflows"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
)

func TestEarliestAddressedStep(t *testing.T) {
	d := workflows.Definition{Steps: []workflows.Step{{ID: "implement"}, {ID: "review"}}, Edges: []workflows.Edge{{From: "implement", To: "review"}}}
	if got := earliestStep(d, []string{"review", "implement"}); got != "implement" {
		t.Fatalf("earliest = %s", got)
	}
}

func TestSubmitRunMessageUsesBackendActivity(t *testing.T) {
	engine := &fifoEngine{started: make(chan agentrun.Request, 4), release: make(chan struct{})}
	service, _ := newLocalTestApp(t, engine)
	ctx := context.Background()
	workspace, err := service.AddWorkspace(ctx, AddWorkspaceInput{Path: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	task, err := service.CreateTask(ctx, CreateTaskInput{WorkspaceID: workspace.ID, WorkflowID: "single_agent", Title: "Follow up", Prompt: "start", Harness: "codex", Sandbox: "read-only", WorktreeMode: "project"})
	if err != nil {
		t.Fatal(err)
	}
	run, err := service.StartRun(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-engine.started:
	case <-time.After(3 * time.Second):
		t.Fatal("run did not start")
	}
	persisted, err := service.SubmitRunMessage(ctx, run.ID, RunMessageInput{Resume: ResumeRunInput{Instruction: "queued while running"}})
	if err != nil || !persisted {
		t.Fatalf("active followup: %v %v", persisted, err)
	}
	detail, err := service.GetRunDetail(ctx, run.ID)
	if err != nil || !detail.Active || len(detail.Instructions) != 1 || detail.Instructions[0].Content != "queued while running" {
		t.Fatalf("followup: %+v %v", detail, err)
	}
	persisted, err = service.SubmitRunMessage(ctx, run.ID, RunMessageInput{Resume: ResumeRunInput{Instruction: "step message"}, StepIDs: []string{"execute"}})
	if err != nil || !persisted {
		t.Fatalf("step followup: %v %v", persisted, err)
	}
	detail, err = service.GetRunDetail(ctx, run.ID)
	if err != nil || len(detail.Instructions) != 2 || detail.Instructions[1].StepID != "execute" {
		t.Fatalf("step message: %+v %v", detail, err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if persisted, err := service.SubmitRunMessage(cancelled, run.ID, RunMessageInput{Resume: ResumeRunInput{Instruction: "cancelled"}, Steer: true}); err == nil || persisted {
		t.Fatalf("cancelled send: %v %v", persisted, err)
	}
}
