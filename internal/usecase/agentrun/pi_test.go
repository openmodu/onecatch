package agentrun

import (
	"context"
	"strings"
	"testing"
	"time"
)

// piErrorStream is a verbatim capture of `pi -p --mode json` against a rejected
// API key. It pins the session header and the shape of a turn that ends in an
// error message rather than in text content.
const piErrorStream = `{"type":"session","version":3,"id":"01a02d5d-8fe0-741d-a3fe-b572eeadddc6","timestamp":"2026-08-23T06:45:01.537Z","cwd":"/tmp/pi-probe"}
{"type":"agent_start"}
{"type":"turn_start"}
{"type":"message_start","message":{"role":"user","content":[{"type":"text","text":"say hi"}],"timestamp":1787467501554}}
{"type":"message_end","message":{"role":"user","content":[{"type":"text","text":"say hi"}],"timestamp":1787467501554}}
{"type":"message_start","message":{"role":"assistant","content":[],"api":"openai-responses","provider":"openai","model":"gpt-4o-mini","usage":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0,"totalTokens":0,"cost":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0,"total":0}},"stopReason":"error","timestamp":1787467501584,"errorMessage":"401 Incorrect API key provided."}}
{"type":"message_end","message":{"role":"assistant","content":[],"api":"openai-responses","provider":"openai","model":"gpt-4o-mini","usage":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0,"totalTokens":0,"cost":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0,"total":0}},"stopReason":"error","timestamp":1787467501584,"errorMessage":"401 Incorrect API key provided."}}
{"type":"turn_end","message":{"role":"assistant","content":[],"api":"openai-responses","provider":"openai","model":"gpt-4o-mini","usage":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0,"totalTokens":0,"cost":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0,"total":0}},"stopReason":"error","timestamp":1787467501584,"errorMessage":"401 Incorrect API key provided."},"toolResults":[]}
{"type":"agent_end","messages":[]}`

// piToolStream is a successful run: streamed thinking and text, one bash call,
// one file write, and a settled assistant message carrying usage. It follows
// pi's published AgentEvent and AssistantMessage types; the envelope and
// session header are the ones observed in piErrorStream.
const piToolStream = `{"type":"session","version":3,"id":"01a02d60-1111-2222-3333-444455556666","timestamp":"2026-08-23T06:50:00.000Z","cwd":"/tmp/pi-probe"}
{"type":"agent_start"}
{"type":"turn_start"}
{"type":"message_start","message":{"role":"assistant","content":[],"usage":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0,"totalTokens":0},"stopReason":"toolUse","timestamp":1787467800000}}
{"type":"message_update","message":{"role":"assistant","content":[]},"assistantMessageEvent":{"type":"thinking_delta","contentIndex":0,"delta":"Checking the repo"}}
{"type":"message_update","message":{"role":"assistant","content":[]},"assistantMessageEvent":{"type":"text_delta","contentIndex":1,"delta":"Listing "}}
{"type":"message_update","message":{"role":"assistant","content":[]},"assistantMessageEvent":{"type":"text_delta","contentIndex":1,"delta":"files."}}
{"type":"tool_execution_start","toolCallId":"call_1","toolName":"bash","args":{"command":"ls -la"}}
{"type":"tool_execution_end","toolCallId":"call_1","toolName":"bash","result":{"output":"README.md"},"isError":false}
{"type":"tool_execution_start","toolCallId":"call_2","toolName":"write","args":{"path":"SUMMARY.md","content":"done"}}
{"type":"tool_execution_end","toolCallId":"call_2","toolName":"write","result":{},"isError":false}
{"type":"tool_execution_start","toolCallId":"call_3","toolName":"read","args":{"path":"README.md"}}
{"type":"tool_execution_end","toolCallId":"call_3","toolName":"read","result":{},"isError":true}
{"type":"message_end","message":{"role":"assistant","content":[{"type":"text","text":"Wrote SUMMARY.md."}],"usage":{"input":1200,"output":340,"cacheRead":800,"cacheWrite":50,"totalTokens":2390},"stopReason":"stop","timestamp":1787467801000}}
{"type":"turn_end","message":{"role":"assistant","content":[{"type":"text","text":"Wrote SUMMARY.md."}],"stopReason":"stop","timestamp":1787467801000},"toolResults":[]}
{"type":"agent_end","messages":[]}`

func runPiStub(t *testing.T, stream string, req Request) ([]Event, Result, error) {
	t.Helper()
	runner := NewPiRunner(stubBinary(t, stream, "", 0))
	runner.now = fixedNow()
	var events []Event
	if req.Workspace == "" {
		req.Workspace = t.TempDir()
	}
	result, err := runner.Run(context.Background(), req, func(event Event) { events = append(events, event) })
	return events, result, err
}

