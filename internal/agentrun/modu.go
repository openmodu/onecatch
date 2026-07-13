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

const (
	moduBinaryDefault = "modu_code"
	moduBinaryLegacy  = "modu-code"
)

// ModuRunner drives Modu Code through ACP JSON-RPC 2.0 LDJSON over stdio.
// The currently supported Modu Code keeps sessions only inside its process,
// so every Run creates a fresh ACP session and intentionally returns no
// resumable SessionID.
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

type acpEnvelope struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int64          `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *acpError       `json:"error,omitempty"`
}

type acpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (r *ModuRunner) Run(ctx context.Context, req Request, sink Sink) (Result, error) {
	if sink == nil {
		sink = func(Event) {}
	}
	// Current Modu Code exposes ACP explicitly. Writable workflows are already
	// authorized by Oneshot, so run them non-interactively; read-only workflows
	// keep approval enabled and reject risky reverse requests below. The legacy
	// modu-code binary ignores extra arguments, so these remain compatible.
	cmd := exec.CommandContext(ctx, r.binary, moduCommandArgs(req.Sandbox)...)
	cmd.Dir = req.Workspace
	cmd.Env = moduEnvironment(req.Environment, req.Model)
	if req.InterruptGrace > 0 {
		cmd.Cancel = func() error { return cmd.Process.Signal(os.Interrupt) }
		cmd.WaitDelay = req.InterruptGrace
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return Result{}, fmt.Errorf("modu stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return Result{}, fmt.Errorf("modu stdout pipe: %w", err)
	}
	var stderr lineCapture
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return Result{}, fmt.Errorf("start %s: %w", cmd.Path, err)
	}

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	request := func(id int64, method string, params any) error {
		frame := map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}
		encoded, marshalErr := json.Marshal(frame)
		if marshalErr != nil {
			return marshalErr
		}
		encoded = append(encoded, '\n')
		if _, writeErr := stdin.Write(encoded); writeErr != nil {
			return fmt.Errorf("write ACP %s: %w", method, writeErr)
		}
		return nil
	}
	stop := func() error {
		_ = stdin.Close()
		for scanner.Scan() {
		}
		scanErr := scanner.Err()
		waitErr := cmd.Wait()
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if scanErr != nil && !errors.Is(scanErr, io.ErrClosedPipe) {
			return fmt.Errorf("read %s stdout: %w", cmd.Path, scanErr)
		}
		if waitErr != nil {
			return fmt.Errorf("%s exited: %w%s", cmd.Path, waitErr, stderr.tail())
		}
		return nil
	}
	fail := func(cause error) (Result, error) {
		if stopErr := stop(); stopErr != nil {
			if ctx.Err() != nil {
				return Result{}, ctx.Err()
			}
			// An ACP EOF is only a symptom when the child process has already
			// exited. Surface stderr and the exit status as the primary failure so
			// users see the actionable provider/authentication error first.
			if errors.Is(cause, io.ErrUnexpectedEOF) {
				return Result{}, stopErr
			}
			return Result{}, fmt.Errorf("%v; %w", cause, stopErr)
		}
		return Result{}, cause
	}

	if err := request(1, "initialize", map[string]any{
		"protocolVersion":    1,
		"clientInfo":         map[string]any{"name": "oneshot", "version": "1"},
		"clientCapabilities": map[string]any{},
	}); err != nil {
		return fail(err)
	}
	if _, err := readACPResponse(scanner, 1, sink, r.now, nil); err != nil {
		return fail(err)
	}
	if err := request(2, "session/new", map[string]any{"cwd": req.Workspace, "mcpServers": []any{}}); err != nil {
		return fail(err)
	}
	reverse := func(message acpEnvelope) error {
		return respondACPReverseRequest(stdin, message, req.Sandbox, sink, r.now)
	}
	created, err := readACPResponse(scanner, 2, sink, r.now, nil, reverse)
	if err != nil {
		return fail(err)
	}
	var session struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(created.Result, &session); err != nil || strings.TrimSpace(session.SessionID) == "" {
		return fail(errors.New("modu ACP session/new returned no sessionId"))
	}
	sink(Event{Kind: KindStarted, Text: "ACP session started", Raw: string(created.Result), At: r.now()})

	if err := request(3, "session/prompt", map[string]any{
		"sessionId": session.SessionID,
		"prompt":    []map[string]string{{"type": "text", "text": req.Prompt}},
	}); err != nil {
		return fail(err)
	}
	var final strings.Builder
	completed, err := readACPResponse(scanner, 3, sink, r.now, &final, reverse)
	if err != nil {
		return fail(err)
	}
	message := final.String()
	if strings.TrimSpace(message) != "" {
		sink(Event{Kind: KindMessage, Text: message, Raw: string(completed.Result), At: r.now()})
	}
	result := Result{FinalMessage: message, Succeeded: true}
	sink(Event{Kind: KindResult, Text: message, Raw: string(completed.Result), At: r.now()})
	if err := stop(); err != nil {
		result.Succeeded = false
		return result, err
	}
	return result, nil
}

func moduCommandArgs(sandbox Sandbox) []string {
	args := []string{"--acp"}
	if sandbox != SandboxReadOnly {
		args = append(args, "--no-approve")
	}
	return args
}

type acpReverseHandler func(acpEnvelope) error

func readACPResponse(scanner *bufio.Scanner, wanted int64, sink Sink, now nowFunc, final *strings.Builder, reverse ...acpReverseHandler) (acpEnvelope, error) {
	for scanner.Scan() {
		line := scanner.Text()
		var message acpEnvelope
		if err := json.Unmarshal([]byte(line), &message); err != nil {
			return acpEnvelope{}, fmt.Errorf("decode Modu ACP response: %w", err)
		}
		if message.Method != "" && message.ID != nil {
			if len(reverse) == 0 || reverse[0] == nil {
				return acpEnvelope{}, fmt.Errorf("unsupported Modu ACP reverse request %q", message.Method)
			}
			if err := reverse[0](message); err != nil {
				return acpEnvelope{}, err
			}
			continue
		}
		if message.Method == "session/update" {
			if final != nil {
				var params struct {
					Update struct {
						SessionUpdate string `json:"sessionUpdate"`
						Content       struct {
							Type string `json:"type"`
							Text string `json:"text"`
						} `json:"content"`
					} `json:"update"`
				}
				if err := json.Unmarshal(message.Params, &params); err != nil {
					return acpEnvelope{}, fmt.Errorf("decode Modu ACP update: %w", err)
				}
				if params.Update.SessionUpdate == "agent_message_chunk" && params.Update.Content.Type == "text" {
					final.WriteString(params.Update.Content.Text)
				}
			}
			continue
		}
		if message.ID == nil || *message.ID != wanted {
			continue
		}
		if message.Error != nil {
			sink(Event{Kind: KindError, Text: message.Error.Message, Raw: line, At: now()})
			return acpEnvelope{}, fmt.Errorf("Modu ACP error %d: %s", message.Error.Code, message.Error.Message)
		}
		return message, nil
	}
	if err := scanner.Err(); err != nil {
		return acpEnvelope{}, fmt.Errorf("read Modu ACP stream: %w", err)
	}
	return acpEnvelope{}, io.ErrUnexpectedEOF
}

func respondACPReverseRequest(writer io.Writer, message acpEnvelope, sandbox Sandbox, sink Sink, now nowFunc) error {
	if sink == nil {
		sink = func(Event) {}
	}
	if message.ID == nil {
		return errors.New("Modu ACP reverse request has no id")
	}
	if message.Method != "session/request_permission" {
		return fmt.Errorf("unsupported Modu ACP reverse request %q", message.Method)
	}
	var params struct {
		ToolCall struct {
			ToolCallID string          `json:"toolCallId"`
			Title      string          `json:"title"`
			Kind       string          `json:"kind"`
			Arguments  json.RawMessage `json:"arguments"`
		} `json:"toolCall"`
		Options []struct {
			OptionID string `json:"optionId"`
			Kind     string `json:"kind"`
		} `json:"options"`
	}
	if err := json.Unmarshal(message.Params, &params); err != nil {
		return fmt.Errorf("decode Modu ACP permission request: %w", err)
	}
	allow := sandbox != SandboxReadOnly
	wanted := []string{"allow_once", "allow"}
	if !allow {
		wanted = []string{"reject_once", "deny_once", "deny"}
	}
	optionID := ""
	for _, candidate := range wanted {
		for _, option := range params.Options {
			if option.OptionID == candidate || option.Kind == candidate {
				optionID = option.OptionID
				break
			}
		}
		if optionID != "" {
			break
		}
	}
	if optionID == "" {
		return fmt.Errorf("Modu ACP permission request has no %s option", map[bool]string{true: "allow-once", false: "reject-once"}[allow])
	}
	title := strings.TrimSpace(params.ToolCall.Title)
	if title == "" {
		title = "Modu tool permission"
	}
	raw := string(message.Params)
	sink(Event{Kind: KindToolUse, Text: title, Raw: raw, At: now()})
	result := map[string]any{"outcome": map[string]string{"optionId": optionID}}
	frame := map[string]any{"jsonrpc": "2.0", "id": *message.ID, "result": result}
	encoded, err := json.Marshal(frame)
	if err != nil {
		return fmt.Errorf("encode Modu ACP permission response: %w", err)
	}
	encoded = append(encoded, '\n')
	if _, err := writer.Write(encoded); err != nil {
		return fmt.Errorf("write Modu ACP permission response: %w", err)
	}
	decision := "permission rejected"
	if allow {
		decision = "permission allowed once"
	}
	sink(Event{Kind: KindToolResult, Text: decision, Raw: string(encoded), At: now()})
	return nil
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
