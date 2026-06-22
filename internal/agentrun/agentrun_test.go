package agentrun

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// stubBinary writes an executable shell script that prints the given stdout
// payload (and optional stderr), then exits with code. It lets the adapters be
// driven against the exact JSONL the real CLIs emit, with zero network or cost.
func stubBinary(t *testing.T, stdout, stderr string, code int) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("stub binary uses a POSIX shell script")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "stub.sh")
	script := "#!/bin/sh\n"
	if stdout != "" {
		script += "cat <<'ONESHOT_EOF'\n" + stdout + "\nONESHOT_EOF\n"
	}
	if stderr != "" {
		script += "printf '%s' " + shellQuote(stderr) + " 1>&2\n"
	}
	script += "exit " + itoa(code) + "\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	return path
}

func shellQuote(s string) string { return "'" + s + "'" }

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}

// collectSink records every event for assertions.
func collectSink(events *[]Event) Sink {
	return func(e Event) { *events = append(*events, e) }
}

func countKind(events []Event, kind EventKind) int {
	n := 0
	for _, e := range events {
		if e.Kind == kind {
			n++
		}
	}
	return n
}

const codexStream = `{"type":"thread.started","thread_id":"thread-abc"}
{"type":"turn.started"}
{"type":"item.completed","item":{"id":"item_0","type":"reasoning","text":"thinking about it"}}
{"type":"item.completed","item":{"id":"item_1","type":"command_execution","command":"echo hi > out.txt"}}
{"type":"item.completed","item":{"id":"item_2","type":"file_change","path":"out.txt"}}
{"type":"item.completed","item":{"id":"item_3","type":"agent_message","text":"Done. Wrote out.txt."}}
{"type":"turn.completed","usage":{"input_tokens":1200,"output_tokens":42}}`

func TestCodexRunnerParsesStream(t *testing.T) {
	bin := stubBinary(t, codexStream, "", 0)
	r := NewCodexRunner(bin)
	r.now = fixedClock()

	var events []Event
	res, err := r.Run(context.Background(), Request{Workspace: t.TempDir(), Prompt: "go"}, collectSink(&events))
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if !res.Succeeded {
		t.Fatalf("expected success, got %+v", res)
	}
	if res.FinalMessage != "Done. Wrote out.txt." {
		t.Fatalf("FinalMessage = %q", res.FinalMessage)
	}
	if res.SessionID != "thread-abc" {
		t.Fatalf("SessionID = %q", res.SessionID)
	}
	if res.Usage.InputTokens != 1200 || res.Usage.OutputTokens != 42 {
		t.Fatalf("Usage = %+v", res.Usage)
	}
	if got := countKind(events, KindMessage); got != 1 {
		t.Fatalf("message events = %d, want 1", got)
	}
	if got := countKind(events, KindToolUse); got != 1 {
		t.Fatalf("tool_use events = %d, want 1", got)
	}
	if got := countKind(events, KindFileChange); got != 1 {
		t.Fatalf("file_change events = %d, want 1", got)
	}
	if got := countKind(events, KindStarted); got != 1 {
		t.Fatalf("started events = %d, want 1", got)
	}
}

const claudeStream = `{"type":"system","subtype":"init","session_id":"sess-xyz","model":"claude-haiku-4-5"}
{"type":"assistant","message":{"content":[{"type":"thinking","thinking":"plan"}],"usage":{"input_tokens":10,"output_tokens":2}}}
{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Write","input":{"file_path":"out.txt"}}]}}
{"type":"user","message":{"content":[{"type":"tool_result","content":"ok"}]}}
{"type":"assistant","message":{"content":[{"type":"text","text":"Created out.txt."}]}}
{"type":"result","subtype":"success","is_error":false,"result":"Created out.txt.","session_id":"sess-xyz","usage":{"input_tokens":10,"output_tokens":61}}`

