package localapp

import (
	"context"
	"sync"
	"testing"
	"time"

	domainworkflows "github.com/openmodu/oneshot/internal/domain/workflows"
	"github.com/openmodu/oneshot/internal/runstate"
)

// recordingRunStateHub captures every pushed View so a test can assert on the
// exact sequence of active transitions, instead of only the value GetRunDetail
// would report if asked at an arbitrary time.
type recordingRunStateHub struct {
	*runstate.Hub
	mu    sync.Mutex
	views []runstate.View
}

// newRecordingRunStateHub builds a hub and installs it on app (which also
// wires the resolver), leaving only the emitter for the caller to attach.
func newRecordingRunStateHub(app *App) *recordingRunStateHub {
	hub := &recordingRunStateHub{Hub: runstate.NewHub()}
	app.SetRunStateHub(hub.Hub)
	hub.SetEmitter(func(view runstate.View) {
		hub.mu.Lock()
		hub.views = append(hub.views, view)
		hub.mu.Unlock()
	})
	return hub
}

func (h *recordingRunStateHub) snapshot() []runstate.View {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]runstate.View(nil), h.views...)
}

func (h *recordingRunStateHub) hasInactiveView(runID string) bool {
	for _, view := range h.snapshot() {
		if view.RunID == runID && !view.Active {
			return true
		}
	}
	return false
}

// TestDispatchPushesActiveFalseWhenTheGoroutineCompletes is a regression test
// for a stuck-disabled Resume button. Before this fix, `a.active` (the map
// isActive reads) was mutated by dispatch's own goroutine — set true at start,
// deleted at completion — without ever calling MarkDirty. Every OTHER piece of
// bounded state (run status, step runs, instructions) is written through the
// notifier-decorated repository and pushes itself automatically; active was
// the one exception. Whenever the gap between the run's last repository write
// and dispatch's own completion exceeds the hub's coalesce window, nothing
// tells the frontend that active flipped false, so a paused run's Resume
// button (disabled while active) can stay disabled long after it should work —
// from the user's side, that reads as "the button needs a second click."
func TestDispatchPushesActiveFalseWhenTheGoroutineCompletes(t *testing.T) {
	ctx := context.Background()
	app, _ := newLocalTestApp(t, completingEngine{})
	hub := newRecordingRunStateHub(app)

	workspace, err := app.AddWorkspace(ctx, AddWorkspaceInput{Path: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	definitions, err := app.ListDefinitions(ctx)
	if err != nil || len(definitions) == 0 {
		t.Fatalf("ListDefinitions() = %+v, %v", definitions, err)
	}
	definition := definitions[0]
	task, err := app.CreateTask(ctx, CreateTaskInput{WorkspaceID: workspace.ID, WorkflowID: definition.ID, Title: "t", Prompt: "p"})
	if err != nil {
		t.Fatal(err)
	}
	run := domainworkflows.Run{
		ID: "run_dispatch_active", TaskID: task.ID, WorkflowID: definition.ID,
		CurrentStepID: definition.Steps[0].ID, Status: domainworkflows.RunRunning, StartedAt: time.Now().UTC(),
	}
	if err := app.store.Repos.Workflows.SaveRun(ctx, run, definition); err != nil {
		t.Fatal(err)
	}

	// Drive dispatch directly: full control over exactly when the "goroutine"
	// finishes, independent of the orchestrator's real interrupt-grace timing.
	release := make(chan struct{})
	app.dispatch(run.ID, func(dispatchCtx context.Context) (domainworkflows.Run, error) {
		<-release
		return run, nil
	})

	if !app.isActive(run.ID) {
		t.Fatal("expected active=true immediately after dispatch starts")
	}
	detail, err := app.GetRunDetail(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !detail.Active {
		t.Fatal("GetRunDetail must agree: active=true while the goroutine is still running")
	}

	close(release)

	// Wait for dispatch's own goroutine to actually finish.
	deadline := time.Now().Add(2 * time.Second)
	for app.isActive(run.ID) {
		if time.Now().After(deadline) {
			t.Fatal("dispatch's goroutine never completed")
		}
		time.Sleep(5 * time.Millisecond)
	}

	// The regression: a push reporting active=false must eventually arrive for
	// this run, without any further repository write and without any external
	// trigger (focus change, re-selection) — dispatch's own completion must be
	// what causes it.
	for !hub.hasInactiveView(run.ID) {
		if time.Now().After(deadline) {
			t.Fatalf("no push ever reported active=false for %s; pushes seen: %+v", run.ID, hub.snapshot())
		}
		time.Sleep(5 * time.Millisecond)
	}
}
