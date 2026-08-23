package agentrun

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/openmodu/onecatch/internal/remotefs"
	"github.com/openmodu/onecatch/internal/usecase/agentrun/seam"
)

const acpRemoteMaxTextBytes = 8 * 1024 * 1024

const acpRemoteGuidance = `This session operates on a REMOTE target through OneCatch.

The client file and terminal operations are redirected to the remote target;
they do not operate on the local directory where the Grok process is running.
Use the remote workspace path shown below for all workspace operations and do
not translate it to a path on this machine.`

var acpEnvironmentName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

type acpRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func acpInvalidParams(format string, args ...any) *acpRPCError {
	return &acpRPCError{Code: -32602, Message: fmt.Sprintf(format, args...)}
}

func acpInternalError(format string, args ...any) *acpRPCError {
	return &acpRPCError{Code: -32603, Message: fmt.Sprintf(format, args...)}
}

// acpRemoteFiles is the subset of the remote filesystem needed by ACP's two
// text-file methods. Keeping it small also lets the conformance test exercise
// the exact bridge against a second local directory without an SSH server.
type acpRemoteFiles interface {
	Lstat(string) (os.FileInfo, error)
	OpenFile(string, int, os.FileMode) (remotefs.File, error)
	Mkdir(string, os.FileMode) error
	Close() error
}

type acpRemoteBridge struct {
	root     string
	readOnly bool
	files    acpRemoteFiles
	executor seam.Executor
	ctx      context.Context
	cancel   context.CancelFunc

	mu        sync.Mutex
	terminals map[string]*acpRemoteTerminal
	nextID    uint64
}

func newACPRemoteBridge(ctx context.Context, target seam.Target, sandbox Sandbox) (*acpRemoteBridge, error) {
	root := path.Clean(target.Root)
	if !path.IsAbs(root) {
		return nil, fmt.Errorf("remote workspace root %q must be absolute", target.Root)
	}
	if sandbox == SandboxReadOnly {
		// ACP exposes arbitrary terminal commands as one boolean capability.
		// Advertising it would bypass the target-side read-only boundary, while
		// omitting it leaves Grok unable to list or search the remote tree.
		return nil, fmt.Errorf("remote FS runs require workspace-write; ACP terminal commands cannot be made safely read-only")
	}

	var files acpRemoteFiles
	if strings.TrimSpace(target.Host) == "" {
		files = &localACPRemoteFiles{root: filepath.Clean(target.Root)}
	} else {
		backend, err := remotefs.NewSFTPBackend(ctx, remotefs.SFTPConfig{
			Host:          target.Host,
			Root:          root,
			Username:      target.Username,
			CredentialID:  target.CredentialID,
			SSHBinary:     target.SSHBinary,
			AskPassBinary: target.AskPassBinary,
			SSHOptions:    target.SSHOptions,
		})
		if err != nil {
			return nil, err
		}
		files = backend
	}
	bridgeCtx, cancel := context.WithCancel(ctx)
	return &acpRemoteBridge{
		root: root, files: files, executor: seam.NewExecutor(target),
		ctx: bridgeCtx, cancel: cancel, terminals: make(map[string]*acpRemoteTerminal),
	}, nil
}

func (b *acpRemoteBridge) capabilities() map[string]any {
	return map[string]any{
		"fs": map[string]bool{
			"readTextFile":  true,
			"writeTextFile": !b.readOnly,
		},
		"terminal": !b.readOnly,
	}
}

func (b *acpRemoteBridge) prompt(userPrompt string) string {
	return acpRemoteGuidance + "\n\nRemote workspace: " + b.root + "\n\n" + userPrompt
}

func (b *acpRemoteBridge) supports(method string) bool {
	switch method {
	case "fs/read_text_file", "fs/write_text_file",
		"terminal/create", "terminal/output", "terminal/wait_for_exit",
		"terminal/kill", "terminal/release":
		return true
	default:
		return false
	}
}

func (b *acpRemoteBridge) handle(ctx context.Context, method string, raw json.RawMessage) (any, *acpRPCError) {
	switch method {
	case "fs/read_text_file":
		return b.readTextFile(raw)
	case "fs/write_text_file":
		return b.writeTextFile(raw)
	case "terminal/create":
		return b.createTerminal(raw)
	case "terminal/output":
		return b.terminalOutput(raw)
	case "terminal/wait_for_exit":
		return b.waitForTerminal(ctx, raw)
	case "terminal/kill":
		return b.killTerminal(raw)
	case "terminal/release":
		return b.releaseTerminal(raw)
	default:
		return nil, &acpRPCError{Code: -32601, Message: "method not supported by OneCatch"}
	}
}

