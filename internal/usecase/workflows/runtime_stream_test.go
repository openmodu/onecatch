package workflows

import (
	"testing"
	"time"

	"github.com/openmodu/oneshot/internal/usecase/agentrun"
)

type collectingPublisher struct{ frames []RuntimeEventFrame }

func (p *collectingPublisher) Publish(frame RuntimeEventFrame) {
	p.frames = append(p.frames, frame)
}

func TestRuntimeEventCollectorBatchesDeltaAndPersistsAuthoritativeEnd(t *testing.T) {
	var stored []agentrun.Event
	publisher := &collectingPublisher{}
	collector := newRuntimeEventCollector("run-1", "step-1", func(event agentrun.Event) (int64, error) {
		stored = append(stored, event)
		return int64(len(stored)), nil
	}, publisher)
	collector.liveEvery, collector.durableEvery = time.Hour, time.Hour
	now := time.Unix(1, 0)
	collector.Push(agentrun.Event{Kind: agentrun.KindMessage, StreamID: "message-1", Phase: agentrun.StreamStart, At: now})
	collector.Push(agentrun.Event{Kind: agentrun.KindMessage, StreamID: "message-1", Phase: agentrun.StreamDelta, Text: "hel", At: now})
	collector.Push(agentrun.Event{Kind: agentrun.KindMessage, StreamID: "message-1", Phase: agentrun.StreamDelta, Text: "lo", At: now})
	if err := collector.Close(); err != nil {
		t.Fatal(err)
	}

	if len(stored) != 3 || stored[0].Phase != agentrun.StreamStart || stored[1].Phase != agentrun.StreamDelta || stored[1].Text != "hello" || stored[2].Phase != agentrun.StreamEnd || stored[2].Text != "hello" {
		t.Fatalf("stored = %+v", stored)
	}
	if len(publisher.frames) != 3 || publisher.frames[1].Text != "hello" || publisher.frames[1].Revision != 1 || publisher.frames[2].Revision != 2 {
		t.Fatalf("published = %+v", publisher.frames)
	}
}

func TestRuntimeEventCollectorKeepsAtomicEvents(t *testing.T) {
	var stored []agentrun.Event
	publisher := &collectingPublisher{}
	collector := newRuntimeEventCollector("run-1", "step-1", func(event agentrun.Event) (int64, error) {
		stored = append(stored, event)
		return 7, nil
	}, publisher)
	collector.Push(agentrun.Event{Kind: agentrun.KindToolUse, Text: "go test ./..."})
	if err := collector.Close(); err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1 || len(publisher.frames) != 1 || publisher.frames[0].Seq != 7 {
		t.Fatalf("stored/published = %+v / %+v", stored, publisher.frames)
	}
}