func TestClaudeRunnerParsesStream(t *testing.T) {
	bin := stubBinary(t, claudeStream, "", 0)
	r := NewClaudeRunner(bin)
	r.now = fixedClock()

	var events []Event
	res, err := r.Run(context.Background(), Request{Workspace: t.TempDir(), Prompt: "go"}, collectSink(&events))
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if !res.Succeeded {
		t.Fatalf("expected success, got %+v", res)
	}
	if res.FinalMessage != "Created out.txt." {
		t.Fatalf("FinalMessage = %q", res.FinalMessage)
	}
	if res.SessionID != "sess-xyz" {
		t.Fatalf("SessionID = %q", res.SessionID)
	}
	if res.Usage.OutputTokens != 61 {
		t.Fatalf("Usage = %+v", res.Usage)
	}
	if got := countKind(events, KindToolUse); got != 1 {
		t.Fatalf("tool_use events = %d, want 1", got)
	}
	if got := countKind(events, KindResult); got != 1 {
		t.Fatalf("result events = %d, want 1", got)
	}
}

func TestClaudeRunnerReportsAgentFailure(t *testing.T) {
	stream := `{"type":"system","subtype":"init","session_id":"s1"}
{"type":"result","subtype":"error_max_turns","is_error":true,"result":"hit turn limit","session_id":"s1"}`
	bin := stubBinary(t, stream, "", 0)
	r := NewClaudeRunner(bin)
	r.now = fixedClock()

	var events []Event
	res, err := r.Run(context.Background(), Request{Workspace: t.TempDir(), Prompt: "go"}, collectSink(&events))
	if err != nil {
		t.Fatalf("unexpected process error: %v", err)
	}
	if res.Succeeded {
		t.Fatal("expected Succeeded=false for agent-reported error")
	}
	if got := countKind(events, KindError); got != 1 {
		t.Fatalf("error events = %d, want 1", got)
	}
}

func TestRunnerSurfacesProcessFailure(t *testing.T) {
	bin := stubBinary(t, "", "boom: model auth failed", 3)
	r := NewCodexRunner(bin)
	r.now = fixedClock()

	_, err := r.Run(context.Background(), Request{Workspace: t.TempDir(), Prompt: "go"}, nil)
	if err == nil {
		t.Fatal("expected error from non-zero exit")
	}
	if got := err.Error(); got == "" || !contains(got, "boom: model auth failed") {
		t.Fatalf("error %q should include stderr tail", got)
	}
}

func TestEngineUnknownRuntime(t *testing.T) {
	e := NewEngineWithRunners()
	_, err := e.Run(context.Background(), Request{Runtime: RuntimeCodex, Workspace: t.TempDir(), Prompt: "x"}, nil)
	if _, ok := err.(ErrUnknownRuntime); !ok {
		t.Fatalf("err = %v, want ErrUnknownRuntime", err)
	}
}

func TestEngineRoutesByRuntime(t *testing.T) {
	codexBin := stubBinary(t, codexStream, "", 0)
	claudeBin := stubBinary(t, claudeStream, "", 0)
	codex := NewCodexRunner(codexBin)
	codex.now = fixedClock()
	claude := NewClaudeRunner(claudeBin)
	claude.now = fixedClock()
	e := NewEngineWithRunners(codex, claude)

	res, err := e.Run(context.Background(), Request{Runtime: RuntimeClaude, Workspace: t.TempDir(), Prompt: "x"}, nil)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if res.SessionID != "sess-xyz" {
		t.Fatalf("routed to wrong runner: %+v", res)
	}
}

func TestEngineAvailableRuntimes(t *testing.T) {
	codexBin := stubBinary(t, codexStream, "", 0)
	codex := NewCodexRunner(codexBin)
	// claude points at a nonexistent binary -> unavailable.
	claude := NewClaudeRunner(filepath.Join(t.TempDir(), "does-not-exist"))
	e := NewEngineWithRunners(codex, claude)

	got := e.AvailableRuntimes()
	if len(got) != 1 || got[0] != RuntimeCodex {
		t.Fatalf("AvailableRuntimes = %v, want [codex]", got)
	}
}

func fixedClock() nowFunc {
	t := time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC)
	return func() time.Time { return t }
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
