package agentrun

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
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

func TestCodexRunnerStreamsAppServerMessagesAndCommandOutput(t *testing.T) {
	bin := stubCodexAppServerBinary(t)
	runner := NewCodexRunner(bin)
	runner.now = fixedClock()
	var events []Event

	result, err := runner.Run(context.Background(), Request{Workspace: t.TempDir(), Prompt: "implement it", Sandbox: SandboxWorkspaceWrite}, collectSink(&events))
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if !result.Succeeded || result.SessionID != "thread-live" || result.FinalMessage != "Hello" {
		t.Fatalf("result = %+v", result)
	}
	if result.Usage.InputTokens != 17 || result.Usage.CachedInputTokens != 11 || result.Usage.OutputTokens != 5 || result.Usage.ReasoningOutputTokens != 2 {
		t.Fatalf("usage = %+v", result.Usage)
	}

	var messages, outputs []Event
	for _, event := range events {
		switch event.StreamID {
		case "codex-message-message-1":
			messages = append(messages, event)
		case "codex-tool-output-command-1":
			outputs = append(outputs, event)
		}
	}
	if len(messages) != 4 || messages[0].Phase != StreamStart || messages[1].Text != "Hel" || messages[2].Text != "lo" || messages[3].Phase != StreamEnd || messages[3].Text != "Hello" {
		t.Fatalf("message stream = %+v", messages)
	}
	if len(outputs) != 3 || outputs[0].Phase != StreamStart || outputs[1].Text != "ok\n" || outputs[2].Phase != StreamEnd || outputs[2].Text != "ok\n" {
		t.Fatalf("command output stream = %+v", outputs)
	}
	if countKind(events, KindToolUse) != 1 || countKind(events, KindUsage) != 1 {
		t.Fatalf("events = %+v", events)
	}
}

func TestCodexRunnerPassesModelSettingsToAppServer(t *testing.T) {
	bin := stubCodexAppServerBinary(t)
	capture := filepath.Join(t.TempDir(), "requests.jsonl")
	runner := NewCodexRunner(bin)
	request := Request{
		Workspace: t.TempDir(), Prompt: "implement it", Sandbox: SandboxWorkspaceWrite,
		Model: "gpt-test", ReasoningEffort: "high", ServiceTier: "priority",
		Environment: append(os.Environ(), "ONESHOT_CODEX_CAPTURE="+capture),
	}
	if _, err := runner.Run(context.Background(), request, nil); err != nil {
		t.Fatal(err)
	}
	payload, err := os.ReadFile(capture)
	if err != nil {
		t.Fatal(err)
	}
	value := string(payload)
	for _, expected := range []string{`"method":"thread/start"`, `"model":"gpt-test"`, `"serviceTier":"priority"`, `"method":"turn/start"`, `"effort":"high"`} {
		if !strings.Contains(value, expected) {
			t.Fatalf("app-server requests missing %s: %s", expected, value)
		}
	}

	standardCapture := filepath.Join(t.TempDir(), "standard.jsonl")
	request.ServiceTier = "standard"
	request.Environment = append(os.Environ(), "ONESHOT_CODEX_CAPTURE="+standardCapture)
	if _, err := runner.Run(context.Background(), request, nil); err != nil {
		t.Fatal(err)
	}
	standardPayload, err := os.ReadFile(standardCapture)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(standardPayload), `"serviceTier":null`) {
		t.Fatalf("standard tier was not reset: %s", standardPayload)
	}
}

func TestCodexRunnerDoesNotFallbackWhenAppServerIsUnavailable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("stub binary uses a POSIX shell script")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "codex")
	capture := filepath.Join(dir, "invocations.txt")
	script := `#!/bin/sh
printf '%s\n' "$*" >> "$ONESHOT_CODEX_CAPTURE"
echo 'app-server unavailable' >&2
exit 1
`
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := NewCodexRunner(bin).Run(context.Background(), Request{
		Workspace:   dir,
		Prompt:      "must not run through exec",
		Environment: append(os.Environ(), "ONESHOT_CODEX_CAPTURE="+capture),
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "app-server unavailable") {
		t.Fatalf("error = %v", err)
	}
	payload, readErr := os.ReadFile(capture)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if got := strings.TrimSpace(string(payload)); got != "app-server --listen stdio://" {
		t.Fatalf("Codex invocations = %q", got)
	}
}

