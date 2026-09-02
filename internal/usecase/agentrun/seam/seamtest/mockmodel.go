// Package seamtest provides the offline model server the seam conformance
// suites drive their harnesses with.
//
// It is a package rather than test-only code inside seam because two different
// packages need it: seam's own conformance suite, and agentrun's, which cannot
// import seam's tests and must not import agentrun in reverse.
package seamtest

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"
)

// Dialect selects which wire protocol the mock model speaks.
type Dialect string

const (
	// DialectAnthropic is the Anthropic Messages API streaming format, which
	// Claude Code speaks through its @anthropic-ai/sdk client. ANTHROPIC_BASE_URL
	// redirects it; the SDK appends /v1/messages to whatever it is given.
	DialectAnthropic Dialect = "anthropic"

	// DialectResponses is the OpenAI Responses API, which Codex speaks. It is
	// selected per run through -c model_providers.<id>.wire_api="responses".
	DialectResponses Dialect = "responses"
)

// Mock is a model server that scripts exactly one tool call and records what
// the harness reports back.
//
// It exists because the question this suite asks — does the harness's shell
// tool still route through our seam, and what shape does it arrive in — cannot
// be answered by reading a version string. It has to be observed. Observing it
// against a real model would need an API key and a bill, which means the check
// could never run in CI. Scripting the model keeps the harness, the part whose
// behaviour is actually in question, fully in the loop while removing the
// model, the network and the cost.
//
// The default script has two turns. Turn one answers the harness's first
// request with an instruction to run one command. Turn two arrives when the
// harness posts the command's output back; the mock stores it and replies with
// a plain assistant message so the harness finishes cleanly and exits. The
// explicit-tool constructor can script several calls before that final turn.
type Mock struct {
	srv        *httptest.Server
	dialect    Dialect
	command    string
	toolScript []ToolCall

	mu         sync.Mutex
	toolResult string
	hasResult  bool
	// tools is every tool name the harness advertised. It is the only view
	// anyone gets of the model's actual tool surface, which is what a deny
	// list is supposed to shrink — asserting on it is how we know a deny
	// still denies.
	tools    map[string]bool
	done     chan struct{}
	doneOnce sync.Once
}

// ToolCall is one scripted model-requested tool invocation.
type ToolCall struct {
	Name  string
	Input map[string]any
}

// StartMock starts a mock model on a random localhost port. command is what
// the scripted turn instructs the harness to run.
func StartMock(dialect Dialect, command string) *Mock {
	m := &Mock{
		dialect: dialect, command: command,
		tools: map[string]bool{}, done: make(chan struct{}),
	}
	m.srv = httptest.NewServer(m)
	return m
}

// StartMockWithTool scripts one explicit tool invocation. It is primarily for
// conformance checks that must exercise a native file tool rather than the
// shell tool selected by StartMock.
func StartMockWithTool(dialect Dialect, toolName string, input map[string]any) *Mock {
	return StartMockWithTools(dialect, []ToolCall{{Name: toolName, Input: input}})
}

// StartMockWithTools scripts an ordered series of tool invocations. Each tool
// result advances to the next call, and the following request ends the turn.
func StartMockWithTools(dialect Dialect, script []ToolCall) *Mock {
	m := &Mock{
		dialect: dialect, toolScript: append([]ToolCall(nil), script...),
		tools: map[string]bool{}, done: make(chan struct{}),
	}
	m.srv = httptest.NewServer(m)
	return m
}

// BaseURL is what the harness should be pointed at.
//
// The two clients disagree about who owns the /v1 segment: the Anthropic SDK
// appends it to the base URL, while codex's provider config expects it to be
// part of the base URL already. Getting this wrong produces a 404 that reads
// like an authentication problem.
func (m *Mock) BaseURL() string {
	if m.dialect == DialectAnthropic {
		return m.srv.URL
	}
	return m.srv.URL + "/v1"
}

func (m *Mock) Close() { m.srv.Close() }

// Result returns the tool output the harness reported, if it reported any.
func (m *Mock) Result() (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.toolResult, m.hasResult
}

// Wait blocks until the scripted conversation completes or the bound elapses.
// It returns false on timeout, which means the harness never made the tool
// call — a conclusion about the harness, not about the seam.
func (m *Mock) Wait(timeout time.Duration) bool {
	select {
	case <-m.done:
		return true
	case <-time.After(timeout):
		return false
	}
}

// SawTool reports whether the harness advertised a tool by this name.
func (m *Mock) SawTool(name string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.tools[name]
}

func (m *Mock) recordTools(names []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, n := range names {
		if n != "" {
			m.tools[n] = true
		}
	}
}

func (m *Mock) record(out string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.hasResult {
		m.toolResult, m.hasResult = out, true
	}
}

