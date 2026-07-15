package agentrun

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

// codexBinaryDefault is the CLI invoked when no override is configured.
const codexBinaryDefault = "codex"

// CodexRunner prefers the app-server JSON-RPC protocol so text and command
// output deltas are available. Older binaries fall back to `codex exec --json`
// before a thread is created, preventing accidental duplicate prompts.
type CodexRunner struct {
	// binary is the codex executable; overridable for tests.
	binary string
	now    nowFunc
}

// NewCodexRunner builds a runner driving the given codex binary. An empty
// binary falls back to "codex" resolved from PATH.
func NewCodexRunner(binary string) *CodexRunner {
	if binary == "" {
		binary = codexBinaryDefault
	}
	return &CodexRunner{binary: binary, now: time.Now}
}

func (r *CodexRunner) Runtime() Runtime { return RuntimeCodex }

func (r *CodexRunner) Available() bool {
	_, err := exec.LookPath(r.binary)
	return err == nil
}

func (r *CodexRunner) Run(ctx context.Context, req Request, sink Sink) (Result, error) {
	result, err := r.runAppServer(ctx, req, sink)
	if !errors.Is(err, errCodexAppServerUnavailable) {
		return result, err
	}
	// Older Codex builds do not expose app-server. Falling back is safe only
	// before the app-server handshake creates a thread, so the prompt cannot be
	// executed twice.
	return r.runExec(ctx, req, sink)
}

func (r *CodexRunner) runExec(ctx context.Context, req Request, sink Sink) (Result, error) {
	var args []string
	if req.ResumeSessionID != "" {
		// `codex exec resume` accepts a narrower flag set than a fresh exec:
		// no --sandbox / -C. The sandbox is set via a config override and the
		// working directory comes from the process cwd (cmd.Dir below), which
		// also lets codex match the recorded session by cwd.
		args = []string{"exec", "resume", req.ResumeSessionID, "--json", "--skip-git-repo-check",
			"-c", "sandbox_mode=" + string(codexSandbox(req.Sandbox))}
	} else {
		args = []string{"exec", "--json",
			// Workspaces are bare task directories, not git repos.
			"--skip-git-repo-check",
			"--sandbox", string(codexSandbox(req.Sandbox)),
			"-C", req.Workspace,
		}
	}
	if req.Model != "" {
		args = append(args, "-m", req.Model)
	}
	args = append(args, req.Prompt)

	cmd := exec.CommandContext(ctx, r.binary, args...)
	cmd.Dir = req.Workspace
	cmd.Env = req.Environment
	if req.InterruptGrace > 0 {
		cmd.Cancel = func() error { return cmd.Process.Signal(os.Interrupt) }
		cmd.WaitDelay = req.InterruptGrace
	}
	// Closing stdin avoids codex blocking on "reading additional input".
	cmd.Stdin = nil
	return streamProcess(ctx, cmd, &codexParser{}, r.now, sink)
}

var errCodexAppServerUnavailable = errors.New("Codex app-server is unavailable")

type codexAppEnvelope struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
	Result json.RawMessage `json:"result"`
	Error  json.RawMessage `json:"error"`
}

type codexAppParams struct {
	ThreadID   string       `json:"threadId"`
	TurnID     string       `json:"turnId"`
	ItemID     string       `json:"itemId"`
	Delta      string       `json:"delta"`
	Item       codexAppItem `json:"item"`
	Turn       codexAppTurn `json:"turn"`
	TokenUsage struct {
		Last  codexTokenUsageBreakdown `json:"last"`
		Total codexTokenUsageBreakdown `json:"total"`
	} `json:"tokenUsage"`
}

type codexTokenUsageBreakdown struct {
	InputTokens           int `json:"inputTokens"`
	CachedInputTokens     int `json:"cachedInputTokens"`
	OutputTokens          int `json:"outputTokens"`
	ReasoningOutputTokens int `json:"reasoningOutputTokens"`
}

func (u codexTokenUsageBreakdown) empty() bool {
	return u.InputTokens == 0 && u.CachedInputTokens == 0 && u.OutputTokens == 0 && u.ReasoningOutputTokens == 0
}

func (u codexTokenUsageBreakdown) usage() Usage {
	return Usage{InputTokens: u.InputTokens, CachedInputTokens: u.CachedInputTokens, OutputTokens: u.OutputTokens, ReasoningOutputTokens: u.ReasoningOutputTokens}
}

type codexAppTurn struct {
	ID     string          `json:"id"`
	Status string          `json:"status"`
	Error  json.RawMessage `json:"error"`
}

