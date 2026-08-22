// Codex exec-server support.
//
// Codex can delegate its complete execution environment to a local stdio
// program named in CODEX_HOME/environments.toml. OneCatch is that program for
// a remote run: commands go through the session executor and native fs/* calls
// go through SFTP. The Codex process and its provider credentials never leave
// the local machine.
package seam

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/openmodu/onecatch/internal/remotefs"
)

const (
	codeInvalidRequest = -32600
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
	codeInternal       = -32603

	maxFinishedProcesses   = 32
	remoteOperationTimeout = 2 * time.Minute
)

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func invalidRequest(format string, args ...any) *rpcError {
	return &rpcError{Code: codeInvalidRequest, Message: fmt.Sprintf(format, args...)}
}

func invalidParams(format string, args ...any) *rpcError {
	return &rpcError{Code: codeInvalidParams, Message: fmt.Sprintf(format, args...)}
}

func internalError(format string, args ...any) *rpcError {
	return &rpcError{Code: codeInternal, Message: fmt.Sprintf(format, args...)}
}

type execRPCRequest struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

// ExecServer is one Codex remote-environment connection. It never falls back
// to local execution for an SSH target: failure to construct either transport
// prevents the environment handshake from completing.
type ExecServer struct {
	session   *Session
	executor  Executor
	files     targetFiles
	workspace string
	root      string
	shellName string
	shellPath string
	sessionID string

	out     io.Writer
	writeMu sync.Mutex

	mu            sync.Mutex
	sawInitialize bool
	processes     map[string]*remoteProcess
	finished      []string
	handles       map[string]string
	order         *requestSequencer
	closed        bool
}

// NewExecServer binds the protocol server to a persisted run session.
// workspace is the real local directory in which Codex was launched; paths
// under it are translated to the target root without ever opening them here.
func NewExecServer(ctx context.Context, session *Session, workspace string) (*ExecServer, error) {
	if session == nil {
		return nil, fmt.Errorf("exec-server session is required")
	}
	executor := NewExecutor(session.Target)
	name, shellPath, err := probeTargetShell(ctx, executor, session.Target.Root)
	if err != nil {
		return nil, err
	}

	var files targetFiles
	if strings.TrimSpace(session.Target.Host) == "" {
		files = localTargetFiles{}
	} else {
		backend, err := remotefs.NewSFTPBackend(ctx, remotefs.SFTPConfig{
			Host:          session.Target.Host,
			Root:          "/",
			Username:      session.Target.Username,
			CredentialID:  session.Target.CredentialID,
			SSHBinary:     session.Target.SSHBinary,
			AskPassBinary: session.Target.AskPassBinary,
			SSHOptions:    session.Target.SSHOptions,
			Stderr:        os.Stderr,
		})
		if err != nil {
			return nil, fmt.Errorf("open the target filesystem: %w", err)
		}
		files = &sftpTargetFiles{backend: backend}
	}
	if workspace == "" {
		workspace, _ = os.Getwd()
	}
	return &ExecServer{
		session:   session,
		executor:  executor,
		files:     files,
		workspace: filepath.Clean(workspace),
		root:      path.Clean(session.Target.Root),
		shellName: name,
		shellPath: shellPath,
		sessionID: "onecatch-" + randomHex(8),
		processes: map[string]*remoteProcess{},
		handles:   map[string]string{},
		order:     newRequestSequencer(),
	}, nil
}

func probeTargetShell(ctx context.Context, executor Executor, root string) (string, string, error) {
	var out, stderr bytes.Buffer
	result, err := executor.Run(ctx, Command{
		Command: "for sh in bash sh; do p=$(command -v \"$sh\" 2>/dev/null) && { printf '%s %s\\n' \"$sh\" \"$p\"; exit 0; }; done; exit 1",
		Dir:     root, Stdout: &out, Stderr: &stderr, Timeout: 15 * time.Second,
	})
	if err != nil {
		return "", "", fmt.Errorf("probe the target shell: %w", err)
	}
	if result.ExitCode != 0 {
		return "", "", fmt.Errorf("the target has neither bash nor sh on PATH: %s", strings.TrimSpace(stderr.String()))
	}
	fields := strings.Fields(strings.TrimSpace(out.String()))
	if len(fields) != 2 {
		return "", "", fmt.Errorf("unexpected answer probing the target shell: %q", strings.TrimSpace(out.String()))
	}
	return fields[0], fields[1], nil
}