func TestCodexRunnerInspectsConfiguration(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("stub binary uses a POSIX shell script")
	}
	bin := filepath.Join(t.TempDir(), "codex")
	script := `#!/bin/sh
while IFS= read -r line; do
  case "$line" in
    *'"method":"initialize"'*) printf '%s\n' '{"id":1,"result":{}}' ;;
    *'"method":"config/read"'*) printf '%s\n' '{"id":2,"result":{"config":{"model":"gpt-test","model_reasoning_effort":"high","service_tier":"priority"},"origins":{}}}' ;;
    *'"method":"model/list"'*) printf '%s\n' '{"id":3,"result":{"data":[{"id":"gpt-test","model":"gpt-test","displayName":"GPT Test","description":"Test model","hidden":false,"supportedReasoningEfforts":[{"reasoningEffort":"low","description":"Fast"},{"reasoningEffort":"high","description":"Deep"}],"defaultReasoningEffort":"low","serviceTiers":[{"id":"priority","name":"Priority","description":"Faster"}],"defaultServiceTier":null,"isDefault":true}]}}' ;;
  esac
done
`
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	configuration, err := NewCodexRunner(bin).InspectConfiguration(context.Background(), t.TempDir(), os.Environ())
	if err != nil {
		t.Fatal(err)
	}
	if configuration.Model != "gpt-test" || configuration.ReasoningEffort != "high" || configuration.ServiceTier != "priority" {
		t.Fatalf("configuration = %+v", configuration)
	}
	if len(configuration.Models) != 1 || configuration.Models[0].DisplayName != "GPT Test" || len(configuration.Models[0].ReasoningEfforts) != 2 || len(configuration.Models[0].ServiceTiers) != 1 {
		t.Fatalf("models = %+v", configuration.Models)
	}
}

