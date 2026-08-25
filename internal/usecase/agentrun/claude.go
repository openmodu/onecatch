package agentrun

import (
	"bufio"
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

type ClaudeModelInfo struct {
	Model       string `json:"model"`
	DisplayName string `json:"displayName"`
	Alias       bool   `json:"alias"`
}

type ClaudeConfiguration struct {
	Models  []ClaudeModelInfo `json:"models"`
	Efforts []string          `json:"efforts"`
}

// InspectConfiguration discovers the model aliases advertised by the installed
// Claude Code CLI. Claude Code does not expose a model-list command, so this
// reads --help without starting a session or consuming model quota.
func (r *ClaudeRunner) InspectConfiguration(ctx context.Context, cwd string, environment []string) (ClaudeConfiguration, error) {
	cmd := exec.CommandContext(ctx, r.binary, "--help")
	if cwd != "" {
		cmd.Dir = cwd
	}
	cmd.Env = environment
	output, err := cmd.CombinedOutput()
	if err != nil {
		return ClaudeConfiguration{}, fmt.Errorf("read Claude Code model options: %w: %s", err, strings.TrimSpace(string(output)))
	}
	models := parseClaudeModelOptions(string(output))
	if len(models) == 0 {
		return ClaudeConfiguration{}, fmt.Errorf("Claude Code did not advertise any model aliases")
	}
	return ClaudeConfiguration{Models: models, Efforts: parseClaudeEffortOptions(string(output))}, nil
}

// claudeCatalogModels compensates for `claude --help`, which documents --model
// with a couple of examples rather than a catalog: probing 2.1.241 yields only
// 'fable', 'opus', 'sonnet' and 'claude-fable-5', so every pinned version was
// unreachable from the picker. Claude Code has no equivalent of Codex's
// model/list, so the pinned names have to be carried here.
//
// This list therefore goes stale on its own schedule. It is additive and
// deduplicated against whatever --help advertises, so a name the installed CLI
// no longer accepts costs a failed run rather than a broken picker, and a
// newly shipped model still shows up through the alias parsed from --help.
// Verified present in the 2.1.241 binary's model table.
var claudeCatalogModels = []string{
	"claude-opus-5",
	"claude-sonnet-5",
	"claude-opus-4-8",
	"claude-opus-4-7",
	"claude-opus-4-6",
	"claude-sonnet-4-6",
	"claude-haiku-4-5",
}

var claudeQuotedModel = regexp.MustCompile(`'([A-Za-z0-9][A-Za-z0-9._:-]*)'`)

func parseClaudeModelOptions(help string) []ClaudeModelInfo {
	seen := make(map[string]struct{})
	models := make([]ClaudeModelInfo, 0)
	for _, match := range claudeQuotedModel.FindAllStringSubmatch(claudeHelpOptionSection(help, "--model <model>"), -1) {
		model := match[1]
		if _, ok := seen[model]; ok {
			continue
		}
		seen[model] = struct{}{}
		alias := !strings.HasPrefix(model, "claude-")
		displayName := model
		if alias {
			displayName = strings.ToUpper(model[:1]) + model[1:]
		}
		models = append(models, ClaudeModelInfo{Model: model, DisplayName: displayName, Alias: alias})
	}
	// The aliases parsed above stay first: they are what the installed CLI
	// actually advertises, and they track the newest model without an update
	// here. The pinned names follow as the "more models" tail.
	for _, model := range claudeCatalogModels {
		if _, ok := seen[model]; ok {
			continue
		}
		seen[model] = struct{}{}
		models = append(models, ClaudeModelInfo{Model: model, DisplayName: model})
	}
	return models
}

func parseClaudeEffortOptions(help string) []string {
	section := claudeHelpOptionSection(help, "--effort <level>")
	efforts := make([]string, 0, 5)
	for _, effort := range []string{"low", "medium", "high", "xhigh", "max"} {
		if regexp.MustCompile(`\b` + effort + `\b`).MatchString(section) {
			efforts = append(efforts, effort)
		}
	}
	return efforts
}

func claudeHelpOptionSection(help, option string) string {
	lines := strings.Split(help, "\n")
	var section strings.Builder
	collecting := false
	for _, line := range lines {
		if !collecting {
			index := strings.Index(line, option)
			if index < 0 {
				continue
			}
			collecting = true
			section.WriteString(line[index+len(option):])
			section.WriteByte('\n')
			continue
		}
		if strings.HasPrefix(line, "  -") {
			break
		}
		section.WriteString(line)
		section.WriteByte('\n')
	}
	return section.String()
}

// SupportsInteractivePermissions is true only for a read-only run. Claude Code
// reaches its can_use_tool control channel through `--input-format stream-json`,
// which changes the whole invocation; a write-capable run instead passes
// `--dangerously-skip-permissions` because its prompts would hang a headless
// process. Widening this means reworking how the write path is launched.
func (r *ClaudeRunner) SupportsInteractivePermissions(sandbox Sandbox) bool {
	return sandbox == SandboxReadOnly
}

func (r *ClaudeRunner) Run(ctx context.Context, req Request, sink Sink) (Result, error) {
	interactivePermissions := req.Sandbox == SandboxReadOnly && req.PermissionHandler != nil
	args := []string{
		"--output-format", "stream-json",
		// stream-json requires --verbose to emit per-step events.
		"--verbose",
		// Include raw content_block_delta events so text and thinking can be
		// rendered while the model is still generating them.
		"--include-partial-messages",
	}
	if interactivePermissions {
		// Claude's Agent SDK uses this bidirectional JSONL control channel for
		// can_use_tool requests. The CLI flag is intentionally compatible with
		// the SDK transport even though it is not advertised in --help.
		args = append(args,
			"--input-format", "stream-json",
			"--permission-prompt-tool", "stdio",
			// Preserve the product's read-only contract even when a user can
			// approve network access from the permission card.
			"--disallowedTools", "Bash,Edit,Write,NotebookEdit",
		)
	} else {
		args = append([]string{"-p", req.Prompt}, args...)
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
	if req.ReasoningEffort != "" {
		args = append(args, "--effort", req.ReasoningEffort)
	}

	environment := req.Environment
	if req.Remote != nil {
		var err error
		req, err = prepareRemoteRequest(req)
		if err != nil {
			return Result{}, err
		}
		remote, err := setupRemoteClaude(req)
		if err != nil {
			return Result{}, err
		}
		defer remote.cleanup()
		args = append(args, remote.args...)
		environment = mergeEnvironment(environment, remote.env)
	}

	cmd := exec.CommandContext(ctx, r.binary, args...)
	// Claude Code operates on its current working directory. For a remote run
	// this is only where the harness process lives — the agent's commands run
	// on the target, in the session's own directory.
	cmd.Dir = req.Workspace
	cmd.Env = environment
	if req.InterruptGrace > 0 {
		cmd.Cancel = func() error { return cmd.Process.Signal(os.Interrupt) }
		cmd.WaitDelay = req.InterruptGrace
	}
	if interactivePermissions {
		return r.runInteractive(ctx, cmd, req, sink)
	}
	cmd.Stdin = nil
	return streamProcess(ctx, cmd, &claudeParser{}, r.now, sink)
}

type claudeControlEnvelope struct {
	Type      string               `json:"type"`
	RequestID string               `json:"request_id"`
	Request   claudeControlRequest `json:"request"`
}

type claudeControlRequest struct {
	Subtype                 string             `json:"subtype"`
	ToolName                string             `json:"tool_name"`
	Input                   map[string]any     `json:"input"`
	PermissionSuggestions   []PermissionUpdate `json:"permission_suggestions"`
	DecisionReason          string             `json:"decision_reason"`
	Title                   string             `json:"title"`
	DisplayName             string             `json:"display_name"`
	Description             string             `json:"description"`
	ToolUseID               string             `json:"tool_use_id"`
	SuppressAlwaysAllowRule bool               `json:"suppress_always_allow_rule"`
	RequiresUserInteraction bool               `json:"requires_user_interaction"`
}

type claudeControlResponse struct {
	Type     string                    `json:"type"`
	Response claudeControlResponseBody `json:"response"`
}

type claudeControlResponseBody struct {
	Subtype   string             `json:"subtype"`
	RequestID string             `json:"request_id"`
	Response  PermissionDecision `json:"response,omitempty"`
	Error     string             `json:"error,omitempty"`
}

type claudeInputMessage struct {
	Type            string                 `json:"type"`
	SessionID       string                 `json:"session_id"`
	Message         claudeInputMessageBody `json:"message"`
	ParentToolUseID *string                `json:"parent_tool_use_id"`
}

type claudeInputMessageBody struct {
	Role    string               `json:"role"`
	Content []claudeInputContent `json:"content"`
}

type claudeInputContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func (r *ClaudeRunner) runInteractive(ctx context.Context, cmd *exec.Cmd, req Request, sink Sink) (Result, error) {
	if sink == nil {
		sink = func(Event) {}
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return Result{}, fmt.Errorf("Claude Code stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return Result{}, fmt.Errorf("Claude Code stdout: %w", err)
	}
	var stderr lineCapture
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return Result{}, fmt.Errorf("start %s: %w", cmd.Path, err)
	}
	encoder := json.NewEncoder(stdin)
	if err := encoder.Encode(claudeInputMessage{
		Type: "user", SessionID: "", ParentToolUseID: nil,
		Message: claudeInputMessageBody{Role: "user", Content: []claudeInputContent{{Type: "text", Text: req.Prompt}}},
	}); err != nil {
		_ = stdin.Close()
		_ = cmd.Wait()
		return Result{}, fmt.Errorf("write Claude Code prompt: %w", err)
	}

	parser := &claudeParser{}
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	stdinClosed := false
	closeStdin := func() {
		if !stdinClosed {
			stdinClosed = true
			_ = stdin.Close()
		}
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		var control claudeControlEnvelope
		if json.Unmarshal([]byte(line), &control) == nil && control.Type == "control_request" {
			if err := r.handlePermissionControl(ctx, encoder, control, line, req.PermissionHandler, sink); err != nil {
				closeStdin()
				_ = cmd.Wait()
				return parser.result(), err
			}
			continue
		}
		parser.parse(line, r.now(), sink)
		var envelope struct {
			Type string `json:"type"`
		}
		if json.Unmarshal([]byte(line), &envelope) == nil && envelope.Type == "result" {
			closeStdin()
		}
	}
	closeStdin()
	if err := scanner.Err(); err != nil {
		_ = cmd.Wait()
		return parser.result(), fmt.Errorf("scan %s output: %w", cmd.Path, err)
	}
	if err := cmd.Wait(); err != nil {
		if ctx.Err() != nil {
			return parser.result(), ctx.Err()
		}
		return parser.result(), fmt.Errorf("%s failed: %w%s", cmd.Path, err, stderr.tail())
	}
	return parser.result(), nil
}

func (r *ClaudeRunner) handlePermissionControl(ctx context.Context, encoder *json.Encoder, control claudeControlEnvelope, raw string, handler func(context.Context, PermissionRequest) (PermissionDecision, error), sink Sink) error {
	if control.Request.Subtype != "can_use_tool" {
		return encoder.Encode(claudeControlResponse{Type: "control_response", Response: claudeControlResponseBody{Subtype: "error", RequestID: control.RequestID, Error: "unsupported control request subtype: " + control.Request.Subtype}})
	}
	request := PermissionRequest{
		ID: control.RequestID, ToolUseID: control.Request.ToolUseID, ToolName: control.Request.ToolName,
		Input: control.Request.Input, Suggestions: control.Request.PermissionSuggestions,
		Title: control.Request.Title, DisplayName: control.Request.DisplayName, Description: control.Request.Description,
		DecisionReason: control.Request.DecisionReason,
		// "Always allow" persists the provider-authored rules in Suggestions.
		// With none to persist there is nothing for the decision to mean, so
		// the option is suppressed rather than silently degrading to one-shot.
		SuppressAlwaysAllow:     control.Request.SuppressAlwaysAllowRule || len(control.Request.PermissionSuggestions) == 0,
		RequiresUserInteraction: control.Request.RequiresUserInteraction,
	}
	text := request.Title
	if text == "" {
		text = "Claude wants to use " + request.ToolName
	}
	sink(Event{Kind: KindPermissionRequest, Text: text, Raw: raw, Permission: &request, At: r.now()})
	decision, err := handler(ctx, request)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		decision = PermissionDecision{Behavior: "deny", Message: err.Error(), DecisionClassification: "user_reject"}
	}
	if decision.Behavior != "allow" && decision.Behavior != "deny" {
		decision = PermissionDecision{Behavior: "deny", Message: "invalid permission decision", DecisionClassification: "user_reject"}
	}
	if decision.Behavior == "deny" && decision.Message == "" {
		decision.Message = "Permission denied by user"
	}
	decision.ToolUseID = request.ToolUseID
	if err := encoder.Encode(claudeControlResponse{Type: "control_response", Response: claudeControlResponseBody{Subtype: "success", RequestID: control.RequestID, Response: decision}}); err != nil {
		return fmt.Errorf("write Claude Code permission response: %w", err)
	}
	sink(Event{Kind: KindPermissionResolved, Text: decision.Behavior, Permission: &request, PermissionDecision: decision.Behavior, At: r.now()})
	return nil
}

// claudeParser accumulates terminal state from Claude Code's stream-json.
type claudeParser struct {
	final             string
	sessionID         string
	usage             Usage
	context           ContextUsage
	succeeded         bool
	done              bool
	messageSeq        int
	messageOpen       bool
	textStreaming     bool
	thinkingStreaming bool
}

type claudeEnvelope struct {
	Type      string            `json:"type"`
	Subtype   string            `json:"subtype"`
	SessionID string            `json:"session_id"`
	Message   claudeMessage     `json:"message"`
	Result    string            `json:"result"`
	IsError   bool              `json:"is_error"`
	Usage     claudeUsage       `json:"usage"`
	Event     claudeStreamEvent `json:"event"`
	// ModelUsage is keyed by model id and is the only place `claude -p`
	// reports the context window — and only on the terminal result event, so
	// the window is unknown for the whole run that needed it.
	ModelUsage map[string]struct {
		ContextWindow int `json:"contextWindow"`
	} `json:"modelUsage"`
}

type claudeStreamEvent struct {
	Type  string            `json:"type"`
	Index int               `json:"index"`
	Delta claudeStreamDelta `json:"delta"`
}

type claudeStreamDelta struct {
	Type     string `json:"type"`
	Text     string `json:"text"`
	Thinking string `json:"thinking"`
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
	InputTokens              int `json:"input_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
	OutputTokens             int `json:"output_tokens"`
}

// contextTokens is everything the model read for this call. The cache-read
// portion is billed differently but still occupies the window, so leaving it
// out reports a nearly empty context for a long cached session: a real probe
// showed input_tokens=2 against 26,219 tokens actually resident.
func (u claudeUsage) contextTokens() int {
	return u.InputTokens + u.CacheCreationInputTokens + u.CacheReadInputTokens
}

func (u claudeUsage) usage() Usage {
	return Usage{
		InputTokens:              u.InputTokens + u.CacheCreationInputTokens + u.CacheReadInputTokens,
		CachedInputTokens:        u.CacheReadInputTokens,
		CacheCreationInputTokens: u.CacheCreationInputTokens,
		OutputTokens:             u.OutputTokens,
	}
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
		p.observeAssistantUsage(env.Message.Usage, line, at, sink)
	case "stream_event":
		p.handleStreamEvent(env.Event, line, at, sink)
	case "user":
		p.handleToolResults(env.Message, line, at, sink)
	case "result":
		p.handleResult(env, line, at, sink)
	default:
		// rate_limit_event and friends: keep raw, no user-facing text.
	}
}

// observeAssistantUsage publishes the per-message accounting Claude attaches to
// every assistant event. Without it the desktop learns nothing until the run
// ends, which is exactly when a live occupancy gauge stops being useful.
func (p *claudeParser) observeAssistantUsage(u claudeUsage, line string, at time.Time, sink Sink) {
	tokens := u.contextTokens()
	if tokens == 0 && u.OutputTokens == 0 {
		return
	}
	// Each assistant message reports its own call, so the step total is a sum,
	// while occupancy is the newest prompt and simply replaces the last one.
	turn := u.usage()
	p.usage.InputTokens += turn.InputTokens
	p.usage.CachedInputTokens += turn.CachedInputTokens
	p.usage.CacheCreationInputTokens += turn.CacheCreationInputTokens
	p.usage.OutputTokens += turn.OutputTokens
	p.context.Tokens = tokens
	usage := p.usage
	context := p.context
	sink(Event{Kind: KindUsage, Usage: &usage, Context: &context, Raw: line, At: at})
}

func (p *claudeParser) beginMessage() {
	p.messageSeq++
	p.messageOpen = true
	p.textStreaming = false
	p.thinkingStreaming = false
}

func (p *claudeParser) ensureMessage() {
	if !p.messageOpen {
		p.beginMessage()
	}
}

func (p *claudeParser) streamID(kind string) string {
	p.ensureMessage()
	return "claude-" + kind + "-" + strconv.Itoa(p.messageSeq)
}

func (p *claudeParser) handleStreamEvent(event claudeStreamEvent, line string, at time.Time, sink Sink) {
	switch event.Type {
	case "message_start":
		p.beginMessage()
	case "content_block_delta":
		switch event.Delta.Type {
		case "text_delta":
			id := p.streamID("message")
			if !p.textStreaming {
				p.textStreaming = true
				sink(Event{Kind: KindMessage, StreamID: id, Phase: StreamStart, Raw: line, At: at})
			}
			if event.Delta.Text != "" {
				sink(Event{Kind: KindMessage, StreamID: id, Phase: StreamDelta, Text: event.Delta.Text, Raw: line, At: at})
			}
		case "thinking_delta":
			id := p.streamID("thinking")
			if !p.thinkingStreaming {
				p.thinkingStreaming = true
				sink(Event{Kind: KindReasoning, StreamID: id, Phase: StreamStart, Raw: line, At: at})
			}
			if event.Delta.Thinking != "" {
				sink(Event{Kind: KindReasoning, StreamID: id, Phase: StreamDelta, Text: event.Delta.Thinking, Raw: line, At: at})
			}
		}
	}
}

func (p *claudeParser) handleAssistant(msg claudeMessage, line string, at time.Time, sink Sink) {
	p.ensureMessage()
	var text, thinking strings.Builder
	for _, c := range msg.Content {
		switch c.Type {
		case "text":
			text.WriteString(c.Text)
		case "thinking":
			thinking.WriteString(c.Thinking)
		case "tool_use":
			sink(Event{Kind: KindToolUse, Text: claudeToolText(c), Raw: line, At: at})
		}
	}
	if value := thinking.String(); strings.TrimSpace(value) != "" {
		if p.thinkingStreaming {
			sink(Event{Kind: KindReasoning, StreamID: p.streamID("thinking"), Phase: StreamEnd, Text: value, Raw: line, At: at})
		} else {
			sink(Event{Kind: KindReasoning, Text: value, Raw: line, At: at})
		}
	}
	if value := text.String(); strings.TrimSpace(value) != "" {
		p.final = value
		if p.textStreaming {
			sink(Event{Kind: KindMessage, StreamID: p.streamID("message"), Phase: StreamEnd, Text: value, Raw: line, At: at})
		} else {
			sink(Event{Kind: KindMessage, Text: value, Raw: line, At: at})
		}
	}
	p.messageOpen = false
	p.textStreaming = false
	p.thinkingStreaming = false
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
	// The result event carries the authoritative step total, so it supersedes
	// what was accumulated from the assistant messages.
	p.usage = env.Usage.usage()
	for _, model := range env.ModelUsage {
		if model.ContextWindow > p.context.Window {
			p.context.Window = model.ContextWindow
		}
	}
	if tokens := env.Usage.contextTokens(); tokens > 0 && p.context.Tokens == 0 {
		p.context.Tokens = tokens
	}
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
		Context:      p.context,
		SessionID:    p.sessionID,
		Succeeded:    p.succeeded && p.done,
	}
}