// Serve runs newline-delimited JSON-RPC until Codex closes stdin or ctx ends.
func (s *ExecServer) Serve(ctx context.Context, in io.Reader, out io.Writer) error {
	s.out = out
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 0, 1<<20), 64<<20)
	var handlers sync.WaitGroup
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var request execRPCRequest
		if err := json.Unmarshal(line, &request); err != nil {
			fmt.Fprintf(os.Stderr, "onecatch exec-server: dropping invalid frame: %v\n", err)
			continue
		}
		if len(request.ID) == 0 {
			continue
		}
		// The handshake and process registration must be visible before a
		// pipelined request can address them.
		if request.Method == "initialize" || request.Method == "process/start" {
			s.dispatch(ctx, request)
			continue
		}
		slot := s.order.enter(execServerOrderKey(request.Method, request.Params))
		handlers.Add(1)
		go func() {
			defer handlers.Done()
			slot.wait()
			defer slot.done()
			s.dispatch(ctx, request)
		}()
	}
	s.terminateAll()
	handlers.Wait()
	if err := scanner.Err(); err != nil && ctx.Err() == nil {
		return err
	}
	return nil
}

func (s *ExecServer) send(value any) {
	data, err := json.Marshal(value)
	if err != nil {
		return
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_, _ = s.out.Write(append(data, '\n'))
}

func (s *ExecServer) notify(method string, params any) {
	s.send(map[string]any{"method": method, "params": params})
}

func (s *ExecServer) dispatch(ctx context.Context, request execRPCRequest) {
	var result any
	var rpcErr *rpcError
	func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				rpcErr = internalError("internal error: %v", recovered)
			}
		}()
		result, rpcErr = s.handle(ctx, request.Method, request.Params)
	}()
	if rpcErr != nil {
		s.send(map[string]any{"id": request.ID, "error": rpcErr})
		return
	}
	s.send(map[string]any{"id": request.ID, "result": result})
}

func (s *ExecServer) handle(ctx context.Context, method string, params json.RawMessage) (any, *rpcError) {
	if method == "initialize" {
		return s.handleInitialize(params)
	}
	s.mu.Lock()
	initialized := s.sawInitialize
	s.mu.Unlock()
	if !initialized {
		return nil, invalidRequest("client must call initialize before using methods")
	}
	switch method {
	case "environment/info":
		return s.environmentInfo(), nil
	case "environment/status":
		return map[string]any{"status": "ready"}, nil
	case "process/start":
		if s.session.ReadOnly {
			return nil, internalError("the remote run is read-only; starting commands is disabled")
		}
		return s.handleProcessStart(ctx, params)
	case "process/read":
		return s.handleProcessRead(ctx, params)
	case "process/write":
		return s.handleProcessWrite(params)
	case "process/signal":
		return s.handleProcessSignal(params)
	case "process/terminate":
		return s.handleProcessTerminate(params)
	case "fs/readFile":
		return s.handleFSReadFile(ctx, params)
	case "fs/writeFile":
		if s.session.ReadOnly {
			return nil, internalError("the remote run is read-only")
		}
		return s.handleFSWriteFile(ctx, params)
	case "fs/createDirectory":
		if s.session.ReadOnly {
			return nil, internalError("the remote run is read-only")
		}
		return s.handleFSCreateDirectory(ctx, params)
	case "fs/getMetadata":
		return s.handleFSGetMetadata(ctx, params)
	case "fs/canonicalize":
		return s.handleFSCanonicalize(ctx, params)
	case "fs/readDirectory":
		return s.handleFSReadDirectory(ctx, params)
	case "fs/walk":
		return s.handleFSWalk(ctx, params)
	case "fs/remove":
		if s.session.ReadOnly {
			return nil, internalError("the remote run is read-only")
		}
		return s.handleFSRemove(ctx, params)
	case "fs/copy":
		if s.session.ReadOnly {
			return nil, internalError("the remote run is read-only")
		}
		return s.handleFSCopy(ctx, params)
	case "fs/open":
		return s.handleFSOpen(params)
	case "fs/readBlock":
		return s.handleFSReadBlock(ctx, params)
	case "fs/close":
		return s.handleFSClose(params)
	case "capabilityRoots/discoverV1":
		return s.handleCapabilityDiscover(params)
	case "http/request":
		return nil, internalError("http/request is not supported by the OneCatch remote environment")
	default:
		return nil, &rpcError{Code: codeMethodNotFound, Message: "unknown method " + method}
	}
}

