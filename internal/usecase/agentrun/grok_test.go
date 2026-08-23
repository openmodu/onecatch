package agentrun

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/openmodu/onecatch/internal/usecase/agentrun/seam"
)

// grokInitializeResult is excerpted verbatim from a live `grok agent stdio`
// handshake. The handshake needs no credentials, so this is exactly what the
// model probe reads.
const grokInitializeResult = `{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":1,"agentCapabilities":{"loadSession":true,"sessionCapabilities":{"list":{},"resume":{},"close":{}}},"authMethods":[{"id":"grok.com","name":"Grok"}],"_meta":{"agentVersion":"1.0.5","modelState":{"currentModelId":"grok-4.6","availableModels":[{"modelId":"grok-4.6","name":"Grok 4.6","description":"SpaceXAI's latest frontier model","_meta":{"supportsReasoningEffort":true,"reasoningEfforts":[{"id":"xhigh","value":"xhigh","label":"Extra High Effort","default":false},{"id":"high","value":"high","label":"High Effort","default":true},{"id":"medium","value":"medium","label":"Medium Effort","default":false},{"id":"low","value":"low","label":"Low Effort","default":false}]}},{"modelId":"grok-4.5","name":"Grok 4.5","_meta":{"supportsReasoningEffort":true,"reasoningEfforts":[{"id":"high","value":"high","label":"High Effort","default":true},{"id":"low","value":"low","label":"Low Effort","default":false}]}}]}}}}`

// acpStub builds a stdio agent that answers the three handshake requests in
// order and streams promptLines while answering the prompt. It is the protocol
// half of a harness, so the client can be exercised without one installed.
func acpStub(t *testing.T, initializeResult, sessionResult, promptResult string, promptLines []string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("stub agent uses a POSIX shell script")
	}
	var streamed strings.Builder
	for _, line := range promptLines {
		streamed.WriteString("        printf '%s\\n' " + shellQuote(line) + "\n")
	}
	script := `#!/bin/sh
n=0
while IFS= read -r line; do
  n=$((n+1))
  case $n in
    1) printf '%s\n' ` + shellQuote(initializeResult) + ` ;;
    2) printf '%s\n' ` + shellQuote(sessionResult) + ` ;;
    3)
` + streamed.String() + `        printf '%s\n' ` + shellQuote(promptResult) + `
       ;;
  esac
done
`
	path := filepath.Join(t.TempDir(), "acp-stub.sh")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	return path
}

// grokUpdates is one successful turn's notification stream: thinking, prose, a
// tool call that touches a file, and its completion. Field names follow the
// session/update frames captured from grok agent stdio.
var grokUpdates = []string{
	`{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"s-1","update":{"sessionUpdate":"agent_thought_chunk","content":{"type":"text","text":"Planning."}}}}`,
	`{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"s-1","update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"Adding "}}}}`,
	`{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"s-1","update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"the file."}}}}`,
	`{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"s-1","update":{"sessionUpdate":"tool_call","toolCallId":"t-1","title":"Write SUMMARY.md","kind":"edit","status":"pending","rawInput":{"path":"SUMMARY.md"},"locations":[{"path":"SUMMARY.md"}]}}}`,
	`{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"s-1","update":{"sessionUpdate":"tool_call_update","toolCallId":"t-1","status":"in_progress"}}}`,
	`{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"s-1","update":{"sessionUpdate":"tool_call_update","toolCallId":"t-1","status":"completed"},"_meta":{"eventId":"s-1-6","usage":{"inputTokens":900,"outputTokens":120,"cachedInputTokens":400}}}}`,
	// A vendor extension notification: ignored, never surfaced as an event.
	`{"jsonrpc":"2.0","method":"_x.ai/queue/changed","params":{"sessionId":"s-1","entries":[]}}`,
}

func runGrokStub(t *testing.T, req Request, promptResult string, updates []string) ([]Event, Result, error) {
	t.Helper()
	runner := NewGrokRunner(acpStub(t, grokInitializeResult,
		`{"jsonrpc":"2.0","id":2,"result":{"sessionId":"s-1"}}`, promptResult, updates))
	runner.now = fixedNow()
	if req.Workspace == "" {
		req.Workspace = t.TempDir()
	}
	var events []Event
	result, err := runner.Run(context.Background(), req, func(event Event) { events = append(events, event) })
	return events, result, err
}

