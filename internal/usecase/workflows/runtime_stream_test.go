package workflows

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/openmodu/onecatch/internal/usecase/agentrun"
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

// Raw is the whole JSONL line the provider emitted, so for a tool result or a
// message it duplicates the payload the event already carries — on a real
// transcript that was 2.7MB of a 3.3MB file. Nothing reads it back except the
// usage backfill, which only ever finds usage on the terminal events, so those
// are the only kinds that may keep it.
func TestRetainsRawOnlyForEventsThatCanCarryUsage(t *testing.T) {
	for _, kind := range []agentrun.EventKind{agentrun.KindUsage, agentrun.KindResult} {
		if !retainsRaw(kind) {
			t.Errorf("%s carries the provider's usage object and must keep Raw", kind)
		}
	}
	for _, kind := range []agentrun.EventKind{
		agentrun.KindMessage, agentrun.KindReasoning, agentrun.KindToolUse,
		agentrun.KindToolResult, agentrun.KindFileChange, agentrun.KindStarted, agentrun.KindError,
	} {
		if retainsRaw(kind) {
			t.Errorf("%s duplicates its own payload in Raw and must not keep it", kind)
		}
	}
}

// rawEmittingEngine reproduces what a real runtime adapter does: it hands every
// event the provider's original JSONL line alongside the fields already parsed
// out of it.
type rawEmittingEngine struct{}

func (rawEmittingEngine) Available(agentrun.Runtime) bool { return true }

func (rawEmittingEngine) Run(_ context.Context, _ agentrun.Request, sink agentrun.Sink) (agentrun.Result, error) {
	at := time.Date(2026, 7, 10, 15, 0, 0, 0, time.UTC)
	sink(agentrun.Event{Kind: agentrun.KindMessage, Text: "hello", Raw: `{"type":"assistant","duplicated":"payload"}`, At: at})
	sink(agentrun.Event{Kind: agentrun.KindToolResult, Text: "ok", Raw: `{"type":"tool_result","duplicated":"payload"}`, At: at})
	sink(agentrun.Event{Kind: agentrun.KindUsage, Raw: `{"type":"turn.completed","usage":{"input_tokens":11,"output_tokens":3}}`, At: at})
	return agentrun.Result{Succeeded: true, SessionID: "session_1", FinalMessage: `{"signal":"approved","content":"done"}`}, nil
}

// Raw dominated the stored transcript — on a real run, 2.7MB of a 3.3MB file —
// while duplicating text the event already carries. It is dropped on the way to
// disk for every kind that cannot carry usage, which is the only thing that
// ever reads it back.
func TestPersistedRuntimeEventsKeepRawOnlyWhereItIsRead(t *testing.T) {
	usecase, store, task := setupUsecase(t, oneStepPauseWorkflow(), rawEmittingEngine{})
	run, err := usecase.ExecuteTask(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	stepRuns, err := store.Repos.Workflows.ListStepRuns(context.Background(), run.ID)
	if err != nil || len(stepRuns) == 0 {
		t.Fatalf("step runs = %+v, %v", stepRuns, err)
	}

	seen := map[agentrun.EventKind]agentrun.Event{}
	for _, stepRun := range stepRuns {
		items, listErr := store.Repos.Workflows.ListRuntimeEvents(context.Background(), run.ID, stepRun.ID, 0, 100)
		if listErr != nil {
			t.Fatal(listErr)
		}
		for _, item := range items {
			var event agentrun.Event
			if err := json.Unmarshal(item.Payload, &event); err != nil {
				t.Fatal(err)
			}
			seen[event.Kind] = event
		}
	}

	for _, kind := range []agentrun.EventKind{agentrun.KindMessage, agentrun.KindToolResult, agentrun.KindUsage} {
		if _, ok := seen[kind]; !ok {
			t.Fatalf("no %s event was persisted; seen %v", kind, seen)
		}
	}
	if raw := seen[agentrun.KindMessage].Raw; raw != "" {
		t.Errorf("message kept Raw = %q, want it dropped", raw)
	}
	if raw := seen[agentrun.KindToolResult].Raw; raw != "" {
		t.Errorf("tool result kept Raw = %q, want it dropped", raw)
	}
	if raw := seen[agentrun.KindUsage].Raw; raw == "" {
		t.Error("usage lost Raw, which is where the token breakdown is recovered from")
	}
	// The text the UI renders must survive the trim untouched.
	if text := seen[agentrun.KindMessage].Text; text != "hello" {
		t.Errorf("message text = %q, want it preserved", text)
	}
}
