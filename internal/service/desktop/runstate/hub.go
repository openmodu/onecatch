// Package runstate pushes the small, bounded half of a run's state to the
// desktop frontend.
//
// A run detail is really two different things glued together: a bounded state
// object (run + step runs + instructions) that changes often, and an unbounded
// append-only transcript that only grows. Polling the pair forces a full
// transcript re-read on every status tick, which is what made the desktop UI
// stutter as a run got longer. The transcript already streams incrementally via
// the runstream package, so this hub carries only the bounded half and lets the
// frontend drop its reconcile polling entirely.
package runstate

import (
	"sync"
	"time"

	domainworkflows "github.com/openmodu/oneshot/internal/domain/workflows"
)

const EventName = "oneshot:run-state"

// defaultCoalesce batches the burst of writes a single step transition
// produces (update run, save step run, claim instructions) into one frame.
const defaultCoalesce = 60 * time.Millisecond

// View is the bounded slice of a run the frontend needs to keep its header,
// status pills, inspector and composer honest. It deliberately carries no
// runtime events: those arrive as runstream frames.
type View struct {
	RunID        string                        `json:"runId"`
	Run          domainworkflows.Run           `json:"run"`
	StepRuns     []domainworkflows.StepRun     `json:"stepRuns"`
	Instructions []domainworkflows.Instruction `json:"instructions"`
	Active       bool                          `json:"active"`
}

// Notifier is the tiny surface the repository decorator depends on, so the
// storage layer does not need to know about Wails or view assembly.
type Notifier interface {
	MarkDirty(runID string)
}

// Hub coalesces change notifications and turns each into one resolved View.
// Resolve reads only bounded data, so a flush stays cheap no matter how long
// the run's transcript has grown.
type Hub struct {
	mu       sync.Mutex
	dirty    map[string]struct{}
	timer    *time.Timer
	coalesce time.Duration
	resolve  func(runID string) (View, bool)
	emit     func(View)
	stopped  bool
}

func NewHub() *Hub {
	return &Hub{dirty: make(map[string]struct{}), coalesce: defaultCoalesce}
}

// SetResolver supplies the assembly step. It runs off the caller's goroutine so
// a slow read never blocks a repository write.
func (h *Hub) SetResolver(resolve func(runID string) (View, bool)) {
	h.mu.Lock()
	h.resolve = resolve
	h.mu.Unlock()
}

// SetEmitter installs the transport, typically a Wails event emit.
func (h *Hub) SetEmitter(emit func(View)) {
	h.mu.Lock()
	h.emit = emit
	h.mu.Unlock()
}

// MarkDirty records that a run's bounded state changed. It never blocks and is
// safe to call while holding a repository lock.
func (h *Hub) MarkDirty(runID string) {
	if h == nil || runID == "" {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.stopped {
		return
	}
	h.dirty[runID] = struct{}{}
	if h.timer == nil {
		h.timer = time.AfterFunc(h.coalesce, h.flush)
	}
}

func (h *Hub) flush() {
	h.mu.Lock()
	if h.stopped {
		h.mu.Unlock()
		return
	}
	h.timer = nil
	runIDs := make([]string, 0, len(h.dirty))
	for runID := range h.dirty {
		runIDs = append(runIDs, runID)
	}
	h.dirty = make(map[string]struct{})
	resolve, emit := h.resolve, h.emit
	h.mu.Unlock()

	if resolve == nil || emit == nil {
		return
	}
	for _, runID := range runIDs {
		if view, ok := resolve(runID); ok {
			emit(view)
		}
	}
}

// Close stops further flushes so shutdown does not race the transport.
func (h *Hub) Close() {
	if h == nil {
		return
	}
	h.mu.Lock()
	h.stopped = true
	if h.timer != nil {
		h.timer.Stop()
		h.timer = nil
	}
	h.mu.Unlock()
}
