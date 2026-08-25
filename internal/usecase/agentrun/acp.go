package agentrun

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// The Agent Client Protocol is a JSON-RPC 2.0 conversation over stdio in which
// the client drives the agent through `initialize` -> `session/new` ->
// `session/prompt`, and the agent streams `session/update` notifications back
// until the prompt response arrives.
//
// Unlike the per-runtime JSONL parsers, nothing here is harness-specific: an
// adapter supplies only the command to launch and how its flags express model,
// effort, and sandbox. Grok Build is the first ACP harness OneCatch drives;
// others plug in by filling out an [acpLaunch].
const (
	acpProtocolVersion = 1

	acpIDInitialize = 1
	acpIDSession    = 2
	acpIDPrompt     = 3
)

// acpLaunch is everything the shared client needs to drive one harness. The
// adapter owns the command line; the client owns the protocol.
type acpLaunch struct {
	// runtime and displayName identify the harness in events and errors.
	runtime     Runtime
	displayName string
	// binary is the executable to run.
	binary string
	// contextWindow resolves the model's window from the initialize result.
	// ACP does not standardize a window any more than it standardizes usage,
	// so the harness that knows where its own handshake keeps it supplies the
	// reader; the client stays protocol-generic. Nil means the harness does
	// not report one, which reads downstream as "unknown".
	contextWindow func(initializeResult json.RawMessage, model string) int
	// command builds the whole invocation for one request.
	//
	// It returns the complete argument vector rather than a suffix appended to
	// a fixed prefix: a harness whose subcommand takes no options needs its
	// model and effort flags placed before that subcommand, which a suffix
	// cannot express. It returns an error when the request asks for something
	// the harness cannot express, so a run fails before it starts rather than
	// silently dropping the constraint.
	command func(req Request) (acpCommand, error)
}

// acpCommand is one harness invocation.
type acpCommand struct {
	// args is the complete argument vector, subcommands included.
	args []string
	// environment is applied over the request's environment. Some harnesses
	// only expose a setting — a sandbox profile, for instance — as a variable
	// rather than as a flag on the subcommand being launched.
	environment []string
}

// acpClient holds the state of one ACP conversation.
type acpClient struct {
	launch  acpLaunch
	now     nowFunc
	encoder *json.Encoder
	remote  *acpRemoteBridge
	// writeMu guards stdin: notifications are answered from the read loop, but
	// a permission response may be written while a prompt request is in flight.
	writeMu sync.Mutex

	sessionID    string
	finalMessage strings.Builder
	usage        Usage
	context      ContextUsage
	usageSeen    bool
	succeeded    bool
	completed    bool

	// toolTitles remembers each tool call's human label so a later
	// tool_call_update, which carries only the id, can still be described.
	toolTitles map[string]string
}

// acpEnvelope is one JSON-RPC frame in either direction.
type acpEnvelope struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  json.RawMessage `json:"error,omitempty"`
}

// acpUpdate is the payload of a session/update notification. Only the fields
// OneCatch renders are modelled; the rest survives in the event's Raw line.
type acpUpdate struct {
	SessionUpdate string `json:"sessionUpdate"`
	Content       *struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content,omitempty"`
	ToolCallID string          `json:"toolCallId,omitempty"`
	Title      string          `json:"title,omitempty"`
	Kind       string          `json:"kind,omitempty"`
	Status     string          `json:"status,omitempty"`
	RawInput   json.RawMessage `json:"rawInput,omitempty"`
	Locations  []struct {
		Path string `json:"path"`
	} `json:"locations,omitempty"`
	// StopReason and AgentResult appear on the harness's turn-completion
	// update. Grok spells them snake_case on its proprietary notification.
	StopReason  string `json:"stop_reason,omitempty"`
	AgentResult string `json:"agent_result,omitempty"`
	Message     string `json:"message,omitempty"`
}