func TestGrokRunnerNormalizesACPUpdates(t *testing.T) {
	events, result, err := runGrokStub(t, Request{Prompt: "write a summary"},
		`{"jsonrpc":"2.0","id":3,"result":{"stopReason":"end_turn"}}`, grokUpdates)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !result.Succeeded {
		t.Fatal("end_turn must report success")
	}
	if result.SessionID != "s-1" {
		t.Fatalf("session id = %q", result.SessionID)
	}
	if result.FinalMessage != "Adding the file." {
		t.Fatalf("final message = %q", result.FinalMessage)
	}
	want := Usage{InputTokens: 900, OutputTokens: 120, CachedInputTokens: 400}
	if result.Usage != want {
		t.Fatalf("usage = %+v, want %+v", result.Usage, want)
	}

	kinds := map[EventKind]int{}
	var reasoning, fileChange string
	for _, event := range events {
		kinds[event.Kind]++
		switch event.Kind {
		case KindReasoning:
			reasoning += event.Text
		case KindFileChange:
			fileChange = event.Text
		}
	}
	if kinds[KindStarted] != 1 || kinds[KindResult] != 1 {
		t.Fatalf("expected one started and one result event, got %v", kinds)
	}
	if kinds[KindToolUse] != 1 {
		t.Fatalf("tool uses = %d, want 1", kinds[KindToolUse])
	}
	// Only the settled update is a result; the in_progress one would otherwise
	// duplicate the entry.
	if kinds[KindToolResult] != 1 {
		t.Fatalf("tool results = %d, want 1", kinds[KindToolResult])
	}
	if reasoning != "Planning." {
		t.Fatalf("reasoning = %q", reasoning)
	}
	if fileChange != "SUMMARY.md" {
		t.Fatalf("file change = %q", fileChange)
	}
}

func TestGrokRunnerFailsOnNonTerminalStopReason(t *testing.T) {
	events, result, err := runGrokStub(t, Request{Prompt: "write a summary"},
		`{"jsonrpc":"2.0","id":3,"result":{"stopReason":"refusal"}}`, nil)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if result.Succeeded {
		t.Fatal("a refused turn must not report success")
	}
	var sawError bool
	for _, event := range events {
		if event.Kind == KindError {
			sawError = true
		}
	}
	if !sawError {
		t.Fatal("a non-terminal stop reason must surface an error event")
	}
}

func TestGrokRunnerSurfacesPromptError(t *testing.T) {
	_, result, err := runGrokStub(t, Request{Prompt: "write a summary"},
		`{"jsonrpc":"2.0","id":3,"error":{"code":-32603,"message":"Internal error","data":{"message":"API error (status 400): Incorrect API key provided.","http_status":400}}}`, nil)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if result.Succeeded {
		t.Fatal("a JSON-RPC error response must not report success")
	}
}

func TestGrokRunnerReadsUsageFromPromptResponse(t *testing.T) {
	events, result, err := runGrokStub(t, Request{Prompt: "hello"},
		`{"jsonrpc":"2.0","id":3,"result":{"stopReason":"end_turn","_meta":{"sessionId":"s-1","totalTokens":18017,"inputTokens":17965,"outputTokens":51,"cachedReadTokens":5888,"reasoningTokens":38,"usage":{"inputTokens":17965,"outputTokens":51,"totalTokens":18016,"cachedReadTokens":5888,"cacheCreationTokens":128,"reasoningTokens":38,"modelCalls":1}}}}`, nil)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	want := Usage{
		InputTokens: 17965, CachedInputTokens: 5888, CacheCreationInputTokens: 128,
		OutputTokens: 51, ReasoningOutputTokens: 38,
	}
	if result.Usage != want {
		t.Fatalf("usage = %+v, want %+v", result.Usage, want)
	}
	var usageIndex, resultIndex = -1, -1
	for index, event := range events {
		switch event.Kind {
		case KindUsage:
			usageIndex = index
			if event.Usage == nil || *event.Usage != want {
				t.Fatalf("usage event = %+v, want %+v", event.Usage, want)
			}
		case KindResult:
			resultIndex = index
			if event.Usage == nil || *event.Usage != want {
				t.Fatalf("result event usage = %+v, want %+v", event.Usage, want)
			}
		}
	}
	if usageIndex < 0 || resultIndex < 0 || usageIndex >= resultIndex {
		t.Fatalf("usage/result event order = %d/%d", usageIndex, resultIndex)
	}
}

