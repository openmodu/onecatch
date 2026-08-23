package agentrun

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const piBinaryDefault = "pi"

// piReadOnlyTools is the allowlist a read-only run is confined to.
//
// Pi has no OS sandbox: its permission model is the tool catalog, so removing
// bash/edit/write is what read-only means here. The list is spelled positively
// because `--tools` is an allowlist that also covers extension and custom
// tools, so a user's extension cannot reintroduce a way to write.
var piReadOnlyTools = []string{"read", "grep", "find", "ls"}

// PiRunner drives the Pi coding agent through its single-shot JSON mode
// (`pi -p --mode json`), which emits the agent's own event union as JSONL: a
// session header, then agent/turn/message lifecycle events and tool executions.
type PiRunner struct {
	binary string
	now    nowFunc
}

// NewPiRunner builds a runner driving the given pi binary. An empty binary
// falls back to "pi" resolved from PATH.
func NewPiRunner(binary string) *PiRunner {
	if binary == "" {
		binary = piBinaryDefault
	}
	return &PiRunner{binary: binary, now: time.Now}
}

func (r *PiRunner) Runtime() Runtime { return RuntimePi }

func (r *PiRunner) Available() bool {
	_, err := exec.LookPath(r.binary)
	return err == nil
}

func (r *PiRunner) Run(ctx context.Context, req Request, sink Sink) (Result, error) {
	if req.Remote != nil {
		return Result{}, fmt.Errorf("Pi does not support remote FS runs")
	}
	cmd := exec.CommandContext(ctx, r.binary, piCommandArgs(req)...)
	cmd.Dir = req.Workspace
	cmd.Env = req.Environment
	cmd.Stdin = nil
	if req.InterruptGrace > 0 {
		cmd.Cancel = func() error { return cmd.Process.Signal(os.Interrupt) }
		cmd.WaitDelay = req.InterruptGrace
	}
	return streamProcess(ctx, cmd, &piParser{}, r.now, sink)
}

// piCommandArgs builds the single-shot invocation. The prompt goes last: pi
// takes it as a positional argument, and keeping it after every flag stops a
// prompt that begins with a dash from being read as one.
func piCommandArgs(req Request) []string {
	args := []string{"-p", "--mode", "json"}
	if req.Sandbox == SandboxReadOnly {
		args = append(args, "--tools", strings.Join(piReadOnlyTools, ","))
	}
	if req.Model != "" {
		args = append(args, "--model", req.Model)
	}
	if req.ReasoningEffort != "" {
		args = append(args, "--thinking", req.ReasoningEffort)
	}
	if req.ResumeSessionID != "" {
		// Reopens the stored session, keeping its context, and applies the
		// prompt as the next turn.
		args = append(args, "--session", req.ResumeSessionID)
	}
	return append(args, req.Prompt)
}

// piEnvelope is the discriminated JSONL line pi writes in --mode json. The
// first line is the session header; every later line is one agent event.
type piEnvelope struct {
	Type string `json:"type"`
	// Session header fields.
	ID  string `json:"id,omitempty"`
	Cwd string `json:"cwd,omitempty"`
	// Message lifecycle.
	Message               json.RawMessage `json:"message,omitempty"`
	AssistantMessageEvent *struct {
		Type         string `json:"type"`
		ContentIndex int    `json:"contentIndex"`
		Delta        string `json:"delta"`
	} `json:"assistantMessageEvent,omitempty"`
	// Tool execution.
	ToolCallID string          `json:"toolCallId,omitempty"`
	ToolName   string          `json:"toolName,omitempty"`
	Args       json.RawMessage `json:"args,omitempty"`
	IsError    bool            `json:"isError,omitempty"`
}

// piMessage is the subset of pi's message shape OneCatch reads.
type piMessage struct {
	Role    string `json:"role"`
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Usage struct {
		Input      int `json:"input"`
		Output     int `json:"output"`
		CacheRead  int `json:"cacheRead"`
		CacheWrite int `json:"cacheWrite"`
	} `json:"usage"`
	StopReason   string `json:"stopReason"`
	ErrorMessage string `json:"errorMessage"`
}

type piParser struct {
	sessionID    string
	finalMessage string
	usage        Usage
	usageSeen    bool
	succeeded    bool
	// messageIndex distinguishes one assistant message's content blocks from
	// the next turn's, since pi's contentIndex restarts at zero each message.
	messageIndex int
	started      bool
}