// acpPermissionOption is one choice the agent offers for a blocked tool call.
type acpPermissionOption struct {
	OptionID string `json:"optionId"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
}

// runACPSession drives one prompt to completion over ACP.
func runACPSession(ctx context.Context, launch acpLaunch, req Request, sink Sink, now nowFunc) (Result, error) {
	if now == nil {
		now = time.Now
	}
	if sink == nil {
		sink = func(Event) {}
	}
	command, err := launch.command(req)
	if err != nil {
		return Result{}, err
	}

	var remote *acpRemoteBridge
	if req.Remote != nil {
		req, err = prepareRemoteRequest(req)
		if err != nil {
			return Result{}, err
		}
		remote, err = newACPRemoteBridge(ctx, *req.Remote, req.Sandbox)
		if err != nil {
			return Result{}, fmt.Errorf("open %s remote workspace: %w", launch.displayName, err)
		}
		defer remote.Close()
	}

	cmd := exec.CommandContext(ctx, launch.binary, command.args...)
	cmd.Dir = req.Workspace
	cmd.Env = req.Environment
	for _, entry := range command.environment {
		key, value, found := strings.Cut(entry, "=")
		if !found {
			continue
		}
		cmd.Env = setEnvironmentValue(cmd.Env, key, value)
	}
	if req.InterruptGrace > 0 {
		cmd.Cancel = func() error { return cmd.Process.Signal(os.Interrupt) }
		cmd.WaitDelay = req.InterruptGrace
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return Result{}, fmt.Errorf("%s stdin: %w", launch.displayName, err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return Result{}, fmt.Errorf("%s stdout: %w", launch.displayName, err)
	}
	var stderr lineCapture
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return Result{}, fmt.Errorf("start %s: %w", launch.displayName, err)
	}
	defer stopACPServer(cmd, stdin)

	client := &acpClient{
		launch:     launch,
		now:        now,
		encoder:    json.NewEncoder(stdin),
		remote:     remote,
		toolTitles: make(map[string]string),
	}
	capabilities := map[string]any{
		"fs":       map[string]bool{"readTextFile": false, "writeTextFile": false},
		"terminal": false,
	}
	if remote != nil {
		capabilities = remote.capabilities()
	}
	if err := client.send(map[string]any{
		"jsonrpc": "2.0", "id": acpIDInitialize, "method": "initialize",
		"params": map[string]any{
			"protocolVersion": acpProtocolVersion,
			"clientInfo":      map[string]string{"name": "onecatch", "title": "OneCatch", "version": "0.1.0"},
			// A local harness already runs inside its requested sandbox and must
			// not get a second, unsandboxed I/O path. For a remote run the bridge
			// is that sandbox boundary: its advertised operations are redirected
			// to the selected target and never touch the local harness workspace.
			"clientCapabilities": capabilities,
		},
	}); err != nil {
		return Result{}, fmt.Errorf("initialize %s: %w", launch.displayName, err)
	}

	sessionOpened := false
	scanner := newJSONLineScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		var envelope acpEnvelope
		if err := json.Unmarshal([]byte(line), &envelope); err != nil {
			// A non-JSON line is harness chatter, not protocol. Keep it as
			// reasoning rather than failing the run over it.
			sink(Event{Kind: KindReasoning, Text: strings.TrimSpace(line), Raw: line, At: now()})
			continue
		}
		// A frame with both a method and an id is a request *from* the agent.
		if envelope.Method != "" && len(envelope.ID) > 0 {
			client.handleRequest(ctx, envelope, req, line, sink)
			continue
		}
		if envelope.Method != "" {
			client.handleNotification(envelope, line, sink)
			continue
		}
		switch acpResponseID(envelope.ID) {
		case acpIDInitialize:
			if len(envelope.Error) > 0 {
				return Result{}, fmt.Errorf("initialize %s: %s", launch.displayName, envelope.Error)
			}
			if launch.contextWindow != nil {
				client.context.Window = launch.contextWindow(envelope.Result, req.Model)
			}
			cwd := req.Workspace
			if remote != nil {
				cwd = remote.root
			}
			method, params := "session/new", map[string]any{"cwd": cwd, "mcpServers": []any{}}
			if req.ResumeSessionID != "" {
				method = "session/load"
				params["sessionId"] = req.ResumeSessionID
			}
			if err := client.send(map[string]any{"jsonrpc": "2.0", "id": acpIDSession, "method": method, "params": params}); err != nil {
				return Result{}, err
			}
		case acpIDSession:
			if len(envelope.Error) > 0 {
				return Result{}, fmt.Errorf("open %s session: %s", launch.displayName, envelope.Error)
			}
			var response struct {
				SessionID string `json:"sessionId"`
			}
			if err := json.Unmarshal(envelope.Result, &response); err != nil {
				return Result{}, fmt.Errorf("decode %s session: %w", launch.displayName, err)
			}
			// session/load replays an existing session and may answer without
			// echoing the id back; the requested one is still authoritative.
			if response.SessionID == "" {
				response.SessionID = req.ResumeSessionID
			}
			if response.SessionID == "" {
				return Result{}, fmt.Errorf("decode %s session: missing session id", launch.displayName)
			}
			sessionOpened = true
			client.sessionID = response.SessionID
			sink(Event{Kind: KindStarted, Text: response.SessionID, Raw: line, At: now()})
			prompt := req.Prompt
			if remote != nil {
				prompt = remote.prompt(prompt)
			}
			if err := client.send(map[string]any{
				"jsonrpc": "2.0", "id": acpIDPrompt, "method": "session/prompt",
				"params": map[string]any{
					"sessionId": response.SessionID,
					"prompt":    []map[string]string{{"type": "text", "text": prompt}},
				},
			}); err != nil {
				return Result{}, err
			}
		case acpIDPrompt:
			if len(envelope.Error) > 0 {
				client.completed = true
				client.succeeded = false
				sink(Event{Kind: KindError, Text: acpErrorText(envelope.Error), Raw: line, At: now()})
				return client.result(), nil
			}
			var response struct {
				StopReason string          `json:"stopReason"`
				Meta       json.RawMessage `json:"_meta,omitempty"`
			}
			_ = json.Unmarshal(envelope.Result, &response)
			// Grok publishes the authoritative turn usage on the prompt
			// response, not on a standard session/update notification. Record it
			// before the result so both the terminal event and returned Result
			// carry the same totals.
			client.recordUsage(response.Meta, line, sink)
			client.finishTurn(response.StopReason, "", line, sink)
			return client.result(), nil
		}
	}

	if err := scanner.Err(); err != nil && ctx.Err() == nil {
		return client.result(), fmt.Errorf("read %s: %w", launch.displayName, err)
	}
	if ctx.Err() != nil {
		return client.result(), ctx.Err()
	}
	if !sessionOpened {
		return Result{}, fmt.Errorf("%s ended before its session opened%s", launch.displayName, stderr.tail())
	}
	return client.result(), fmt.Errorf("%s ended before the turn completed%s", launch.displayName, stderr.tail())
}

func (c *acpClient) send(payload any) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return c.encoder.Encode(payload)
}

// handleNotification translates one agent-initiated notification. Methods
// outside the protocol — the `_`-prefixed vendor extensions harnesses layer on
// top — are ignored rather than surfaced, since the standard session/update
// already carries the same content.
func (c *acpClient) handleNotification(envelope acpEnvelope, line string, sink Sink) {
	if envelope.Method != "session/update" {
		return
	}
	var params struct {
		SessionID string          `json:"sessionId"`
		Update    acpUpdate       `json:"update"`
		Meta      json.RawMessage `json:"_meta,omitempty"`
	}
	if err := json.Unmarshal(envelope.Params, &params); err != nil {
		sink(Event{Kind: KindError, Text: fmt.Sprintf("decode %s update: %v", c.launch.displayName, err), Raw: line, At: c.now()})
		return
	}
	update := params.Update
	at := c.now()
	switch update.SessionUpdate {
	case "agent_message_chunk":
		text := acpContentText(update)
		if text == "" {
			return
		}
		c.finalMessage.WriteString(text)
		sink(Event{Kind: KindMessage, Text: text, StreamID: c.streamID("message"), Phase: StreamDelta, Raw: line, At: at})
	case "agent_thought_chunk":
		text := acpContentText(update)
		if text == "" {
			return
		}
		sink(Event{Kind: KindReasoning, Text: text, StreamID: c.streamID("reasoning"), Phase: StreamDelta, Raw: line, At: at})
	case "tool_call":
		title := strings.TrimSpace(update.Title)
		if title == "" {
			title = update.Kind
		}
		if update.ToolCallID != "" {
			c.toolTitles[update.ToolCallID] = title
		}
		sink(Event{Kind: KindToolUse, Text: acpToolText(title, update.RawInput), StreamID: c.streamID("tool-" + update.ToolCallID), Raw: line, At: at})
		for _, location := range update.Locations {
			if location.Path != "" {
				sink(Event{Kind: KindFileChange, Text: location.Path, Raw: line, At: at})
			}
		}
	case "tool_call_update":
		// Only a settled tool call is a result; in-progress updates are just
		// the same call moving along and would double every entry.
		if update.Status != "completed" && update.Status != "failed" {
			return
		}
		title := c.toolTitles[update.ToolCallID]
		if title == "" {
			title = update.ToolCallID
		}
		sink(Event{
			Kind:     KindToolResult,
			Text:     title,
			StreamID: c.streamID("tool-" + update.ToolCallID),
			Failed:   update.Status == "failed",
			Raw:      line, At: at,
		})
	case "plan":
		sink(Event{Kind: KindReasoning, Text: acpRawText(envelope.Params), Raw: line, At: at})
	case "turn_completed":
		c.finishTurn(update.StopReason, update.AgentResult, line, sink)
	case "retry_state":
		if update.Message != "" {
			sink(Event{Kind: KindError, Text: update.Message, Raw: line, At: at})
		}
	}
	c.recordUsage(params.Meta, line, sink)
}

func (c *acpClient) recordUsage(raw json.RawMessage, line string, sink Sink) {
	usage, ok := acpUsage(raw)
	if !ok {
		return
	}
	c.usage = usage
	c.usageSeen = true
	// Occupancy is the prompt behind this accounting — a cached prefix still
	// occupies the window — and it replaces rather than accumulates.
	c.context.Tokens = usage.InputTokens
	context := c.context
	sink(Event{Kind: KindUsage, Usage: &usage, Context: &context, Raw: line, At: c.now()})
}

// handleRequest answers a request the agent sent us. Every request must get a
// response or the agent blocks forever, so the default branch replies with a
// method-not-found error rather than dropping the frame.
func (c *acpClient) handleRequest(ctx context.Context, envelope acpEnvelope, req Request, line string, sink Sink) {
	if envelope.Method == "session/request_permission" {
		c.handlePermission(ctx, envelope, req, line, sink)
		return
	}
	if c.remote != nil && c.remote.supports(envelope.Method) {
		// terminal/wait_for_exit is intentionally blocking. Dispatch remote
		// client requests away from the read loop so the agent can continue to
		// send notifications and independent requests while a command runs.
		go c.handleRemoteRequest(ctx, envelope)
		return
	}
	_ = c.send(map[string]any{
		"jsonrpc": "2.0", "id": json.RawMessage(envelope.ID),
		"error": map[string]any{"code": -32601, "message": "method not supported by OneCatch"},
	})
}

func (c *acpClient) handleRemoteRequest(ctx context.Context, envelope acpEnvelope) {
	result, rpcErr := c.remote.handle(ctx, envelope.Method, envelope.Params)
	payload := map[string]any{
		"jsonrpc": "2.0",
		"id":      json.RawMessage(envelope.ID),
	}
	if rpcErr != nil {
		payload["error"] = rpcErr
	} else {
		payload["result"] = result
	}
	_ = c.send(payload)
}

// handlePermission bridges the agent's blocking approval request to the host's
// permission handler, mapping ACP's option list onto OneCatch's allow/deny
// decision. Without a handler the run is non-interactive, so the request is
// resolved from the sandbox: a read-only run must never auto-approve.
func (c *acpClient) handlePermission(ctx context.Context, envelope acpEnvelope, req Request, line string, sink Sink) {
	var params struct {
		SessionID string `json:"sessionId"`
		ToolCall  struct {
			ToolCallID string          `json:"toolCallId"`
			Title      string          `json:"title"`
			Kind       string          `json:"kind"`
			RawInput   json.RawMessage `json:"rawInput"`
		} `json:"toolCall"`
		Options []acpPermissionOption `json:"options"`
	}
	if err := json.Unmarshal(envelope.Params, &params); err != nil {
		_ = c.send(map[string]any{
			"jsonrpc": "2.0", "id": json.RawMessage(envelope.ID),
			"error": map[string]any{"code": -32602, "message": "invalid permission request"},
		})
		return
	}

	allowOnce := acpOptionID(params.Options, "allow_once", "allow_always")
	allowAlways := acpOptionID(params.Options, "allow_always")
	rejectOnce := acpOptionID(params.Options, "reject_once", "reject_always")

	request := PermissionRequest{
		ID:          fmt.Sprintf("%s-%s", c.launch.runtime, strings.Trim(string(envelope.ID), `"`)),
		ToolUseID:   params.ToolCall.ToolCallID,
		ToolName:    params.ToolCall.Title,
		Title:       params.ToolCall.Title,
		DisplayName: params.ToolCall.Title,
		Description: params.ToolCall.Kind,
		// ACP offers no rule payload to persist, so "always allow" can only be
		// offered when the agent itself listed that option — and what it then
		// remembers, and for how long, is the agent's business, not ours.
		SuppressAlwaysAllow: allowAlways == "",
	}
	if len(params.ToolCall.RawInput) > 0 {
		_ = json.Unmarshal(params.ToolCall.RawInput, &request.Input)
	}

	decision := PermissionDecision{Behavior: "deny", Message: "Permission denied"}
	if req.PermissionHandler != nil {
		sink(Event{Kind: KindPermissionRequest, Text: request.Title, Permission: &request, Raw: line, At: c.now()})
		if resolved, err := req.PermissionHandler(ctx, request); err == nil {
			decision = resolved
		}
	} else if req.Sandbox != SandboxReadOnly {
		decision = PermissionDecision{Behavior: "allow", DecisionClassification: "auto_workspace"}
	}

	optionID := rejectOnce
	if decision.Behavior == "allow" {
		optionID = allowOnce
		if decision.DecisionClassification == "user_permanent" && allowAlways != "" {
			optionID = allowAlways
		}
	}
	if optionID == "" {
		// The agent offered nothing matching the decision; cancelling is the
		// only honest answer and leaves the tool call unexecuted.
		_ = c.send(map[string]any{
			"jsonrpc": "2.0", "id": json.RawMessage(envelope.ID),
			"result": map[string]any{"outcome": map[string]any{"outcome": "cancelled"}},
		})
		return
	}
	if req.PermissionHandler != nil {
		sink(Event{
			Kind: KindPermissionResolved, Text: request.Title, Permission: &request,
			PermissionDecision: decision.Behavior, Raw: line, At: c.now(),
		})
	}
	_ = c.send(map[string]any{
		"jsonrpc": "2.0", "id": json.RawMessage(envelope.ID),
		"result": map[string]any{"outcome": map[string]any{"outcome": "selected", "optionId": optionID}},
	})
}

