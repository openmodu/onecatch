package agentrun

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const (
	moduBinaryDefault = "modu_code"
	moduBinaryLegacy  = "modu-code"
)

// ModuRunner drives Modu Code through its non-interactive print mode
// (`modu_code -p ... -json`), the same role `codex exec --json` serves for
// Codex. The NDJSON stream includes a persisted session id that later runs can
// continue with `--resume ID -p ...`.
type ModuRunner struct {
	binary string
	now    nowFunc
}

func NewModuRunner(binary string) *ModuRunner {
	if binary == "" {
		binary = defaultModuBinary()
	}
	return &ModuRunner{binary: binary, now: time.Now}
}

func defaultModuBinary() string {
	if binary, err := exec.LookPath(moduBinaryDefault); err == nil {
		return binary
	}
	return moduBinaryLegacy
}

func (r *ModuRunner) Runtime() Runtime { return RuntimeModu }

func (r *ModuRunner) Available() bool {
	_, err := exec.LookPath(r.binary)
	return err == nil
}

func (r *ModuRunner) Run(ctx context.Context, req Request, sink Sink) (Result, error) {
	cmd := exec.CommandContext(ctx, r.binary, moduCommandArgs(req)...)
	cmd.Dir = req.Workspace
	cmd.Env = moduEnvironment(req.Environment, req.Model)
	if req.InterruptGrace > 0 {
		cmd.Cancel = func() error { return cmd.Process.Signal(os.Interrupt) }
		cmd.WaitDelay = req.InterruptGrace
	}
	cmd.Stdin = nil
	return streamProcess(ctx, cmd, &moduParser{}, r.now, sink)
}

func moduCommandArgs(req Request) []string {
	args := make([]string, 0, 7)
	if strings.TrimSpace(req.ResumeSessionID) != "" {
		args = append(args, "--resume", strings.TrimSpace(req.ResumeSessionID))
	}
	args = append(args, "-p", req.Prompt, "-json")
	if req.Sandbox != SandboxReadOnly {
		// Writable Oneshot runs are already authorized and cannot stop for an
		// interactive approval prompt in print mode.
		args = append(args, "--no-approve")
	}
	return args
}

// moduParser accumulates terminal state from Modu Code's print-mode NDJSON
// stream. Its event names mirror pkg/coding_agent/modes in Modu Code.
type moduParser struct {
	final             string
	sessionID         string
	usage             Usage
	completed         bool
	failed            bool
	messageSeq        int
	messageOpen       bool
	textStreaming     bool
	thinkingStreaming bool
}

type moduEnvelope struct {
	Type       string          `json:"type"`
	SessionID  string          `json:"sessionId"`
	Model      string          `json:"model"`
	Message    json.RawMessage `json:"message"`
	Stream     moduStreamEvent `json:"streamEvent"`
	ToolName   string          `json:"toolName"`
	ToolCallID string          `json:"toolCallId"`
	Args       json.RawMessage `json:"args"`
	Result     json.RawMessage `json:"result"`
	IsError    bool            `json:"isError"`
}

type moduStreamEvent struct {
	Type         string `json:"Type"`
	ContentIndex int    `json:"ContentIndex"`
	Delta        string `json:"Delta"`
}

type moduMessage struct {
	Role         string        `json:"role"`
	Content      []moduContent `json:"content"`
	Usage        moduUsage     `json:"usage"`
	StopReason   string        `json:"stopReason"`
	ErrorMessage string        `json:"errorMessage"`
}

