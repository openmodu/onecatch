package keeldesktop

import (
	"context"
	"testing"
	"time"

	domainworkflows "github.com/openmodu/onecatch/internal/domain/workflows"
	desktopservice "github.com/openmodu/onecatch/internal/service/desktop"
	"github.com/openmodu/onecatch/internal/service/desktop/runstate"
	"github.com/openmodu/onecatch/internal/service/desktop/runstream"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
)

type testEngine struct{}

func (testEngine) Available(agentrun.Runtime) bool { return true }
func (testEngine) Run(_ context.Context, _ agentrun.Request, sink agentrun.Sink) (agentrun.Result, error) {
	sink(agentrun.Event{Kind: agentrun.KindMessage, StreamID: "answer", Phase: agentrun.StreamStart, At: time.Now().UTC()})
	sink(agentrun.Event{Kind: agentrun.KindMessage, StreamID: "answer", Phase: agentrun.StreamDelta, Text: "Hello from Go", At: time.Now().UTC()})
	sink(agentrun.Event{Kind: agentrun.KindMessage, StreamID: "answer", Phase: agentrun.StreamEnd, Text: "Hello from Go", At: time.Now().UTC()})
	return agentrun.Result{Succeeded: true, SessionID: "keel-test", FinalMessage: `{"signal":"completed","content":"Hello from Go"}`}, nil
}

func TestKeelBackendRunsAndReopensExistingOneCatchData(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	b, err := openBackendWithEngine(ctx, root, testEngine{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { b.close() })
	w, err := b.service.AddWorkspace(ctx, desktopservice.AddWorkspaceInput{Path: t.TempDir(), Name: "Keel integration"})
	if err != nil {
		t.Fatal(err)
	}
	task, err := b.service.CreateTask(ctx, desktopservice.CreateTaskInput{WorkspaceID: w.ID, Title: "Go client integration", Prompt: "Say hello", WorkflowID: "single_agent", Harness: "codex", Sandbox: "read-only", WorktreeMode: "project"})
	if err != nil {
		t.Fatal(err)
	}
	frames := make(chan runstream.Frame, 16)
	finished := make(chan runstate.View, 16)
	b.states.SetEmitter(func(v runstate.View) {
		if !v.Active && v.Run.Status == domainworkflows.RunCompleted {
			select {
			case finished <- v:
			default:
			}
		}
	})
	unsubscribe := b.streams.Subscribe(func(f runstream.Frame) { frames <- f })
	defer unsubscribe()
	run, err := b.service.StartRun(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.After(5 * time.Second)
	for {
		select {
		case f := <-frames:
			if f.RunID != run.ID || f.Text != "Hello from Go" {
				continue
			}
		case <-deadline:
			t.Fatal("run did not deliver streamed output")
		}
		break
	}
	// Bounded state notification is the completion signal, not a UI polling loop.
	waitCompleted(t, finished, 0)
	detail, err := b.service.GetRunDetail(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Active || detail.Run.Status != domainworkflows.RunCompleted {
		t.Fatalf("run did not complete: %+v", detail.Run)
	}
	if len(detail.RuntimeEvents) == 0 {
		t.Fatal("no durable transcript")
	}
	if _, err := b.service.ResumeRun(ctx, run.ID, "Say hello again"); err != nil {
		t.Fatal(err)
	}
	waitCompleted(t, finished, detail.Run.Revision)
	// Text-only sends must be visible as durable user messages immediately,
	// including two intentionally identical turns. Do not deduplicate by text.
	for range 2 {
		before, err := b.service.GetRunDetail(ctx, run.ID)
		if err != nil {
			t.Fatal(err)
		}
		persisted, err := submitContinuation(ctx, b.service, run.ID, desktopservice.ResumeRunInput{Instruction: "再说一次"}, nil, false)
		if err != nil || !persisted {
			t.Fatalf("send: persisted=%v, err=%v", persisted, err)
		}
		sent, err := b.service.GetRunDetail(ctx, run.ID)
		if err != nil || len(sent.Instructions) == 0 || sent.Instructions[len(sent.Instructions)-1].Content != "再说一次" {
			t.Fatalf("user text absent before completion: %+v, %v", sent.Instructions, err)
		}
		waitCompleted(t, finished, before.Run.Revision)
	}
	if err := b.close(); err != nil {
		t.Fatal(err)
	}
	// The same repository format is consumed directly, without an export or migration.
	reopened, err := openBackendWithEngine(ctx, root, testEngine{})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.close()
	tasks, err := reopened.service.ListTasks(ctx, w.ID)
	if err != nil || len(tasks) != 1 || tasks[0].ID != task.ID {
		t.Fatalf("reopened tasks = %+v, err = %v", tasks, err)
	}
	history, err := reopened.service.GetRunTranscript(ctx, run.ID)
	if err != nil || len(history) == 0 {
		t.Fatalf("reopened history = %+v, err = %v", history, err)
	}
	detail, err = reopened.service.GetRunDetail(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	tx := newTranscript()
	tx.detail(detail)
	assertUserMessages(t, tx, "Say hello", "Say hello again", "再说一次", "再说一次")
	verifyPersistedUserBubble(t, reopened, detail)
}

func waitCompleted(t *testing.T, finished <-chan runstate.View, after int64) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case view := <-finished:
			if view.Run.Revision > after {
				return
			}
		case <-deadline:
			t.Fatal("run did not publish completion")
		}
	}
}