// TestGrokRunnerDeniesToolsWithoutAHandler pins the read-only contract: with no
// host to ask, an approval request must be refused rather than waved through.
func TestGrokRunnerDeniesToolsWithoutAHandler(t *testing.T) {
	permission := `{"jsonrpc":"2.0","id":7,"method":"session/request_permission","params":{"sessionId":"s-1","toolCall":{"toolCallId":"t-1","title":"Delete everything","kind":"execute"},"options":[{"optionId":"yes","name":"Allow","kind":"allow_once"},{"optionId":"no","name":"Reject","kind":"reject_once"}]}}`

	for _, testCase := range []struct {
		name    string
		sandbox Sandbox
		want    string
	}{
		{"read-only denies", SandboxReadOnly, "no"},
		{"workspace-write auto-approves", SandboxWorkspaceWrite, "yes"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			// The stub echoes what the client wrote back to it, so the chosen
			// option id can be observed in the transcript.
			dir := t.TempDir()
			transcript := filepath.Join(dir, "client.jsonl")
			path := filepath.Join(dir, "acp-stub.sh")
			script := `#!/bin/sh
n=0
while IFS= read -r line; do
  n=$((n+1))
  printf '%s\n' "$line" >> ` + shellQuote(transcript) + `
  case $n in
    1) printf '%s\n' ` + shellQuote(grokInitializeResult) + ` ;;
    2) printf '%s\n' ` + shellQuote(`{"jsonrpc":"2.0","id":2,"result":{"sessionId":"s-1"}}`) + ` ;;
    3) printf '%s\n' ` + shellQuote(permission) + ` ;;
    4) printf '%s\n' ` + shellQuote(`{"jsonrpc":"2.0","id":3,"result":{"stopReason":"end_turn"}}`) + ` ;;
  esac
done
`
			if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
				t.Fatalf("write stub: %v", err)
			}
			runner := NewGrokRunner(path)
			runner.now = fixedNow()
			if _, err := runner.Run(context.Background(), Request{
				Prompt: "clean up", Workspace: t.TempDir(), Sandbox: testCase.sandbox,
			}, nil); err != nil {
				t.Fatalf("run: %v", err)
			}
			written, err := os.ReadFile(transcript)
			if err != nil {
				t.Fatalf("read transcript: %v", err)
			}
			if !strings.Contains(string(written), `"optionId":"`+testCase.want+`"`) {
				t.Fatalf("expected optionId %q in client transcript:\n%s", testCase.want, written)
			}
		})
	}
}

func TestGrokSandboxMapping(t *testing.T) {
	for _, testCase := range []struct {
		sandbox Sandbox
		want    string
	}{
		{SandboxReadOnly, "read-only"},
		{SandboxWorkspaceWrite, "workspace"},
		{"", "workspace"},
		{SandboxFull, "none"},
	} {
		command, err := grokCommand(Request{Sandbox: testCase.sandbox})
		if err != nil {
			t.Fatalf("sandbox %q: %v", testCase.sandbox, err)
		}
		if !containsArgs(command.environment, "GROK_SANDBOX="+testCase.want) {
			t.Fatalf("sandbox %q produced %v, want GROK_SANDBOX=%s", testCase.sandbox, command.environment, testCase.want)
		}
	}
	// An unmapped sandbox must fail the run rather than silently start Grok
	// with whatever profile happened to be configured.
	if _, err := grokCommand(Request{Sandbox: Sandbox("invented")}); err == nil {
		t.Fatal("an unknown sandbox must be rejected")
	}
}

