package agentrun

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/openmodu/onecatch/internal/usecase/agentrun/seam"
)

// Sandbox controls how much access the agent's tool calls are granted. It maps
// onto each runtime's native permission model.
type Sandbox string

const (
	// SandboxReadOnly lets the agent read the workspace but not write or run
	// commands. Useful for analysis-only tasks.
	SandboxReadOnly Sandbox = "read-only"
	// SandboxWorkspaceWrite lets the agent read and write within its workspace
	// (the default for producing deliverables).
	SandboxWorkspaceWrite Sandbox = "workspace-write"
	// SandboxFull removes sandboxing entirely. Reserved for trusted automation.
	SandboxFull Sandbox = "full"
)

// Request describes one long-horizon task for a runtime to execute.
type Request struct {
	// Runtime selects which local CLI to drive.
	Runtime Runtime
	// Workspace is the working directory the agent operates in. Files it
	// produces here become the task's deliverables. Must exist.
	//
	// When Remote is set this is still a real local directory — the harness
	// process needs one — but it is not where the agent's work happens. The
	// agent's commands run in Remote.Root on the target, and the deliverables
	// land there.
	Workspace string

	// Remote, when set, runs the agent's shell commands on another machine
	// while the agent itself, and the credentials it holds, stay here.
	//
	// Claude reaches the target through a transparently remote shell; its
	// Read/Edit/Write tools use a conflict-checked sparse mirror while search
	// stays on the remote shell. Codex uses its remote-environment protocol, so
	// both shell commands and native fs tools are redirected. Grok's ACP client
	// operations and Modu's native SDK workspace tools are backed by SSH/SFTP
	// implementations. No adapter falls back to local execution if its seam
	// cannot be established.
	Remote *seam.Target
	// Prompt is the task description handed to the agent.
	Prompt string
	// Model optionally overrides the runtime's default model.
	Model string
	// ReasoningEffort optionally overrides the Codex model's reasoning effort.
	ReasoningEffort string
	// ServiceTier optionally selects Codex speed/processing tier. "standard"
	// explicitly resets app-server runs to the model's standard tier.
	ServiceTier string
	// MaxContextWindow asks the harness to run the model at the largest window
	// it accepts rather than the harness's default. Off unless the user turns
	// it on: a bigger window is not free — a full context makes every turn's
	// input larger — and only some models have headroom at all.
	MaxContextWindow bool
	// Provider optionally selects a runtime provider. It is currently consumed
	// by Modu Code and ignored by runtimes with a fixed provider.
	Provider string
	// Sandbox selects the permission level; defaults to SandboxWorkspaceWrite.
	Sandbox Sandbox
	// ResumeSessionID, when set, continues a prior run instead of starting a
	// fresh one: the runtime reopens that session (keeping its context and
	// history) and applies Prompt as the next turn. The id is the SessionID a
	// previous Result reported.
	ResumeSessionID string
	// Environment is the complete environment inherited by the child process.
	// Nil preserves the parent process environment.
	Environment []string
	// EnvironmentAllowlist freezes the names inherited for this Run. A non-nil
	// empty slice means inherit only the required baseline environment.
	EnvironmentAllowlist []string
	// InterruptGrace controls how long a cancelled process may exit cleanly.
	InterruptGrace time.Duration
	// RuntimeDefaultsResolved means the caller already froze runtime defaults
	// into a durable run snapshot. The registry must not apply newer settings.
	RuntimeDefaultsResolved bool
	// RunID and StepRunID associate interactive runtime requests with their
	// durable workflow records. They are intentionally optional for standalone
	// helpers that do not expose approval UI.
	RunID     string
	StepRunID string
	// PermissionHandler blocks until the host approves or denies a tool call.
	// It is never serialized to a remote worker.
	PermissionHandler func(context.Context, PermissionRequest) (PermissionDecision, error)
}

// Sink receives normalized events as they stream from the agent. It is called
// from the engine's goroutine; implementations must not block for long.
type Sink func(Event)

// Runner drives a single local agent runtime through either an SDK or process adapter.
type Runner interface {
	// Runtime is the runtime this runner drives.
	Runtime() Runtime
	// Available reports whether the configured adapter is available.
	Available() bool
	// Run executes req to completion, delivering events to sink as they
	// arrive, and returns the terminal result. A non-nil error means the run
	// could not complete (process failure, context cancellation); a completed
	// run that the agent itself reported as failed returns a Result with
	// Succeeded=false and a nil error.
	Run(ctx context.Context, req Request, sink Sink) (Result, error)
}

