package agentrun

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
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
	// contextWindows caches the model catalog's window column. Pi names the
	// model it used on every turn but never its size, so the size has to come
	// from `pi --list-models` — a local registry read that costs no model
	// quota. It is fetched on the first turn that names a model rather than at
	// launch, so a run that never reports one never pays for the lookup.
	contextOnce    sync.Once
	contextWindows map[string]int
}

// contextWindow resolves a model reported on the wire to its window. Pi reports
// a bare id ("deepseek-v4-flash") while the catalog is keyed by the
// unambiguous "provider/id" form, so both spellings are accepted.
func (r *PiRunner) contextWindow(provider, model string) int {
	model = strings.TrimSpace(model)
	if model == "" {
		return 0
	}
	r.contextOnce.Do(func() {
		r.contextWindows = make(map[string]int)
		output, err := exec.Command(r.binary, "--list-models").CombinedOutput()
		if err != nil {
			return
		}
		for _, entry := range parsePiModelList(string(output)) {
			if entry.ContextWindow <= 0 {
				continue
			}
			r.contextWindows[strings.ToLower(entry.Model)] = entry.ContextWindow
			// The bare id is ambiguous when two providers serve the same
			// name, so it is only a fallback and never overwrites a
			// qualified entry.
			bare := strings.ToLower(entry.DisplayName)
			if _, taken := r.contextWindows[bare]; !taken {
				r.contextWindows[bare] = entry.ContextWindow
			}
		}
	})
	if provider != "" {
		if window, ok := r.contextWindows[strings.ToLower(provider+"/"+model)]; ok {
			return window
		}
	}
	return r.contextWindows[strings.ToLower(model)]
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

// ListSkills asks Pi's RPC resource loader for the effective slash-command
// catalog, then keeps only Skills. This honors Pi's configured paths,
// packages, project precedence, and enableSkillCommands setting without
// starting a model turn.
func (r *PiRunner) ListSkills(ctx context.Context, cwd string, environment []string) ([]Skill, error) {
	cmd := exec.CommandContext(ctx, r.binary, "--mode", "rpc", "--no-session")
	if strings.TrimSpace(cwd) != "" {
		cmd.Dir = cwd
	}
	cmd.Env = environment
	cmd.Stdin = strings.NewReader("{\"type\":\"get_commands\"}\n")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("list Pi skills: %w%s", err, stderrSuffix(stderr.String()))
	}

	decoder := json.NewDecoder(&stdout)
	for {
		var response struct {
			Type    string `json:"type"`
			Command string `json:"command"`
			Success bool   `json:"success"`
			Data    struct {
				Commands []struct {
					Name        string `json:"name"`
					Description string `json:"description"`
					Source      string `json:"source"`
					Location    string `json:"location"`
					Path        string `json:"path"`
					SourceInfo  struct {
						Path  string `json:"path"`
						Scope string `json:"scope"`
					} `json:"sourceInfo"`
				} `json:"commands"`
			} `json:"data"`
		}
		if err := decoder.Decode(&response); err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("decode Pi skills: %w", err)
		}
		if response.Type != "response" || response.Command != "get_commands" {
			continue
		}
		if !response.Success {
			return nil, fmt.Errorf("Pi get_commands failed%s", stderrSuffix(stderr.String()))
		}
		items := make([]Skill, 0, len(response.Data.Commands))
		for _, command := range response.Data.Commands {
			if command.Source != "skill" || !strings.HasPrefix(command.Name, "skill:") {
				continue
			}
			name := strings.TrimPrefix(command.Name, "skill:")
			path, scope := command.SourceInfo.Path, command.SourceInfo.Scope
			if path == "" {
				path = command.Path
			}
			if scope == "" {
				scope = command.Location
			}
			items = append(items, Skill{Name: name, DisplayName: name, Description: command.Description, Path: path, Scope: scope})
		}
		sort.Slice(items, func(i, j int) bool { return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name) })
		return items, nil
	}
	return nil, fmt.Errorf("Pi RPC ended before skills were available%s", stderrSuffix(stderr.String()))
}

func stderrSuffix(stderr string) string {
	stderr = strings.TrimSpace(stderr)
	if stderr == "" {
		return ""
	}
	return ": " + stderr
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
	return streamProcess(ctx, cmd, &piParser{contextWindow: r.contextWindow}, r.now, sink)
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
	return append(args, adaptSkillMentions(req.Prompt, "/skill:"))
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
	// Model and Provider appear on the turn-ending message. Pi names what it
	// ran but not how big that model's window is.
	Model    string `json:"model"`
	Provider string `json:"provider"`
}