func (m *Mock) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodPost && m.dialect == DialectAnthropic &&
		strings.HasSuffix(r.URL.Path, "/v1/messages"):
		m.serveAnthropic(w, r)
	case r.Method == http.MethodPost && m.dialect == DialectResponses &&
		strings.HasSuffix(r.URL.Path, "/responses"):
		m.serveResponses(w, r)
	default:
		http.Error(w, "seam mock: unhandled "+r.Method+" "+r.URL.Path, http.StatusNotFound)
	}
}

func startStream(w http.ResponseWriter) http.Flusher {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	f, _ := w.(http.Flusher)
	if f != nil {
		f.Flush()
	}
	return f
}

// writeEvent emits one server-sent event and flushes it. The harness consumes
// the stream incrementally, and an event sitting in a buffer reads to it as a
// model that has hung.
func (m *Mock) writeEvent(w http.ResponseWriter, f http.Flusher, eventType string, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, data); err != nil {
		return // the harness went away mid-stream
	}
	if f != nil {
		f.Flush()
	}
}

// --- Anthropic Messages API (Claude Code) ---------------------------------

type anthropicRequest struct {
	Messages []struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	} `json:"messages"`
	Tools []struct {
		Name string `json:"name"`
	} `json:"tools"`
}

func (m *Mock) serveAnthropic(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 16<<20))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var req anthropicRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "seam mock: malformed request: "+err.Error(), http.StatusBadRequest)
		return
	}
	resultCount := 0
	for _, msg := range req.Messages {
		if msg.Role != "user" {
			continue
		}
		var parts []struct {
			Type    string          `json:"type"`
			Content json.RawMessage `json:"content"`
		}
		if json.Unmarshal(msg.Content, &parts) != nil {
			continue
		}
		for _, p := range parts {
			if p.Type != "tool_result" || len(p.Content) == 0 {
				continue
			}
			var out string
			if json.Unmarshal(p.Content, &out) != nil {
				var textParts []struct {
					Text string `json:"text"`
				}
				if json.Unmarshal(p.Content, &textParts) == nil {
					var sb strings.Builder
					for _, tp := range textParts {
						sb.WriteString(tp.Text)
					}
					out = sb.String()
				}
			}
			m.record(out)
			resultCount++
		}
	}

	names := make([]string, 0, len(req.Tools))
	for _, t := range req.Tools {
		names = append(names, t.Name)
	}
	m.recordTools(names)

	_, hasResult := m.Result()
	finished := hasResult
	if len(m.toolScript) > 0 {
		finished = resultCount >= len(m.toolScript)
	}
	f := startStream(w)

	m.writeEvent(w, f, "message_start", map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"id": "msg_seam_1", "type": "message", "role": "assistant",
			"content": []any{}, "model": "seam-mock",
			"stop_reason": nil, "stop_sequence": nil,
			"usage": map[string]any{"input_tokens": 0, "output_tokens": 0},
		},
	})
	m.writeEvent(w, f, "ping", map[string]any{"type": "ping"})

	if !finished {
		name := pickAnthropicTool(req.Tools)
		input := map[string]any{"command": m.command}
		if len(m.toolScript) > 0 {
			call := m.toolScript[resultCount]
			name = call.Name
			input = call.Input
		}
		argJSON, _ := json.Marshal(input)
		m.writeEvent(w, f, "content_block_start", map[string]any{
			"type": "content_block_start", "index": 0,
			"content_block": map[string]any{
				"type": "tool_use", "id": fmt.Sprintf("toolu_seam_%d", resultCount+1),
				"name": name, "input": map[string]any{},
			},
		})
		m.writeEvent(w, f, "content_block_delta", map[string]any{
			"type": "content_block_delta", "index": 0,
			"delta": map[string]any{"type": "input_json_delta", "partial_json": string(argJSON)},
		})
		m.writeEvent(w, f, "content_block_stop", map[string]any{
			"type": "content_block_stop", "index": 0,
		})
		m.writeEvent(w, f, "message_delta", map[string]any{
			"type":  "message_delta",
			"delta": map[string]any{"stop_reason": "tool_use", "stop_sequence": nil},
			"usage": map[string]any{"output_tokens": 0},
		})
	} else {
		m.writeEvent(w, f, "content_block_start", map[string]any{
			"type": "content_block_start", "index": 0,
			"content_block": map[string]any{"type": "text", "text": ""},
		})
		m.writeEvent(w, f, "content_block_delta", map[string]any{
			"type": "content_block_delta", "index": 0,
			"delta": map[string]any{"type": "text_delta", "text": "done"},
		})
		m.writeEvent(w, f, "content_block_stop", map[string]any{
			"type": "content_block_stop", "index": 0,
		})
		m.writeEvent(w, f, "message_delta", map[string]any{
			"type":  "message_delta",
			"delta": map[string]any{"stop_reason": "end_turn", "stop_sequence": nil},
			"usage": map[string]any{"output_tokens": 0},
		})
		m.doneOnce.Do(func() { close(m.done) })
	}
	m.writeEvent(w, f, "message_stop", map[string]any{"type": "message_stop"})
}

