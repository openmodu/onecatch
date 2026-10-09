package keeldesktop

import (
	"testing"
	"time"

	domaintasks "github.com/openmodu/onecatch/internal/domain/tasks"
	domainworkflows "github.com/openmodu/onecatch/internal/domain/workflows"
	desktopservice "github.com/openmodu/onecatch/internal/service/desktop"
	"github.com/openmodu/onecatch/internal/service/desktop/runstream"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
)

func assertUserMessages(t *testing.T, tx *transcript, want ...string) {
	t.Helper()
	var got []string
	for _, item := range buildConversationEntries(tx.visible()) {
		if item.user {
			got = append(got, item.text)
		}
	}
	if len(got) != len(want) {
		t.Fatalf("user messages = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("user messages = %q, want %q", got, want)
		}
	}
}

func TestTranscriptRestoresLegacyUserBubblesWithoutDuplicatingRefreshes(t *testing.T) {
	at := time.Now().UTC()
	detail := desktopservice.RunDetail{
		Task: domaintasks.Task{Prompt: "initial", CreatedAt: at},
		Events: []desktopservice.WorkflowEventView{
			{Seq: 2, Type: "run.resumed", Payload: `{"instruction":"hello"}`, At: at.Add(time.Second).Format(time.RFC3339Nano)},
			{Seq: 4, Type: "run.resumed", Payload: `{"instruction":"hello"}`, At: at.Add(2 * time.Second).Format(time.RFC3339Nano)},
			{Seq: 6, Type: "run.resumed", Payload: `{"instruction":""}`},
			{Seq: 7, Type: "run.resumed", Payload: `invalid`},
		},
	}
	tx := newTranscript()
	tx.detail(detail)
	tx.detail(detail)
	// A live state snapshot has no event history. Keep restored legacy turns.
	tx.detail(desktopservice.RunDetail{Task: detail.Task})
	assertUserMessages(t, tx, "initial", "hello", "hello")
}

func TestTranscriptRemovesDeletedQueuedBubbles(t *testing.T) {
	tx := newTranscript()
	detail := desktopservice.RunDetail{Task: domaintasks.Task{Prompt: "initial"}, Instructions: []domainworkflows.Instruction{{ID: "queued", Content: "remove me", Status: domainworkflows.InstructionPending, CreatedAt: time.Now()}}}
	tx.detail(detail)
	assertUserMessages(t, tx, "initial", "remove me")
	detail.Instructions[0].Status = domainworkflows.InstructionRemoved
	tx.detail(detail)
	assertUserMessages(t, tx, "initial")
	detail.Instructions = nil
	tx.detail(detail)
	assertUserMessages(t, tx, "initial")
}

func TestTranscriptReconcilesLiveHistoryAndRepeatedFrames(t *testing.T) {
	tx := newTranscript()
	at := time.Now().UTC()
	f := runstream.Frame{StepRunID: "step", Seq: 1, Event: agentrun.Event{Kind: agentrun.KindMessage, StreamID: "answer", Phase: agentrun.StreamSnapshot, Revision: 2, Text: "Hello", At: at}}
	tx.frame(f)
	// A late disk read must not erase tokens that were already painted.
	tx.history([]desktopservice.RuntimeEventView{{StepRunID: "step", Seq: 1, Kind: "message", StreamID: "answer", Revision: 1, Text: "Hel", At: at.Format(time.RFC3339Nano)}})
	f.Phase, f.Revision, f.Text = agentrun.StreamDelta, 3, " world"
	tx.frame(f)
	tx.frame(f)
	items := tx.visible()
	if len(items) != 1 || items[0].text != "Hello world" || !items[0].streaming {
		t.Fatalf("live items = %+v", items)
	}
	f.Phase, f.Revision, f.Text = agentrun.StreamEnd, 4, "Hello world!"
	tx.frame(f)
	f.Phase, f.Revision, f.Text = agentrun.StreamSnapshot, 3, "Hello world"
	tx.frame(f)
	if got := tx.visible()[0]; got.text != "Hello world!" || got.streaming {
		t.Fatalf("finished entry = %+v", got)
	}
}

func TestTranscriptSeparatesStepsWithEqualStreamIDs(t *testing.T) {
	tx := newTranscript()
	for _, step := range []string{"first", "second"} {
		tx.frame(runstream.Frame{StepRunID: step, Event: agentrun.Event{Kind: agentrun.KindMessage, StreamID: "one", Phase: agentrun.StreamEnd, Revision: 1, Text: step}})
	}
	if len(tx.visible()) != 2 {
		t.Fatalf("merged different steps: %+v", tx.visible())
	}
}

func TestTranscriptResolvedRequestsStayResolvedAfterLateHistory(t *testing.T) {
	tx := newTranscript()
	p := &agentrun.PermissionRequest{ID: "permission", ToolName: "shell"}
	q := &agentrun.UserInputRequest{ID: "question"}
	tx.frame(runstream.Frame{Seq: 2, Event: agentrun.Event{Kind: agentrun.KindPermissionResolved, Permission: p}})
	tx.frame(runstream.Frame{Seq: 4, Event: agentrun.Event{Kind: agentrun.KindUserInputResolved, UserInput: q}})
	tx.history([]desktopservice.RuntimeEventView{{Seq: 1, Kind: "permission_request", Permission: p}, {Seq: 3, Kind: "user_input_request", UserInput: q}})
	if len(tx.visible()) != 0 {
		t.Fatalf("resolved cards reappeared: %+v", tx.visible())
	}
}
