package runstream

import (
	"testing"
	"time"

	"github.com/openmodu/oneshot/internal/usecase/agentrun"
)

func TestHubAccumulatesSnapshotsAndBroadcastsFrames(t *testing.T) {
	hub := NewHub()
	var received []Frame
	unsubscribe := hub.Subscribe(func(frame Frame) { received = append(received, frame) })
	now := time.Unix(1, 0)
	base := Frame{RunID: "run-1", StepRunID: "step-1", Seq: 4, Event: agentrun.Event{Kind: agentrun.KindMessage, StreamID: "message-1", At: now}}
	start := base
	start.Phase = agentrun.StreamStart
	hub.Publish(start)
	first := base
	first.Phase, first.Revision, first.Text = agentrun.StreamDelta, 1, "hel"
	hub.Publish(first)
	second := base
	second.Phase, second.Revision, second.Text = agentrun.StreamDelta, 2, "lo"
	hub.Publish(second)

	snapshot := hub.Snapshot("run-1")
	if len(snapshot) != 1 || snapshot[0].Text != "hello" || snapshot[0].Phase != agentrun.StreamSnapshot || snapshot[0].Revision != 2 {
		t.Fatalf("snapshot = %+v", snapshot)
	}
	if len(received) != 3 {
		t.Fatalf("received %d frames", len(received))
	}

	// A stale frame must neither roll text back nor overwrite the revision.
	stale := base
	stale.Phase, stale.Revision, stale.Text = agentrun.StreamSnapshot, 1, "wrong"
	hub.Publish(stale)
	if got := hub.Snapshot("run-1")[0].Text; got != "hello" {
		t.Fatalf("stale snapshot changed text to %q", got)
	}

	unsubscribe()
	hub.Publish(second)
	if len(received) != 4 { // stale frame was broadcast; post-unsubscribe was not.
		t.Fatalf("received %d frames after unsubscribe", len(received))
	}
	hub.ClearRun("run-1")
	if got := hub.Snapshot("run-1"); len(got) != 0 {
		t.Fatalf("snapshot after clear = %+v", got)
	}
}

func TestHubSnapshotPreservesCompletedStream(t *testing.T) {
	hub := NewHub()
	hub.Publish(Frame{RunID: "run-1", StepRunID: "step-1", Seq: 3, Event: agentrun.Event{Kind: agentrun.KindMessage, StreamID: "message-1", Phase: agentrun.StreamEnd, Revision: 2, Text: "done"}})

	frames := hub.Snapshot("run-1")
	if len(frames) != 1 || frames[0].Phase != agentrun.StreamEnd {
		t.Fatalf("snapshot = %#v, want completed frame", frames)
	}
}
