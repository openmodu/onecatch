// Package listchange tells the desktop frontend that the task and run lists it
// is showing may be stale.
//
// The workbench used to keep those lists fresh by re-fetching them on a timer —
// every 1.4 seconds while anything was running. Each tick re-read every task
// file on disk twice and up to fifty run files, serialised the lot across the
// bridge, and compared it against what was already on screen, almost always to
// conclude that nothing had changed. This hub replaces the question with an
// answer: the repositories say when something moved, and only then does the
// frontend go and look.
//
// The signal is deliberately contentless. A list refresh is cheap and already
// implemented; what was expensive was asking on a timer. Naming what changed
// would only tempt callers into partial updates that then have to be kept
// consistent with the real query.
package listchange

import (
	"sync"
	"time"
)

const EventName = "onecatch:lists-changed"

// defaultCoalesce absorbs the burst a single transition produces — a task
// saved, its run created, its first step recorded — into one notification.
const defaultCoalesce = 250 * time.Millisecond

// Hub coalesces change marks and emits at most one notification per window.
type Hub struct {
	mu       sync.Mutex
	timer    *time.Timer
	coalesce time.Duration
	emit     func()
	stopped  bool
}

func NewHub() *Hub {
	return &Hub{coalesce: defaultCoalesce}
}

// SetEmitter installs the transport, typically a Wails event emit.
func (h *Hub) SetEmitter(emit func()) {
	h.mu.Lock()
	h.emit = emit
	h.mu.Unlock()
}

// MarkDirty records that a list-visible write happened. It is called from
// repository writes, so it must not block: the emit happens on a timer.
func (h *Hub) MarkDirty() {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.stopped || h.timer != nil {
		return
	}
	h.timer = time.AfterFunc(h.coalesce, h.flush)
}

func (h *Hub) flush() {
	h.mu.Lock()
	h.timer = nil
	emit := h.emit
	stopped := h.stopped
	h.mu.Unlock()
	if stopped || emit == nil {
		return
	}
	emit()
}

// Close stops any pending notification. A hub that has been closed ignores
// further marks, so shutdown cannot emit into a torn-down transport.
func (h *Hub) Close() {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.stopped = true
	if h.timer != nil {
		h.timer.Stop()
		h.timer = nil
	}
}
