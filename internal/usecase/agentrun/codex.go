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

// CodexRunner drives Codex through the app-server JSON-RPC protocol so text and
// command output deltas are available.
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

type CodexServiceTier struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type CodexModelInfo struct {
	ID                     string             `json:"id"`
	Model                  string             `json:"model"`
	DisplayName            string             `json:"displayName"`
	Description            string             `json:"description,omitempty"`
	DefaultReasoningEffort string             `json:"defaultReasoningEffort"`
	ReasoningEfforts       []string           `json:"reasoningEfforts"`
	ServiceTiers           []CodexServiceTier `json:"serviceTiers,omitempty"`
	DefaultServiceTier     string             `json:"defaultServiceTier,omitempty"`
	IsDefault              bool               `json:"isDefault"`
}

type CodexConfiguration struct {
	Model           string           `json:"model,omitempty"`
	ReasoningEffort string           `json:"reasoningEffort,omitempty"`
	ServiceTier     string           `json:"serviceTier,omitempty"`
	Models          []CodexModelInfo `json:"models"`
}

// InspectConfiguration reads Codex's effective configuration and model catalog
// through app-server without starting a thread or consuming model quota.
func (r *CodexRunner) InspectConfiguration(ctx context.Context, cwd string, environment []string) (CodexConfiguration, error) {
	cmd := exec.CommandContext(ctx, r.binary, "app-server", "--listen", "stdio://")
	if cwd != "" {
		cmd.Dir = cwd
	}
	cmd.Env = environment
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return CodexConfiguration{}, fmt.Errorf("Codex app-server stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return CodexConfiguration{}, fmt.Errorf("Codex app-server stdout: %w", err)
	}
	var stderr lineCapture
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return CodexConfiguration{}, fmt.Errorf("start Codex app-server: %w", err)
	}
	defer stopCodexAppServer(cmd, stdin)

	encoder := json.NewEncoder(stdin)
	if err := encoder.Encode(map[string]any{
		"id": 1, "method": "initialize",
		"params": map[string]any{
			"clientInfo":   map[string]string{"name": "onecatch", "title": "OneCatch", "version": "0.1.0"},
			"capabilities": map[string]any{"experimentalApi": true, "requestAttestation": false},
		},
	}); err != nil {
		return CodexConfiguration{}, fmt.Errorf("initialize Codex app-server: %w", err)
	}

	var configuration CodexConfiguration
	gotConfig, gotModels := false, false
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		var envelope codexAppEnvelope
		if json.Unmarshal([]byte(line), &envelope) != nil {
			continue
		}
		if envelope.Method != "" && len(envelope.ID) > 0 {
			_ = respondUnsupportedCodexRequest(encoder, envelope)
			continue
		}
		switch responseID(envelope.ID) {
		case 1:
			if len(envelope.Error) > 0 {
				return CodexConfiguration{}, fmt.Errorf("initialize Codex app-server: %s", envelope.Error)
			}
			if err := encoder.Encode(map[string]any{"method": "initialized", "params": map[string]any{}}); err != nil {
				return CodexConfiguration{}, err
			}
			if err := encoder.Encode(map[string]any{"id": 2, "method": "config/read", "params": map[string]any{"includeLayers": false}}); err != nil {
				return CodexConfiguration{}, err
			}
			if err := encoder.Encode(map[string]any{"id": 3, "method": "model/list", "params": map[string]any{"includeHidden": false}}); err != nil {
				return CodexConfiguration{}, err
			}
		case 2:
			if len(envelope.Error) > 0 {
				return CodexConfiguration{}, fmt.Errorf("read Codex configuration: %s", envelope.Error)
			}
			var response struct {
				Config struct {
					Model           string `json:"model"`
					ReasoningEffort string `json:"model_reasoning_effort"`
					ServiceTier     string `json:"service_tier"`
				} `json:"config"`
			}
			if err := json.Unmarshal(envelope.Result, &response); err != nil {
				return CodexConfiguration{}, fmt.Errorf("decode Codex configuration: %w", err)
			}
			configuration.Model = response.Config.Model
			configuration.ReasoningEffort = response.Config.ReasoningEffort
			configuration.ServiceTier = response.Config.ServiceTier
			gotConfig = true
		case 3:
			if len(envelope.Error) > 0 {
				return CodexConfiguration{}, fmt.Errorf("list Codex models: %s", envelope.Error)
			}
			var response struct {
				Data []struct {
					ID                     string `json:"id"`
					Model                  string `json:"model"`
					DisplayName            string `json:"displayName"`
					Description            string `json:"description"`
					DefaultReasoningEffort string `json:"defaultReasoningEffort"`
					SupportedEfforts       []struct {
						ReasoningEffort string `json:"reasoningEffort"`
					} `json:"supportedReasoningEfforts"`
					ServiceTiers       []CodexServiceTier `json:"serviceTiers"`
					DefaultServiceTier string             `json:"defaultServiceTier"`
					IsDefault          bool               `json:"isDefault"`
				} `json:"data"`
			}
			if err := json.Unmarshal(envelope.Result, &response); err != nil {
				return CodexConfiguration{}, fmt.Errorf("decode Codex models: %w", err)
			}
			configuration.Models = make([]CodexModelInfo, 0, len(response.Data))
			for _, item := range response.Data {
				model := CodexModelInfo{ID: item.ID, Model: item.Model, DisplayName: item.DisplayName, Description: item.Description, DefaultReasoningEffort: item.DefaultReasoningEffort, ServiceTiers: item.ServiceTiers, DefaultServiceTier: item.DefaultServiceTier, IsDefault: item.IsDefault}
				for _, effort := range item.SupportedEfforts {
					model.ReasoningEfforts = append(model.ReasoningEfforts, effort.ReasoningEffort)
				}
				configuration.Models = append(configuration.Models, model)
			}
			gotModels = true
		}
		if gotConfig && gotModels {
			return configuration, nil
		}
	}
	if err := scanner.Err(); err != nil && ctx.Err() == nil {
		return CodexConfiguration{}, fmt.Errorf("read Codex app-server: %w", err)
	}
	if ctx.Err() != nil {
		return CodexConfiguration{}, ctx.Err()
	}
	return CodexConfiguration{}, fmt.Errorf("Codex app-server ended before configuration was available%s", stderr.tail())
}