func TestPiRunnerStreamsToolsAndUsage(t *testing.T) {
	events, result, err := runPiStub(t, piToolStream, Request{Prompt: "tidy up"})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if result.SessionID != "01a02d60-1111-2222-3333-444455556666" {
		t.Fatalf("session id = %q", result.SessionID)
	}
	if result.FinalMessage != "Wrote SUMMARY.md." {
		t.Fatalf("final message = %q", result.FinalMessage)
	}
	if !result.Succeeded {
		t.Fatal("expected a successful run")
	}
	// pi reports cache reads and writes beside the fresh input tokens, so the
	// normalized total has to fold all three together.
	want := Usage{InputTokens: 2050, CachedInputTokens: 800, CacheCreationInputTokens: 50, OutputTokens: 340}
	if result.Usage != want {
		t.Fatalf("usage = %+v, want %+v", result.Usage, want)
	}

	var message, reasoning strings.Builder
	toolUses, failedResults, fileChanges := 0, 0, []string{}
	for _, event := range events {
		switch event.Kind {
		case KindMessage:
			message.WriteString(event.Text)
		case KindReasoning:
			reasoning.WriteString(event.Text)
		case KindToolUse:
			toolUses++
		case KindToolResult:
			if event.Failed {
				failedResults++
			}
		case KindFileChange:
			fileChanges = append(fileChanges, event.Text)
		}
	}
	if message.String() != "Listing files." {
		t.Fatalf("streamed message = %q", message.String())
	}
	if reasoning.String() != "Checking the repo" {
		t.Fatalf("streamed reasoning = %q", reasoning.String())
	}
	if toolUses != 3 {
		t.Fatalf("tool uses = %d, want 3", toolUses)
	}
	if failedResults != 1 {
		t.Fatalf("failed tool results = %d, want 1", failedResults)
	}
	// Only the mutating tool earns a file_change; `read` touches the same file
	// argument without changing anything.
	if len(fileChanges) != 1 || fileChanges[0] != "SUMMARY.md" {
		t.Fatalf("file changes = %v, want [SUMMARY.md]", fileChanges)
	}
}

func TestPiRunnerSeparatesStreamsPerContentBlock(t *testing.T) {
	events, _, err := runPiStub(t, piToolStream, Request{Prompt: "tidy up"})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	var reasoningID, messageID string
	for _, event := range events {
		switch event.Kind {
		case KindReasoning:
			reasoningID = event.StreamID
		case KindMessage:
			if messageID != "" && event.StreamID != messageID {
				t.Fatalf("one message split across stream ids %q and %q", messageID, event.StreamID)
			}
			messageID = event.StreamID
		}
	}
	if reasoningID == "" || messageID == "" || reasoningID == messageID {
		t.Fatalf("reasoning (%q) and message (%q) must be distinct, non-empty streams", reasoningID, messageID)
	}
}

func TestPiRunnerReportsProviderFailure(t *testing.T) {
	events, result, err := runPiStub(t, piErrorStream, Request{Prompt: "say hi"})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if result.Succeeded {
		t.Fatal("a run whose turn ended in error must not report success")
	}
	if result.SessionID != "01a02d5d-8fe0-741d-a3fe-b572eeadddc6" {
		t.Fatalf("session id = %q", result.SessionID)
	}
	var sawError bool
	for _, event := range events {
		if event.Kind == KindError && strings.Contains(event.Text, "401") {
			sawError = true
		}
	}
	if !sawError {
		t.Fatal("provider error message was not surfaced as an error event")
	}
}

func TestPiCommandArgs(t *testing.T) {
	base := piCommandArgs(Request{Prompt: "do the thing"})
	if base[len(base)-1] != "do the thing" {
		t.Fatalf("prompt must be the final argument, got %v", base)
	}
	if !containsArgs(base, "-p", "--mode", "json") {
		t.Fatalf("missing single-shot JSON flags: %v", base)
	}

	readOnly := piCommandArgs(Request{Prompt: "look", Sandbox: SandboxReadOnly})
	tools := argValue(readOnly, "--tools")
	for _, denied := range []string{"bash", "edit", "write"} {
		if strings.Contains(tools, denied) {
			t.Fatalf("read-only allowlist %q must not include %q", tools, denied)
		}
	}
	if !strings.Contains(tools, "read") {
		t.Fatalf("read-only allowlist %q must still allow reading", tools)
	}
	if strings.Contains(strings.Join(piCommandArgs(Request{Prompt: "edit"}), " "), "--tools") {
		t.Fatal("a write-capable run must not restrict the tool catalog")
	}

	resumed := piCommandArgs(Request{Prompt: "carry on", Model: "anthropic/claude-opus-4-5", ReasoningEffort: "high", ResumeSessionID: "abc-123"})
	if argValue(resumed, "--session") != "abc-123" {
		t.Fatalf("resume session not passed: %v", resumed)
	}
	if argValue(resumed, "--model") != "anthropic/claude-opus-4-5" {
		t.Fatalf("model not passed: %v", resumed)
	}
	// Pi spells reasoning effort --thinking.
	if argValue(resumed, "--thinking") != "high" {
		t.Fatalf("reasoning effort not passed: %v", resumed)
	}
}