type codexAppItem struct {
	ID               string          `json:"id"`
	Type             string          `json:"type"`
	Text             string          `json:"text"`
	Command          string          `json:"command"`
	Status           string          `json:"status"`
	AggregatedOutput *string         `json:"aggregatedOutput"`
	ExitCode         *int            `json:"exitCode"`
	Arguments        json.RawMessage `json:"arguments"`
	Result           json.RawMessage `json:"result"`
	Error            json.RawMessage `json:"error"`
	Server           string          `json:"server"`
	Tool             string          `json:"tool"`
	Content          []string        `json:"content"`
	Summary          []string        `json:"summary"`
	Changes          []struct {
		Path string `json:"path"`
	} `json:"changes"`
}

type codexAppState struct {
	final         string
	sessionID     string
	usage         Usage
	completed     bool
	failed        bool
	messageOpen   map[string]bool
	reasoningOpen map[string]bool
	toolOpen      map[string]bool
}

func newCodexAppState() *codexAppState {
	return &codexAppState{messageOpen: make(map[string]bool), reasoningOpen: make(map[string]bool), toolOpen: make(map[string]bool)}
}

func (r *CodexRunner) runAppServer(ctx context.Context, req Request, sink Sink) (Result, error) {
	if sink == nil {
		sink = func(Event) {}
	}
	cmd := exec.CommandContext(ctx, r.binary, "app-server", "--listen", "stdio://")
	cmd.Dir = req.Workspace
	cmd.Env = req.Environment
	if req.InterruptGrace > 0 {
		cmd.Cancel = func() error { return cmd.Process.Signal(os.Interrupt) }
		cmd.WaitDelay = req.InterruptGrace
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return Result{}, fmt.Errorf("Codex app-server stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return Result{}, fmt.Errorf("Codex app-server stdout: %w", err)
	}
	var stderr lineCapture
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return Result{}, fmt.Errorf("%w: %v", errCodexAppServerUnavailable, err)
	}
	defer stopCodexAppServer(cmd, stdin)

	encoder := json.NewEncoder(stdin)
	if err := encoder.Encode(map[string]any{
		"id": 1, "method": "initialize",
		"params": map[string]any{"clientInfo": map[string]string{"name": "oneshot", "title": "Oneshot", "version": "0.1.0"}},
	}); err != nil {
		return Result{}, fmt.Errorf("initialize Codex app-server: %w", err)
	}

	state := newCodexAppState()
	initialized := false
	threadStarted := false
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		var envelope codexAppEnvelope
		if err := json.Unmarshal([]byte(line), &envelope); err != nil {
			sink(Event{Kind: KindReasoning, Text: strings.TrimSpace(line), Raw: line, At: r.now()})
			continue
		}
		if envelope.Method != "" && len(envelope.ID) > 0 {
			_ = respondUnsupportedCodexRequest(encoder, envelope)
			continue
		}
		if responseID(envelope.ID) == 1 {
			if len(envelope.Error) > 0 {
				return Result{}, fmt.Errorf("%w: initialize failed: %s", errCodexAppServerUnavailable, envelope.Error)
			}
			initialized = true
			if err := encoder.Encode(map[string]any{"method": "initialized", "params": map[string]any{}}); err != nil {
				return Result{}, err
			}
			method := "thread/start"
			params := map[string]any{"cwd": req.Workspace, "approvalPolicy": "never", "sandbox": codexSandbox(req.Sandbox)}
			if req.ResumeSessionID != "" {
				method = "thread/resume"
				params["threadId"] = req.ResumeSessionID
			}
			if req.Model != "" {
				params["model"] = req.Model
			}
			if err := encoder.Encode(map[string]any{"id": 2, "method": method, "params": params}); err != nil {
				return Result{}, err
			}
			continue
		}
		if responseID(envelope.ID) == 2 {
			if len(envelope.Error) > 0 {
				return Result{}, fmt.Errorf("start Codex thread: %s", envelope.Error)
			}
			var response struct {
				Thread struct {
					ID string `json:"id"`
				} `json:"thread"`
			}
			if err := json.Unmarshal(envelope.Result, &response); err != nil {
				return Result{}, fmt.Errorf("decode Codex thread response: %w", err)
			}
			if response.Thread.ID == "" {
				return Result{}, errors.New("decode Codex thread response: missing thread id")
			}
			threadStarted = true
			state.sessionID = response.Thread.ID
			sink(Event{Kind: KindStarted, Text: response.Thread.ID, Raw: line, At: r.now()})
			if err := encoder.Encode(map[string]any{
				"id": 3, "method": "turn/start",
				"params": map[string]any{"threadId": response.Thread.ID, "input": []map[string]string{{"type": "text", "text": req.Prompt}}},
			}); err != nil {
				return Result{}, err
			}
			continue
		}
		if responseID(envelope.ID) == 3 {
			if len(envelope.Error) > 0 {
				return state.result(), fmt.Errorf("start Codex turn: %s", envelope.Error)
			}
			continue
		}
		if envelope.Method != "" {
			state.handleNotification(envelope.Method, envelope.Params, line, r.now(), sink)
			if state.completed || state.failed {
				break
			}
		}
	}
	if err := scanner.Err(); err != nil && ctx.Err() == nil {
		return state.result(), fmt.Errorf("read Codex app-server: %w", err)
	}
	if ctx.Err() != nil {
		return state.result(), ctx.Err()
	}
	if !initialized || !threadStarted {
		return Result{}, fmt.Errorf("%w%s", errCodexAppServerUnavailable, stderr.tail())
	}
	if !state.completed && !state.failed {
		return state.result(), fmt.Errorf("Codex app-server ended before turn completion%s", stderr.tail())
	}
	return state.result(), nil
}