// pickAnthropicTool chooses the shell tool from what the request advertises,
// rather than assuming a name. A harness that renames its shell tool between
// versions would otherwise fail this suite for a reason that has nothing to do
// with the seam.
func pickAnthropicTool(tools []struct {
	Name string `json:"name"`
}) string {
	advertised := map[string]bool{}
	for _, t := range tools {
		advertised[t.Name] = true
	}
	for _, known := range []string{"Bash", "bash", "shell"} {
		if advertised[known] {
			return known
		}
	}
	return "Bash"
}

// --- OpenAI Responses API (Codex) -----------------------------------------

type responsesRequest struct {
	Input []struct {
		Type   string          `json:"type"`
		Output json.RawMessage `json:"output"`
	} `json:"input"`
	Tools []struct {
		Type string `json:"type"`
		Name string `json:"name"`
	} `json:"tools"`
}

func (m *Mock) serveResponses(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 16<<20))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var req responsesRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "seam mock: malformed request: "+err.Error(), http.StatusBadRequest)
		return
	}
	for _, item := range req.Input {
		if item.Type != "function_call_output" || len(item.Output) == 0 {
			continue
		}
		var out string
		if json.Unmarshal(item.Output, &out) != nil {
			out = string(item.Output)
		}
		m.record(out)
	}

	names := make([]string, 0, len(req.Tools))
	for _, t := range req.Tools {
		names = append(names, t.Name)
	}
	m.recordTools(names)

	_, hasResult := m.Result()
	f := startStream(w)

	m.writeEvent(w, f, "response.created", map[string]any{
		"type": "response.created", "response": map[string]any{"id": "resp_seam_1"},
	})
	if !hasResult {
		name, args := m.pickCodexTool(req.Tools)
		m.writeEvent(w, f, "response.output_item.done", map[string]any{
			"type": "response.output_item.done",
			"item": map[string]any{
				"type": "function_call", "call_id": "call_seam_1",
				"name": name, "arguments": args,
			},
		})
	} else {
		m.writeEvent(w, f, "response.output_item.done", map[string]any{
			"type": "response.output_item.done",
			"item": map[string]any{
				"type": "message", "role": "assistant",
				"content": []map[string]any{{"type": "output_text", "text": "done"}},
			},
		})
		m.doneOnce.Do(func() { close(m.done) })
	}
	// The usage block is required by codex's stream parser even though every
	// counter is zero; omitting it strands the turn waiting for it.
	m.writeEvent(w, f, "response.completed", map[string]any{
		"type": "response.completed",
		"response": map[string]any{
			"id": "resp_seam_1",
			"usage": map[string]any{
				"input_tokens": 0, "input_tokens_details": nil,
				"output_tokens": 0, "output_tokens_details": nil,
				"total_tokens": 0,
			},
		},
	})
}

// pickCodexTool adapts to codex's shell tool, which is spelled differently
// across versions and behind feature flags: 0.148 advertises "exec_command",
// and disabling unified_exec swaps in "shell_command". Each takes its command
// under a different key, so the arguments are built per tool rather than
// guessed once.
func (m *Mock) pickCodexTool(tools []struct {
	Type string `json:"type"`
	Name string `json:"name"`
}) (name, arguments string) {
	advertised := map[string]bool{}
	var firstFunction string
	for _, t := range tools {
		if t.Type != "function" {
			continue
		}
		advertised[t.Name] = true
		if firstFunction == "" {
			firstFunction = t.Name
		}
	}
	known := []struct {
		name string
		args func(string) map[string]any
	}{
		{"exec_command", func(c string) map[string]any {
			return map[string]any{"cmd": c, "yield_time_ms": 1000, "max_output_tokens": 4000}
		}},
		{"shell_command", func(c string) map[string]any {
			return map[string]any{"command": c}
		}},
		{"shell", func(c string) map[string]any {
			return map[string]any{"command": c}
		}},
	}
	for _, k := range known {
		if advertised[k.name] {
			data, _ := json.Marshal(k.args(m.command))
			return k.name, string(data)
		}
	}
	data, _ := json.Marshal(map[string]any{"command": m.command})
	if firstFunction != "" {
		return firstFunction, string(data)
	}
	return "shell", string(data)
}
