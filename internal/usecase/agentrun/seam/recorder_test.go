//go:build conformance

package seam

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
)

// The recorders below are this test binary acting as something else.
//
// Both seams need a real executable on disk: Claude Code stats the value of
// CLAUDE_CODE_SHELL_PREFIX and execs it, and codex spawns the program named in
// environments.toml. Rather than building throwaway binaries at test time or
// adding entries under cmd/ that exist only for tests, the test binary re-execs
// itself and dispatches on an environment variable. This is the pattern os/exec
// uses for its own tests, and it keeps the recorder's code beside the
// assertions that read what it recorded.
const (
	recorderRoleEnv = "SEAM_RECORDER"
	recordFileEnv   = "SEAM_RECORD_FILE"
)

// record is one thing a harness did, appended as a JSON line.
type record struct {
	// Role says which recorder wrote this line.
	Role string `json:"role"`
	// Argv is the full argument vector, for the shell seam. Its length is
	// itself an assertion: Claude Code 2.1.239 passes the whole envelope as a
	// single element with no -c in front of it, and a parser written for
	// `bash -c <envelope>` breaks the day that is true.
	Argv []string `json:"argv,omitempty"`
	// Method and Params are the exec-server seam's JSON-RPC traffic.
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
}

func appendRecord(r record) {
	path := os.Getenv(recordFileEnv)
	if path == "" {
		return
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()
	line, err := json.Marshal(r)
	if err != nil {
		return
	}
	_, _ = f.Write(append(line, '\n'))
}

func readRecords(path string) ([]record, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out []record
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var r record
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			return nil, fmt.Errorf("malformed record %q: %w", line, err)
		}
		out = append(out, r)
	}
	return out, nil
}

// runShellRecorder impersonates the program named by CLAUDE_CODE_SHELL_PREFIX.
//
// It records the invocation and then runs it locally, unchanged. Running it is
// what lets the harness finish its turn and the mock reach turn two; the point
// of this suite is the *shape* of what arrives, not where it executes, and
// keeping execution local means the suite needs no remote host to be useful.
func runShellRecorder() int {
	argv := os.Args[1:]
	appendRecord(record{Role: "shell", Argv: argv})
	if len(argv) == 0 {
		return 0
	}
	cmd := exec.Command("/bin/bash", "-c", strings.Join(argv, " "))
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if ok := asExitError(err, &ee); ok {
			return ee.ExitCode()
		}
		fmt.Fprintln(os.Stderr, "seam shell recorder:", err)
		return 127
	}
	return 0
}

func asExitError(err error, target **exec.ExitError) bool {
	if ee, ok := err.(*exec.ExitError); ok {
		*target = ee
		return true
	}
	return false
}

// --- codex exec-server recorder -------------------------------------------

// rpcRequest is one inbound message. Codex's dialect omits the "jsonrpc"
// field and its id may be a string or an integer, so it is echoed verbatim.
type rpcRequest struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

// execServerRecorder implements just enough of codex's exec-server protocol to
// let a scripted turn complete, recording every method it is asked for.
//
// Implementing it rather than stubbing it is the point: a stub that answers
// everything with an error tells you codex tried, but not whether the command
// actually arrived, in what argv, under which working directory. Those are the
// things that change between codex versions.
type execServerRecorder struct {
	out io.Writer

	mu      sync.Mutex
	writeMu sync.Mutex
	proc    map[string]*recordedProcess
}

type recordedProcess struct {
	output   []byte
	exitCode int
	drained  bool
}

func runExecServerRecorder() int {
	r := &execServerRecorder{out: os.Stdout, proc: map[string]*recordedProcess{}}
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 1<<20), 64<<20)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var req rpcRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			continue
		}
		appendRecord(record{Role: "execserver", Method: req.Method, Params: req.Params})
		if len(req.ID) == 0 {
			continue // a notification; nothing to answer
		}
		result, rerr := r.dispatch(req)
		r.reply(req.ID, result, rerr)
	}
	return 0
}