// piModelTable is the padded table `pi --list-models` prints, including the
// header row a parser must skip.
const piModelTable = `provider   model             context  max-out  thinking  images
anthropic  claude-opus-4-5   200K     64K      yes       yes
google     gemini-3-pro      1M       64K      yes       yes
openai     gpt-5.2           400K     128K     yes       no
`

func TestParsePiModelList(t *testing.T) {
	models := parsePiModelList(piModelTable)
	if len(models) != 3 {
		t.Fatalf("models = %d, want 3: %+v", len(models), models)
	}
	// The --model flag wants provider/id, which is what disambiguates a model
	// name that more than one provider serves.
	if models[0].Model != "anthropic/claude-opus-4-5" || models[0].DisplayName != "claude-opus-4-5" || models[0].Description != "anthropic" {
		t.Fatalf("first model = %+v", models[0])
	}
	if models[2].Model != "openai/gpt-5.2" {
		t.Fatalf("third model = %+v", models[2])
	}
}

// Pi names the model it ran on every turn but never its size, so the window
// has to come from the catalog's `context` column — written for people
// ("200K", "1M"), not for parsers.
func TestParsePiModelListReadsTheContextColumn(t *testing.T) {
	models := parsePiModelList(piModelTable)
	windows := make(map[string]int, len(models))
	for _, model := range models {
		windows[model.Model] = model.ContextWindow
	}
	for model, want := range map[string]int{
		"anthropic/claude-opus-4-5": 200_000,
		"google/gemini-3-pro":       1_000_000,
		"openai/gpt-5.2":            400_000,
	} {
		if windows[model] != want {
			t.Fatalf("%s window = %d, want %d", model, windows[model], want)
		}
	}
}

func TestParsePiContextWindow(t *testing.T) {
	for input, want := range map[string]int{
		"200K": 200_000, "1M": 1_000_000, "1.5M": 1_500_000,
		"128000": 128_000, "  400K  ": 400_000,
		// An unreadable cell is "unknown", which the gauge renders as no
		// reading at all — never as an empty context.
		"": 0, "-": 0, "unknown": 0, "0": 0, "-5K": 0,
	} {
		if got := parsePiContextWindow(input); got != want {
			t.Fatalf("parsePiContextWindow(%q) = %d, want %d", input, got, want)
		}
	}
}

func TestPiParserReportsOccupancyAgainstTheModelWindow(t *testing.T) {
	var events []Event
	parser := &piParser{contextWindow: func(provider, model string) int {
		if provider != "deepseek" || model != "deepseek-v4-flash" {
			t.Fatalf("lookup got provider=%q model=%q", provider, model)
		}
		return 1_000_000
	}}
	line := `{"type":"message_end","message":{"role":"assistant","provider":"deepseek","model":"deepseek-v4-flash","stopReason":"stop","content":[{"type":"text","text":"ok"}],"usage":{"input":2604,"output":13,"cacheRead":512,"cacheWrite":0}}}`
	parser.parse(line, time.Unix(0, 0).UTC(), func(event Event) { events = append(events, event) })

	var usage *Event
	for i := range events {
		if events[i].Kind == KindUsage {
			usage = &events[i]
		}
	}
	if usage == nil || usage.Context == nil {
		t.Fatalf("no usage event with context: %+v", events)
	}
	// A cached prefix still occupies the window, so occupancy is the whole
	// prompt: 2604 + 512.
	if usage.Context.Tokens != 3116 || usage.Context.Window != 1_000_000 {
		t.Fatalf("context = %+v", usage.Context)
	}
	if got := parser.result().Context; got.Tokens != 3116 || got.Window != 1_000_000 {
		t.Fatalf("result context = %+v", got)
	}
}

func TestParsePiModelListIgnoresGuidance(t *testing.T) {
	// A pi with no provider credentials prints prose containing paths, which
	// must not be mistaken for table rows.
	guidance := "No models available. Use /login to log into a provider via OAuth or API key. See:\n" +
		"  /usr/local/lib/node_modules/@mariozechner/pi-coding-agent/docs/providers.md\n" +
		"  /usr/local/lib/node_modules/@mariozechner/pi-coding-agent/docs/models.md\n"
	if models := parsePiModelList(guidance); len(models) != 0 {
		t.Fatalf("guidance parsed as models: %+v", models)
	}
}