func stopCodexAppServer(cmd *exec.Cmd, stdin io.Closer) {
	_ = stdin.Close()
	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		<-done
	}
}

func responseID(raw json.RawMessage) int {
	var id int
	_ = json.Unmarshal(raw, &id)
	return id
}

func respondUnsupportedCodexRequest(encoder *json.Encoder, envelope codexAppEnvelope) error {
	return encoder.Encode(map[string]any{
		"id":    envelope.ID,
		"error": map[string]any{"code": -32601, "message": "Oneshot does not handle interactive app-server requests"},
	})
}

func (s *codexAppState) handleNotification(method string, raw json.RawMessage, line string, at time.Time, sink Sink) {
	var params codexAppParams
	if json.Unmarshal(raw, &params) != nil {
		return
	}
	switch method {
	case "item/started":
		s.handleItemStarted(params.Item, line, at, sink)
	case "item/agentMessage/delta":
		id := codexStreamID("message", params.ItemID)
		if !s.messageOpen[params.ItemID] {
			s.messageOpen[params.ItemID] = true
			sink(Event{Kind: KindMessage, StreamID: id, Phase: StreamStart, Raw: line, At: at})
		}
		if params.Delta != "" {
			sink(Event{Kind: KindMessage, StreamID: id, Phase: StreamDelta, Text: params.Delta, Raw: line, At: at})
		}
	case "item/reasoning/textDelta", "item/reasoning/summaryTextDelta":
		id := codexStreamID("reasoning", params.ItemID)
		if !s.reasoningOpen[params.ItemID] {
			s.reasoningOpen[params.ItemID] = true
			sink(Event{Kind: KindReasoning, StreamID: id, Phase: StreamStart, Raw: line, At: at})
		}
		if params.Delta != "" {
			sink(Event{Kind: KindReasoning, StreamID: id, Phase: StreamDelta, Text: params.Delta, Raw: line, At: at})
		}
	case "item/commandExecution/outputDelta":
		id := codexStreamID("tool-output", params.ItemID)
		if !s.toolOpen[params.ItemID] {
			s.toolOpen[params.ItemID] = true
			sink(Event{Kind: KindToolResult, StreamID: id, Phase: StreamStart, Raw: line, At: at})
		}
		if params.Delta != "" {
			sink(Event{Kind: KindToolResult, StreamID: id, Phase: StreamDelta, Text: params.Delta, Raw: line, At: at})
		}
	case "item/completed":
		s.handleItemCompleted(params.Item, line, at, sink)
	case "thread/tokenUsage/updated":
		// `total` accumulates every model call in the agent turn and matches the
		// legacy `codex exec` turn.completed usage. `last` is only one sampling
		// call and would severely undercount tool-heavy steps.
		breakdown := params.TokenUsage.Total
		if breakdown.empty() {
			breakdown = params.TokenUsage.Last
		}
		s.usage = breakdown.usage()
	case "turn/completed":
		s.completed = params.Turn.Status == "completed"
		s.failed = params.Turn.Status == "failed" || params.Turn.Status == "interrupted"
		if s.failed {
			sink(Event{Kind: KindError, Text: codexRawText(params.Turn.Error), Raw: line, At: at})
		} else {
			sink(Event{Kind: KindUsage, Raw: line, At: at})
		}
	case "error":
		s.failed = true
		sink(Event{Kind: KindError, Text: string(raw), Raw: line, At: at})
	}
}