// pushOutput delivers a finished process's output as server-to-client
// notifications. Codex 0.149 never polls process/read, so if the output is to
// reach the model at all it has to be pushed.
func (r *execServerRecorder) pushOutput(processID string, out []byte, code int) {
	r.notify("process/output", map[string]any{
		"processId": processID, "stream": "stdout",
		"chunk": base64.StdEncoding.EncodeToString(out),
	})
	r.notify("process/exited", map[string]any{
		"processId": processID, "exitCode": code,
	})
}

func (r *execServerRecorder) notify(method string, params any) {
	data, err := json.Marshal(map[string]any{"method": method, "params": params})
	if err != nil {
		return
	}
	r.writeMu.Lock()
	defer r.writeMu.Unlock()
	_, _ = r.out.Write(append(data, '\n'))
}

func (r *execServerRecorder) reply(id json.RawMessage, result any, rerr map[string]any) {
	msg := map[string]any{"id": json.RawMessage(id)}
	if rerr != nil {
		msg["error"] = rerr
	} else {
		msg["result"] = result
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}
	r.writeMu.Lock()
	defer r.writeMu.Unlock()
	_, _ = r.out.Write(append(data, '\n'))
}

func (r *execServerRecorder) dispatch(req rpcRequest) (any, map[string]any) {
	switch req.Method {
	case "initialize":
		return map[string]any{"sessionId": "seam-conformance"}, nil

	case "environment/info":
		cwd, _ := os.Getwd()
		return map[string]any{
			"shell":                map[string]any{"name": "bash", "path": "/bin/bash"},
			"cwd":                  "file://" + cwd,
			"temporaryDirectories": []string{"file:///tmp"},
			"tempDir":              "file:///tmp",
			// Every optional capability is reported absent. Codex gates newer
			// request fields on these, and false keeps it on the protocol
			// surface this recorder actually implements.
			"capabilities": map[string]any{
				"networkProxyLaunch":         false,
				"capabilityDiscoverySandbox": false,
				"environmentConfigRead":      false,
				"sandboxedFileStreaming":     false,
			},
		}, nil

	case "environment/status":
		return map[string]any{"status": "ready"}, nil

	case "process/start":
		var p struct {
			ProcessID string   `json:"processId"`
			Argv      []string `json:"argv"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil || len(p.Argv) == 0 {
			return nil, map[string]any{"code": -32602, "message": "process/start: bad params"}
		}
		out, code := runArgv(p.Argv)
		r.mu.Lock()
		r.proc[p.ProcessID] = &recordedProcess{output: out, exitCode: code}
		r.mu.Unlock()
		// The reply shape is load-bearing. Answering {} is accepted by the
		// JSON-RPC layer and then silently abandoned by codex: it never polls
		// process/read, and the model is told the command produced nothing.
		go r.pushOutput(p.ProcessID, out, code)
		return map[string]any{"processId": p.ProcessID, "sandboxType": "none"}, nil

	case "process/read":
		var p struct {
			ProcessID string `json:"processId"`
		}
		_ = json.Unmarshal(req.Params, &p)
		r.mu.Lock()
		proc, ok := r.proc[p.ProcessID]
		r.mu.Unlock()
		if !ok {
			return nil, map[string]any{"code": -32600, "message": "unknown process id"}
		}
		chunks := []map[string]any{}
		if !proc.drained {
			chunks = append(chunks, map[string]any{
				"seq": 1, "stream": "stdout",
				"chunk": base64.StdEncoding.EncodeToString(proc.output),
			})
			proc.drained = true
		}
		return map[string]any{
			"chunks": chunks, "nextSeq": 2,
			"exited": true, "closed": true,
			"exitCode": proc.exitCode, "failure": nil, "sandboxDenied": false,
		}, nil

	case "process/terminate", "process/signal", "process/write":
		return map[string]any{}, nil

	default:
		return nil, map[string]any{
			"code": -32601, "message": "seam recorder does not implement " + req.Method,
		}
	}
}

// runArgv executes what codex asked for, locally. Same reasoning as the shell
// recorder: this suite measures shape, not destination.
func runArgv(argv []string) ([]byte, int) {
	cmd := exec.Command(argv[0], argv[1:]...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		var ee *exec.ExitError
		if asExitError(err, &ee) {
			return out, ee.ExitCode()
		}
		return append(out, []byte(err.Error())...), 127
	}
	return out, 0
}
