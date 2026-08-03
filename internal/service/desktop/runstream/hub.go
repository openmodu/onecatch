// Package runstream provides the in-memory, best-effort side of runtime event
// delivery. Durable JSONL events remain the source of truth; the hub only keeps
// authoritative snapshots for active streams and fans low-latency frames out
// to desktop transports such as Wails custom events.
package runstream

import (
	"sort"
	"sync"

	"github.com/openmodu/oneshot/internal/usecase/agentrun"
	workflowuc "github.com/openmodu/oneshot/internal/usecase/workflows"
)

const EventName = "oneshot:runtime-frame"

type Frame = workflowuc.RuntimeEventFrame

type streamKey struct {
	stepRunID string
	streamID  string
}

// Hub retains current full-text snapshots and broadcasts frames. Subscribers
// must return quickly; Wails Emit only serializes the small frame passed to it.
type Hub struct {
	mu          sync.RWMutex
	runs        map[string]map[streamKey]Frame
	subscribers map[uint64]func(Frame)
	nextID      uint64
}

func NewHub() *Hub {
	return &Hub{runs: make(map[string]map[streamKey]Frame), subscribers: make(map[uint64]func(Frame))}
}

func (h *Hub) Publish(frame Frame) {
	if h == nil || frame.RunID == "" {
		return
	}
	h.mu.Lock()
	if frame.StreamID != "" && frame.Phase != "" {
		streams := h.runs[frame.RunID]
		if streams == nil {
			streams = make(map[streamKey]Frame)
			h.runs[frame.RunID] = streams
		}
		key := streamKey{stepRunID: frame.StepRunID, streamID: frame.StreamID}
		current, exists := streams[key]
		if !exists || frame.Revision > current.Revision {
			switch frame.Phase {
			case agentrun.StreamStart:
				current = frame
				current.Text = ""
			case agentrun.StreamDelta:
				if !exists {
					current = frame
					current.Text = ""
				}
				current.Text += frame.Text
				current.Kind = frame.Kind
				current.Phase = agentrun.StreamSnapshot
				current.Revision = frame.Revision
				current.At = frame.At
			case agentrun.StreamSnapshot, agentrun.StreamEnd:
				current = frame
			}
			streams[key] = current
		}
	}
	callbacks := make([]func(Frame), 0, len(h.subscribers))
	for _, callback := range h.subscribers {
		callbacks = append(callbacks, callback)
	}
	h.mu.Unlock()
	for _, callback := range callbacks {
		callback(frame)
	}
}

// Snapshot returns authoritative full-text frames for a run. Subscribe before
// requesting this snapshot; Revision lets the receiver discard queued frames
// that the snapshot already includes.
func (h *Hub) Snapshot(runID string) []Frame {
	if h == nil {
		return nil
	}
	h.mu.RLock()
	streams := h.runs[runID]
	out := make([]Frame, 0, len(streams))
	for _, frame := range streams {
		// A completed stream must stay completed. Turning an end frame back into
		// a snapshot would make a reconnecting frontend show a permanent typing
		// indicator even though the provider already supplied the final value.
		if frame.Phase != agentrun.StreamEnd {
			frame.Phase = agentrun.StreamSnapshot
		}
		out = append(out, frame)
	}
	h.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool {
		if out[i].Seq != out[j].Seq {
			return out[i].Seq < out[j].Seq
		}
		if out[i].StepRunID != out[j].StepRunID {
			return out[i].StepRunID < out[j].StepRunID
		}
		return out[i].StreamID < out[j].StreamID
	})
	return out
}

func (h *Hub) ClearRun(runID string) {
	if h == nil {
		return
	}
	h.mu.Lock()
	delete(h.runs, runID)
	h.mu.Unlock()
}

// Subscribe registers a callback and returns an idempotent unsubscribe
// function.
func (h *Hub) Subscribe(callback func(Frame)) func() {
	if h == nil || callback == nil {
		return func() {}
	}
	h.mu.Lock()
	h.nextID++
	id := h.nextID
	h.subscribers[id] = callback
	h.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			h.mu.Lock()
			delete(h.subscribers, id)
			h.mu.Unlock()
		})
	}
}