func (s *codexAppState) handleItemStarted(item codexAppItem, line string, at time.Time, sink Sink) {
	switch item.Type {
	case "agentMessage":
		id := codexStreamID("message", item.ID)
		s.messageOpen[item.ID] = true
		sink(Event{Kind: KindMessage, StreamID: id, Phase: StreamStart, Raw: line, At: at})
	case "reasoning":
		id := codexStreamID("reasoning", item.ID)
		s.reasoningOpen[item.ID] = true
		sink(Event{Kind: KindReasoning, StreamID: id, Phase: StreamStart, Raw: line, At: at})
	case "commandExecution":
		sink(Event{Kind: KindToolUse, Text: item.Command, Raw: line, At: at})
	case "mcpToolCall", "dynamicToolCall":
		sink(Event{Kind: KindToolUse, Text: codexAppToolText(item), Raw: line, At: at})
	}
}

func (s *codexAppState) handleItemCompleted(item codexAppItem, line string, at time.Time, sink Sink) {
	switch item.Type {
	case "agentMessage":
		s.final = item.Text
		if s.messageOpen[item.ID] {
			sink(Event{Kind: KindMessage, StreamID: codexStreamID("message", item.ID), Phase: StreamEnd, Text: item.Text, Raw: line, At: at})
			delete(s.messageOpen, item.ID)
		} else if strings.TrimSpace(item.Text) != "" {
			sink(Event{Kind: KindMessage, Text: item.Text, Raw: line, At: at})
		}
	case "reasoning":
		text := strings.Join(item.Summary, "\n")
		if text == "" {
			text = strings.Join(item.Content, "\n")
		}
		if s.reasoningOpen[item.ID] {
			sink(Event{Kind: KindReasoning, StreamID: codexStreamID("reasoning", item.ID), Phase: StreamEnd, Text: text, Raw: line, At: at})
			delete(s.reasoningOpen, item.ID)
		} else if strings.TrimSpace(text) != "" {
			sink(Event{Kind: KindReasoning, Text: text, Raw: line, At: at})
		}
	case "commandExecution":
		output := ""
		if item.AggregatedOutput != nil {
			output = *item.AggregatedOutput
		}
		failed := item.Status == "failed" || item.Status == "declined" || (item.ExitCode != nil && *item.ExitCode != 0)
		if s.toolOpen[item.ID] {
			sink(Event{Kind: KindToolResult, StreamID: codexStreamID("tool-output", item.ID), Phase: StreamEnd, Text: output, Failed: failed, Raw: line, At: at})
			delete(s.toolOpen, item.ID)
		} else {
			sink(Event{Kind: KindToolResult, Text: output, Failed: failed, Raw: line, At: at})
		}
	case "fileChange":
		paths := make([]string, 0, len(item.Changes))
		for _, change := range item.Changes {
			if change.Path != "" {
				paths = append(paths, change.Path)
			}
		}
		sink(Event{Kind: KindFileChange, Text: strings.Join(paths, "\n"), Failed: item.Status == "failed", Raw: line, At: at})
	case "mcpToolCall", "dynamicToolCall":
		text := codexRawText(item.Result)
		if text == "" {
			text = codexRawText(item.Error)
		}
		sink(Event{Kind: KindToolResult, Text: text, Failed: item.Status == "failed", Raw: line, At: at})
	}
}

func codexStreamID(kind, itemID string) string { return "codex-" + kind + "-" + itemID }

func codexAppToolText(item codexAppItem) string {
	name := item.Tool
	if item.Server != "" {
		name = item.Server + "." + name
	}
	if name == "" {
		name = "tool"
	}
	args := codexRawText(item.Arguments)
	if args == "" {
		return name
	}
	return name + " " + args
}

func codexRawText(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text
	}
	var value struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(raw, &value) == nil && value.Message != "" {
		return value.Message
	}
	return strings.TrimSpace(string(raw))
}

func (s *codexAppState) result() Result {
	return Result{FinalMessage: s.final, Usage: s.usage, SessionID: s.sessionID, Succeeded: s.completed && !s.failed}
}

// codexSandbox maps the engine's sandbox vocabulary onto codex's flag values.
func codexSandbox(s Sandbox) string {
	switch s {
	case SandboxReadOnly:
		return "read-only"
	case SandboxFull:
		return "danger-full-access"
	default:
		return "workspace-write"
	}
}

// codexParser accumulates terminal state from the codex JSONL stream.
type codexParser struct {
	final     string
	sessionID string
	usage     Usage
	completed bool
}

