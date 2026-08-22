package seam

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

const (
	retainedProcessOutput = 1 << 20
	maxRememberedWrites   = 256
)

type processChunk struct {
	seq    uint64
	stream string
	data   []byte
}

type remoteProcess struct {
	id      string
	command string
	dir     string

	mu         sync.Mutex
	chunks     []processChunk
	retained   int
	nextSeq    uint64
	exited     bool
	exitCode   int
	closed     bool
	failure    string
	stdin      *io.PipeWriter
	stdinOpen  bool
	writeIDs   map[string]bool
	writeOrder []string
	terminated bool
	cancel     context.CancelFunc
	notifyCh   chan struct{}
	done       chan struct{}
}

func (p *remoteProcess) broadcast() {
	close(p.notifyCh)
	p.notifyCh = make(chan struct{})
}

type processStartParams struct {
	ProcessID string            `json:"processId"`
	Argv      []string          `json:"argv"`
	Cwd       string            `json:"cwd"`
	Env       map[string]string `json:"env"`
	Tty       bool              `json:"tty"`
	PipeStdin bool              `json:"pipeStdin"`
}

func (s *ExecServer) handleProcessStart(_ context.Context, raw json.RawMessage) (any, *rpcError) {
	var params processStartParams
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, invalidParams("process/start: %v", err)
	}
	if params.ProcessID == "" || len(params.Argv) == 0 {
		return nil, invalidParams("process/start: processId and argv are required")
	}
	s.mu.Lock()
	if _, exists := s.processes[params.ProcessID]; exists {
		s.mu.Unlock()
		return nil, invalidParams("process/start: processId %q is already in use", params.ProcessID)
	}
	s.mu.Unlock()

	dir, rpcErr := s.mapURI(params.Cwd)
	if rpcErr != nil {
		return nil, rpcErr
	}
	command := argvToCommand(params.Argv)
	processCtx, cancel := context.WithCancel(context.Background())
	process := &remoteProcess{
		id: params.ProcessID, command: command, dir: dir,
		nextSeq: 1, writeIDs: map[string]bool{}, cancel: cancel,
		notifyCh: make(chan struct{}), done: make(chan struct{}),
	}

	var stdin io.Reader
	if params.PipeStdin || params.Tty {
		reader, writer := io.Pipe()
		stdin = reader
		process.stdin = writer
		process.stdinOpen = true
	}

	s.mu.Lock()
	s.processes[params.ProcessID] = process
	s.mu.Unlock()

	go s.runRemoteProcess(processCtx, process, Command{
		Command: command,
		Dir:     dir,
		Env:     safeRemoteEnvironment(params.Env),
		Stdin:   stdin,
		Stdout:  &processChunkWriter{server: s, process: process, stream: "stdout"},
		Stderr:  &processChunkWriter{server: s, process: process, stream: "stderr"},
	})

	return map[string]any{"processId": params.ProcessID, "sandboxType": "none"}, nil
}

func argvToCommand(argv []string) string {
	if len(argv) == 3 && (argv[1] == "-lc" || argv[1] == "-c") {
		return argv[2]
	}
	if len(argv) == 4 && argv[1] == "-l" && argv[2] == "-c" {
		return argv[3]
	}
	quoted := make([]string, len(argv))
	for index, value := range argv {
		quoted[index] = shellQuote(value)
	}
	return strings.Join(quoted, " ")
}

// safeRemoteEnvironment removes values which identify or authenticate the
// machine running Codex. The remaining entries are explicit command
// environment requested through the remote-environment protocol.
func safeRemoteEnvironment(input map[string]string) map[string]string {
	result := make(map[string]string, len(input))
	for key, value := range input {
		upper := strings.ToUpper(key)
		switch {
		case upper == "HOME", upper == "PWD", upper == "OLDPWD", upper == "PATH",
			upper == "USER", upper == "LOGNAME", upper == "SSH_AUTH_SOCK",
			upper == "CODEX_HOME", strings.HasPrefix(upper, "OPENAI_"),
			strings.HasPrefix(upper, "ANTHROPIC_"), strings.HasPrefix(upper, "CLAUDE_"):
			continue
		}
		result[key] = value
	}
	return result
}

