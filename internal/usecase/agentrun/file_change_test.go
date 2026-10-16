package agentrun

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestAppServerFileChangesCarryDiffs(t *testing.T) {
	item := `{"id":"patch","type":"fileChange","status":"completed","changes":[` +
		`{"path":"/repo/a.go","kind":{"type":"update","move_path":null},"diff":"@@ -2,2 +2,2 @@\n ctx\n-old\n+new"},` +
		`{"path":"/repo/new.go","kind":{"type":"add"},"diff":"package repo\n\nfunc A() {}\n"}]}`
	check := func(t *testing.T, events []Event) {
		t.Helper()
		var change *Event
		for i := range events {
			if events[i].Kind == KindFileChange {
				change = &events[i]
			}
		}
		if change == nil || change.Text != "/repo/a.go\n/repo/new.go" || len(change.FileChanges) != 2 {
			t.Fatalf("file change = %+v", events)
		}
		if got := change.FileChanges[0]; got.Kind != "update" || got.Diff != "@@ -2,2 +2,2 @@\n ctx\n-old\n+new" {
			t.Fatalf("update = %+v", got)
		}
		// An added file arrives as its content and is stored as a hunk.
		if got := change.FileChanges[1]; got.Kind != "add" || got.Diff != "@@ -0,0 +1,3 @@\n+package repo\n+\n+func A() {}" {
			t.Fatalf("add = %+v", got)
		}
	}
	t.Run("codex", func(t *testing.T) {
		var events []Event
		params := `{"item":` + item + `}`
		newCodexAppState().handleNotification("item/completed", json.RawMessage(params), params, time.Now(), func(e Event) { events = append(events, e) })
		check(t, events)
	})
	t.Run("trae", func(t *testing.T) {
		state := traeTurnState{result: Result{SessionID: "thread"}, streams: map[string]bool{}}
		var events []Event
		frame := traeFrame{Method: "item/completed", Params: json.RawMessage(`{"threadId":"thread","turnId":"turn","item":` + item + `}`)}
		if _, err := state.update(frame, "turn", func(e Event) { events = append(events, e) }); err != nil {
			t.Fatal(err)
		}
		check(t, events)
	})
}

func TestToolInputFileChange(t *testing.T) {
	for _, testCase := range []struct {
		name  string
		input string
		diff  string
	}{
		{"edit", `{"file_path":"a.go","old_string":"a\nb\nc","new_string":"a\nB\nc"}`, "@@ @@\n a\n-b\n+B\n c"},
		{"pi edits", `{"path":"a.go","edits":[{"oldText":"x","newText":"y"},{"oldText":"","newText":"z"}]}`, "@@ @@\n-x\n+y\n@@ @@\n+z"},
		{"write", `{"path":"a.go","content":"one\ntwo\n"}`, "@@ -0,0 +1,2 @@\n+one\n+two"},
		{"path only", `{"path":"a.go"}`, ""},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			var input map[string]any
			if err := json.Unmarshal([]byte(testCase.input), &input); err != nil {
				t.Fatal(err)
			}
			change, ok := toolInputFileChange(input)
			if !ok || change.Path != "a.go" || change.Diff != testCase.diff {
				t.Fatalf("change = %+v, %v", change, ok)
			}
		})
	}
}

func TestACPFileChangesReadDiffContent(t *testing.T) {
	changes := acpFileChanges(json.RawMessage(`[{"type":"content","content":{"type":"text","text":"x"}},{"type":"diff","path":"a.go","oldText":"a","newText":"b"},{"type":"diff","path":"n.go","newText":"n"}]`))
	if len(changes) != 2 || changes[0].Diff != "@@ @@\n-a\n+b" || changes[1].Kind != "add" || changes[1].Diff != "@@ -0,0 +1,1 @@\n+n" {
		t.Fatalf("changes = %+v", changes)
	}
}

func TestFileChangeDiffIsBounded(t *testing.T) {
	change := NewFileChange("big.txt", "add", "", strings.Repeat("line\n", maxFileChangeDiff/4))
	if !change.Truncated || len(change.Diff) > maxFileChangeDiff || strings.HasSuffix(change.Diff, "\n") {
		t.Fatalf("diff len = %d, truncated = %v", len(change.Diff), change.Truncated)
	}
	change = NewFileChange("long.txt", "update", "", strings.Repeat("中", maxFileChangeDiff))
	if !change.Truncated || !utf8.ValidString(change.Diff) {
		t.Fatal("truncation broke UTF-8")
	}
}

func TestACPFileChangesFromToolUpdates(t *testing.T) {
	c := &acpClient{}
	var events []Event
	sink := func(e Event) { events = append(events, e) }
	for _, raw := range []string{
		`{"sessionUpdate":"tool_call","toolCallId":"edit","kind":"edit","locations":[{"path":"a.go"}]}`,
		`{"sessionUpdate":"tool_call_update","toolCallId":"edit","content":[{"type":"diff","path":"a.go","oldText":"old","newText":"new"}]}`,
		`{"sessionUpdate":"tool_call_update","toolCallId":"edit","status":"failed"}`,
		`{"sessionUpdate":"tool_call","toolCallId":"read","kind":"read","locations":[{"path":"a.go"}]}`,
	} {
		var update acpUpdate
		if err := json.Unmarshal([]byte(raw), &update); err != nil {
			t.Fatal(err)
		}
		c.emitFileChanges(update, raw, time.Now(), sink)
	}
	if len(events) != 3 || events[0].StreamID == "" || events[0].StreamID != events[1].StreamID || events[1].StreamID != events[2].StreamID || !events[2].Failed || events[2].FileChanges[0].Diff != "@@ @@\n-old\n+new" {
		t.Fatalf("ACP lost updates, failure, or reported a read as an edit: %+v", events)
	}
}