// finishTurn records the terminal outcome once. The turn can be announced both
// by a turn_completed update and by the prompt response, and the first one wins
// so a later, less specific stop reason cannot overwrite it.
func (c *acpClient) finishTurn(stopReason, agentResult, line string, sink Sink) {
	if c.completed {
		return
	}
	c.completed = true
	// "end_turn" is ACP's clean finish; "cancelled", "refusal", "error", and
	// "max_tokens" all mean the agent stopped short of doing the work.
	c.succeeded = stopReason == "end_turn" || stopReason == ""
	if agentResult != "" && c.finalMessage.Len() == 0 {
		c.finalMessage.WriteString(agentResult)
	}
	if !c.succeeded {
		text := agentResult
		if text == "" {
			text = fmt.Sprintf("%s stopped: %s", c.launch.displayName, stopReason)
		}
		sink(Event{Kind: KindError, Text: text, Raw: line, At: c.now()})
	}
	event := Event{Kind: KindResult, Text: c.finalMessage.String(), Failed: !c.succeeded, Raw: line, At: c.now()}
	if c.usageSeen {
		usage := c.usage
		event.Usage = &usage
	}
	sink(event)
}

func (c *acpClient) result() Result {
	return Result{
		FinalMessage: strings.TrimSpace(c.finalMessage.String()),
		Usage:        c.usage,
		Context:      c.context,
		SessionID:    c.sessionID,
		Succeeded:    c.succeeded,
	}
}

