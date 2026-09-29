package agentrun

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"
)

// Wire fields are defined by traecli 0.207.1 app-server generate-json-schema.
// Keep these types and the process lifecycle local to TRAE: sharing a command
// name with another CLI does not guarantee protocol compatibility.
type traeFrame struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
	Result json.RawMessage `json:"result"`
	Error  json.RawMessage `json:"error"`
}
type traeProcess struct {
	ctx     context.Context
	cancel  context.CancelFunc
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	encoder *json.Encoder
	scanner *bufio.Scanner
	stderr  lineCapture
	nextID  int
	pending []traeFrame
}

func (r *TraeRunner) connect(ctx context.Context, cwd string, env []string) (*traeProcess, error) {
	child, cancel := context.WithCancel(ctx)
	p := &traeProcess{ctx: child, cancel: cancel}
	p.cmd = exec.CommandContext(child, r.binary, "app-server", "--listen", "stdio://")
	configureProcessWindow(p.cmd)
	p.cmd.Dir = cwd
	p.cmd.Env = env
	p.cmd.Stderr = &p.stderr
	p.cmd.WaitDelay = 2 * time.Second
	var err error
	p.stdin, err = p.cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("TRAE stdin: %w", err)
	}
	stdout, err := p.cmd.StdoutPipe()
	if err != nil {
		p.stdin.Close()
		cancel()
		return nil, fmt.Errorf("TRAE stdout: %w", err)
	}
	if err = p.cmd.Start(); err != nil {
		p.stdin.Close()
		stdout.Close()
		cancel()
		return nil, fmt.Errorf("start TRAE app-server: %w", err)
	}
	p.encoder = json.NewEncoder(p.stdin)
	p.scanner = newJSONLineScanner(stdout)
	_, err = p.call("initialize", map[string]any{"clientInfo": map[string]string{"name": "onecatch", "version": "0.1.0"}, "capabilities": map[string]bool{"experimentalApi": true}})
	if err == nil {
		err = p.encoder.Encode(map[string]any{"method": "initialized", "params": map[string]any{}})
	}
	if err != nil {
		p.close()
		return nil, err
	}
	return p, nil
}
func (p *traeProcess) close() { _ = p.stdin.Close(); p.cancel(); _ = p.cmd.Wait() }
func (p *traeProcess) read() (traeFrame, error) {
	if len(p.pending) > 0 {
		f := p.pending[0]
		p.pending = p.pending[1:]
		return f, nil
	}
	return p.readWire()
}
func (p *traeProcess) readWire() (traeFrame, error) {
	for p.scanner.Scan() {
		if strings.TrimSpace(p.scanner.Text()) == "" {
			continue
		}
		var f traeFrame
		if err := json.Unmarshal(p.scanner.Bytes(), &f); err != nil {
			return f, fmt.Errorf("invalid TRAE app-server frame: %w", err)
		}
		return f, nil
	}
	if p.ctx.Err() != nil {
		return traeFrame{}, p.ctx.Err()
	}
	if err := p.scanner.Err(); err != nil {
		return traeFrame{}, fmt.Errorf("read TRAE app-server: %w", err)
	}
	return traeFrame{}, fmt.Errorf("TRAE app-server closed before response%s", p.stderr.tail())
}
func (p *traeProcess) unsupported(f traeFrame) error {
	return p.encoder.Encode(map[string]any{"id": f.ID, "error": map[string]any{"code": -32601, "message": "Unsupported TRAE request: " + f.Method}})
}
func (p *traeProcess) call(method string, params any) (json.RawMessage, error) {
	p.nextID++
	id := p.nextID
	if err := p.encoder.Encode(map[string]any{"id": id, "method": method, "params": params}); err != nil {
		return nil, err
	}
	for {
		f, err := p.readWire()
		if err != nil {
			return nil, err
		}
		if f.Method != "" {
			// Notifications can precede the turn/start response. Replay them once
			// the turn id is known so queue transitions are not lost.
			if method == "turn/start" && (len(f.ID) == 0 || string(f.ID) == "null") {
				p.pending = append(p.pending, f)
				continue
			}
			if len(f.ID) > 0 {
				if err := p.unsupported(f); err != nil {
					return nil, err
				}
			}
			continue
		}
		var responseID int
		if json.Unmarshal(f.ID, &responseID) != nil || responseID != id {
			continue
		}
		if len(f.Error) > 0 && string(f.Error) != "null" {
			return nil, fmt.Errorf("TRAE %s: %s", method, f.Error)
		}
		return f.Result, nil
	}
}

