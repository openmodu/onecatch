package desktop

import (
	"context"
	"strings"
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

func TestFollowUpsRunOneMessagePerTurn(t *testing.T) {
	ctx := context.Background()
	engine := &fifoEngine{started: make(chan agentrun.Request, 8), release: make(chan struct{}, 8)}
	service, _ := newLocalTestApp(t, engine)
	workspace, err := service.AddWorkspace(ctx, AddWorkspaceInput{Path: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	task, err := service.CreateTask(ctx, CreateTaskInput{WorkspaceID: workspace.ID, WorkflowID: "single_agent", Title: "FIFO", Prompt: "initial", Harness: "codex", Sandbox: "read-only", WorktreeMode: "project"})
	if err != nil {
		t.Fatal(err)
	}
	run, err := service.StartRun(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	next := func() agentrun.Request {
		t.Helper()
		select {
		case request := <-engine.started:
			return request
		case <-time.After(3 * time.Second):
			t.Fatal("next turn did not start")
			return agentrun.Request{}
		}
	}
	next()
	texts := []string{"first follow-up", "second follow-up", "third follow-up"}
	var ids []string
	for _, text := range texts {
		item, err := service.QueueFollowUp(ctx, run.ID, InstructionInput{Content: text})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, item.ID)
	}
	for i, text := range texts {
		engine.release <- struct{}{}
		request := next()
		if request.ResumeSessionID != "session_fifo" {
			t.Fatalf("lost session: %+v", request)
		}
		for j, other := range texts {
			if strings.Contains(request.Prompt, other) != (i == j) {
				t.Fatalf("turn %d wanted only %q, got %q", i, text, request.Prompt)
			}
		}
		detail, err := service.GetRunDetail(ctx, run.ID)
		if err != nil {
			t.Fatal(err)
		}
		for j, item := range detail.Instructions {
			status := workflows.InstructionPending
			if j <= i {
				status = workflows.InstructionApplied
			}
			if item.ID != ids[j] || item.Status != status {
				t.Fatalf("turn %d reordered/drained queue: %+v", i, detail.Instructions)
			}
		}
	}
	engine.release <- struct{}{}
	deadline := time.Now().Add(3 * time.Second)
	for service.isActive(run.ID) {
		if time.Now().After(deadline) {
			t.Fatal("final turn did not finish")
		}
		time.Sleep(10 * time.Millisecond)
	}
	// A stopped instance sends directly without a pending queue record.
	persisted, err := service.SubmitRunMessage(ctx, run.ID, RunMessageInput{Resume: ResumeRunInput{Instruction: "direct continuation"}})
	if err != nil || !persisted {
		t.Fatalf("idle send: %v %v", persisted, err)
	}
	request := next()
	if !strings.Contains(request.Prompt, "direct continuation") {
		t.Fatalf("direct message missing: %q", request.Prompt)
	}
	detail, err := service.GetRunDetail(ctx, run.ID)
	if err != nil || len(detail.Instructions) != 4 {
		t.Fatalf("direct send not durable: %+v %v", detail.Instructions, err)
	}
	for _, item := range detail.Instructions {
		if item.Status == workflows.InstructionPending {
			t.Fatalf("idle send entered queue: %+v", item)
		}
	}
	engine.release <- struct{}{}
}
