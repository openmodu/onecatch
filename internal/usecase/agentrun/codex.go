package agentrun

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// codexBinaryDefault is the CLI invoked when no override is configured.
const codexBinaryDefault = "codex"

// CodexRunner drives Codex through the app-server JSON-RPC protocol so text and
// command output deltas are available.
type CodexRunner struct {
	// binary is the codex executable; overridable for tests.
	binary     string
	now        nowFunc
	sessionsMu sync.Mutex
	sessions   map[string]*codexAppProcess
}

const codexAppServerIdleTimeout = 5 * time.Minute

// codexAppProcess owns one warm app-server connection. It is deliberately
// retained per Codex thread instead of shared globally: the child process has
// a frozen environment, and sharing it across unrelated tasks would bypass
// OneCatch's per-runtime environment allowlist.
type codexAppProcess struct {
	mu        sync.Mutex
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	encoder   *json.Encoder
	scanner   *bufio.Scanner
	stderr    *lineCapture
	threadID  string
	key       string
	idleTimer *time.Timer
	stopOnce  sync.Once
}

// NewCodexRunner builds a runner driving the given codex binary. An empty
// binary falls back to "codex" resolved from PATH.
func NewCodexRunner(binary string) *CodexRunner {
	if binary == "" {
		binary = codexBinaryDefault
	}
	return &CodexRunner{binary: binary, now: time.Now, sessions: make(map[string]*codexAppProcess)}
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
		// ModelContextWindow is null for models whose window Codex does not
		// know, so it must stay a pointer: 0 and "unreported" are different
		// answers and only one of them may be drawn as a full gauge.
		ModelContextWindow *int `json:"modelContextWindow"`
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
	context       ContextUsage
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

// codexModelContextMax is the largest window each model accepts, mirroring the
// per-model table inside the Codex binary. Codex defaults every model to
// 272000 even where the model itself allows more, and app-server's model/list
// reports neither figure — its Model objects carry reasoning efforts and
// service tiers but nothing about context — so raising the window means
// carrying the ceiling here.
//
// A model absent from this table is left at Codex's default rather than
// guessed at: the failure mode of guessing high is every request erroring
// against the real limit, while the failure mode of not knowing is only that
// the window stays where Codex put it. Models whose ceiling already equals the
// default (gpt-5.5, gpt-5.4-mini, gpt-5.2) are deliberately absent — there is
// nothing to raise, and listing them would imply otherwise.
//
// Read out of codex-cli 0.149.0.
var codexModelContextMax = map[string]int{
	"gpt-5.6-sol":   872000,
	"gpt-5.6-terra": 872000,
	"gpt-5.6-luna":  872000,
	"gpt-5.4":       1000000,
}

// codexMaxContextWindowOverride returns the `-c` override that opts a model
// into its full window, or false when the model is unknown, unnamed, or has no
// headroom. An unnamed model cannot be resolved without asking app-server what
// its configured default is, and that answer arrives too late to appear on the
// launch command.
func codexMaxContextWindowOverride(model string) (string, bool) {
	maximum, known := codexModelContextMax[strings.TrimSpace(model)]
	if !known {
		return "", false
	}
	return fmt.Sprintf("model_context_window=%d", maximum), true
}

func (r *CodexRunner) runAppServer(ctx context.Context, req Request, sink Sink) (Result, error) {
	if sink == nil {
		sink = func(Event) {}
	}
	commandArgs := []string{"app-server", "--listen", "stdio://"}
	if req.ServiceTier == "fast" {
		commandArgs = append(commandArgs, "-c", "features.fast_mode=true")
	}
	if req.MaxContextWindow {
		if override, ok := codexMaxContextWindowOverride(req.Model); ok {
			commandArgs = append(commandArgs, "-c", override)
		}
	}
	environment := req.Environment
	if req.Remote != nil {
		var err error
		req, err = prepareRemoteRequest(req)
		if err != nil {
			return Result{}, err
		}
		remote, err := setupRemoteCodex(req)
		if err != nil {
			return Result{}, err
		}
		defer remote.cleanup()
		commandArgs = append(commandArgs, remote.args...)
		environment = mergeEnvironment(environment, remote.env)
	}

	// Workflow turns have a durable run ID and may be resumed later. Keep that
	// thread's local app-server alive so the next message does not pay the CLI,
	// plugin and skill cold-start cost again. Standalone helpers (for example
	// title generation) and remote seams remain one-shot processes.
	cacheable := req.RunID != "" && req.Remote == nil
	processKey := codexAppProcessKey(req.Workspace, commandArgs, environment)
	if cacheable && strings.TrimSpace(req.ResumeSessionID) != "" {
		if process := r.takeSession(strings.TrimSpace(req.ResumeSessionID), processKey); process != nil {
			result, err := r.runCodexTurn(ctx, process, req, sink, true)
			if err == nil && result.Succeeded {
				r.keepSession(result.SessionID, process)
				return result, nil
			}
			r.discardProcess(process)
			process.stop()
			return result, err
		}
	}

	process, err := r.startCodexAppProcess(req, commandArgs, environment)
	if err != nil {
		return Result{}, err
	}
	keepAlive := false
	defer func() {
		if !keepAlive {
			process.stop()
		}
	}()
	result, err := r.runCodexTurn(ctx, process, req, sink, false)
	if err == nil && result.Succeeded && cacheable && result.SessionID != "" {
		keepAlive = true
		r.keepSession(result.SessionID, process)
	}
	return result, err
}

func (r *CodexRunner) startCodexAppProcess(req Request, commandArgs, environment []string) (*codexAppProcess, error) {
	cmd := exec.Command(r.binary, commandArgs...)
	cmd.Dir = req.Workspace
	cmd.Env = environment
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("Codex app-server stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("Codex app-server stdout: %w", err)
	}
	stderr := &lineCapture{}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start Codex app-server: %w", err)
	}
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	return &codexAppProcess{
		cmd: cmd, stdin: stdin, encoder: json.NewEncoder(stdin), scanner: scanner,
		stderr: stderr, key: codexAppProcessKey(req.Workspace, commandArgs, environment),
	}, nil
}