func (r *TraeRunner) Run(ctx context.Context, req Request, sink Sink) (Result, error) {
	if sink == nil {
		sink = func(Event) {}
	}
	if req.Remote != nil {
		return Result{}, fmt.Errorf("TRAE CLI does not support remote workspaces")
	}
	if req.ServiceTier != "" || req.MaxContextWindow {
		return Result{}, fmt.Errorf("TRAE CLI does not support service-tier or maximum-context overrides")
	}
	sandbox := string(req.Sandbox)
	switch req.Sandbox {
	case "":
		sandbox = "workspace-write"
	case SandboxFull:
		sandbox = "danger-full-access"
	case SandboxReadOnly, SandboxWorkspaceWrite:
	default:
		return Result{}, fmt.Errorf("TRAE: unsupported sandbox %q", req.Sandbox)
	}
	p, err := r.connect(ctx, req.Workspace, req.Environment)
	if err != nil {
		return Result{}, err
	}
	defer p.close()
	skillRaw, err := p.call("skills/list", map[string]any{"cwds": []string{req.Workspace}, "forceReload": true})
	if err != nil {
		return Result{}, err
	}
	skills, err := traeSkills(skillRaw)
	if err != nil {
		return Result{}, err
	}
	approval := "never"
	if req.PermissionHandler != nil && r.SupportsInteractivePermissions(req.Sandbox) {
		approval = "on-request"
	}
	method := "thread/start"
	params := map[string]any{"cwd": req.Workspace, "sandbox": sandbox, "approvalPolicy": approval}
	if req.Model != "" {
		params["model"] = req.Model
	}
	if req.ResumeSessionID != "" {
		method = "thread/resume"
		params["threadId"] = req.ResumeSessionID
	}
	raw, err := p.call(method, params)
	if err != nil {
		return Result{}, err
	}
	var thread struct {
		Thread struct {
			ID string `json:"id"`
		} `json:"thread"`
	}
	if err = json.Unmarshal(raw, &thread); err != nil || thread.Thread.ID == "" {
		return Result{}, fmt.Errorf("TRAE %s: missing thread id", method)
	}
	state := traeTurnState{result: Result{SessionID: thread.Thread.ID}, streams: map[string]bool{}}
	sink(Event{Kind: KindStarted, Text: thread.Thread.ID, At: time.Now()})
	input := []map[string]string{{"type": "text", "text": req.Prompt}}
	for _, skill := range referencedSkills(req.Prompt, skills) {
		input = append(input, map[string]string{"type": "skill", "name": skill.Name, "path": skill.Path})
	}
	params = map[string]any{"threadId": thread.Thread.ID, "input": input}
	if req.Model != "" {
		params["model"] = req.Model
	}
	if req.ReasoningEffort != "" {
		params["effort"] = req.ReasoningEffort
	}
	raw, err = p.call("turn/start", params)
	if err != nil {
		return state.result, err
	}
	var turn struct {
		Turn struct {
			ID string `json:"id"`
		} `json:"turn"`
	}
	if err = json.Unmarshal(raw, &turn); err != nil || turn.Turn.ID == "" {
		return state.result, fmt.Errorf("TRAE turn/start: missing turn id")
	}
	for {
		f, err := p.read()
		if err != nil {
			return state.result, err
		}
		if f.Method != "" && len(f.ID) > 0 {
			if err = r.permission(ctx, p, f, req, thread.Thread.ID, turn.Turn.ID, sink); err != nil {
				return state.result, err
			}
			continue
		}
		done, err := state.update(f, turn.Turn.ID, sink)
		if err != nil {
			return state.result, err
		}
		if done {
			return state.result, nil
		}
	}
}
