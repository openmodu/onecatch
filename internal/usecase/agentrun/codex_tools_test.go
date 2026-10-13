package agentrun

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestCodexParallelToolsRetainArgumentsAndIdentity(t *testing.T) {
	state := newCodexAppState()
	var events []Event
	sink := func(e Event) { events = append(events, e) }
	notifications := []struct{ method, params string }{
		{"item/started", `{"item":{"id":"shell-a","type":"commandExecution","command":"go test ./...","status":"inProgress"}}`},
		{"item/started", `{"item":{"id":"read-b","type":"mcpToolCall","server":"files","tool":"Read","arguments":{"file_path":"/repo/a.go","offset":65}}}`},
		{"item/commandExecution/outputDelta", `{"itemId":"shell-a","delta":"ok\n"}`},
		{"item/completed", `{"item":{"id":"read-b","type":"mcpToolCall","status":"completed","result":{"content":[{"type":"text","text":"65 func example()"}]}}}`},
		{"item/completed", `{"item":{"id":"shell-a","type":"commandExecution","command":"go test ./...","status":"completed","aggregatedOutput":"ok\n","exitCode":0}}`},
		{"item/started", `{"item":{"id":"dynamic-c","type":"dynamicToolCall","tool":"Read","arguments":{"path":"/repo/b.go"}}}`},
		{"item/completed", `{"item":{"id":"dynamic-c","type":"dynamicToolCall","status":"completed","contentItems":[{"type":"inputText","text":"missing file"}],"success":false}}`},
	}
	for i, n := range notifications {
		state.handleNotification(n.method, json.RawMessage(n.params), n.params, time.Unix(int64(i), 0), sink)
	}
	calls := map[string]Event{}
	returns := map[string]Event{}
	for _, e := range events {
		if e.Kind == KindToolUse {
			if _, exists := calls[e.StreamID]; exists {
				t.Fatalf("duplicate call: %+v", e)
			}
			calls[e.StreamID] = e
		}
		if e.Kind == KindToolResult && (e.Phase == StreamEnd || e.Phase == "") {
			returns[e.StreamID] = e
		}
	}
	if len(calls) != 3 || len(returns) != 3 {
		t.Fatalf("lost parallel tools: %+v", events)
	}
	for id := range returns {
		if calls[id].StreamID == "" {
			t.Fatalf("unpaired return: %q", id)
		}
	}
	if calls[codexToolID("shell-a")].Text != "go test ./..." || returns[codexToolID("shell-a")].Text != "ok\n" {
		t.Fatal("lost command or streamed output")
	}
	if !strings.Contains(calls[codexToolID("read-b")].Text, `"offset":65`) || returns[codexToolID("read-b")].Text != "65 func example()" {
		t.Fatal("lost read parameters or readable result")
	}
	if !returns[codexToolID("dynamic-c")].Failed || returns[codexToolID("dynamic-c")].Text != "missing file" {
		t.Fatal("lost dynamic tool failure/content")
	}
}

func TestCodexToolCompletionRecoversLateArguments(t *testing.T) {
	state := newCodexAppState()
	var calls []Event
	sink := func(e Event) {
		if e.Kind == KindToolUse {
			calls = append(calls, e)
		}
	}
	state.handleNotification("item/started", json.RawMessage(`{"item":{"id":"read","type":"dynamicToolCall","tool":"Read"}}`), "", time.Now(), sink)
	state.handleNotification("item/completed", json.RawMessage(`{"item":{"id":"read","type":"dynamicToolCall","tool":"Read","arguments":{"file_path":"README.md"},"result":"ok"}}`), "", time.Now(), sink)
	if len(calls) != 2 || calls[1].Revision <= calls[0].Revision || !strings.Contains(calls[1].Text, "README.md") {
		t.Fatalf("late arguments not updated: %+v", calls)
	}
}

func TestReadCodexToolHistoryIgnoresConversationAndKeepsNativeIDs(t *testing.T) {
	records := readCodexToolHistory(strings.NewReader(
		`{"type":"response_item","payload":{"type":"message","content":"not a tool"}}` + "\n" +
			`{"type":"event_msg","timestamp":"2026-10-13T07:31:01Z","payload":{"type":"item_completed","item":{"id":"exec-a","type":"CommandExecution","command":["/bin/zsh","-lc","echo 'hello'"],"aggregated_output":"hello\n","duration":{"secs":17,"nanos":500000000}}}}` + "\n" +
			`{"type":"event_msg","timestamp":"2026-10-13T07:31:02Z","payload":{"type":"item_completed","item":{"id":"mcp-b","type":"McpToolCall","server":"files","tool":"Read","arguments":{"path":"README.md"},"result":{"content":[{"text":"readme"}]}}}}`))
	if len(records) != 2 || records[0].ID != codexToolID("exec-a") || len(records[0].Command) != 3 || records[0].Output != "hello\n" || records[1].Output != "readme" || records[0].FinishedAt.Sub(records[0].At) != 17500*time.Millisecond {
		t.Fatalf("history mismatch: %+v", records)
	}
}