func (c *acpClient) streamID(suffix string) string {
	return string(c.launch.runtime) + "-" + suffix
}

// acpOptionID returns the id of the first offered option whose kind matches one
// of kinds, in the order given, so a caller can express a preference and a
// fallback in one call.
func acpOptionID(options []acpPermissionOption, kinds ...string) string {
	for _, kind := range kinds {
		for _, option := range options {
			if option.Kind == kind {
				return option.OptionID
			}
		}
	}
	return ""
}

func acpContentText(update acpUpdate) string {
	if update.Content == nil {
		return ""
	}
	return update.Content.Text
}

func acpToolText(title string, rawInput json.RawMessage) string {
	title = strings.TrimSpace(title)
	if len(rawInput) == 0 {
		return title
	}
	var input map[string]any
	if err := json.Unmarshal(rawInput, &input); err != nil {
		return title
	}
	// Shell commands are the one input worth spelling out inline; anything
	// else is better summarized by its title than by a JSON blob.
	for _, key := range []string{"command", "cmd", "script"} {
		if value, ok := input[key].(string); ok && strings.TrimSpace(value) != "" {
			if title == "" {
				return value
			}
			return title + ": " + value
		}
	}
	return title
}

func acpRawText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	return strings.TrimSpace(string(raw))
}