func (r *CodexRunner) Run(ctx context.Context, req Request, sink Sink) (Result, error) {
	return r.runAppServer(ctx, req, sink)
}

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
	usageEmitted  bool
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
	commandArgs := []string{"app-server", "--listen", "stdio://"}
	if req.ServiceTier == "fast" {
		commandArgs = append(commandArgs, "-c", "features.fast_mode=true")
	}
	cmd := exec.CommandContext(ctx, r.binary, commandArgs...)
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
		return Result{}, fmt.Errorf("start Codex app-server: %w", err)
	}
	defer stopCodexAppServer(cmd, stdin)

	encoder := json.NewEncoder(stdin)
	if err := encoder.Encode(map[string]any{
		"id": 1, "method": "initialize",
		"params": map[string]any{"clientInfo": map[string]string{"name": "onecatch", "title": "OneCatch", "version": "0.1.0"}},
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
				return Result{}, fmt.Errorf("initialize Codex app-server: %s", envelope.Error)
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
			if req.ServiceTier != "" {
				if req.ServiceTier == "standard" {
					params["serviceTier"] = nil
				} else {
					params["serviceTier"] = req.ServiceTier
				}
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
			turnParams := map[string]any{"threadId": response.Thread.ID, "input": []map[string]string{{"type": "text", "text": req.Prompt}}}
			if req.Model != "" {
				turnParams["model"] = req.Model
			}
			if req.ReasoningEffort != "" {
				turnParams["effort"] = req.ReasoningEffort
			}
			if req.ServiceTier != "" {
				if req.ServiceTier == "standard" {
					turnParams["serviceTier"] = nil
				} else {
					turnParams["serviceTier"] = req.ServiceTier
				}
			}
			if err := encoder.Encode(map[string]any{
				"id": 3, "method": "turn/start",
				"params": turnParams,
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
		return Result{}, fmt.Errorf("Codex app-server ended before thread started%s", stderr.tail())
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
		"error": map[string]any{"code": -32601, "message": "OneCatch does not handle interactive app-server requests"},
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
		// `total` accumulates every model call in the agent turn. `last` is only
		// one sampling call and would severely undercount tool-heavy steps.
		breakdown := params.TokenUsage.Total
		if breakdown.empty() {
			breakdown = params.TokenUsage.Last
		}
		s.usage = breakdown.usage()
		usage := s.usage
		s.usageEmitted = true
		sink(Event{Kind: KindUsage, Usage: &usage, Raw: line, At: at})
	case "turn/completed":
		s.completed = params.Turn.Status == "completed"
		s.failed = params.Turn.Status == "failed" || params.Turn.Status == "interrupted"
		if s.failed {
			sink(Event{Kind: KindError, Text: codexRawText(params.Turn.Error), Raw: line, At: at})
		} else if !s.usageEmitted {
			// Older app-server versions may only make usage available at the end.
			usage := s.usage
			sink(Event{Kind: KindUsage, Usage: &usage, Raw: line, At: at})
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