func (p *piParser) parse(line string, at time.Time, sink Sink) {
	var envelope piEnvelope
	if err := json.Unmarshal([]byte(line), &envelope); err != nil {
		sink(Event{Kind: KindError, Text: fmt.Sprintf("parse Pi event: %v", err), Raw: line, At: at})
		return
	}
	switch envelope.Type {
	case "session":
		p.sessionID = envelope.ID
		p.started = true
		sink(Event{Kind: KindStarted, Text: envelope.ID, Raw: line, At: at})
	case "message_start":
		if piRole(envelope.Message) == "assistant" {
			p.messageIndex++
		}
	case "message_update":
		p.handleMessageUpdate(envelope, line, at, sink)
	case "message_end":
		p.handleAssistantMessage(envelope.Message, line, at, sink)
	case "tool_execution_start":
		sink(Event{
			Kind: KindToolUse, Text: piToolText(envelope.ToolName, envelope.Args),
			StreamID: p.toolStreamID(envelope.ToolCallID), Raw: line, At: at,
		})
		if path := piToolPath(envelope.ToolName, envelope.Args); path != "" {
			sink(Event{Kind: KindFileChange, Text: path, Raw: line, At: at})
		}
	case "tool_execution_end":
		sink(Event{
			Kind: KindToolResult, Text: envelope.ToolName, Failed: envelope.IsError,
			StreamID: p.toolStreamID(envelope.ToolCallID), Raw: line, At: at,
		})
	case "agent_end":
		sink(Event{Kind: KindResult, Text: p.finalMessage, Failed: !p.succeeded, Usage: p.usagePointer(), Raw: line, At: at})
	}
}

func (p *piParser) handleMessageUpdate(envelope piEnvelope, line string, at time.Time, sink Sink) {
	event := envelope.AssistantMessageEvent
	if event == nil || event.Delta == "" {
		return
	}
	streamID := p.contentStreamID(event.ContentIndex)
	switch event.Type {
	case "text_delta":
		sink(Event{Kind: KindMessage, Text: event.Delta, StreamID: streamID, Phase: StreamDelta, Raw: line, At: at})
	case "thinking_delta":
		sink(Event{Kind: KindReasoning, Text: event.Delta, StreamID: streamID, Phase: StreamDelta, Raw: line, At: at})
	}
}

// handleAssistantMessage records the settled state of one assistant turn: its
// text becomes the candidate final message, its usage the latest accounting,
// and its stop reason decides whether the run succeeded. Later turns replace
// earlier ones, so the last message the agent produced is the one reported.
func (p *piParser) handleAssistantMessage(raw json.RawMessage, line string, at time.Time, sink Sink) {
	if len(raw) == 0 {
		return
	}
	var message piMessage
	if err := json.Unmarshal(raw, &message); err != nil || message.Role != "assistant" {
		return
	}
	var text strings.Builder
	for _, content := range message.Content {
		if content.Type == "text" {
			text.WriteString(content.Text)
		}
	}
	p.finalMessage = text.String()
	// pi reports "toolUse" when the turn ends to run tools; the run continues
	// and a later turn carries the real terminal reason.
	p.succeeded = message.StopReason != "error" && message.StopReason != "aborted"
	if message.ErrorMessage != "" {
		sink(Event{Kind: KindError, Text: message.ErrorMessage, Raw: line, At: at})
	}
	usage := Usage{
		InputTokens:       message.Usage.Input + message.Usage.CacheRead + message.Usage.CacheWrite,
		CachedInputTokens: message.Usage.CacheRead,

		CacheCreationInputTokens: message.Usage.CacheWrite,
		OutputTokens:             message.Usage.Output,
	}
	if usage == (Usage{}) {
		return
	}
	p.usage = usage
	p.usageSeen = true
	sink(Event{Kind: KindUsage, Usage: &usage, Raw: line, At: at})
}

func (p *piParser) result() Result {
	return Result{
		FinalMessage: strings.TrimSpace(p.finalMessage),
		Usage:        p.usage,
		SessionID:    p.sessionID,
		// A stream that never produced a session header never ran; treat that
		// as failure regardless of what the last message claimed.
		Succeeded: p.started && p.succeeded,
	}
}

func (p *piParser) usagePointer() *Usage {
	if !p.usageSeen {
		return nil
	}
	usage := p.usage
	return &usage
}

func (p *piParser) contentStreamID(contentIndex int) string {
	return "pi-message-" + strconv.Itoa(p.messageIndex) + "-" + strconv.Itoa(contentIndex)
}

