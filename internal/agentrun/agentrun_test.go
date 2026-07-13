package agentrun

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
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

func TestModuRunnerSpeaksACPAndAggregatesMessage(t *testing.T) {
	bin := stubACPBinary(t, false)
	runner := NewModuRunner(bin)
	runner.now = fixedClock()

	var events []Event
	result, err := runner.Run(context.Background(), Request{
		Workspace:   t.TempDir(),
		Prompt:      "finish the task",
		Model:       "custom-model",
		Environment: []string{"PATH=" + os.Getenv("PATH")},
	}, collectSink(&events))
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if !result.Succeeded || result.FinalMessage != "custom-model: done" {
		t.Fatalf("result = %+v", result)
	}
	if result.SessionID != "" {
		t.Fatalf("short-lived Modu session must not be exposed, got %q", result.SessionID)
	}
	if countKind(events, KindStarted) != 1 || countKind(events, KindMessage) != 1 || countKind(events, KindResult) != 1 {
		t.Fatalf("events = %+v", events)
	}
}

func TestModuRunnerPrefersCurrentBinary(t *testing.T) {
	directory := t.TempDir()
	for _, name := range []string{moduBinaryLegacy, moduBinaryDefault} {
		if err := os.WriteFile(filepath.Join(directory, name), []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", directory)
	if got := NewModuRunner("").binary; got != filepath.Join(directory, moduBinaryDefault) {
		t.Fatalf("default binary = %q", got)
	}
}

func TestModuRunnerReportsACPError(t *testing.T) {
	runner := NewModuRunner(stubACPBinary(t, true))
	var events []Event
	result, err := runner.Run(context.Background(), Request{Workspace: t.TempDir(), Prompt: "fail"}, collectSink(&events))
	if err == nil || !contains(err.Error(), "provider failed") {
		t.Fatalf("err = %v", err)
	}
	if result.Succeeded || countKind(events, KindError) != 1 {
		t.Fatalf("result/events = %+v / %+v", result, events)
	}
}

func TestModuRunnerPrefersProcessFailureOverUnexpectedEOF(t *testing.T) {
	bin := stubBinary(t, "", "no API key found", 1)
	runner := NewModuRunner(bin)

	_, err := runner.Run(context.Background(), Request{Workspace: t.TempDir(), Prompt: "fail"}, nil)
	if err == nil || !contains(err.Error(), "no API key found") {
		t.Fatalf("err = %v", err)
	}
	if contains(err.Error(), "unexpected EOF") {
		t.Fatalf("unexpected EOF obscured process error: %v", err)
	}
}

func TestRespondACPReversePermissionUsesSandboxDecision(t *testing.T) {
	permission := acpEnvelope{
		JSONRPC: "2.0",
		ID:      int64Pointer(41),
		Method:  "session/request_permission",
		Params: json.RawMessage(`{
			"toolCall":{"toolCallId":"call-1","title":"bash","kind":"execute","arguments":{"command":"go test ./..."}},
			"options":[
				{"optionId":"allow_once","kind":"allow_once"},
				{"optionId":"reject_once","kind":"reject_once"}
			]
		}`),
	}
	tests := []struct {
		name    string
		sandbox Sandbox
		want    string
	}{
		{name: "workspace write allows once", sandbox: SandboxWorkspaceWrite, want: `"optionId":"allow_once"`},
		{name: "full allows once", sandbox: SandboxFull, want: `"optionId":"allow_once"`},
		{name: "read only rejects once", sandbox: SandboxReadOnly, want: `"optionId":"reject_once"`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			var events []Event
			if err := respondACPReverseRequest(&output, permission, test.sandbox, collectSink(&events), fixedClock()); err != nil {
				t.Fatal(err)
			}
			if got := output.String(); !contains(got, `"id":41`) || !contains(got, test.want) {
				t.Fatalf("response = %s", got)
			}
			if countKind(events, KindToolUse) != 1 || countKind(events, KindToolResult) != 1 {
				t.Fatalf("events = %+v", events)
			}
		})
	}
}

func TestRespondACPReversePermissionRejectsUnknownMethod(t *testing.T) {
	id := int64(9)
	err := respondACPReverseRequest(&bytes.Buffer{}, acpEnvelope{ID: &id, Method: "session/unknown"}, SandboxWorkspaceWrite, nil, fixedClock())
	if err == nil || !contains(err.Error(), "unsupported Modu ACP reverse request") {
		t.Fatalf("err = %v", err)
	}
}

func int64Pointer(value int64) *int64 { return &value }

func TestModuRunnerHandlesPermissionReverseRequest(t *testing.T) {
	runner := NewModuRunner(stubACPPermissionBinary(t))
	runner.now = fixedClock()
	var events []Event
	result, err := runner.Run(context.Background(), Request{
		Workspace: t.TempDir(),
		Prompt:    "make the change",
		Sandbox:   SandboxWorkspaceWrite,
	}, collectSink(&events))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Succeeded || result.FinalMessage != "permission accepted" {
		t.Fatalf("result = %+v", result)
	}
	if countKind(events, KindToolUse) != 1 || countKind(events, KindToolResult) != 1 {
		t.Fatalf("events = %+v", events)
	}
}

func stubACPPermissionBinary(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("stub binary uses a POSIX shell script")
	}
	path := filepath.Join(t.TempDir(), "modu_code")
	script := `#!/bin/sh
[ "$1" = "--acp" ] || exit 2
while IFS= read -r line; do
  case "$line" in
    *'"id":1'*) printf '%s\n' '{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":1}}' ;;
    *'"id":2'*) printf '%s\n' '{"jsonrpc":"2.0","id":2,"result":{"sessionId":"modu-sess-1"}}' ;;
    *'"id":3'*)
      printf '%s\n' '{"jsonrpc":"2.0","id":41,"method":"session/request_permission","params":{"toolCall":{"toolCallId":"call-1","title":"bash","kind":"execute","arguments":{"command":"go test ./..."}},"options":[{"optionId":"allow_once","kind":"allow_once"},{"optionId":"reject_once","kind":"reject_once"}]}}'
      IFS= read -r permission
      case "$permission" in
        *'"optionId":"allow_once"'*) ;;
        *) printf '%s\n' '{"jsonrpc":"2.0","id":3,"error":{"code":-32603,"message":"permission response missing"}}'; continue ;;
      esac
      printf '%s\n' '{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"modu-sess-1","update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"permission accepted"}}}}'
      printf '%s\n' '{"jsonrpc":"2.0","id":3,"result":{"stopReason":"end_turn"}}'
      ;;
  esac
done
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func stubACPBinary(t *testing.T, promptError bool) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("stub binary uses a POSIX shell script")
	}
	path := filepath.Join(t.TempDir(), "modu-code")
	promptReply := `printf '%s\n' '{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"modu-sess-1","update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"'"$MODU_CODE_MODEL"': "}}}}'` + "\n" +
		`printf '%s\n' '{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"modu-sess-1","update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"done"}}}}'` + "\n" +
		`printf '%s\n' '{"jsonrpc":"2.0","id":3,"result":{"stopReason":"end_turn"}}'`
	if promptError {
		promptReply = `printf '%s\n' '{"jsonrpc":"2.0","id":3,"error":{"code":-32603,"message":"provider failed"}}'`
	}
	script := `#!/bin/sh
[ "$1" = "--acp" ] || { echo "missing --acp" >&2; exit 2; }
while IFS= read -r line; do
  case "$line" in
    *'"id":1'*) printf '%s\n' '{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":1}}' ;;
    *'"id":2'*) printf '%s\n' '{"jsonrpc":"2.0","id":2,"result":{"sessionId":"modu-sess-1"}}' ;;
    *'"id":3'*) ` + promptReply + ` ;;
  esac
done
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write ACP stub: %v", err)
	}
	return path
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

func TestRunnerDrainsStdoutAfterOversizedLine(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=TestOversizedLineHelperProcess")
	cmd.Env = append(os.Environ(), "ONESHOT_OVERSIZED_LINE_HELPER=1")

	_, err := streamProcess(ctx, cmd, &codexParser{}, fixedClock(), nil)
	if err == nil {
		t.Fatal("expected oversized stdout line to fail")
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		t.Fatalf("runner blocked instead of draining stdout: %v", err)
	}
	if !contains(err.Error(), "read") || !contains(err.Error(), "stdout") {
		t.Fatalf("error %q should identify stdout read failure", err)
	}
}

func TestOversizedLineHelperProcess(t *testing.T) {
	if os.Getenv("ONESHOT_OVERSIZED_LINE_HELPER") != "1" {
		return
	}
	_, _ = os.Stdout.Write(bytes.Repeat([]byte{'x'}, 9*1024*1024))
	os.Exit(0)
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