func (s *ExecServer) handleInitialize(params json.RawMessage) (any, *rpcError) {
	if len(params) > 0 && string(params) != "null" {
		var value any
		if err := json.Unmarshal(params, &value); err != nil {
			return nil, invalidParams("initialize: %v", err)
		}
	}
	s.mu.Lock()
	s.sawInitialize = true
	s.mu.Unlock()
	return map[string]any{"sessionId": s.sessionID}, nil
}

func (s *ExecServer) environmentInfo() map[string]any {
	return map[string]any{
		"shell":                map[string]any{"name": s.shellName, "path": s.shellPath},
		"cwd":                  pathToURI(s.root),
		"temporaryDirectories": []string{pathToURI("/tmp")},
		"tempDir":              pathToURI("/tmp"),
		"capabilities": map[string]any{
			"networkProxyLaunch":         false,
			"capabilityDiscoverySandbox": false,
			"environmentConfigRead":      false,
			"sandboxedFileStreaming":     false,
		},
	}
}

func uriToPath(raw string) (string, *rpcError) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "file" || parsed.Path == "" {
		return "", invalidParams("path %q is not a file:// URI", raw)
	}
	return parsed.Path, nil
}

func pathToURI(value string) string {
	return (&url.URL{Scheme: "file", Path: value}).String()
}

func (s *ExecServer) mapPath(value string) string {
	workspace := filepath.ToSlash(s.workspace)
	from := filepath.ToSlash(value)
	if workspace != "" && workspace != "/" {
		if from == workspace {
			return s.root
		}
		if strings.HasPrefix(from, workspace+"/") {
			return path.Join(s.root, from[len(workspace)+1:])
		}
	}
	if strings.HasPrefix(from, "/") {
		return path.Clean(from)
	}
	return path.Join(s.root, from)
}

func (s *ExecServer) mapURI(raw string) (string, *rpcError) {
	value, rpcErr := uriToPath(raw)
	if rpcErr != nil {
		return "", rpcErr
	}
	return s.mapPath(value), nil
}

func (s *ExecServer) operationContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, remoteOperationTimeout)
}

func (s *ExecServer) retireProcess(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.finished = append(s.finished, id)
	for len(s.finished) > maxFinishedProcesses {
		delete(s.processes, s.finished[0])
		s.finished = s.finished[1:]
	}
}

func (s *ExecServer) terminateAll() {
	s.mu.Lock()
	processes := make([]*remoteProcess, 0, len(s.processes))
	for _, process := range s.processes {
		processes = append(processes, process)
	}
	s.mu.Unlock()
	for _, process := range processes {
		process.mu.Lock()
		exited := process.exited
		process.mu.Unlock()
		if !exited {
			process.cancel()
		}
	}
}

// Close terminates commands and closes the SFTP channel. It does not remove
// the run session; the runner owns that lifecycle.
func (s *ExecServer) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.mu.Unlock()
	s.terminateAll()
	if s.files != nil {
		return s.files.Close()
	}
	return nil
}

// EnvironmentsTOML binds Codex's only environment to onecatchsh's exec-server
// mode. include_local=false is fail-closed: if this program fails, Codex must
// report the environment failure rather than silently use the local machine.
func EnvironmentsTOML(program, sessionName string) string {
	return "default = \"onecatch\"\n" +
		"include_local = false\n\n" +
		"[[environments]]\n" +
		"id = \"onecatch\"\n" +
		"program = " + tomlString(program) + "\n" +
		"args = [\"exec-server\"]\n" +
		"env = { " + SessionEnv + " = " + tomlString(sessionName) + " }\n"
}

func tomlString(value string) string {
	data, _ := json.Marshal(value)
	return string(data)
}

func randomHex(size int) string {
	data := make([]byte, size)
	if _, err := rand.Read(data); err != nil {
		return fmt.Sprintf("fallback-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(data)
}