func (r *CodexRunner) runCodexTurn(ctx context.Context, process *codexAppProcess, req Request, sink Sink, warm bool) (Result, error) {
	process.mu.Lock()
	defer process.mu.Unlock()

	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			process.stopWithGrace(req.InterruptGrace)
		case <-done:
		}
	}()
	defer close(done)

	state := newCodexAppState()
	initialized := warm
	threadStarted := warm
	turnStarted := false
	activeTurnID := ""
	if warm {
		state.sessionID = process.threadID
		sink(Event{Kind: KindStarted, Text: process.threadID, At: r.now()})
		if err := sendCodexTurnStart(process.encoder, req, process.threadID); err != nil {
			return Result{}, err
		}
	} else if err := process.encoder.Encode(map[string]any{
		"id": 1, "method": "initialize",
		"params": map[string]any{"clientInfo": map[string]string{"name": "onecatch", "title": "OneCatch", "version": "0.1.0"}},
	}); err != nil {
		return Result{}, fmt.Errorf("initialize Codex app-server: %w", err)
	}

	for process.scanner.Scan() {
		line := process.scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		var envelope codexAppEnvelope
		if err := json.Unmarshal([]byte(line), &envelope); err != nil {
			sink(Event{Kind: KindReasoning, Text: strings.TrimSpace(line), Raw: line, At: r.now()})
			continue
		}
		if envelope.Method != "" && len(envelope.ID) > 0 {
			_ = respondUnsupportedCodexRequest(process.encoder, envelope)
			continue
		}
		if responseID(envelope.ID) == 1 {
			if len(envelope.Error) > 0 {
				return Result{}, fmt.Errorf("initialize Codex app-server: %s", envelope.Error)
			}
			initialized = true
			if err := process.encoder.Encode(map[string]any{"method": "initialized", "params": map[string]any{}}); err != nil {
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
			if err := process.encoder.Encode(map[string]any{"id": 2, "method": method, "params": params}); err != nil {
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
			process.threadID = response.Thread.ID
			sink(Event{Kind: KindStarted, Text: response.Thread.ID, Raw: line, At: r.now()})
			if err := sendCodexTurnStart(process.encoder, req, response.Thread.ID); err != nil {
				return Result{}, err
			}
			continue
		}
		if responseID(envelope.ID) == 3 {
			if len(envelope.Error) > 0 {
				return state.result(), fmt.Errorf("start Codex turn: %s", envelope.Error)
			}
			var response struct {
				Turn struct {
					ID string `json:"id"`
				} `json:"turn"`
			}
			_ = json.Unmarshal(envelope.Result, &response)
			activeTurnID = response.Turn.ID
			turnStarted = true
			continue
		}
		if envelope.Method != "" {
			if !turnStarted || !codexNotificationMatches(envelope.Params, state.sessionID, activeTurnID) {
				continue
			}
			state.handleNotification(envelope.Method, envelope.Params, line, r.now(), sink)
			if state.completed || state.failed {
				break
			}
		}
	}
	if err := process.scanner.Err(); err != nil && ctx.Err() == nil {
		return state.result(), fmt.Errorf("read Codex app-server: %w", err)
	}
	if ctx.Err() != nil {
		return state.result(), ctx.Err()
	}
	if !initialized || !threadStarted {
		return Result{}, fmt.Errorf("Codex app-server ended before thread started%s", process.stderr.tail())
	}
	if !state.completed && !state.failed {
		return state.result(), fmt.Errorf("Codex app-server ended before turn completion%s", process.stderr.tail())
	}
	return state.result(), nil
}

func sendCodexTurnStart(encoder *json.Encoder, req Request, threadID string) error {
	turnParams := map[string]any{"threadId": threadID, "input": []map[string]string{{"type": "text", "text": req.Prompt}}}
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
	return encoder.Encode(map[string]any{"id": 3, "method": "turn/start", "params": turnParams})
}

func codexNotificationMatches(raw json.RawMessage, threadID, turnID string) bool {
	var ids struct {
		ThreadID string `json:"threadId"`
		TurnID   string `json:"turnId"`
	}
	if json.Unmarshal(raw, &ids) != nil {
		return true
	}
	if ids.ThreadID != "" && threadID != "" && ids.ThreadID != threadID {
		return false
	}
	return ids.TurnID == "" || turnID == "" || ids.TurnID == turnID
}

func codexAppProcessKey(workspace string, args, environment []string) string {
	hash := sha256.New()
	for _, value := range append(append([]string{workspace}, args...), environment...) {
		_, _ = io.WriteString(hash, value)
		_, _ = hash.Write([]byte{0})
	}
	return fmt.Sprintf("%x", hash.Sum(nil))
}

func (r *CodexRunner) takeSession(sessionID, key string) *codexAppProcess {
	r.sessionsMu.Lock()
	defer r.sessionsMu.Unlock()
	process := r.sessions[sessionID]
	if process == nil || process.key != key {
		return nil
	}
	if process.idleTimer != nil {
		process.idleTimer.Stop()
		process.idleTimer = nil
	}
	// While a turn is using the process it is not idle and must not remain
	// reachable by the timer callback or a second resume call.
	delete(r.sessions, sessionID)
	return process
}

func (r *CodexRunner) keepSession(sessionID string, process *codexAppProcess) {
	if sessionID == "" {
		return
	}
	r.sessionsMu.Lock()
	if previous := r.sessions[sessionID]; previous != nil && previous != process {
		go previous.stop()
	}
	r.sessions[sessionID] = process
	if process.idleTimer != nil {
		process.idleTimer.Stop()
	}
	process.idleTimer = time.AfterFunc(codexAppServerIdleTimeout, func() {
		r.sessionsMu.Lock()
		shouldStop := false
		if r.sessions[sessionID] == process {
			delete(r.sessions, sessionID)
			shouldStop = true
		}
		r.sessionsMu.Unlock()
		if shouldStop {
			process.stop()
		}
	})
	r.sessionsMu.Unlock()
}

func (r *CodexRunner) discardProcess(process *codexAppProcess) {
	r.sessionsMu.Lock()
	for sessionID, candidate := range r.sessions {
		if candidate == process {
			delete(r.sessions, sessionID)
		}
	}
	if process.idleTimer != nil {
		process.idleTimer.Stop()
		process.idleTimer = nil
	}
	r.sessionsMu.Unlock()
}

// Close stops warm app-server processes. CodexRunner also closes each process
// after an idle timeout, so callers that do not own runner lifecycle remain
// bounded.
func (r *CodexRunner) Close() error {
	r.sessionsMu.Lock()
	processes := make([]*codexAppProcess, 0, len(r.sessions))
	for _, process := range r.sessions {
		if process.idleTimer != nil {
			process.idleTimer.Stop()
		}
		processes = append(processes, process)
	}
	r.sessions = make(map[string]*codexAppProcess)
	r.sessionsMu.Unlock()
	for _, process := range processes {
		process.stop()
	}
	return nil
}

func (p *codexAppProcess) stop() {
	p.stopWithGrace(2 * time.Second)
}

func (p *codexAppProcess) stopWithGrace(grace time.Duration) {
	p.stopOnce.Do(func() {
		if grace <= 0 {
			grace = 2 * time.Second
		}
		if p.cmd.Process != nil {
			_ = p.cmd.Process.Signal(os.Interrupt)
		}
		_ = p.stdin.Close()
		done := make(chan struct{})
		go func() {
			_ = p.cmd.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(grace):
			if p.cmd.Process != nil {
				_ = p.cmd.Process.Kill()
			}
			<-done
		}
	})
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
		// Occupancy is a different question from cost and takes a different
		// number. `total` is every call in the turn added up; the window only
		// ever held the newest prompt, so `last.InputTokens` — which already
		// includes the cached prefix — is what actually sits in the window.
		if params.TokenUsage.ModelContextWindow != nil {
			s.context.Window = *params.TokenUsage.ModelContextWindow
		}
		if !params.TokenUsage.Last.empty() {
			s.context.Tokens = params.TokenUsage.Last.InputTokens
		}
		usage := s.usage
		context := s.context
		s.usageEmitted = true
		sink(Event{Kind: KindUsage, Usage: &usage, Context: &context, Raw: line, At: at})
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
	return Result{FinalMessage: s.final, Usage: s.usage, Context: s.context, SessionID: s.sessionID, Succeeded: s.completed && !s.failed}
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
