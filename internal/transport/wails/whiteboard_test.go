package wailstransport

import (
	"strings"
	"testing"
	"time"

	"github.com/openmodu/oneshot/internal/usecase/agentrun"
)

func TestWhiteboardRuntimeEventKeepsSafeProgressOnly(t *testing.T) {
	frame, ok := whiteboardRuntimeEvent(" request-1 ", 3, agentrun.Event{Kind: agentrun.KindReasoning, Text: "private provider reasoning", At: time.Unix(1, 0)})
	if !ok || frame.RequestID != "request-1" || frame.Seq != 3 {
		t.Fatalf("frame = %+v, ok = %v", frame, ok)
	}
	if strings.Contains(frame.Text, "private") || frame.Text == "" {
		t.Fatalf("reasoning text leaked or missing: %q", frame.Text)
	}
	if _, ok := whiteboardRuntimeEvent("request-1", 4, agentrun.Event{Kind: agentrun.KindUsage}); ok {
		t.Fatal("usage frames should not enter the whiteboard operation stream")
	}
}

func TestCompactWhiteboardEventTextBoundsOutput(t *testing.T) {
	value := strings.Repeat("白板操作 ", 100)
	got := compactWhiteboardEventText(value)
	if len([]rune(got)) != 181 || !strings.HasSuffix(got, "…") {
		t.Fatalf("compacted text = %q", got)
	}
}

func TestWhiteboardChangeStreamEmitsCompletedOperationsOnce(t *testing.T) {
	stream := newWhiteboardChangeStream()
	start := agentrun.Event{Kind: agentrun.KindMessage, StreamID: "message-1", Phase: agentrun.StreamStart}
	if changes := stream.Push(start); len(changes) != 0 {
		t.Fatalf("start emitted changes: %+v", changes)
	}
	firstHalf := `{"summary":"拓扑","changes":[{"id":"gateway","action":"add","objectType":"card","category":"new","title":"网关 {入口}","content":"连接到`
	if changes := stream.Push(agentrun.Event{Kind: agentrun.KindMessage, StreamID: "message-1", Phase: agentrun.StreamDelta, Text: firstHalf}); len(changes) != 0 {
		t.Fatalf("partial object emitted changes: %+v", changes)
	}
	secondHalf := `路由器","x":520,"y":120,"width":220,"height":100},{"id":"router","action":"link","objectType":"card","category":"linked","title":"路由器","content":"分发","targetId":"gateway","x":760,"y":120,"width":220,"height":100}]}`
	changes := stream.Push(agentrun.Event{Kind: agentrun.KindMessage, StreamID: "message-1", Phase: agentrun.StreamDelta, Text: secondHalf})
	if len(changes) != 2 || changes[0].ID != "gateway" || changes[1].ID != "router" {
		t.Fatalf("completed changes = %+v", changes)
	}
	if changes[0].Title != "网关 {入口}" || changes[1].Action != "link" {
		t.Fatalf("changes were parsed incorrectly: %+v", changes)
	}
	if repeated := stream.Push(agentrun.Event{Kind: agentrun.KindMessage, StreamID: "message-1", Phase: agentrun.StreamEnd, Text: firstHalf + secondHalf}); len(repeated) != 0 {
		t.Fatalf("completed snapshot repeated changes: %+v", repeated)
	}
}

func TestCompletedWhiteboardChangeObjectsIgnoresOtherJSONObjects(t *testing.T) {
	message := `{"meta":{"count":2},"changes":[{"id":"one","content":"escaped \\"quote\\" and } brace"}]}`
	objects := completedWhiteboardChangeObjects(message)
	if len(objects) != 1 || !strings.Contains(objects[0], `"id":"one"`) {
		t.Fatalf("objects = %#v", objects)
	}
}

func TestWhiteboardChangeStreamKeepsFallbackIDsStable(t *testing.T) {
	stream := newWhiteboardChangeStream()
	first := `{"changes":[{"title":"网关","content":"入口"}`
	if changes := stream.Push(agentrun.Event{Kind: agentrun.KindMessage, StreamID: "message", Phase: agentrun.StreamDelta, Text: first}); len(changes) != 1 || changes[0].ID != "agent-change-1" {
		t.Fatalf("first changes = %+v", changes)
	}
	second := `,{"title":"路由器","content":"分发"}]}`
	changes := stream.Push(agentrun.Event{Kind: agentrun.KindMessage, StreamID: "message", Phase: agentrun.StreamDelta, Text: second})
	if len(changes) != 1 || changes[0].ID != "agent-change-2" {
		t.Fatalf("second changes = %+v", changes)
	}
}