func (s *ExecServer) runRemoteProcess(ctx context.Context, process *remoteProcess, command Command) {
	defer close(process.done)
	result, err := s.executor.Run(ctx, command)
	if process.stdin != nil {
		_ = process.stdin.Close()
	}

	process.mu.Lock()
	process.exited = true
	if err != nil {
		process.exitCode = -1
		if process.terminated {
			process.failure = "process terminated"
		} else {
			process.failure = err.Error()
		}
	} else {
		process.exitCode = result.ExitCode
	}
	exitCode := process.exitCode
	failure := process.failure
	seq := process.nextSeq
	process.nextSeq++
	process.broadcast()
	process.mu.Unlock()

	s.notify("process/exited", map[string]any{
		"processId": process.id,
		"seq":       seq,
		"exitCode":  exitCode,
		"failure":   nullableString(failure),
	})

	process.mu.Lock()
	process.closed = true
	closeSeq := process.nextSeq
	process.nextSeq++
	process.broadcast()
	process.mu.Unlock()
	s.notify("process/closed", map[string]any{"processId": process.id, "seq": closeSeq})
	s.retireProcess(process.id)
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

type processChunkWriter struct {
	server  *ExecServer
	process *remoteProcess
	stream  string
}

func (w *processChunkWriter) Write(data []byte) (int, error) {
	copyOfData := append([]byte(nil), data...)
	w.process.mu.Lock()
	seq := w.process.nextSeq
	w.process.nextSeq++
	w.process.chunks = append(w.process.chunks, processChunk{seq: seq, stream: w.stream, data: copyOfData})
	w.process.retained += len(copyOfData)
	for w.process.retained > retainedProcessOutput && len(w.process.chunks) > 1 {
		w.process.retained -= len(w.process.chunks[0].data)
		w.process.chunks = w.process.chunks[1:]
	}
	w.process.broadcast()
	w.process.mu.Unlock()

	// The exec-server wire method is process/output. Codex app-server later
	// translates it to its public process/outputDelta notification; using the
	// public name here is silently ignored and leaves the tool "running".
	w.server.notify("process/output", map[string]any{
		"processId": w.process.id,
		"seq":       seq,
		"stream":    w.stream,
		"chunk":     base64.StdEncoding.EncodeToString(copyOfData),
	})
	return len(data), nil
}

type processReadParams struct {
	ProcessID string  `json:"processId"`
	AfterSeq  *uint64 `json:"afterSeq"`
	MaxBytes  *int64  `json:"maxBytes"`
	WaitMs    *uint64 `json:"waitMs"`
}

func (s *ExecServer) handleProcessRead(ctx context.Context, raw json.RawMessage) (any, *rpcError) {
	var params processReadParams
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, invalidParams("process/read: %v", err)
	}
	s.mu.Lock()
	process, exists := s.processes[params.ProcessID]
	s.mu.Unlock()
	if !exists {
		return nil, invalidRequest("unknown process id %s", params.ProcessID)
	}

	var after uint64
	if params.AfterSeq != nil {
		after = *params.AfterSeq
	}
	maxBytes := int64(1) << 62
	if params.MaxBytes != nil {
		maxBytes = *params.MaxBytes
	}
	deadline := time.Now()
	if params.WaitMs != nil {
		deadline = deadline.Add(time.Duration(*params.WaitMs) * time.Millisecond)
	}
	for {
		process.mu.Lock()
		chunks := []map[string]any{}
		var total int64
		next := process.nextSeq
		for _, chunk := range process.chunks {
			if chunk.seq <= after {
				continue
			}
			if len(chunks) > 0 && total+int64(len(chunk.data)) > maxBytes {
				break
			}
			total += int64(len(chunk.data))
			chunks = append(chunks, map[string]any{
				"seq": chunk.seq, "stream": chunk.stream,
				"chunk": base64.StdEncoding.EncodeToString(chunk.data),
			})
			next = chunk.seq + 1
			if total >= maxBytes {
				break
			}
		}
		done := len(chunks) > 0 || process.closed || !time.Now().Before(deadline)
		notify := process.notifyCh
		if done {
			response := map[string]any{
				"chunks": chunks, "nextSeq": next,
				"exited": process.exited, "closed": process.closed,
				"failure": nullableString(process.failure), "sandboxDenied": false,
			}
			if process.exited {
				response["exitCode"] = process.exitCode
			} else {
				response["exitCode"] = nil
			}
			process.mu.Unlock()
			return response, nil
		}
		process.mu.Unlock()
		wait := time.Until(deadline)
		if wait <= 0 {
			continue
		}
		timer := time.NewTimer(wait)
		select {
		case <-notify:
		case <-timer.C:
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return nil, internalError("process/read: %v", ctx.Err())
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
	}
}

type processWriteParams struct {
	ProcessID string `json:"processId"`
	Chunk     string `json:"chunk"`
	WriteID   string `json:"writeId"`
}

func (s *ExecServer) handleProcessWrite(raw json.RawMessage) (any, *rpcError) {
	var params processWriteParams
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, invalidParams("process/write: %v", err)
	}
	if params.WriteID == "" {
		return nil, invalidParams("process/write: writeId is required")
	}
	data, err := base64.StdEncoding.DecodeString(params.Chunk)
	if err != nil {
		return nil, invalidParams("process/write: invalid base64: %v", err)
	}
	s.mu.Lock()
	process, exists := s.processes[params.ProcessID]
	s.mu.Unlock()
	if !exists {
		return map[string]any{"status": "unknownProcess"}, nil
	}
	process.mu.Lock()
	if !process.stdinOpen || process.exited {
		process.mu.Unlock()
		return map[string]any{"status": "stdinClosed"}, nil
	}
	if process.writeIDs[params.WriteID] {
		process.mu.Unlock()
		return map[string]any{"status": "accepted"}, nil
	}
	stdin := process.stdin
	process.mu.Unlock()
	if _, err := stdin.Write(data); err != nil {
		return nil, internalError("write to process stdin: %v", err)
	}
	process.mu.Lock()
	process.writeIDs[params.WriteID] = true
	process.writeOrder = append(process.writeOrder, params.WriteID)
	for len(process.writeOrder) > maxRememberedWrites {
		delete(process.writeIDs, process.writeOrder[0])
		process.writeOrder = process.writeOrder[1:]
	}
	process.mu.Unlock()
	return map[string]any{"status": "accepted"}, nil
}

type processSignalParams struct {
	ProcessID string `json:"processId"`
	Signal    string `json:"signal"`
}

func (s *ExecServer) stopProcess(id string) (found, running bool) {
	s.mu.Lock()
	process, found := s.processes[id]
	s.mu.Unlock()
	if !found {
		return false, false
	}
	process.mu.Lock()
	running = !process.exited
	if running {
		process.terminated = true
		process.stdinOpen = false
		if process.stdin != nil {
			_ = process.stdin.Close()
		}
	}
	process.mu.Unlock()
	if running {
		process.cancel()
	}
	return true, running
}

func (s *ExecServer) handleProcessSignal(raw json.RawMessage) (any, *rpcError) {
	var params processSignalParams
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, invalidParams("process/signal: %v", err)
	}
	s.stopProcess(params.ProcessID)
	return map[string]any{}, nil
}

func (s *ExecServer) handleProcessTerminate(raw json.RawMessage) (any, *rpcError) {
	var params processSignalParams
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, invalidParams("process/terminate: %v", err)
	}
	_, running := s.stopProcess(params.ProcessID)
	return map[string]any{"running": running}, nil
}

func (p *remoteProcess) String() string {
	return fmt.Sprintf("%s (%s)", p.id, p.command)
}