func TestGrokRunnerRoutesACPClientOperationsToRemote(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("stub agent uses a POSIX shell script")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	workspace := filepath.Join(dir, "workspace")
	for _, directory := range []string{target, workspace} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv(seam.DirEnv, filepath.Join(dir, "sessions"))

	transcript := filepath.Join(dir, "client.jsonl")
	stub := filepath.Join(dir, "acp-remote-stub.sh")
	targetJSON, _ := json.Marshal(target)
	writePathJSON, _ := json.Marshal(filepath.Join(target, "nested", "written.txt"))
	script := `#!/bin/sh
n=0
while IFS= read -r line; do
  n=$((n+1))
  printf '%s\n' "$line" >> ` + shellQuote(transcript) + `
  case $n in
    1) printf '%s\n' ` + shellQuote(grokInitializeResult) + ` ;;
    2) printf '%s\n' ` + shellQuote(`{"jsonrpc":"2.0","id":2,"result":{"sessionId":"s-remote"}}`) + ` ;;
    3) printf '%s\n' ` + shellQuote(`{"jsonrpc":"2.0","id":10,"method":"fs/write_text_file","params":{"sessionId":"s-remote","path":`+string(writePathJSON)+`,"content":"remote-data"}}`) + ` ;;
    4) printf '%s\n' ` + shellQuote(`{"jsonrpc":"2.0","id":11,"method":"fs/read_text_file","params":{"sessionId":"s-remote","path":`+string(writePathJSON)+`}}`) + ` ;;
    5) printf '%s\n' ` + shellQuote(`{"jsonrpc":"2.0","id":12,"method":"terminal/create","params":{"sessionId":"s-remote","command":"sh","args":["-c","printf terminal-ok > terminal.txt"],"cwd":`+string(targetJSON)+`}}`) + ` ;;
    6) printf '%s\n' ` + shellQuote(`{"jsonrpc":"2.0","id":13,"method":"terminal/wait_for_exit","params":{"sessionId":"s-remote","terminalId":"onecatch-terminal-1"}}`) + ` ;;
    7) printf '%s\n' ` + shellQuote(`{"jsonrpc":"2.0","id":14,"method":"terminal/release","params":{"sessionId":"s-remote","terminalId":"onecatch-terminal-1"}}`) + ` ;;
    8) printf '%s\n' ` + shellQuote(`{"jsonrpc":"2.0","id":3,"result":{"stopReason":"end_turn"}}`) + ` ;;
  esac
done
`
	if err := os.WriteFile(stub, []byte(script), 0o755); err != nil {
		t.Fatalf("write stub: %v", err)
	}

	runner := NewGrokRunner(stub)
	_, err := runner.Run(context.Background(), Request{
		Prompt: "inspect and edit", Workspace: workspace, Sandbox: SandboxWorkspaceWrite,
		Remote: &seam.Target{Root: target},
	}, nil)
	if err != nil {
		t.Fatalf("run remote Grok: %v", err)
	}
	for name, want := range map[string]string{
		filepath.Join(target, "nested", "written.txt"): "remote-data",
		filepath.Join(target, "terminal.txt"):          "terminal-ok",
	} {
		got, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("read remote result %s: %v", name, err)
		}
		if string(got) != want {
			t.Fatalf("remote result %s = %q, want %q", name, got, want)
		}
	}
	written, err := os.ReadFile(transcript)
	if err != nil {
		t.Fatalf("read transcript: %v", err)
	}
	conversation := string(written)
	for _, want := range []string{
		`"readTextFile":true`, `"writeTextFile":true`, `"terminal":true`,
		`"cwd":` + string(targetJSON), `"content":"remote-data"`,
		`Remote workspace: ` + target,
	} {
		if !strings.Contains(conversation, want) {
			t.Fatalf("remote ACP transcript does not contain %q:\n%s", want, conversation)
		}
	}
}

