package agentrun

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
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
		script += "cat <<'ONECATCH_EOF'\n" + stdout + "\nONECATCH_EOF\n"
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

// shellQuote renders a POSIX single-quoted word. A single quote cannot appear
// inside such a word, so each one is closed, escaped, and reopened — without
// this, fixture text as ordinary as "SpaceXAI's" ends the quote early and
// leaves the stub script unparsable.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

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
	for _, event := range events {
		if event.Kind == KindUsage && (event.Usage == nil || event.Usage.InputTokens != 17 || event.Usage.CachedInputTokens != 11) {
			t.Fatalf("live usage event = %+v", event)
		}
	}
}

func TestCodexRunnerReusesAppServerForConversationFollowUp(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("stub binary uses a POSIX shell script")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "codex")
	processes := filepath.Join(dir, "processes.txt")
	requests := filepath.Join(dir, "requests.jsonl")
	script := `#!/bin/sh
printf '%s\n' started >> "$ONECATCH_CODEX_PROCESSES"
turn=0
while IFS= read -r line; do
  printf '%s\n' "$line" >> "$ONECATCH_CODEX_REQUESTS"
  case "$line" in
    *'"id":1'*) printf '%s\n' '{"id":1,"result":{}}' ;;
    *'"method":"skills/list"'*) printf '%s\n' '{"id":2,"result":{"data":[{"cwd":"REPLACE_CWD","skills":[],"errors":[]}]}}' ;;
    *'"id":3'*) printf '%s\n' '{"id":3,"result":{"thread":{"id":"thread-warm"}}}' ;;
    *'"id":4'*)
      turn=$((turn + 1))
      printf '{"id":4,"result":{"turn":{"id":"turn-%s","status":"inProgress"}}}\n' "$turn"
      printf '{"method":"item/started","params":{"threadId":"thread-warm","turnId":"turn-%s","item":{"id":"message-%s","type":"agentMessage","text":""}}}\n' "$turn" "$turn"
      printf '{"method":"item/completed","params":{"threadId":"thread-warm","turnId":"turn-%s","item":{"id":"message-%s","type":"agentMessage","text":"Hello %s"}}}\n' "$turn" "$turn" "$turn"
      printf '{"method":"turn/completed","params":{"threadId":"thread-warm","turn":{"id":"turn-%s","status":"completed"}}}\n' "$turn"
      ;;
  esac
done
`
	script = strings.ReplaceAll(script, "REPLACE_CWD", dir)
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	runner := NewCodexRunner(bin)
	defer runner.Close()
	environment := append(os.Environ(), "ONECATCH_CODEX_PROCESSES="+processes, "ONECATCH_CODEX_REQUESTS="+requests)
	request := Request{RunID: "run-warm", Workspace: dir, Prompt: "first", Sandbox: SandboxWorkspaceWrite, Environment: environment}
	first, err := runner.Run(context.Background(), request, nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Prompt = "second"
	request.ResumeSessionID = first.SessionID
	second, err := runner.Run(context.Background(), request, nil)
	if err != nil {
		t.Fatal(err)
	}
	if first.FinalMessage != "Hello 1" || second.FinalMessage != "Hello 2" {
		t.Fatalf("results = %+v / %+v", first, second)
	}
	started, err := os.ReadFile(processes)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(started), "started\n"); got != 1 {
		t.Fatalf("Codex app-server starts = %d, want 1", got)
	}
	payload, err := os.ReadFile(requests)
	if err != nil {
		t.Fatal(err)
	}
	value := string(payload)
	if strings.Count(value, `"method":"initialize"`) != 1 || strings.Count(value, `"method":"thread/start"`) != 1 || strings.Count(value, `"method":"turn/start"`) != 2 {
		t.Fatalf("unexpected app-server requests: %s", value)
	}
	if strings.Contains(value, `"method":"thread/resume"`) {
		t.Fatalf("warm follow-up unnecessarily resumed the loaded thread: %s", value)
	}

	// A changed environment must never inherit the warm process. Resume the
	// durable thread through a fresh app-server instead.
	request.Prompt = "third"
	request.Environment = append(slices.Clone(environment), "ONECATCH_ENVIRONMENT_REVISION=2")
	third, err := runner.Run(context.Background(), request, nil)
	if err != nil {
		t.Fatal(err)
	}
	if third.FinalMessage != "Hello 1" {
		t.Fatalf("fresh-process result = %+v", third)
	}
	started, err = os.ReadFile(processes)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(started), "started\n"); got != 2 {
		t.Fatalf("Codex app-server starts after environment change = %d, want 2", got)
	}
	payload, err = os.ReadFile(requests)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(payload), `"method":"thread/resume"`) {
		t.Fatalf("fresh app-server did not resume the durable thread: %s", payload)
	}
}

