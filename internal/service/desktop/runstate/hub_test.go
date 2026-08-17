package runstate

import (
	"sync"
	"testing"
	"time"

	domainworkflows "github.com/openmodu/onecatch/internal/domain/workflows"
)

func newTestHub(t *testing.T) *Hub {
	t.Helper()
	hub := NewHub()
	hub.coalesce = 5 * time.Millisecond
	return hub
}

func TestHubCoalescesDirtyMarksIntoOneResolve(t *testing.T) {
	hub := newTestHub(t)

	var resolveCount int
	var mu sync.Mutex
	hub.SetResolver(func(runID string) (View, bool) {
		mu.Lock()
		resolveCount++
		mu.Unlock()
		return View{RunID: runID, Run: domainworkflows.Run{ID: runID}}, true
	})

	emitted := make(chan View, 4)
	hub.SetEmitter(func(view View) { emitted <- view })

	for range 5 {
		hub.MarkDirty("run_1")
	}

	select {
	case view := <-emitted:
		if view.RunID != "run_1" {
			t.Fatalf("emitted wrong run: %q", view.RunID)
		}
	case <-time.After(time.Second):
		t.Fatal("expected an emitted view")
	}

	// A burst collapses to a single resolve+emit.
	select {
	case <-emitted:
		t.Fatal("expected only one emit for a coalesced burst")
	case <-time.After(50 * time.Millisecond):
	}
	mu.Lock()
	if resolveCount != 1 {
		t.Fatalf("expected 1 resolve, got %d", resolveCount)
	}
	mu.Unlock()
}

func TestHubDropsRunResolvedFalse(t *testing.T) {
	hub := newTestHub(t)
	hub.SetResolver(func(string) (View, bool) { return View{}, false })
	emitted := make(chan View, 1)
	hub.SetEmitter(func(view View) { emitted <- view })

	hub.MarkDirty("gone")
	select {
	case <-emitted:
		t.Fatal("a deleted run must not emit")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestHubCloseStopsFlush(t *testing.T) {
	hub := newTestHub(t)
	hub.SetResolver(func(runID string) (View, bool) { return View{RunID: runID}, true })
	emitted := make(chan View, 1)
	hub.SetEmitter(func(view View) { emitted <- view })

	hub.Close()
	hub.MarkDirty("run_1")
	select {
	case <-emitted:
		t.Fatal("a closed hub must not emit")
	case <-time.After(50 * time.Millisecond):
	}
}