// InteractivePermissionRunner is implemented by runners that can pause on a
// tool call and block until the host approves or denies it.
//
// Whether a harness can do this is a property of the harness and the sandbox it
// was asked for, not of the caller: Claude Code only opens its control channel
// for a read-only run, while an ACP harness carries permission requests in
// every mode. Hosts ask the engine rather than testing the runtime id, so a new
// harness does not need the question answered again in each caller.
type InteractivePermissionRunner interface {
	// SupportsInteractivePermissions reports whether a run with this sandbox
	// will route its tool approvals to Request.PermissionHandler.
	SupportsInteractivePermissions(sandbox Sandbox) bool
}

// lineParser turns one stdout JSONL line into zero or more normalized events
// and accumulates terminal state. Each runtime adapter provides one.
type lineParser interface {
	// parse handles a single line, emitting events through sink.
	parse(line string, at time.Time, sink Sink)
	// result returns the terminal outcome after the stream closes.
	result() Result
}

// nowFunc is overridable in tests for deterministic timestamps.
type nowFunc func() time.Time

// streamProcess runs cmd, scanning stdout line by line through parser and
// forwarding normalized events to sink. stderr is captured and, on a non-zero
// exit, folded into the returned error so failures are diagnosable.
//
// It is the shared spine of the JSONL adapters: they only build the command and
// supply a parser.
func streamProcess(ctx context.Context, cmd *exec.Cmd, parser lineParser, now nowFunc, sink Sink) (Result, error) {
	if now == nil {
		now = time.Now
	}
	if sink == nil {
		sink = func(Event) {}
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return Result{}, fmt.Errorf("stdout pipe: %w", err)
	}
	var stderr lineCapture
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return Result{}, fmt.Errorf("start %s: %w", cmd.Path, err)
	}

	scanner := newJSONLineScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		parser.parse(line, now(), sink)
	}
	scanErr := scanner.Err()
	var drainErr error
	if scanErr != nil && !errors.Is(scanErr, io.ErrClosedPipe) {
		sink(Event{Kind: KindError, Text: fmt.Sprintf("read stream: %v", scanErr), At: now()})
		// Scanner stops consuming after errors such as ErrTooLong. Keep draining
		// the pipe before Wait so a child with more output cannot block forever
		// on a full stdout pipe. The unread scanner buffer is intentionally
		// discarded because the JSONL token is already incomplete.
		_, drainErr = io.Copy(io.Discard, stdout)
	}

	waitErr := cmd.Wait()
	res := parser.result()
	if waitErr != nil {
		// Context cancellation is a deliberate stop, not an agent failure;
		// surface it plainly so callers can distinguish.
		if ctx.Err() != nil {
			return res, ctx.Err()
		}
		res.Succeeded = false
		return res, fmt.Errorf("%s exited: %w%s", cmd.Path, waitErr, stderr.tail())
	}
	if scanErr != nil && !errors.Is(scanErr, io.ErrClosedPipe) {
		res.Succeeded = false
		if drainErr != nil && !errors.Is(drainErr, io.ErrClosedPipe) {
			return res, fmt.Errorf("read %s stdout: %w (drain: %v)", cmd.Path, scanErr, drainErr)
		}
		return res, fmt.Errorf("read %s stdout: %w", cmd.Path, scanErr)
	}
	return res, nil
}

// toolInputPath reads the file a tool call targets. Harnesses declare `path`
// on their filesystem tools but also accept `file_path` from models that reach
// for the other spelling, so both are checked.
func toolInputPath(input map[string]any) string {
	for _, key := range []string{"path", "file_path"} {
		if value, ok := input[key].(string); ok && strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

// newJSONLineScanner returns a scanner sized for agent JSONL.
//
// Harness lines are routinely far larger than bufio's default 64KiB token —
// Claude's init event and Grok's initialize result both carry whole catalogs —
// and a line silently dropped for being too long would lose a protocol frame.
func newJSONLineScanner(r io.Reader) *bufio.Scanner {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	return scanner
}

// lineCapture is an io.Writer that retains the tail of a stream so process
// failures can include the agent's stderr without buffering it unbounded.
type lineCapture struct {
	mu  sync.Mutex
	buf []byte
}

const stderrTailMax = 4 * 1024

func (c *lineCapture) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.buf = append(c.buf, p...)
	if len(c.buf) > stderrTailMax {
		c.buf = c.buf[len(c.buf)-stderrTailMax:]
	}
	return len(p), nil
}

func (c *lineCapture) tail() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return streamTail(c.buf)
}

// streamTail renders the tail of a captured stream for an error message. It is
// the same framing lineCapture applies, for callers that buffer a stream in
// full because they also have to parse it.
func streamTail(buf []byte) string {
	if len(buf) > stderrTailMax {
		buf = buf[len(buf)-stderrTailMax:]
	}
	if len(buf) == 0 {
		return ""
	}
	return ": " + string(buf)
}