func TestCodexRunnerPassesModelSettingsToAppServer(t *testing.T) {
	bin := stubCodexAppServerBinary(t)
	capture := filepath.Join(t.TempDir(), "requests.jsonl")
	runner := NewCodexRunner(bin)
	request := Request{
		Workspace: t.TempDir(), Prompt: "$git-commit implement it", Sandbox: SandboxWorkspaceWrite,
		Model: "gpt-test", ReasoningEffort: "high", ServiceTier: "priority",
		Environment: append(os.Environ(), "ONECATCH_CODEX_CAPTURE="+capture),
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
	if !strings.Contains(value, `{"name":"git-commit","path":"/skills/git-commit/SKILL.md","type":"skill"}`) {
		t.Fatalf("turn/start did not include the referenced Skill: %s", value)
	}

	standardCapture := filepath.Join(t.TempDir(), "standard.jsonl")
	request.ServiceTier = "standard"
	request.Environment = append(os.Environ(), "ONECATCH_CODEX_CAPTURE="+standardCapture)
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
printf '%s\n' "$*" >> "$ONECATCH_CODEX_CAPTURE"
echo 'app-server unavailable' >&2
exit 1
`
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := NewCodexRunner(bin).Run(context.Background(), Request{
		Workspace:   dir,
		Prompt:      "must not run through exec",
		Environment: append(os.Environ(), "ONECATCH_CODEX_CAPTURE="+capture),
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

func TestCodexRunnerListsEnabledSkills(t *testing.T) {
	bin := stubCodexAppServerBinary(t)
	skills, err := NewCodexRunner(bin).ListSkills(context.Background(), t.TempDir(), os.Environ())
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 1 {
		t.Fatalf("skills = %+v", skills)
	}
	if skills[0].Name != "git-commit" || skills[0].DisplayName != "Git Commit" || skills[0].ShortDescription != "Create a clean commit" || skills[0].Path != "/skills/git-commit/SKILL.md" {
		t.Fatalf("skill = %+v", skills[0])
	}
}

func TestReferencedSkillsRequireWhitespaceBoundaryAndDeduplicate(t *testing.T) {
	skills := []Skill{{Name: "git-commit", Path: "/skills/git-commit/SKILL.md"}}
	got := referencedSkills("price$git-commit $git-commit then $git-commit", skills)
	if len(got) != 1 || got[0].Name != "git-commit" {
		t.Fatalf("referenced skills = %+v", got)
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
[ -n "$ONECATCH_CODEX_ARGV" ] && printf '%s\n' "$*" > "$ONECATCH_CODEX_ARGV"
while IFS= read -r line; do
  [ -n "$ONECATCH_CODEX_CAPTURE" ] && printf '%s\n' "$line" >> "$ONECATCH_CODEX_CAPTURE"
  case "$line" in
    *'"id":1'*)
      printf '%s\n' '{"id":1,"result":{}}'
      ;;
    *'"method":"skills/list"'*)
      printf '%s\n' '{"id":2,"result":{"data":[{"cwd":"REPLACE_CWD","skills":[{"name":"git-commit","description":"Commit changes","path":"/skills/git-commit/SKILL.md","scope":"user","enabled":true,"interface":{"displayName":"Git Commit","shortDescription":"Create a clean commit"}}],"errors":[]}]}}'
      ;;
    *'"id":3'*)
      printf '%s\n' '{"id":3,"result":{"thread":{"id":"thread-live"}}}'
      ;;
    *'"id":4'*)
      printf '%s\n' '{"id":4,"result":{"turn":{"id":"turn-live","status":"inProgress"}}}'
      printf '%s\n' '{"method":"item/started","params":{"threadId":"thread-live","turnId":"turn-live","item":{"id":"message-1","type":"agentMessage","text":""}}}'
      printf '%s\n' '{"method":"item/agentMessage/delta","params":{"threadId":"thread-live","turnId":"turn-live","itemId":"message-1","delta":"Hel"}}'
      printf '%s\n' '{"method":"item/agentMessage/delta","params":{"threadId":"thread-live","turnId":"turn-live","itemId":"message-1","delta":"lo"}}'
      printf '%s\n' '{"method":"item/completed","params":{"threadId":"thread-live","turnId":"turn-live","item":{"id":"message-1","type":"agentMessage","text":"Hello"}}}'
      printf '%s\n' '{"method":"item/started","params":{"threadId":"thread-live","turnId":"turn-live","item":{"id":"command-1","type":"commandExecution","command":"go test ./...","status":"inProgress"}}}'
      printf '%s\n' '{"method":"item/commandExecution/outputDelta","params":{"threadId":"thread-live","turnId":"turn-live","itemId":"command-1","delta":"ok\n"}}'
      printf '%s\n' '{"method":"item/completed","params":{"threadId":"thread-live","turnId":"turn-live","item":{"id":"command-1","type":"commandExecution","command":"go test ./...","status":"completed","aggregatedOutput":"ok\n","exitCode":0}}}'
      printf '%s\n' '{"method":"thread/tokenUsage/updated","params":{"threadId":"thread-live","turnId":"turn-live","tokenUsage":{"modelContextWindow":272000,"last":{"inputTokens":3,"cachedInputTokens":2,"outputTokens":1,"reasoningOutputTokens":0},"total":{"inputTokens":17,"cachedInputTokens":11,"outputTokens":5,"reasoningOutputTokens":2}}}}'
      printf '%s\n' '{"method":"turn/completed","params":{"threadId":"thread-live","turn":{"id":"turn-live","status":"completed"}}}'
      ;;
  esac
done
`
	script = strings.ReplaceAll(script, "REPLACE_CWD", filepath.Dir(path))
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

func TestClaudeRunnerInspectsModelOptions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("stub binary uses a POSIX shell script")
	}
	path := filepath.Join(t.TempDir(), "claude")
	script := `#!/bin/sh
[ "$1" = "--help" ] || { echo "expected --help" >&2; exit 2; }
cat <<'ONECATCH_EOF'
Options:
  --effort <level>  Effort level for the current session (low, medium, high, xhigh, max)
  --model <model>  Model for the current session. Provide an alias for the latest
                   model (e.g. 'fable', 'opus', or 'sonnet') or a model's full
                   name (e.g. 'claude-fable-5').
  -n, --name <name>  Session name
ONECATCH_EOF
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	configuration, err := NewClaudeRunner(path).InspectConfiguration(context.Background(), t.TempDir(), os.Environ())
	if err != nil {
		t.Fatal(err)
	}
	// The four the help text advertises, plus the pinned catalog that --help
	// declines to enumerate. Counting rather than listing keeps this failing if
	// a pinned name ever duplicates one the help already offered.
	if len(configuration.Models) != 4+len(claudeCatalogModels) {
		t.Fatalf("models = %+v", configuration.Models)
	}
	if got := configuration.Models[0]; got.Model != "fable" || got.DisplayName != "Fable" || !got.Alias {
		t.Fatalf("first model = %+v", got)
	}
	if got := configuration.Models[3]; got.Model != "claude-fable-5" || got.Alias {
		t.Fatalf("full model = %+v", got)
	}
	if got := strings.Join(configuration.Efforts, ","); got != "low,medium,high,xhigh,max" {
		t.Fatalf("efforts = %q", got)
	}
}

func TestClaudeRunnerPassesSelectedModel(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("stub binary uses a POSIX shell script")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "claude")
	capture := filepath.Join(dir, "args.txt")
	script := `#!/bin/sh
printf '%s\n' "$@" > "$ONECATCH_CLAUDE_CAPTURE"
cat <<'ONECATCH_EOF'
` + claudeStream + `
ONECATCH_EOF
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := NewClaudeRunner(path).Run(context.Background(), Request{
		Workspace: dir, Prompt: "$a-stock-data go", Model: "opus", ReasoningEffort: "high",
		Environment: append(os.Environ(), "ONECATCH_CLAUDE_CAPTURE="+capture),
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := os.ReadFile(capture)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(payload), "--model\nopus\n") {
		t.Fatalf("Claude args = %q", payload)
	}
	if !strings.Contains(string(payload), "--effort\nhigh\n") {
		t.Fatalf("Claude args = %q", payload)
	}
	if !strings.Contains(string(payload), "-p\n/a-stock-data go\n") || strings.Contains(string(payload), "$a-stock-data") {
		t.Fatalf("Claude Skill prompt was not adapted = %q", payload)
	}
}

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

func TestClaudeRunnerRoundTripsInteractivePermission(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("stub binary uses a POSIX shell script")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "claude")
	capture := filepath.Join(dir, "control.jsonl")
	script := `#!/bin/sh
IFS= read -r prompt
printf '%s\n' '{"type":"system","subtype":"init","session_id":"permission-session"}'
printf '%s\n' '{"type":"control_request","request_id":"permission-1","request":{"subtype":"can_use_tool","tool_name":"WebFetch","input":{"url":"https://v3.wails.io/guides/mobile/"},"permission_suggestions":[{"type":"addRules","rules":[{"toolName":"WebFetch","ruleContent":"domain:v3.wails.io"}],"behavior":"allow","destination":"session"}],"title":"Fetch v3.wails.io","display_name":"Fetch URL","tool_use_id":"tool-1"}}'
IFS= read -r response
printf '%s\n%s\n' "$prompt" "$response" > "$ONECATCH_CLAUDE_CAPTURE"
printf '%s\n' '{"type":"result","subtype":"success","is_error":false,"result":"done","session_id":"permission-session","usage":{"input_tokens":2,"output_tokens":1}}'
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	var events []Event
	result, err := NewClaudeRunner(path).Run(context.Background(), Request{
		Workspace: dir, Prompt: "$openai-docs research Wails mobile", Sandbox: SandboxReadOnly,
		Environment: append(os.Environ(), "ONECATCH_CLAUDE_CAPTURE="+capture),
		PermissionHandler: func(_ context.Context, request PermissionRequest) (PermissionDecision, error) {
			if request.ID != "permission-1" || request.ToolName != "WebFetch" || request.Input["url"] != "https://v3.wails.io/guides/mobile/" {
				t.Fatalf("permission request = %+v", request)
			}
			return PermissionDecision{Behavior: "allow", DecisionClassification: "user_temporary"}, nil
		},
	}, collectSink(&events))
	if err != nil || !result.Succeeded {
		t.Fatalf("Run = %+v, %v", result, err)
	}
	payload, err := os.ReadFile(capture)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(payload)), "\n")
	if len(lines) != 2 || !strings.Contains(lines[0], `"text":"/openai-docs research Wails mobile"`) || !strings.Contains(lines[1], `"request_id":"permission-1"`) || !strings.Contains(lines[1], `"behavior":"allow"`) {
		t.Fatalf("control exchange = %q", payload)
	}
	if countKind(events, KindPermissionRequest) != 1 || countKind(events, KindPermissionResolved) != 1 {
		t.Fatalf("permission events = %+v", events)
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

// Occupancy and cost come from different halves of the same notification:
// `total` is every call in the turn, `last` is the one prompt the window held.
// Reading `total` into the gauge would show 17 of a window that held 3.
func TestCodexRunnerSeparatesContextOccupancyFromCumulativeUsage(t *testing.T) {
	runner := NewCodexRunner(stubCodexAppServerBinary(t))
	runner.now = fixedClock()
	var events []Event

	result, err := runner.Run(context.Background(), Request{Workspace: t.TempDir(), Prompt: "go", Sandbox: SandboxWorkspaceWrite}, collectSink(&events))
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if result.Usage.InputTokens != 17 {
		t.Fatalf("cumulative usage must stay the turn total: %+v", result.Usage)
	}
	if result.Context.Window != 272000 || result.Context.Tokens != 3 {
		t.Fatalf("context = %+v", result.Context)
	}
	if !result.Context.Known() {
		t.Fatal("context should be reportable")
	}
	for _, event := range events {
		if event.Kind != KindUsage {
			continue
		}
		if event.Context == nil || event.Context.Window != 272000 || event.Context.Tokens != 3 {
			t.Fatalf("live context event = %+v", event.Context)
		}
	}
}

// `claude -p` reports the window only on the terminal result event, while
// occupancy has to be read from each assistant message — the run would
// otherwise finish before the desktop learned either number.
func TestClaudeRunnerReportsContextWindowAndLiveOccupancy(t *testing.T) {
	stream := `{"type":"system","subtype":"init","session_id":"s-ctx"}
{"type":"assistant","message":{"content":[{"type":"text","text":"hi"}],"usage":{"input_tokens":2,"cache_creation_input_tokens":26217,"cache_read_input_tokens":0,"output_tokens":4}}}
{"type":"result","subtype":"success","is_error":false,"result":"done","session_id":"s-ctx","usage":{"input_tokens":2,"cache_creation_input_tokens":26217,"cache_read_input_tokens":0,"output_tokens":4},"modelUsage":{"claude-opus-5":{"contextWindow":1000000,"maxOutputTokens":64000}}}`
	runner := NewClaudeRunner(stubBinary(t, stream, "", 0))
	runner.now = fixedClock()
	var events []Event

	result, err := runner.Run(context.Background(), Request{Workspace: t.TempDir(), Prompt: "go"}, collectSink(&events))
	if err != nil {
		t.Fatal(err)
	}
	if result.Context.Window != 1000000 {
		t.Fatalf("context window = %d", result.Context.Window)
	}
	// A cache read still occupies the window, so occupancy is the whole prompt
	// (2+26217), not the 2 tokens that were freshly billed as input.
	if result.Context.Tokens != 26219 {
		t.Fatalf("context tokens = %d", result.Context.Tokens)
	}
	var live []Event
	for _, event := range events {
		if event.Kind == KindUsage {
			live = append(live, event)
		}
	}
	if len(live) != 1 {
		t.Fatalf("expected one live usage event, got %d", len(live))
	}
	if live[0].Context == nil || live[0].Context.Tokens != 26219 {
		t.Fatalf("live occupancy = %+v", live[0].Context)
	}
}

// `claude --help` documents --model with examples, not a catalog, so parsing
// it alone left every pinned version unreachable from the picker.
func TestCodexMaxContextWindowOverrideOnlyForModelsWithHeadroom(t *testing.T) {
	// Codex defaults every model to 272000; only some accept more, and
	// app-server never reports either figure.
	if override, ok := codexMaxContextWindowOverride("gpt-5.6-sol"); !ok || override != "model_context_window=872000" {
		t.Fatalf("sol = %q %v", override, ok)
	}
	if override, ok := codexMaxContextWindowOverride("gpt-5.4"); !ok || override != "model_context_window=1000000" {
		t.Fatalf("5.4 = %q %v", override, ok)
	}
	// Raising these would exceed the model's real limit and error every
	// request, so they must stay at Codex's default rather than be guessed at.
	for _, model := range []string{"gpt-5.5", "gpt-5.4-mini", "gpt-5.2", "some-future-model", ""} {
		if override, ok := codexMaxContextWindowOverride(model); ok {
			t.Fatalf("model %q must not be overridden, got %q", model, override)
		}
	}
}

func TestCodexRunnerRaisesTheContextWindowOnlyWhenAsked(t *testing.T) {
	launched := func(t *testing.T, req Request) string {
		t.Helper()
		bin := stubCodexAppServerBinary(t)
		argv := filepath.Join(t.TempDir(), "argv.txt")
		req.Workspace = t.TempDir()
		req.Environment = append(os.Environ(), "ONECATCH_CODEX_ARGV="+argv)
		runner := NewCodexRunner(bin)
		runner.now = fixedClock()
		if _, err := runner.Run(context.Background(), req, nil); err != nil {
			t.Fatal(err)
		}
		recorded, err := os.ReadFile(argv)
		if err != nil {
			t.Fatal(err)
		}
		return string(recorded)
	}
	base := Request{Prompt: "go", Model: "gpt-5.6-sol", Sandbox: SandboxWorkspaceWrite}

	if off := launched(t, base); strings.Contains(off, "model_context_window") {
		t.Fatalf("window must stay at the harness default unless asked: %s", off)
	}
	on := base
	on.MaxContextWindow = true
	if got := launched(t, on); !strings.Contains(got, "-c model_context_window=872000") {
		t.Fatalf("window override missing: %s", got)
	}
	// A model with no headroom is left alone even when the setting is on:
	// raising it would exceed the real limit and error every request.
	capped := base
	capped.MaxContextWindow = true
	capped.Model = "gpt-5.5"
	if got := launched(t, capped); strings.Contains(got, "model_context_window") {
		t.Fatalf("capped model must not be overridden: %s", got)
	}
}

func TestClaudeConfigurationOffersPinnedModelsBesideTheAdvertisedAliases(t *testing.T) {
	models := parseClaudeModelOptions(`
Options:
  --model <model>                       Model for the current session. Provide
                                        an alias for the latest model (e.g.
                                        'fable', 'opus', or 'sonnet') or a
                                        model's full name (e.g.
                                        'claude-fable-5').
  --effort <level>                      Effort level for the current session
                                        (low, medium, high, xhigh, max)