type piParser struct {
	// contextWindow resolves the model pi names on a turn to its size. Nil in
	// tests that exercise parsing alone.
	contextWindow func(provider, model string) int
	context       ContextUsage
	sessionID     string
	finalMessage  string
	usage         Usage
	usageSeen     bool
	succeeded     bool
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
	// Occupancy is the prompt pi just sent, which is exactly the input total
	// above: a cached prefix still occupies the window.
	p.context.Tokens = usage.InputTokens
	if p.context.Window == 0 && p.contextWindow != nil {
		p.context.Window = p.contextWindow(message.Provider, message.Model)
	}
	context := p.context
	sink(Event{Kind: KindUsage, Usage: &usage, Context: &context, Raw: line, At: at})
}

func (p *piParser) result() Result {
	return Result{
		FinalMessage: strings.TrimSpace(p.finalMessage),
		Usage:        p.usage,
		Context:      p.context,
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

// piThinkingLevels are the values pi accepts for --thinking. They are fixed by
// the CLI rather than by the selected model, so they need no discovery.
var piThinkingLevels = []string{"off", "minimal", "low", "medium", "high", "xhigh"}

// InspectConfiguration lists the models the installed Pi can reach.
//
// `pi --list-models` reads pi's own provider registry and prints a padded
// table without starting a session or spending model quota. A Pi with no
// provider credentials prints guidance instead of rows, which surfaces here as
// "no models" rather than as a hard failure of the probe.
func (r *PiRunner) InspectConfiguration(ctx context.Context, cwd string, environment []string) (HarnessConfiguration, error) {
	cmd := exec.CommandContext(ctx, r.binary, "--list-models")
	if cwd != "" {
		cmd.Dir = cwd
	}
	cmd.Env = environment
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return HarnessConfiguration{}, fmt.Errorf("read Pi models: %w%s", err, streamTail(stderr.Bytes()))
	}
	// Pi 0.73 prints the whole table on stderr, so a stdout-only read finds no
	// rows and reports a signed-in Pi as signed out. stdout is still read
	// first — a build that prints the table there keeps its models.json
	// warnings out of the parse — and stderr is consulted only when stdout
	// yielded nothing.
	models := parsePiModelList(stdout.String())
	if len(models) == 0 {
		models = parsePiModelList(stderr.String())
	}
	if len(models) == 0 {
		return HarnessConfiguration{}, fmt.Errorf("Pi did not advertise any models; sign in to a provider first")
	}
	// Pi's thinking levels come from the CLI, not from the model, so they are
	// reported once for the whole harness rather than per model.
	return HarnessConfiguration{Models: models, Efforts: piThinkingLevels}, nil
}

// piColumnGap splits the padded table: every column is padded to width and
// joined with two spaces, so two or more spaces separate fields while a single
// space can still appear inside one.
var piColumnGap = regexp.MustCompile(`\s{2,}`)

// parsePiModelList reads the `provider model context max-out thinking images`
// table. Model ids are reported as `provider/id`, the form pi's --model flag
// accepts unambiguously when two providers serve the same model name.
func parsePiModelList(output string) []HarnessModel {
	models := make([]HarnessModel, 0)
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
		window := 0
		if len(fields) > 2 {
			window = parsePiContextWindow(fields[2])
		}
		models = append(models, HarnessModel{Model: model, DisplayName: id, Description: provider, ContextWindow: window})
	}
	return models
}

// parsePiContextWindow reads the table's `context` column, which pi prints for
// people rather than for parsers: "1M", "200K", occasionally a bare count. An
// unreadable cell yields zero, which reads downstream as "window unknown"
// rather than as an empty context.
func parsePiContextWindow(value string) int {
	text := strings.ToUpper(strings.TrimSpace(stripANSI(value)))
	if text == "" {
		return 0
	}
	multiplier := 1
	switch {
	case strings.HasSuffix(text, "M"):
		multiplier, text = 1_000_000, strings.TrimSuffix(text, "M")
	case strings.HasSuffix(text, "K"):
		multiplier, text = 1_000, strings.TrimSuffix(text, "K")
	}
	amount, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
	if err != nil || amount <= 0 {
		return 0
	}
	return int(amount * float64(multiplier))
}

// ansiEscape matches the SGR sequences chalk emits when it believes it is
// writing to a terminal.
var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(value string) string { return ansiEscape.ReplaceAllString(value, "") }
