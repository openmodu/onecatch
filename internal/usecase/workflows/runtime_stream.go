package workflows

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/openmodu/onecatch/internal/usecase/agentrun"
)

const (
	defaultLiveFlush    = 80 * time.Millisecond
	defaultDurableFlush = 500 * time.Millisecond
)

type runtimeEventCollector struct {
	mu           sync.Mutex
	runID        string
	stepRunID    string
	persist      func(agentrun.Event) (int64, error)
	publisher    RuntimeEventPublisher
	liveEvery    time.Duration
	durableEvery time.Duration
	streams      map[string]*runtimeStreamState
	liveTimer    *time.Timer
	durableTimer *time.Timer
	closed       bool
	err          error
}

type runtimeStreamState struct {
	kind        agentrun.EventKind
	seq         int64
	revision    uint64
	full        string
	livePending string
	diskPending string
	lastAt      time.Time
	lastRaw     string
	failed      bool
	ended       bool
}

func newRuntimeEventCollector(runID, stepRunID string, persist func(agentrun.Event) (int64, error), publisher RuntimeEventPublisher) *runtimeEventCollector {
	return &runtimeEventCollector{
		runID: runID, stepRunID: stepRunID, persist: persist, publisher: publisher,
		liveEvery: defaultLiveFlush, durableEvery: defaultDurableFlush,
		streams: make(map[string]*runtimeStreamState),
	}
}

func (c *runtimeEventCollector) Sink() agentrun.Sink { return c.Push }

func (c *runtimeEventCollector) Push(event agentrun.Event) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	switch event.Phase {
	case agentrun.StreamDelta:
		state := c.ensureStreamLocked(event)
		state.livePending += event.Text
		state.diskPending += event.Text
		state.lastAt, state.lastRaw, state.failed = event.At, event.Raw, event.Failed
		c.scheduleLocked()
	case agentrun.StreamStart:
		c.flushAllLocked()
		c.startLocked(event)
	case agentrun.StreamSnapshot, agentrun.StreamEnd:
		c.flushAllLocked()
		c.endLocked(event)
	default:
		c.flushAllLocked()
		seq := c.persistLocked(event)
		c.publishLocked(seq, event)
	}
}

func (c *runtimeEventCollector) ensureStreamLocked(event agentrun.Event) *runtimeStreamState {
	if state := c.streams[event.StreamID]; state != nil {
		return state
	}
	start := event
	start.Phase, start.Text, start.Revision = agentrun.StreamStart, "", 0
	c.startLocked(start)
	return c.streams[event.StreamID]
}

func (c *runtimeEventCollector) startLocked(event agentrun.Event) {
	if event.StreamID == "" {
		return
	}
	if state := c.streams[event.StreamID]; state != nil && !state.ended {
		return
	}
	event.Revision = 0
	seq := c.persistLocked(event)
	c.streams[event.StreamID] = &runtimeStreamState{kind: event.Kind, seq: seq, lastAt: event.At, lastRaw: event.Raw, failed: event.Failed}
	c.publishLocked(seq, event)
}

func (c *runtimeEventCollector) endLocked(event agentrun.Event) {
	if event.StreamID == "" {
		seq := c.persistLocked(event)
		c.publishLocked(seq, event)
		return
	}
	state := c.ensureStreamLocked(event)
	if state.ended {
		return
	}
	state.revision++
	event.Revision = state.revision
	if event.Text == "" {
		event.Text = state.full
	}
	if event.At.IsZero() {
		event.At = state.lastAt
	}
	if event.Phase == agentrun.StreamSnapshot {
		event.Phase = agentrun.StreamEnd
	}
	seq := c.persistLocked(event)
	if state.seq == 0 {
		state.seq = seq
	}
	state.full, state.ended = event.Text, true
	c.publishLocked(state.seq, event)
}