func (b *acpRemoteBridge) readTextFile(raw json.RawMessage) (any, *acpRPCError) {
	var request struct {
		Path  string  `json:"path"`
		Line  *uint64 `json:"line,omitempty"`
		Limit *uint64 `json:"limit,omitempty"`
	}
	if err := json.Unmarshal(raw, &request); err != nil || request.Path == "" {
		return nil, acpInvalidParams("invalid fs/read_text_file request")
	}
	relative, err := acpRemotePath(b.root, request.Path)
	if err != nil || relative == "" {
		return nil, acpInvalidParams("%v", acpPathError(err, "path is a directory"))
	}
	info, err := b.files.Lstat(relative)
	if err != nil {
		return nil, acpInternalError("read %s: %v", request.Path, err)
	}
	if info.IsDir() {
		return nil, acpInvalidParams("path %q is a directory", request.Path)
	}
	if info.Size() > acpRemoteMaxTextBytes {
		return nil, acpInternalError("read %s: file is larger than %d bytes", request.Path, acpRemoteMaxTextBytes)
	}
	file, err := b.files.OpenFile(relative, os.O_RDONLY, 0)
	if err != nil {
		return nil, acpInternalError("read %s: %v", request.Path, err)
	}
	defer file.Close()
	content, err := io.ReadAll(io.NewSectionReader(file, 0, info.Size()))
	if err != nil {
		return nil, acpInternalError("read %s: %v", request.Path, err)
	}
	if !utf8.Valid(content) {
		return nil, acpInternalError("read %s: file is not valid UTF-8 text", request.Path)
	}
	text, rangeErr := acpTextRange(string(content), request.Line, request.Limit)
	if rangeErr != nil {
		return nil, rangeErr
	}
	return map[string]any{"content": text}, nil
}

func acpTextRange(content string, line, limit *uint64) (string, *acpRPCError) {
	if line == nil && limit == nil {
		return content, nil
	}
	start := uint64(1)
	if line != nil {
		start = *line
	}
	if start == 0 {
		return "", acpInvalidParams("line must be 1 or greater")
	}
	lines := strings.SplitAfter(content, "\n")
	index := start - 1
	if index >= uint64(len(lines)) {
		return "", nil
	}
	end := uint64(len(lines))
	if limit != nil && *limit < end-index {
		end = index + *limit
	}
	return strings.Join(lines[index:end], ""), nil
}

func (b *acpRemoteBridge) writeTextFile(raw json.RawMessage) (any, *acpRPCError) {
	if b.readOnly {
		return nil, acpInternalError("the remote run is read-only")
	}
	var request struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(raw, &request); err != nil || request.Path == "" {
		return nil, acpInvalidParams("invalid fs/write_text_file request")
	}
	relative, err := acpRemotePath(b.root, request.Path)
	if err != nil || relative == "" {
		return nil, acpInvalidParams("%v", acpPathError(err, "path is a directory"))
	}
	if err := b.ensureParent(path.Dir(relative)); err != nil {
		return nil, acpInternalError("create parent of %s: %v", request.Path, err)
	}
	file, err := b.files.OpenFile(relative, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return nil, acpInternalError("write %s: %v", request.Path, err)
	}
	defer file.Close()
	data := []byte(request.Content)
	for offset := 0; offset < len(data); {
		written, writeErr := file.WriteAt(data[offset:], int64(offset))
		offset += written
		if writeErr != nil {
			return nil, acpInternalError("write %s: %v", request.Path, writeErr)
		}
		if written == 0 {
			return nil, acpInternalError("write %s: %v", request.Path, io.ErrShortWrite)
		}
	}
	if err := file.Sync(); err != nil {
		return nil, acpInternalError("write %s: %v", request.Path, err)
	}
	return map[string]any{}, nil
}

func (b *acpRemoteBridge) ensureParent(relative string) error {
	if relative == "." || relative == "" {
		return nil
	}
	current := ""
	for _, part := range strings.Split(relative, "/") {
		current = path.Join(current, part)
		info, err := b.files.Lstat(current)
		if err == nil {
			if !info.IsDir() {
				return fmt.Errorf("%s is not a directory", current)
			}
			continue
		}
		if !os.IsNotExist(err) {
			return err
		}
		if err := b.files.Mkdir(current, 0o755); err != nil {
			return err
		}
	}
	return nil
}