type moduContent struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	Thinking  string          `json:"thinking"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type moduUsage struct {
	Input  int `json:"input"`
	Output int `json:"output"`
}

func (p *moduParser) parse(line string, at time.Time, sink Sink) {
	var env moduEnvelope
	if err := json.Unmarshal([]byte(line), &env); err != nil {
		sink(Event{Kind: KindReasoning, Text: strings.TrimSpace(line), Raw: line, At: at})
		return
	}

	switch env.Type {
	case "session_start":
		p.sessionID = env.SessionID
		sink(Event{Kind: KindStarted, Text: env.SessionID, Raw: line, At: at})
	case "agent_start", "agent_end", "turn_start", "turn_end":
		// Lifecycle boundaries carry no additional user-facing payload.
	case "message_start":
		p.beginMessage()
	case "message_update":
		p.handleStreamEvent(env.Stream, line, at, sink)
	case "message_end":
		p.handleMessage(env.Message, line, at, sink)
	case "tool_execution_start":
		sink(Event{Kind: KindToolUse, Text: moduToolText(env.ToolName, env.Args), Raw: line, At: at})
	case "tool_execution_update":
		// Partial tool output is retained raw without pretending the tool has
		// completed; the terminal result arrives in tool_execution_end.
		sink(Event{Kind: KindReasoning, Text: moduResultText(env.Result), Raw: line, At: at})
	case "tool_execution_end":
		sink(Event{Kind: KindToolResult, Text: moduResultText(env.Result), Raw: line, Failed: env.IsError, At: at})
	case "interrupt":
		// Modu also uses this lifecycle event for approval gates. The print
		// stream omits the interrupt reason, so process/session completion is
		// the reliable source of the run's terminal state.
		sink(Event{Kind: KindReasoning, Raw: line, At: at})
	case "session_end":
		p.completed = true
		sink(Event{Kind: KindResult, Text: p.final, Raw: line, At: at})
	default:
		// Retain extension and future event types for forward compatibility.
		sink(Event{Kind: KindReasoning, Raw: line, At: at})
	}
}

func (p *moduParser) beginMessage() {
	p.messageSeq++
	p.messageOpen = true
	p.textStreaming = false
	p.thinkingStreaming = false
}

func (p *moduParser) ensureMessage() {
	if !p.messageOpen {
		p.beginMessage()
	}
}

func (p *moduParser) streamID(kind string) string {
	p.ensureMessage()
	return "modu-" + kind + "-" + strconv.Itoa(p.messageSeq)
}

func (p *moduParser) handleStreamEvent(event moduStreamEvent, line string, at time.Time, sink Sink) {
	switch event.Type {
	case "text_start":
		id := p.streamID("message")
		if !p.textStreaming {
			p.textStreaming = true
			sink(Event{Kind: KindMessage, StreamID: id, Phase: StreamStart, Raw: line, At: at})
		}
	case "text_delta":
		id := p.streamID("message")
		if !p.textStreaming {
			p.textStreaming = true
			sink(Event{Kind: KindMessage, StreamID: id, Phase: StreamStart, Raw: line, At: at})
		}
		if event.Delta != "" {
			sink(Event{Kind: KindMessage, StreamID: id, Phase: StreamDelta, Text: event.Delta, Raw: line, At: at})
		}
	case "thinking_start":
		id := p.streamID("thinking")
		if !p.thinkingStreaming {
			p.thinkingStreaming = true
			sink(Event{Kind: KindReasoning, StreamID: id, Phase: StreamStart, Raw: line, At: at})
		}
	case "thinking_delta":
		id := p.streamID("thinking")
		if !p.thinkingStreaming {
			p.thinkingStreaming = true
			sink(Event{Kind: KindReasoning, StreamID: id, Phase: StreamStart, Raw: line, At: at})
		}
		if event.Delta != "" {
			sink(Event{Kind: KindReasoning, StreamID: id, Phase: StreamDelta, Text: event.Delta, Raw: line, At: at})
		}
	}
}

func (p *moduParser) handleMessage(raw json.RawMessage, line string, at time.Time, sink Sink) {
	var message moduMessage
	if len(raw) == 0 || json.Unmarshal(raw, &message) != nil || message.Role != "assistant" {
		return
	}
	p.ensureMessage()

	p.usage.InputTokens += message.Usage.Input
	p.usage.OutputTokens += message.Usage.Output
	var text, thinking strings.Builder
	for _, content := range message.Content {
		switch content.Type {
		case "text":
			text.WriteString(content.Text)
		case "thinking":
			thinking.WriteString(content.Thinking)
		}
	}
	if reasoning := thinking.String(); strings.TrimSpace(reasoning) != "" {
		if p.thinkingStreaming {
			sink(Event{Kind: KindReasoning, StreamID: p.streamID("thinking"), Phase: StreamEnd, Text: reasoning, Raw: line, At: at})
		} else {
			sink(Event{Kind: KindReasoning, Text: reasoning, Raw: line, At: at})
		}
	}
	if final := text.String(); strings.TrimSpace(final) != "" {
		p.final = final
		if p.textStreaming {
			sink(Event{Kind: KindMessage, StreamID: p.streamID("message"), Phase: StreamEnd, Text: final, Raw: line, At: at})
		} else {
			sink(Event{Kind: KindMessage, Text: final, Raw: line, At: at})
		}
	}
	if strings.TrimSpace(message.ErrorMessage) != "" {
		p.failed = true
		sink(Event{Kind: KindError, Text: message.ErrorMessage, Raw: line, At: at})
	}
	p.messageOpen = false
	p.textStreaming = false
	p.thinkingStreaming = false
}

func moduToolText(name string, args json.RawMessage) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "tool"
	}
	arguments := strings.TrimSpace(string(args))
	if arguments == "" || arguments == "null" {
		return name
	}
	return name + " " + arguments
}

func moduResultText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var result struct {
		Content []moduContent `json:"content"`
	}
	if json.Unmarshal(raw, &result) == nil {
		var text strings.Builder
		for _, content := range result.Content {
			if content.Type == "text" {
				text.WriteString(content.Text)
			}
		}
		if text.Len() > 0 {
			return text.String()
		}
	}
	return strings.TrimSpace(string(raw))
}

func (p *moduParser) result() Result {
	return Result{
		FinalMessage: p.final,
		Usage:        p.usage,
		SessionID:    p.sessionID,
		Succeeded:    p.completed && !p.failed,
	}
}

func moduEnvironment(environment []string, model string) []string {
	if environment == nil {
		environment = os.Environ()
	} else {
		environment = append([]string(nil), environment...)
	}
	if strings.TrimSpace(model) != "" {
		environment = setEnvironmentValue(environment, "MODU_CODE_MODEL", strings.TrimSpace(model))
	}
	return environment
}

func setEnvironmentValue(environment []string, key, value string) []string {
	prefix := key + "="
	for index, item := range environment {
		if strings.HasPrefix(item, prefix) {
			environment[index] = prefix + value
			return environment
		}
	}
	return append(environment, prefix+value)
}