type codexEnvelope struct {
	Type     string          `json:"type"`
	ThreadID string          `json:"thread_id"`
	Item     codexItem       `json:"item"`
	Usage    codexUsage      `json:"usage"`
	Error    json.RawMessage `json:"error"`
}

type codexItem struct {
	Type    string `json:"type"`
	Text    string `json:"text"`
	Command string `json:"command"`
	Path    string `json:"path"`
	// Populated on a command_execution's terminal event. Status is one of
	// in_progress/completed/failed; AggregatedOutput is the combined
	// stdout+stderr; ExitCode is a pointer so a real 0 is distinguishable from
	// "not reported".
	Status           string `json:"status"`
	AggregatedOutput string `json:"aggregated_output"`
	ExitCode         *int   `json:"exit_code"`
}

// commandFailed reports whether a finished command_execution ended badly. A
// non-zero exit is the primary signal; status=="failed" covers commands codex
// could not even launch, where no exit code is reported.
func (i codexItem) commandFailed() bool {
	return i.Status == "failed" || (i.ExitCode != nil && *i.ExitCode != 0)
}

type codexUsage struct {
	InputTokens           int `json:"input_tokens"`
	CachedInputTokens     int `json:"cached_input_tokens"`
	OutputTokens          int `json:"output_tokens"`
	ReasoningOutputTokens int `json:"reasoning_output_tokens"`
}

func (p *codexParser) parse(line string, at time.Time, sink Sink) {
	var env codexEnvelope
	if err := json.Unmarshal([]byte(line), &env); err != nil {
		// Codex occasionally prints non-JSON banner lines; preserve them as
		// raw reasoning rather than failing the run.
		sink(Event{Kind: KindReasoning, Text: strings.TrimSpace(line), Raw: line, At: at})
		return
	}

	switch env.Type {
	case "thread.started":
		p.sessionID = env.ThreadID
		sink(Event{Kind: KindStarted, Text: env.ThreadID, Raw: line, At: at})
	case "turn.started":
		// Turn boundaries carry no user-facing payload.
	case "item.completed":
		p.handleItem(env.Item, line, at, sink, true)
	case "item.updated":
		p.handleItem(env.Item, line, at, sink, false)
	case "turn.completed":
		p.usage = Usage{InputTokens: env.Usage.InputTokens, CachedInputTokens: env.Usage.CachedInputTokens, OutputTokens: env.Usage.OutputTokens, ReasoningOutputTokens: env.Usage.ReasoningOutputTokens}
		p.completed = true
		sink(Event{Kind: KindUsage, Raw: line, At: at})
	case "error", "turn.failed", "thread.error":
		p.completed = false
		sink(Event{Kind: KindError, Text: codexErrorText(env, line), Raw: line, At: at})
	default:
		// Unknown event types are retained raw so nothing is silently lost.
		sink(Event{Kind: KindReasoning, Raw: line, At: at})
	}
}

func (p *codexParser) handleItem(item codexItem, line string, at time.Time, sink Sink, terminal bool) {
	switch item.Type {
	case "agent_message", "assistant_message":
		p.final = item.Text
		sink(Event{Kind: KindMessage, Text: item.Text, Raw: line, At: at})
	case "reasoning":
		sink(Event{Kind: KindReasoning, Text: item.Text, Raw: line, At: at})
	case "command_execution":
		// Codex reports a command across started/updated/completed phases. Act
		// only on the terminal event, emitting the invocation immediately
		// followed by its captured output, so the timeline shows one row whose
		// RESULT holds the command's output and whose state reflects its exit
		// code — rather than a bare command with the output silently dropped.
		if !terminal {
			return
		}
		sink(Event{Kind: KindToolUse, Text: item.Command, Raw: line, At: at})
		sink(Event{Kind: KindToolResult, Text: item.AggregatedOutput, Raw: line, Failed: item.commandFailed(), At: at})
	case "local_shell_call":
		sink(Event{Kind: KindToolUse, Text: item.Command, Raw: line, At: at})
	case "file_change", "patch_apply":
		sink(Event{Kind: KindFileChange, Text: item.Path, Raw: line, At: at})
	default:
		sink(Event{Kind: KindToolResult, Text: item.Text, Raw: line, At: at})
	}
}

func codexErrorText(env codexEnvelope, line string) string {
	if len(env.Error) > 0 {
		return strings.Trim(string(env.Error), `"`)
	}
	return line
}

func (p *codexParser) result() Result {
	return Result{
		FinalMessage: p.final,
		Usage:        p.usage,
		SessionID:    p.sessionID,
		Succeeded:    p.completed,
	}
}