type acpTerminalRequest struct {
	TerminalID string `json:"terminalId"`
}

type acpRemoteTerminal struct {
	cancel context.CancelFunc
	done   chan struct{}
	output *acpTerminalOutput

	mu      sync.Mutex
	outcome seam.Outcome
	err     error
	signal  string
}

func (b *acpRemoteBridge) createTerminal(raw json.RawMessage) (any, *acpRPCError) {
	if b.readOnly {
		return nil, acpInternalError("the remote run is read-only")
	}
	var request struct {
		Command string   `json:"command"`
		Args    []string `json:"args"`
		Env     []struct {
			Name  string `json:"name"`
			Value string `json:"value"`
		} `json:"env"`
		Cwd             string  `json:"cwd,omitempty"`
		OutputByteLimit *uint64 `json:"outputByteLimit,omitempty"`
	}
	if err := json.Unmarshal(raw, &request); err != nil || strings.TrimSpace(request.Command) == "" {
		return nil, acpInvalidParams("invalid terminal/create request")
	}
	cwd := b.root
	if request.Cwd != "" {
		relative, err := acpRemotePath(b.root, request.Cwd)
		if err != nil {
			return nil, acpInvalidParams("%v", err)
		}
		cwd = path.Join(b.root, relative)
	}
	environment := make(map[string]string, len(request.Env))
	for _, entry := range request.Env {
		if !acpEnvironmentName.MatchString(entry.Name) {
			return nil, acpInvalidParams("invalid environment variable name %q", entry.Name)
		}
		environment[entry.Name] = entry.Value
	}
	limit := uint64(acpRemoteMaxTextBytes)
	if request.OutputByteLimit != nil && *request.OutputByteLimit < limit {
		limit = *request.OutputByteLimit
	}
	terminalCtx, cancel := context.WithCancel(b.ctx)
	terminal := &acpRemoteTerminal{
		cancel: cancel, done: make(chan struct{}), output: newACPTerminalOutput(int(limit)),
	}

	b.mu.Lock()
	b.nextID++
	id := fmt.Sprintf("onecatch-terminal-%d", b.nextID)
	b.terminals[id] = terminal
	b.mu.Unlock()

	arguments := append([]string{request.Command}, request.Args...)
	go func() {
		outcome, runErr := b.executor.Run(terminalCtx, seam.Command{
			Command: acpShellJoin(arguments), Dir: cwd, Env: environment,
			Stdout: terminal.output, Stderr: terminal.output,
		})
		terminal.mu.Lock()
		terminal.outcome = outcome
		if terminalCtx.Err() != nil {
			terminal.signal = "SIGTERM"
		} else {
			terminal.err = runErr
		}
		terminal.mu.Unlock()
		close(terminal.done)
	}()
	return map[string]any{"terminalId": id}, nil
}

func (b *acpRemoteBridge) terminalOutput(raw json.RawMessage) (any, *acpRPCError) {
	terminal, rpcErr := b.findTerminal(raw)
	if rpcErr != nil {
		return nil, rpcErr
	}
	result := map[string]any{}
	output, truncated := terminal.output.snapshot()
	result["output"] = output
	result["truncated"] = truncated
	select {
	case <-terminal.done:
		exitStatus, err := terminal.exitStatus()
		if err != nil {
			return nil, acpInternalError("remote terminal failed: %v", err)
		}
		result["exitStatus"] = exitStatus
	default:
	}
	return result, nil
}

func (b *acpRemoteBridge) waitForTerminal(ctx context.Context, raw json.RawMessage) (any, *acpRPCError) {
	terminal, rpcErr := b.findTerminal(raw)
	if rpcErr != nil {
		return nil, rpcErr
	}
	select {
	case <-terminal.done:
	case <-ctx.Done():
		return nil, acpInternalError("wait for remote terminal: %v", ctx.Err())
	}
	status, err := terminal.exitStatus()
	if err != nil {
		return nil, acpInternalError("remote terminal failed: %v", err)
	}
	return status, nil
}

func (b *acpRemoteBridge) killTerminal(raw json.RawMessage) (any, *acpRPCError) {
	terminal, rpcErr := b.findTerminal(raw)
	if rpcErr != nil {
		return nil, rpcErr
	}
	terminal.cancel()
	return map[string]any{}, nil
}