func acpErrorText(raw json.RawMessage) string {
	var failure struct {
		Message string `json:"message"`
		Data    struct {
			Message string `json:"message"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &failure); err != nil {
		return strings.TrimSpace(string(raw))
	}
	if failure.Data.Message != "" {
		return failure.Data.Message
	}
	if failure.Message != "" {
		return failure.Message
	}
	return strings.TrimSpace(string(raw))
}

// acpUsage reads token accounting out of an ACP `_meta` object. ACP itself does
// not standardize usage, so this accepts the field spellings harnesses actually
// use. Grok nests its final counters under `usage`; accepting direct counters as
// a fallback keeps the parser compatible with harness notifications too.
func acpUsage(raw json.RawMessage) (Usage, bool) {
	if len(raw) == 0 {
		return Usage{}, false
	}
	var container struct {
		Usage json.RawMessage `json:"usage"`
	}
	if err := json.Unmarshal(raw, &container); err != nil {
		return Usage{}, false
	}
	counters := raw
	if len(container.Usage) > 0 && string(container.Usage) != "null" {
		counters = container.Usage
	}
	var fields struct {
		InputTokens              int `json:"inputTokens"`
		PromptTokens             int `json:"promptTokens"`
		OutputTokens             int `json:"outputTokens"`
		CompletionTokens         int `json:"completionTokens"`
		CachedInputTokens        int `json:"cachedInputTokens"`
		CachedReadTokens         int `json:"cachedReadTokens"`
		CacheReadTokens          int `json:"cacheReadTokens"`
		CacheReadInputTokens     int `json:"cacheReadInputTokens"`
		CacheCreationTokens      int `json:"cacheCreationTokens"`
		CacheCreationInputTokens int `json:"cacheCreationInputTokens"`
		ReasoningTokens          int `json:"reasoningTokens"`
		ReasoningOutputTokens    int `json:"reasoningOutputTokens"`
	}
	if err := json.Unmarshal(counters, &fields); err != nil {
		return Usage{}, false
	}
	usage := Usage{
		InputTokens:       firstNonZero(fields.InputTokens, fields.PromptTokens),
		OutputTokens:      firstNonZero(fields.OutputTokens, fields.CompletionTokens),
		CachedInputTokens: firstNonZero(fields.CachedInputTokens, fields.CachedReadTokens, fields.CacheReadTokens, fields.CacheReadInputTokens),
		CacheCreationInputTokens: firstNonZero(
			fields.CacheCreationTokens, fields.CacheCreationInputTokens,
		),
		ReasoningOutputTokens: firstNonZero(fields.ReasoningTokens, fields.ReasoningOutputTokens),
	}
	if usage == (Usage{}) {
		return Usage{}, false
	}
	return usage, true
}

func firstNonZero(values ...int) int {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}

func acpResponseID(raw json.RawMessage) int {
	if len(raw) == 0 {
		return 0
	}
	var id int
	if err := json.Unmarshal(raw, &id); err != nil {
		return 0
	}
	return id
}

// stopACPServer closes stdin so the agent can exit on its own, then kills it if
// it lingers. It mirrors the Codex app-server shutdown for the same reason: an
// ACP server that never sees EOF keeps its session resident.
func stopACPServer(cmd *exec.Cmd, stdin io.Closer) {
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
