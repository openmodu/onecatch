package listchange

import (
	"sync"
	"testing"
	"time"
)

func newTestHub(t *testing.T) (*Hub, func() int) {
	t.Helper()
	hub := NewHub()
	hub.coalesce = 5 * time.Millisecond
	var mu sync.Mutex
	count := 0
	hub.SetEmitter(func() {
		mu.Lock()
		count++
		mu.Unlock()
	})
	t.Cleanup(hub.Close)
	return hub, func() int {
		mu.Lock()
		defer mu.Unlock()
		return count
	}
}

// Starting one task writes the task, its run, and its first step run in quick
// succession. The frontend answer to all three is the same single refresh, so
// the burst has to arrive as one notification — otherwise this trades a timer
// for something that fires even more often than the timer did.
func TestMarkDirtyCoalescesABurstIntoOneNotification(t *testing.T) {
	hub, emitted := newTestHub(t)
	for range 20 {
		hub.MarkDirty()
	}
	time.Sleep(40 * time.Millisecond)
	if got := emitted(); got != 1 {
		t.Fatalf("emitted %d notifications for one burst, want 1", got)
	}
}

// A later change is a different change: coalescing must delay a notification,
// never swallow one, or a list silently stops tracking reality.
func TestMarkDirtyAfterAFlushNotifiesAgain(t *testing.T) {
	hub, emitted := newTestHub(t)
	hub.MarkDirty()
	time.Sleep(40 * time.Millisecond)
	hub.MarkDirty()
	time.Sleep(40 * time.Millisecond)
	if got := emitted(); got != 2 {
		t.Fatalf("emitted %d notifications for two separated changes, want 2", got)
	}
}

// Repository writes can outlive the window they were meant to update — a run
// being torn down still writes. Emitting into a closed transport at that point
// is at best pointless.
func TestCloseStopsPendingAndSubsequentNotifications(t *testing.T) {
	hub, emitted := newTestHub(t)
	hub.MarkDirty()
	hub.Close()
	hub.MarkDirty()
	time.Sleep(40 * time.Millisecond)
	if got := emitted(); got != 0 {
		t.Fatalf("emitted %d notifications after close, want 0", got)
	}
}

// A nil hub is what a caller holds before wiring, and the repository decorator
// must not have to care.
func TestNilHubIsInert(t *testing.T) {
	var hub *Hub
	hub.MarkDirty()
	hub.Close()
}
