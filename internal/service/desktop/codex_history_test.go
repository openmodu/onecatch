package desktop

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	workflows "github.com/openmodu/onecatch/internal/domain/workflows"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
)

func TestRestoreCodexParallelHistoryMatchesCommandsNotOrder(t *testing.T) {
	at := time.Date(2026, 10, 13, 15, 31, 0, 0, time.UTC)
	history := []agentrun.CodexToolHistory{
		{ID: "codex-tool-output-a", Command: []string{"/bin/zsh", "-lc", "echo a"}, Output: "a", At: at},
		{ID: "codex-tool-output-b", Command: []string{"/bin/zsh", "-lc", "echo b"}, Output: "b", At: at},
	}
	views := []RuntimeEventView{
		{Kind: "tool_use", Text: "/bin/zsh -lc 'echo a'", At: at.Format(time.RFC3339Nano)},
		{Kind: "tool_use", Text: "/bin/zsh -lc \"echo b\"", At: at.Format(time.RFC3339Nano)},
		{Kind: "tool_result", Text: "b"}, {Kind: "tool_result", Text: "a"},
	}
	restoreCodexToolCallsFromHistory(views, history)
	if views[0].StreamID != views[3].StreamID || views[1].StreamID != views[2].StreamID || views[0].StreamID == views[1].StreamID || views[0].StreamID == "" {
		t.Fatalf("wrong parallel pairing: %+v", views)
	}
	history = append(history, agentrun.CodexToolHistory{ID: "codex-tool-output-c", Command: history[0].Command, At: at})
	views = []RuntimeEventView{{Kind: "tool_use", Text: "/bin/zsh -lc 'echo a'", At: at.Format(time.RFC3339Nano)}}
	restoreCodexToolCallsFromHistory(views, history)
	if views[0].StreamID != "" {
		t.Fatal("guessed an ambiguous command")
	}
}

func TestCodexLateParametersFoldWithoutDuplicateCall(t *testing.T) {
	events := []agentrun.Event{
		{Kind: agentrun.KindToolUse, StreamID: "codex-tool-output-a", Revision: 1, Text: "Read", At: time.Unix(1, 0)},
		{Kind: agentrun.KindToolUse, StreamID: "codex-tool-output-a", Revision: 2, Text: `Read {"path":"README.md"}`, At: time.Unix(2, 0)},
		{Kind: agentrun.KindToolResult, StreamID: "codex-tool-output-a", Phase: agentrun.StreamEnd, Text: "readme", At: time.Unix(3, 0)},
	}
	var stored []workflows.RuntimeEvent
	for i, e := range events {
		data, _ := json.Marshal(e)
		stored = append(stored, workflows.RuntimeEvent{Seq: int64(i + 1), Payload: data})
	}
	views, _ := foldRuntimeEventViews("step", stored, false)
	if len(views) != 2 || views[0].Text != events[1].Text || views[0].At != events[0].At.Format(time.RFC3339Nano) || views[1].Kind != "tool_result" {
		t.Fatalf("bad folded parameters: %+v", views)
	}
}

// Opt-in read-only replay of the user's saved step.
func TestReplayCodexToolHistory(t *testing.T) {
	path := os.Getenv("ONECATCH_CODEX_REPLAY_STEP")
	if path == "" {
		t.Skip("set ONECATCH_CODEX_REPLAY_STEP")
	}
	file, err := os.Open(filepath.Join(path, "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	var stored []workflows.RuntimeEvent
	decoder := json.NewDecoder(file)
	for decoder.More() {
		var row workflows.RuntimeEvent
		if err := decoder.Decode(&row); err != nil {
			t.Fatal(err)
		}
		stored = append(stored, row)
	}
	views, _ := foldRuntimeEventViews("step", stored, false)
	before := 0
	beforeResults := 0
	for _, v := range views {
		if v.Kind == "tool_use" && v.StreamID == "" {
			before++
		}
		if v.Kind == "tool_result" && v.StreamID == "" {
			beforeResults++
		}
	}
	restoreCodexToolCalls(views, map[string][]agentrun.CodexToolHistory{})
	after := 0
	afterResults := 0
	for _, v := range views {
		if v.Kind == "tool_use" && v.StreamID == "" {
			after++
		}
		if v.Kind == "tool_result" && v.StreamID == "" {
			afterResults++
		}
	}
	t.Logf("calls without IDs before=%d after=%d; results without IDs before=%d after=%d", before, after, beforeResults, afterResults)
	if before > 0 && after == before {
		t.Fatal("native history recovered no calls")
	}
}