func stubCodexAppServerBinary(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("stub binary uses a POSIX shell script")
	}
	path := filepath.Join(t.TempDir(), "codex")
	script := `#!/bin/sh
[ "$1" = "app-server" ] || { echo "expected app-server" >&2; exit 9; }
while IFS= read -r line; do
  [ -n "$ONESHOT_CODEX_CAPTURE" ] && printf '%s\n' "$line" >> "$ONESHOT_CODEX_CAPTURE"
  case "$line" in
    *'"id":1'*)
      printf '%s\n' '{"id":1,"result":{}}'
      ;;
    *'"id":2'*)
      printf '%s\n' '{"id":2,"result":{"thread":{"id":"thread-live"}}}'
      ;;
    *'"id":3'*)
      printf '%s\n' '{"id":3,"result":{"turn":{"id":"turn-live","status":"inProgress"}}}'
      printf '%s\n' '{"method":"item/started","params":{"threadId":"thread-live","turnId":"turn-live","item":{"id":"message-1","type":"agentMessage","text":""}}}'
      printf '%s\n' '{"method":"item/agentMessage/delta","params":{"threadId":"thread-live","turnId":"turn-live","itemId":"message-1","delta":"Hel"}}'
      printf '%s\n' '{"method":"item/agentMessage/delta","params":{"threadId":"thread-live","turnId":"turn-live","itemId":"message-1","delta":"lo"}}'
      printf '%s\n' '{"method":"item/completed","params":{"threadId":"thread-live","turnId":"turn-live","item":{"id":"message-1","type":"agentMessage","text":"Hello"}}}'
      printf '%s\n' '{"method":"item/started","params":{"threadId":"thread-live","turnId":"turn-live","item":{"id":"command-1","type":"commandExecution","command":"go test ./...","status":"inProgress"}}}'
      printf '%s\n' '{"method":"item/commandExecution/outputDelta","params":{"threadId":"thread-live","turnId":"turn-live","itemId":"command-1","delta":"ok\n"}}'
      printf '%s\n' '{"method":"item/completed","params":{"threadId":"thread-live","turnId":"turn-live","item":{"id":"command-1","type":"commandExecution","command":"go test ./...","status":"completed","aggregatedOutput":"ok\n","exitCode":0}}}'
      printf '%s\n' '{"method":"thread/tokenUsage/updated","params":{"threadId":"thread-live","turnId":"turn-live","tokenUsage":{"last":{"inputTokens":3,"cachedInputTokens":2,"outputTokens":1,"reasoningOutputTokens":0},"total":{"inputTokens":17,"cachedInputTokens":11,"outputTokens":5,"reasoningOutputTokens":2}}}}'
      printf '%s\n' '{"method":"turn/completed","params":{"threadId":"thread-live","turn":{"id":"turn-live","status":"completed"}}}'
      ;;
  esac
done
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write Codex app-server stub: %v", err)
	}
	return path
}

const claudeStream = `{"type":"system","subtype":"init","session_id":"sess-xyz","model":"claude-haiku-4-5"}
{"type":"assistant","message":{"content":[{"type":"thinking","thinking":"plan"}],"usage":{"input_tokens":10,"output_tokens":2}}}
{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Write","input":{"file_path":"out.txt"}}]}}
{"type":"user","message":{"content":[{"type":"tool_result","content":"ok"}]}}
{"type":"stream_event","event":{"type":"message_start","message":{"id":"msg-1"}}}
{"type":"stream_event","event":{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Created "}}}
{"type":"stream_event","event":{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"out.txt."}}}
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

func TestClaudeRunnerEmitsTextDeltasWithAuthoritativeEnd(t *testing.T) {
	bin := stubBinary(t, claudeStream, "", 0)
	r := NewClaudeRunner(bin)
	var events []Event
	if _, err := r.Run(context.Background(), Request{Workspace: t.TempDir(), Prompt: "go"}, collectSink(&events)); err != nil {
		t.Fatal(err)
	}
	var streamed []Event
	for _, event := range events {
		if strings.HasPrefix(event.StreamID, "claude-message-") {
			streamed = append(streamed, event)
		}
	}
	if len(streamed) != 4 || streamed[0].Phase != StreamStart || streamed[1].Text != "Created " || streamed[2].Text != "out.txt." || streamed[3].Phase != StreamEnd || streamed[3].Text != "Created out.txt." {
		t.Fatalf("streamed = %+v", streamed)
	}
}

func TestClaudeRunnerIncludesCacheUsageInInputTotal(t *testing.T) {
	stream := `{"type":"system","subtype":"init","session_id":"s-cache"}
{"type":"result","subtype":"success","is_error":false,"result":"done","session_id":"s-cache","usage":{"input_tokens":7,"cache_creation_input_tokens":13,"cache_read_input_tokens":80,"output_tokens":9}}`
	runner := NewClaudeRunner(stubBinary(t, stream, "", 0))
	result, err := runner.Run(context.Background(), Request{Workspace: t.TempDir(), Prompt: "go"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Usage.InputTokens != 100 || result.Usage.CachedInputTokens != 80 || result.Usage.CacheCreationInputTokens != 13 || result.Usage.OutputTokens != 9 {
		t.Fatalf("usage = %+v", result.Usage)
	}
}

func TestClaudeRunnerMarksOnlyTheFailingToolResult(t *testing.T) {
	stream := `{"type":"system","subtype":"init","session_id":"s1"}
{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Bash","input":{"command":"git status"}}]}}
{"type":"user","message":{"content":[{"type":"tool_result","content":"clean","is_error":false}]}}
{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Read","input":{"file_path":"missing.js"}}]}}
{"type":"user","message":{"content":[{"type":"tool_result","content":"File does not exist.","is_error":true}]}}
{"type":"result","subtype":"error_max_turns","is_error":true,"result":"hit turn limit","session_id":"s1"}`
	bin := stubBinary(t, stream, "", 0)
	r := NewClaudeRunner(bin)
	r.now = fixedClock()

	var events []Event
	if _, err := r.Run(context.Background(), Request{Workspace: t.TempDir(), Prompt: "go"}, collectSink(&events)); err != nil {
		t.Fatalf("unexpected process error: %v", err)
	}
	var results []Event
	for _, event := range events {
		if event.Kind == KindToolResult {
			results = append(results, event)
		}
	}
	if len(results) != 2 {
		t.Fatalf("tool_result events = %d, want 2", len(results))
	}
	// The run ends on error_max_turns, but the first tool still succeeded — the
	// step's fate must not be stamped onto the calls it made.
	if results[0].Failed {
		t.Errorf("succeeding tool_result marked failed: %+v", results[0])
	}
	if !results[1].Failed {
		t.Errorf("failing tool_result not marked failed: %+v", results[1])
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

const moduStream = `{"type":"session_start","sessionId":"modu-sess-1","model":"custom-model"}
{"type":"agent_start"}
{"type":"turn_start"}
{"type":"message_start","message":{"role":"assistant"}}
{"type":"message_update","streamEvent":{"Type":"thinking_delta","Delta":"checking"},"message":"checking"}
{"type":"message_update","streamEvent":{"Type":"text_delta","ContentIndex":1,"Delta":"I will "},"message":"I will "}
{"type":"message_update","streamEvent":{"Type":"text_delta","ContentIndex":1,"Delta":"update it."},"message":"update it."}
{"type":"message_end","message":{"role":"assistant","content":[{"type":"thinking","thinking":"checking"},{"type":"text","text":"I will update it."}],"usage":{"input":12,"output":4}}}
{"type":"tool_execution_start","toolName":"bash","toolCallId":"tool-1","args":{"command":"go test ./..."}}
{"type":"tool_execution_end","toolName":"bash","toolCallId":"tool-1","result":{"content":[{"type":"text","text":"ok"}]},"isError":false}
{"type":"message_end","message":{"role":"toolResult","content":[{"type":"text","text":"ok"}]}}
{"type":"message_end","message":{"role":"assistant","content":[{"type":"text","text":"Done."}],"usage":{"input":20,"output":6}}}
{"type":"turn_end"}
{"type":"agent_end"}
{"type":"session_end"}`

func TestModuRunnerUsesPrintModeAndParsesStream(t *testing.T) {
	bin := stubModuPrintBinary(t, moduStream)
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
	if !result.Succeeded || result.FinalMessage != "Done." {
		t.Fatalf("result = %+v", result)
	}
	if result.SessionID != "modu-sess-1" {
		t.Fatalf("SessionID = %q", result.SessionID)
	}
	if result.Usage.InputTokens != 32 || result.Usage.OutputTokens != 10 {
		t.Fatalf("Usage = %+v", result.Usage)
	}
	if countKind(events, KindStarted) != 1 || countKind(events, KindMessage) != 5 || countKind(events, KindToolUse) != 1 || countKind(events, KindToolResult) != 1 || countKind(events, KindResult) != 1 {
		t.Fatalf("events = %+v", events)
	}
}

func TestModuRunnerEmitsTextDeltasWithAuthoritativeEnd(t *testing.T) {
	bin := stubModuPrintBinary(t, moduStream)
	runner := NewModuRunner(bin)
	var events []Event
	if _, err := runner.Run(context.Background(), Request{Workspace: t.TempDir(), Prompt: "finish the task"}, collectSink(&events)); err != nil {
		t.Fatal(err)
	}
	var streamed []Event
	for _, event := range events {
		if event.StreamID == "modu-message-1" {
			streamed = append(streamed, event)
		}
	}
	if len(streamed) != 4 || streamed[0].Phase != StreamStart || streamed[1].Text != "I will " || streamed[2].Text != "update it." || streamed[3].Phase != StreamEnd || streamed[3].Text != "I will update it." {
		t.Fatalf("streamed = %+v", streamed)
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

func TestModuCommandArgsUsePrintModeAndResume(t *testing.T) {
	tests := []struct {
		name string
		req  Request
		want string
	}{
		{name: "default writable", req: Request{Prompt: "fix tests"}, want: "-p fix tests -json --no-approve"},
		{name: "workspace write", req: Request{Prompt: "fix tests", Sandbox: SandboxWorkspaceWrite}, want: "-p fix tests -json --no-approve"},
		{name: "full", req: Request{Prompt: "fix tests", Sandbox: SandboxFull}, want: "-p fix tests -json --no-approve"},
		{name: "read only", req: Request{Prompt: "inspect", Sandbox: SandboxReadOnly}, want: "-p inspect -json"},
		{name: "resume", req: Request{Prompt: "continue", ResumeSessionID: " session-42 "}, want: "--resume session-42 -p continue -json --no-approve"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := strings.Join(moduCommandArgs(test.req), " "); got != test.want {
				t.Fatalf("args = %q, want %q", got, test.want)
			}
		})
	}
}

func TestModuRunnerReportsAgentFailure(t *testing.T) {
	stream := `{"type":"session_start","sessionId":"modu-sess-1","model":"custom-model"}
{"type":"message_end","message":{"role":"assistant","errorMessage":"provider failed"}}
{"type":"session_end"}`
	runner := NewModuRunner(stubBinary(t, stream, "", 0))
	var events []Event
	result, err := runner.Run(context.Background(), Request{Workspace: t.TempDir(), Prompt: "fail"}, collectSink(&events))
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if result.Succeeded || countKind(events, KindError) != 1 || result.SessionID != "modu-sess-1" {
		t.Fatalf("result/events = %+v / %+v", result, events)
	}
}

func TestModuRunnerReportsProcessFailure(t *testing.T) {
	bin := stubBinary(t, "", "no API key found", 1)
	runner := NewModuRunner(bin)

	_, err := runner.Run(context.Background(), Request{Workspace: t.TempDir(), Prompt: "fail"}, nil)
	if err == nil || !contains(err.Error(), "no API key found") {
		t.Fatalf("err = %v", err)
	}
}

func stubModuPrintBinary(t *testing.T, output string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("stub binary uses a POSIX shell script")
	}
	path := filepath.Join(t.TempDir(), "modu_code")
	script := `#!/bin/sh
[ "$1" = "-p" ] || { echo "missing -p" >&2; exit 2; }
[ "$2" = "finish the task" ] || { echo "wrong prompt" >&2; exit 2; }
[ "$3" = "-json" ] || { echo "missing -json" >&2; exit 2; }
[ "$4" = "--no-approve" ] || { echo "missing --no-approve" >&2; exit 2; }
cat <<'ONESHOT_EOF'
` + output + `
ONESHOT_EOF
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write Modu print stub: %v", err)
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

	_, err := streamProcess(ctx, cmd, &claudeParser{}, fixedClock(), nil)
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
	codexBin := stubBinary(t, "", "", 0)
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
	codexBin := stubBinary(t, "", "", 0)
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