func (b *acpRemoteBridge) releaseTerminal(raw json.RawMessage) (any, *acpRPCError) {
	var request acpTerminalRequest
	if err := json.Unmarshal(raw, &request); err != nil || request.TerminalID == "" {
		return nil, acpInvalidParams("invalid terminal/release request")
	}
	b.mu.Lock()
	terminal := b.terminals[request.TerminalID]
	if terminal != nil {
		delete(b.terminals, request.TerminalID)
	}
	b.mu.Unlock()
	if terminal == nil {
		return nil, acpInvalidParams("unknown terminal %q", request.TerminalID)
	}
	terminal.cancel()
	return map[string]any{}, nil
}

func (b *acpRemoteBridge) findTerminal(raw json.RawMessage) (*acpRemoteTerminal, *acpRPCError) {
	var request acpTerminalRequest
	if err := json.Unmarshal(raw, &request); err != nil || request.TerminalID == "" {
		return nil, acpInvalidParams("invalid terminal request")
	}
	b.mu.Lock()
	terminal := b.terminals[request.TerminalID]
	b.mu.Unlock()
	if terminal == nil {
		return nil, acpInvalidParams("unknown terminal %q", request.TerminalID)
	}
	return terminal, nil
}

func (t *acpRemoteTerminal) exitStatus() (map[string]any, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.err != nil {
		return nil, t.err
	}
	if t.signal != "" {
		return map[string]any{"signal": t.signal}, nil
	}
	return map[string]any{"exitCode": t.outcome.ExitCode}, nil
}

type acpTerminalOutput struct {
	mu        sync.Mutex
	data      []byte
	limit     int
	truncated bool
}

func newACPTerminalOutput(limit int) *acpTerminalOutput {
	return &acpTerminalOutput{limit: limit}
}

func (o *acpTerminalOutput) Write(input []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if len(input) == 0 {
		return 0, nil
	}
	if o.limit == 0 {
		o.truncated = true
		return len(input), nil
	}
	o.data = append(o.data, input...)
	if overflow := len(o.data) - o.limit; overflow > 0 {
		copy(o.data, o.data[overflow:])
		o.data = o.data[:o.limit]
		o.truncated = true
	}
	return len(input), nil
}

func (o *acpTerminalOutput) snapshot() (string, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return strings.ToValidUTF8(string(o.data), "�"), o.truncated
}

func (b *acpRemoteBridge) Close() error {
	b.cancel()
	b.mu.Lock()
	terminals := make([]*acpRemoteTerminal, 0, len(b.terminals))
	for _, terminal := range b.terminals {
		terminals = append(terminals, terminal)
	}
	b.terminals = make(map[string]*acpRemoteTerminal)
	b.mu.Unlock()
	for _, terminal := range terminals {
		terminal.cancel()
		select {
		case <-terminal.done:
		case <-time.After(2 * time.Second):
		}
	}
	return b.files.Close()
}

func acpRemotePath(root, input string) (string, error) {
	if input == "" || input == "." {
		return "", nil
	}
	cleaned := path.Clean(input)
	if path.IsAbs(cleaned) {
		root = path.Clean(root)
		if cleaned == root {
			return "", nil
		}
		if root != "/" && !strings.HasPrefix(cleaned, root+"/") {
			return "", fmt.Errorf("path %q is outside the remote workspace %q", input, root)
		}
		if root == "/" {
			return strings.TrimPrefix(cleaned, "/"), nil
		}
		return strings.TrimPrefix(cleaned, root+"/"), nil
	}
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("path %q escapes the remote workspace", input)
	}
	return cleaned, nil
}

func acpPathError(err error, fallback string) string {
	if err != nil {
		return err.Error()
	}
	return fallback
}

func acpShellJoin(arguments []string) string {
	quoted := make([]string, len(arguments))
	for index, argument := range arguments {
		quoted[index] = "'" + strings.ReplaceAll(argument, "'", `'"'"'`) + "'"
	}
	return strings.Join(quoted, " ")
}

type localACPRemoteFiles struct {
	root string
}

func (f *localACPRemoteFiles) path(relative string) string {
	return filepath.Join(f.root, filepath.FromSlash(relative))
}

func (f *localACPRemoteFiles) Lstat(relative string) (os.FileInfo, error) {
	return os.Lstat(f.path(relative))
}

func (f *localACPRemoteFiles) OpenFile(relative string, flags int, mode os.FileMode) (remotefs.File, error) {
	return os.OpenFile(f.path(relative), flags, mode)
}

func (f *localACPRemoteFiles) Mkdir(relative string, mode os.FileMode) error {
	return os.Mkdir(f.path(relative), mode)
}

func (f *localACPRemoteFiles) Close() error { return nil }