func TestGrokRunnerRejectsReadOnlyRemote(t *testing.T) {
	runner := NewGrokRunner(acpStub(t, grokInitializeResult, "", "", nil))
	_, err := runner.Run(context.Background(), Request{
		Prompt: "hi", Workspace: t.TempDir(), Sandbox: SandboxReadOnly,
		Remote: &seam.Target{Root: "/srv/project"},
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "workspace-write") {
		t.Fatalf("expected a read-only remote rejection, got %v", err)
	}
}

func TestACPRemotePathStaysInsideWorkspace(t *testing.T) {
	for _, outside := range []string{"../secret", "/srv/application/secret", "/etc/passwd"} {
		if _, err := acpRemotePath("/srv/app", outside); err == nil {
			t.Fatalf("outside path %q was accepted", outside)
		}
	}
	relative, err := acpRemotePath("/srv/app", "/srv/app/nested/file.go")
	if err != nil || relative != "nested/file.go" {
		t.Fatalf("workspace path mapped to %q, %v", relative, err)
	}
}

func TestParseGrokConfiguration(t *testing.T) {
	var envelope acpEnvelope
	if err := json.Unmarshal([]byte(grokInitializeResult), &envelope); err != nil {
		t.Fatalf("decode handshake: %v", err)
	}
	configuration := parseGrokConfiguration(envelope.Result)
	if len(configuration.Models) != 2 {
		t.Fatalf("models = %+v", configuration.Models)
	}
	if configuration.Models[0].Model != "grok-4.6" || configuration.Models[0].DisplayName != "Grok 4.6" {
		t.Fatalf("first model = %+v", configuration.Models[0])
	}
	if configuration.Model != "grok-4.6" {
		t.Fatalf("current model = %q", configuration.Model)
	}
	// Effort levels belong to the model, not to the harness: 4.6 offers xhigh
	// and 4.5 does not, so one flattened list would offer 4.5 a level it
	// rejects.
	if strings.Join(configuration.Models[0].Efforts, ",") != "xhigh,high,medium,low" {
		t.Fatalf("grok-4.6 efforts = %v", configuration.Models[0].Efforts)
	}
	if strings.Join(configuration.Models[1].Efforts, ",") != "high,low" {
		t.Fatalf("grok-4.5 efforts = %v", configuration.Models[1].Efforts)
	}
	if configuration.Models[0].DefaultEffort != "high" || configuration.Models[1].DefaultEffort != "high" {
		t.Fatalf("default efforts = %q / %q", configuration.Models[0].DefaultEffort, configuration.Models[1].DefaultEffort)
	}
}

func TestACPUsageIgnoresMetaWithoutTokens(t *testing.T) {
	if _, ok := acpUsage(nil); ok {
		t.Fatal("absent _meta must report no usage")
	}
	if _, ok := acpUsage([]byte(`{"eventId":"abc"}`)); ok {
		t.Fatal("_meta without a usage object must report no usage")
	}
	if _, ok := acpUsage([]byte(`{"usage":{"inputTokens":0,"outputTokens":0}}`)); ok {
		t.Fatal("an all-zero usage object must not overwrite real accounting")
	}
	usage, ok := acpUsage([]byte(`{"usage":{"promptTokens":10,"completionTokens":4}}`))
	if !ok || usage.InputTokens != 10 || usage.OutputTokens != 4 {
		t.Fatalf("alternate field spellings not read: %+v (ok=%v)", usage, ok)
	}
	grok, ok := acpUsage([]byte(`{"usage":{"inputTokens":17965,"outputTokens":51,"cachedReadTokens":5888,"cacheCreationTokens":128,"reasoningTokens":38}}`))
	want := Usage{InputTokens: 17965, CachedInputTokens: 5888, CacheCreationInputTokens: 128, OutputTokens: 51, ReasoningOutputTokens: 38}
	if !ok || grok != want {
		t.Fatalf("Grok usage = %+v, want %+v (ok=%v)", grok, want, ok)
	}
}

// TestGrokRunnerRoutesPermissionToTheHost covers the path the desktop approval
// card uses: the request surfaces as an event, the host's decision picks the
// matching ACP option, and the resolution is recorded.
func TestGrokRunnerRoutesPermissionToTheHost(t *testing.T) {
	permission := `{"jsonrpc":"2.0","id":7,"method":"session/request_permission","params":{"sessionId":"s-1","toolCall":{"toolCallId":"t-1","title":"Run tests","kind":"execute","rawInput":{"command":"go test ./..."}},"options":[{"optionId":"once","name":"Allow once","kind":"allow_once"},{"optionId":"always","name":"Always allow","kind":"allow_always"},{"optionId":"no","name":"Reject","kind":"reject_once"}]}}`

	for _, testCase := range []struct {
		name     string
		decision PermissionDecision
		wantID   string
	}{
		{"allow once", PermissionDecision{Behavior: "allow", DecisionClassification: "user_temporary"}, "once"},
		{"allow always", PermissionDecision{Behavior: "allow", DecisionClassification: "user_permanent"}, "always"},
		{"deny", PermissionDecision{Behavior: "deny"}, "no"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			dir := t.TempDir()
			transcript := filepath.Join(dir, "client.jsonl")
			path := filepath.Join(dir, "acp-stub.sh")
			script := `#!/bin/sh
n=0
while IFS= read -r line; do
  n=$((n+1))
  printf '%s\n' "$line" >> ` + shellQuote(transcript) + `
  case $n in
    1) printf '%s\n' ` + shellQuote(grokInitializeResult) + ` ;;
    2) printf '%s\n' ` + shellQuote(`{"jsonrpc":"2.0","id":2,"result":{"sessionId":"s-1"}}`) + ` ;;
    3) printf '%s\n' ` + shellQuote(permission) + ` ;;
    4) printf '%s\n' ` + shellQuote(`{"jsonrpc":"2.0","id":3,"result":{"stopReason":"end_turn"}}`) + ` ;;
  esac
done
`
			if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
				t.Fatalf("write stub: %v", err)
			}
			runner := NewGrokRunner(path)
			runner.now = fixedNow()

			var asked PermissionRequest
			var events []Event
			if _, err := runner.Run(context.Background(), Request{
				Prompt: "run the tests", Workspace: t.TempDir(), Sandbox: SandboxWorkspaceWrite,
				PermissionHandler: func(_ context.Context, request PermissionRequest) (PermissionDecision, error) {
					asked = request
					return testCase.decision, nil
				},
			}, func(event Event) { events = append(events, event) }); err != nil {
				t.Fatalf("run: %v", err)
			}

			if asked.ToolName != "Run tests" || asked.ToolUseID != "t-1" {
				t.Fatalf("permission request = %+v", asked)
			}
			// The card renders the tool's input, so rawInput has to survive.
			if asked.Input["command"] != "go test ./..." {
				t.Fatalf("tool input not carried through: %+v", asked.Input)
			}
			// This flag is Claude's "answer it in Claude's own UI" escape hatch;
			// the card hides its buttons when it is set, and an ACP request is
			// answerable right here.
			if asked.RequiresUserInteraction {
				t.Fatal("an ACP permission request must be answerable from the card")
			}
			// The agent offered allow_always, so the card may show it.
			if asked.SuppressAlwaysAllow {
				t.Fatal("always-allow was offered by the agent but suppressed")
			}

			written, err := os.ReadFile(transcript)
			if err != nil {
				t.Fatalf("read transcript: %v", err)
			}
			if !strings.Contains(string(written), `"optionId":"`+testCase.wantID+`"`) {
				t.Fatalf("expected optionId %q in client transcript:\n%s", testCase.wantID, written)
			}

			var requested, resolved int
			for _, event := range events {
				switch event.Kind {
				case KindPermissionRequest:
					requested++
				case KindPermissionResolved:
					resolved++
					if event.PermissionDecision != testCase.decision.Behavior {
						t.Fatalf("resolved decision = %q, want %q", event.PermissionDecision, testCase.decision.Behavior)
					}
				}
			}
			if requested != 1 || resolved != 1 {
				t.Fatalf("permission events: requested=%d resolved=%d, want 1 and 1", requested, resolved)
			}
		})
	}
}