func (p *piParser) toolStreamID(toolCallID string) string {
	return "pi-tool-" + toolCallID
}

func piRole(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var message struct {
		Role string `json:"role"`
	}
	if err := json.Unmarshal(raw, &message); err != nil {
		return ""
	}
	return message.Role
}

// piToolText describes a tool call for the run log, spelling out shell commands
// inline and naming the target file for the filesystem tools.
func piToolText(name string, args json.RawMessage) string {
	if len(args) == 0 {
		return name
	}
	var input map[string]any
	if err := json.Unmarshal(args, &input); err != nil {
		return name
	}
	if command, ok := input["command"].(string); ok && strings.TrimSpace(command) != "" {
		return command
	}
	if path := toolInputPath(input); path != "" {
		return name + " " + path
	}
	return name
}

// piToolPath returns the file a tool call modifies, or empty for tools that
// only read. Only mutations are worth a file_change event.
func piToolPath(name string, args json.RawMessage) string {
	if name != "write" && name != "edit" {
		return ""
	}
	var input map[string]any
	if err := json.Unmarshal(args, &input); err != nil {
		return ""
	}
	return toolInputPath(input)
}

// PiModelInfo is one model the installed Pi advertises.
type PiModelInfo struct {
	Model       string `json:"model"`
	DisplayName string `json:"displayName"`
	Provider    string `json:"provider,omitempty"`
}

// PiConfiguration is the model catalog discovered from a Pi installation.
type PiConfiguration struct {
	Models  []PiModelInfo `json:"models"`
	Efforts []string      `json:"efforts"`
}

// piThinkingLevels are the values pi accepts for --thinking. They are fixed by
// the CLI rather than by the selected model, so they need no discovery.
var piThinkingLevels = []string{"off", "minimal", "low", "medium", "high", "xhigh"}

// InspectConfiguration lists the models the installed Pi can reach.
//
// `pi --list-models` reads pi's own provider registry and prints a padded
// table without starting a session or spending model quota. A Pi with no
// provider credentials prints guidance instead of rows, which surfaces here as
// "no models" rather than as a hard failure of the probe.
func (r *PiRunner) InspectConfiguration(ctx context.Context, cwd string, environment []string) (PiConfiguration, error) {
	cmd := exec.CommandContext(ctx, r.binary, "--list-models")
	if cwd != "" {
		cmd.Dir = cwd
	}
	cmd.Env = environment
	var stderr lineCapture
	cmd.Stderr = &stderr
	// Only stdout carries the table; pi writes models.json load warnings to
	// stderr, and folding them in would look like table rows.
	output, err := cmd.Output()
	if err != nil {
		return PiConfiguration{}, fmt.Errorf("read Pi models: %w%s", err, stderr.tail())
	}
	models := parsePiModelList(string(output))
	if len(models) == 0 {
		return PiConfiguration{}, fmt.Errorf("Pi did not advertise any models; sign in to a provider first")
	}
	return PiConfiguration{Models: models, Efforts: piThinkingLevels}, nil
}

// piColumnGap splits the padded table: every column is padded to width and
// joined with two spaces, so two or more spaces separate fields while a single
// space can still appear inside one.
var piColumnGap = regexp.MustCompile(`\s{2,}`)

// parsePiModelList reads the `provider model context max-out thinking images`
// table. Model ids are reported as `provider/id`, the form pi's --model flag
// accepts unambiguously when two providers serve the same model name.
func parsePiModelList(output string) []PiModelInfo {
	models := make([]PiModelInfo, 0)
	seen := make(map[string]struct{})
	for _, line := range strings.Split(output, "\n") {
		fields := piColumnGap.Split(strings.TrimSpace(stripANSI(line)), -1)
		if len(fields) < 2 {
			continue
		}
		provider, id := fields[0], fields[1]
		// Skip the header row and any prose pi prints when it has no models.
		if provider == "" || provider == "provider" || id == "" || strings.ContainsAny(provider+id, " \t") {
			continue
		}
		model := provider + "/" + id
		if _, ok := seen[model]; ok {
			continue
		}
		seen[model] = struct{}{}
		models = append(models, PiModelInfo{Model: model, DisplayName: id, Provider: provider})
	}
	return models
}

// ansiEscape matches the SGR sequences chalk emits when it believes it is
// writing to a terminal.
var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(value string) string { return ansiEscape.ReplaceAllString(value, "") }