func (c *runtimeEventCollector) scheduleLocked() {
	if c.liveTimer == nil {
		c.liveTimer = time.AfterFunc(c.liveEvery, func() {
			c.mu.Lock()
			c.liveTimer = nil
			if !c.closed {
				c.flushLiveLocked()
			}
			c.mu.Unlock()
		})
	}
	if c.durableTimer == nil {
		c.durableTimer = time.AfterFunc(c.durableEvery, func() {
			c.mu.Lock()
			c.durableTimer = nil
			if !c.closed {
				c.flushLiveLocked()
				c.flushDurableLocked()
			}
			c.mu.Unlock()
		})
	}
}

func (c *runtimeEventCollector) flushAllLocked() {
	if c.liveTimer != nil {
		c.liveTimer.Stop()
		c.liveTimer = nil
	}
	if c.durableTimer != nil {
		c.durableTimer.Stop()
		c.durableTimer = nil
	}
	c.flushLiveLocked()
	c.flushDurableLocked()
}

func (c *runtimeEventCollector) streamIDs() []string {
	ids := make([]string, 0, len(c.streams))
	for id := range c.streams {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func (c *runtimeEventCollector) flushLiveLocked() {
	for _, id := range c.streamIDs() {
		state := c.streams[id]
		if state.ended || state.livePending == "" {
			continue
		}
		state.revision++
		text := state.livePending
		state.livePending = ""
		state.full += text
		event := agentrun.Event{Kind: state.kind, StreamID: id, Phase: agentrun.StreamDelta, Revision: state.revision, Text: text, Raw: state.lastRaw, Failed: state.failed, At: state.lastAt}
		c.publishLocked(state.seq, event)
	}
}

func (c *runtimeEventCollector) flushDurableLocked() {
	for _, id := range c.streamIDs() {
		state := c.streams[id]
		if state.ended || state.diskPending == "" {
			continue
		}
		text := state.diskPending
		state.diskPending = ""
		event := agentrun.Event{Kind: state.kind, StreamID: id, Phase: agentrun.StreamDelta, Revision: state.revision, Text: text, Raw: state.lastRaw, Failed: state.failed, At: state.lastAt}
		c.persistLocked(event)
	}
}

func (c *runtimeEventCollector) persistLocked(event agentrun.Event) int64 {
	if c.persist == nil || c.err != nil {
		return 0
	}
	seq, err := c.persist(event)
	if err != nil {
		c.err = err
		return 0
	}
	return seq
}

func (c *runtimeEventCollector) publishLocked(seq int64, event agentrun.Event) {
	if c.publisher != nil {
		c.publisher.Publish(RuntimeEventFrame{RunID: c.runID, StepRunID: c.stepRunID, Seq: seq, Event: event})
	}
}

func (c *runtimeEventCollector) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return c.err
	}
	c.flushAllLocked()
	for _, id := range c.streamIDs() {
		state := c.streams[id]
		if state.ended {
			continue
		}
		c.endLocked(agentrun.Event{Kind: state.kind, StreamID: id, Phase: agentrun.StreamEnd, Text: state.full, Raw: state.lastRaw, Failed: state.failed, At: state.lastAt})
	}
	c.closed = true
	return c.err
}

// retainsRaw reports whether an event's original provider line is worth
// storing. Raw is the whole JSONL line the runtime emitted — for a tool result
// or a message that is the entire payload a second time, and it dominates the
// file: on a real transcript it was 2.7MB of 3.3MB. Nothing reads it back
// except the usage backfill, which only ever finds usage on the terminal
// events, so those are the only ones that keep it.
func retainsRaw(kind agentrun.EventKind) bool {
	return kind == agentrun.KindUsage || kind == agentrun.KindResult
}

func (s *Usecase) newRuntimeCollector(runID, stepRunID string) *runtimeEventCollector {
	persist := func(event agentrun.Event) (int64, error) {
		if !retainsRaw(event.Kind) {
			event.Raw = ""
		}
		payload, err := json.Marshal(event)
		if err != nil {
			return 0, err
		}
		stored, err := s.workflows.AppendRuntimeEvent(context.Background(), runID, stepRunID, payload)
		if err != nil {
			return 0, err
		}
		return stored.Seq, nil
	}
	return newRuntimeEventCollector(runID, stepRunID, persist, s.runtimePublisher)
}

func collectorError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("persist runtime event: %w", err)
}
