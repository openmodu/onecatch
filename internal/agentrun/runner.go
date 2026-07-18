package agentrun

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"
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
	Workspace string
	// Prompt is the task description handed to the agent.
	Prompt string
	// Model optionally overrides the runtime's default model.
	Model string
	// ReasoningEffort optionally overrides the Codex model's reasoning effort.
	ReasoningEffort string
	// ServiceTier optionally selects Codex speed/processing tier. "standard"
	// explicitly resets app-server runs to the model's standard tier.
	ServiceTier string
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
}

// Sink receives normalized events as they stream from the agent. It is called
// from the engine's goroutine; implementations must not block for long.
type Sink func(Event)

// Runner drives a single local agent runtime.
type Runner interface {
	// Runtime is the runtime this runner drives.
	Runtime() Runtime
	// Available reports whether the runtime's CLI is installed and runnable.
	Available() bool
	// Run executes req to completion, delivering events to sink as they
	// arrive, and returns the terminal result. A non-nil error means the run
	// could not complete (process failure, context cancellation); a completed
	// run that the agent itself reported as failed returns a Result with
	// Succeeded=false and a nil error.
	Run(ctx context.Context, req Request, sink Sink) (Result, error)
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
// It is the shared spine of every adapter: adapters only build the command and
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

	scanner := bufio.NewScanner(stdout)
	// Agent lines (especially Claude's init event) can be large; lift the
	// default 64KiB token cap so we never silently drop a line.
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
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
	if len(c.buf) == 0 {
		return ""
	}
	return ": " + string(c.buf)
}