`)
	byName := make(map[string]ClaudeModelInfo, len(models))
	for _, model := range models {
		byName[model.Model] = model
	}
	// The advertised aliases lead and stay marked as aliases: they track the
	// newest model without anyone editing the pinned list.
	for _, alias := range []string{"fable", "opus", "sonnet"} {
		if info, ok := byName[alias]; !ok || !info.Alias {
			t.Fatalf("alias %q missing or not marked: %+v", alias, info)
		}
	}
	for _, pinned := range []string{"claude-opus-5", "claude-opus-4-8", "claude-opus-4-7", "claude-opus-4-6", "claude-sonnet-5", "claude-sonnet-4-6", "claude-haiku-4-5"} {
		info, ok := byName[pinned]
		if !ok {
			t.Fatalf("pinned model %q missing from %+v", pinned, models)
		}
		if info.Alias {
			t.Fatalf("pinned model %q must not be marked an alias", pinned)
		}
	}
	// claude-fable-5 appears in both the help text and the pinned list; it must
	// not be offered twice.
	count := 0
	for _, model := range models {
		if model.Model == "claude-fable-5" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("claude-fable-5 appears %d times", count)
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
{"type":"message_end","message":{"role":"assistant","content":[{"type":"thinking","thinking":"checking"},{"type":"text","text":"I will update it."}],"usage":{"input":12,"output":4,"cacheRead":30,"cacheWrite":2}}}
{"type":"tool_execution_start","toolName":"bash","toolCallId":"tool-1","args":{"command":"go test ./..."}}
{"type":"tool_execution_end","toolName":"bash","toolCallId":"tool-1","result":{"content":[{"type":"text","text":"ok"}]},"isError":false}
{"type":"message_end","message":{"role":"toolResult","content":[{"type":"text","text":"ok"}]}}
{"type":"message_end","message":{"role":"assistant","content":[{"type":"text","text":"Done."}],"usage":{"input":20,"output":6,"cacheRead":50}}}
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
	if result.Usage.InputTokens != 114 || result.Usage.CachedInputTokens != 80 || result.Usage.CacheCreationInputTokens != 2 || result.Usage.OutputTokens != 10 {
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

func TestModuEnvironmentAppliesProvider(t *testing.T) {
	environment := moduEnvironment([]string{"PATH=/bin", "MODU_CODE_PROVIDER=openai"}, "custom-model", "anthropic")
	if !slices.Contains(environment, "MODU_CODE_MODEL=custom-model") || !slices.Contains(environment, "MODU_CODE_PROVIDER=anthropic") {
		t.Fatalf("environment = %#v", environment)
	}
	environment = moduEnvironment(environment, "", "auto")
	if slices.Contains(environment, "MODU_CODE_PROVIDER=anthropic") {
		t.Fatalf("auto provider did not clear override: %#v", environment)
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
cat <<'ONECATCH_EOF'
` + output + `
ONECATCH_EOF
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
	cmd.Env = append(os.Environ(), "ONECATCH_OVERSIZED_LINE_HELPER=1")

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
	if os.Getenv("ONECATCH_OVERSIZED_LINE_HELPER") != "1" {
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
