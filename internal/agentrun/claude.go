package agentrun

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"time"
)

const claudeBinaryDefault = "claude"

// ClaudeRunner drives Claude Code through its non-interactive print mode
// (`claude -p --output-format stream-json`), parsing the documented event
// stream of system/assistant/user/result records.
type ClaudeRunner struct {
	binary string
	now    nowFunc
}

// NewClaudeRunner builds a runner driving the given claude binary. An empty
// binary falls back to "claude" resolved from PATH.
func NewClaudeRunner(binary string) *ClaudeRunner {
	if binary == "" {
		binary = claudeBinaryDefault
	}
	return &ClaudeRunner{binary: binary, now: time.Now}
}

func (r *ClaudeRunner) Runtime() Runtime { return RuntimeClaude }

func (r *ClaudeRunner) Available() bool {
	_, err := exec.LookPath(r.binary)
	return err == nil
}

func (r *ClaudeRunner) Run(ctx context.Context, req Request, sink Sink) (Result, error) {
	args := []string{
		"-p", req.Prompt,
		"--output-format", "stream-json",
		// stream-json requires --verbose to emit per-step events.
		"--verbose",
	}
	if req.ResumeSessionID != "" {
		// Continue a prior conversation, preserving its context.
		args = append(args, "--resume", req.ResumeSessionID)
	}
	if req.Sandbox != SandboxReadOnly {
		// Claude Code gates writes/commands behind permission prompts that
		// would hang a headless run; bypass them for write-capable tasks.
		args = append(args, "--dangerously-skip-permissions")
	}
	if req.Model != "" {
		args = append(args, "--model", req.Model)
	}

	cmd := exec.CommandContext(ctx, r.binary, args...)
	// Claude Code operates on its current working directory.
	cmd.Dir = req.Workspace
	cmd.Env = req.Environment
	if req.InterruptGrace > 0 {
		cmd.Cancel = func() error { return cmd.Process.Signal(os.Interrupt) }
		cmd.WaitDelay = req.InterruptGrace
	}
	cmd.Stdin = nil
	return streamProcess(ctx, cmd, &claudeParser{}, r.now, sink)
}

// claudeParser accumulates terminal state from Claude Code's stream-json.
type claudeParser struct {
	final     string
	sessionID string
	usage     Usage
	succeeded bool
	done      bool
}

type claudeEnvelope struct {
	Type      string        `json:"type"`
	Subtype   string        `json:"subtype"`
	SessionID string        `json:"session_id"`
	Message   claudeMessage `json:"message"`
	Result    string        `json:"result"`
	IsError   bool          `json:"is_error"`
	Usage     claudeUsage   `json:"usage"`
}

type claudeMessage struct {
	Content []claudeContent `json:"content"`
	Usage   claudeUsage     `json:"usage"`
}

type claudeContent struct {
	Type     string          `json:"type"`
	Text     string          `json:"text"`
	Thinking string          `json:"thinking"`
	Name     string          `json:"name"`
	Input    json.RawMessage `json:"input"`
	Content  json.RawMessage `json:"content"`
	IsError  bool            `json:"is_error"`
}

type claudeUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

func (p *claudeParser) parse(line string, at time.Time, sink Sink) {
	var env claudeEnvelope
	if err := json.Unmarshal([]byte(line), &env); err != nil {
		sink(Event{Kind: KindReasoning, Text: strings.TrimSpace(line), Raw: line, At: at})
		return
	}

	switch env.Type {
	case "system":
		if env.Subtype == "init" {
			p.sessionID = env.SessionID
			sink(Event{Kind: KindStarted, Text: env.SessionID, Raw: line, At: at})
		}
		// Other system events (hooks, thinking-token meters) are progress noise.
	case "assistant":
		p.handleAssistant(env.Message, line, at, sink)
	case "user":
		p.handleToolResults(env.Message, line, at, sink)
	case "result":
		p.handleResult(env, line, at, sink)
	default:
		// rate_limit_event and friends: keep raw, no user-facing text.
	}
}

func (p *claudeParser) handleAssistant(msg claudeMessage, line string, at time.Time, sink Sink) {
	for _, c := range msg.Content {
		switch c.Type {
		case "text":
			if strings.TrimSpace(c.Text) == "" {
				continue
			}
			p.final = c.Text
			sink(Event{Kind: KindMessage, Text: c.Text, Raw: line, At: at})
		case "thinking":
			sink(Event{Kind: KindReasoning, Text: c.Thinking, Raw: line, At: at})
		case "tool_use":
			sink(Event{Kind: KindToolUse, Text: claudeToolText(c), Raw: line, At: at})
		}
	}
}

func (p *claudeParser) handleToolResults(msg claudeMessage, line string, at time.Time, sink Sink) {
	for _, c := range msg.Content {
		if c.Type == "tool_result" {
			sink(Event{Kind: KindToolResult, Text: string(c.Content), Raw: line, Failed: c.IsError, At: at})
		}
	}
}

func (p *claudeParser) handleResult(env claudeEnvelope, line string, at time.Time, sink Sink) {
	p.done = true
	p.usage = Usage{InputTokens: env.Usage.InputTokens, OutputTokens: env.Usage.OutputTokens}
	if env.Result != "" {
		p.final = env.Result
	}
	if env.IsError || env.Subtype != "success" {
		p.succeeded = false
		sink(Event{Kind: KindError, Text: firstNonEmpty(env.Result, env.Subtype), Raw: line, At: at})
		return
	}
	p.succeeded = true
	sink(Event{Kind: KindResult, Text: env.Result, Raw: line, At: at})
}

// claudeToolText renders a tool invocation compactly: "Bash {...}".
func claudeToolText(c claudeContent) string {
	name := c.Name
	if name == "" {
		name = "tool"
	}
	if len(c.Input) == 0 {
		return name
	}
	return name + " " + string(c.Input)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func (p *claudeParser) result() Result {
	return Result{
		FinalMessage: p.final,
		Usage:        p.usage,
		SessionID:    p.sessionID,
		Succeeded:    p.succeeded && p.done,
	}
}