// An agent that offers no always-allow option must not have one shown, since
// ACP carries no rule payload OneCatch could persist on its behalf.
func TestGrokSuppressesAlwaysAllowWhenNotOffered(t *testing.T) {
	permission := `{"jsonrpc":"2.0","id":7,"method":"session/request_permission","params":{"sessionId":"s-1","toolCall":{"toolCallId":"t-1","title":"Run tests","kind":"execute"},"options":[{"optionId":"once","name":"Allow once","kind":"allow_once"},{"optionId":"no","name":"Reject","kind":"reject_once"}]}}`
	dir := t.TempDir()
	path := filepath.Join(dir, "acp-stub.sh")
	script := `#!/bin/sh
n=0
while IFS= read -r line; do
  n=$((n+1))
  case $n in
    1) printf '%s\n' ` + shellQuote(grokInitializeResult) + ` ;;
    2) printf '%s\n' ` + shellQuote(`{"jsonrpc":"2.0","id":2,"result":{"sessionId":"s-1"}}`) + ` ;;
    3) printf '%s\n' ` + shellQuote(permission) + ` ;;
    4) printf '%s\n' ` + shellQuote(`{"jsonrpc":"2.0","id":3,"result":{"stopReason":"end_turn"}}`) + ` ;;
  esac
done
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	runner := NewGrokRunner(path)
	runner.now = fixedNow()
	var asked PermissionRequest
	if _, err := runner.Run(context.Background(), Request{
		Prompt: "run the tests", Workspace: t.TempDir(), Sandbox: SandboxWorkspaceWrite,
		PermissionHandler: func(_ context.Context, request PermissionRequest) (PermissionDecision, error) {
			asked = request
			return PermissionDecision{Behavior: "allow"}, nil
		},
	}, nil); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !asked.SuppressAlwaysAllow {
		t.Fatal("always-allow must be suppressed when the agent did not offer it")
	}
}

// TestGrokLaunchIsAcceptedByTheRealBinary is the test the stub suite cannot be.
//
// A shell stub accepts any argv, so every stub-based test passed while the real
// binary rejected the invocation with "unexpected argument '--sandbox'": model
// and effort belong to the `agent` command, not to its `stdio` subcommand, and
// the sandbox is not a flag on that path at all. Grok's ACP handshake needs no
// credentials and spends no quota, so the argument vector can be checked against
// the real CLI whenever it happens to be installed.
func TestGrokLaunchIsAcceptedByTheRealBinary(t *testing.T) {
	runner := NewGrokRunner("")
	if !runner.Available() {
		t.Skip("grok CLI not installed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	configuration, err := runner.InspectConfiguration(ctx, t.TempDir(), os.Environ())
	if err != nil {
		t.Fatalf("the real grok binary rejected our launch: %v", err)
	}
	if len(configuration.Models) == 0 {
		t.Fatal("the handshake reported no models")
	}
}

func TestGrokCommandPlacesFlagsBeforeTheSubcommand(t *testing.T) {
	command, err := grokCommand(Request{Sandbox: SandboxReadOnly, Model: "grok-4.6", ReasoningEffort: "xhigh"})
	if err != nil {
		t.Fatalf("command: %v", err)
	}
	// `grok agent stdio` itself takes only debug and socket options; model and
	// effort are the parent command's and must precede the subcommand.
	want := []string{"agent", "--model", "grok-4.6", "--reasoning-effort", "xhigh", "stdio"}
	if strings.Join(command.args, " ") != strings.Join(want, " ") {
		t.Fatalf("args = %v, want %v", command.args, want)
	}
	// The sandbox has no flag on this path, so it travels as the environment
	// variable Grok documents as its equivalent.
	if !containsArgs(command.environment, "GROK_SANDBOX=read-only") {
		t.Fatalf("sandbox not applied to the environment: %v", command.environment)
	}
	if containsArgs(command.args, "--sandbox") {
		t.Fatal("--sandbox is not accepted by `grok agent stdio` and must not be passed")
	}

	plain, err := grokCommand(Request{Sandbox: SandboxWorkspaceWrite})
	if err != nil {
		t.Fatalf("command: %v", err)
	}
	if strings.Join(plain.args, " ") != "agent stdio" {
		t.Fatalf("an unconfigured run should launch plainly, got %v", plain.args)
	}
	if !containsArgs(plain.environment, "GROK_SANDBOX=workspace") {
		t.Fatalf("sandbox = %v", plain.environment)
	}
}
