package agentrun

import (
	"context"
	"fmt"
)

// ErrUnknownRuntime is returned when a request names a runtime the engine has
// no runner for.
type ErrUnknownRuntime struct{ Runtime Runtime }

func (e ErrUnknownRuntime) Error() string {
	return fmt.Sprintf("agentrun: no runner for runtime %q", e.Runtime)
}

// ErrRuntimeUnavailable is returned when the runtime's CLI is not installed.
type ErrRuntimeUnavailable struct{ Runtime Runtime }

func (e ErrRuntimeUnavailable) Error() string {
	return fmt.Sprintf("agentrun: runtime %q is not installed", e.Runtime)
}

// Engine selects the right runner for each request and is the single entry
// point the rest of the product uses to run local agents.
type Engine struct {
	runners map[Runtime]Runner
}

// Config configures the engine's adapters and optional binary overrides. Empty
// CLI paths fall back to the command name resolved from PATH.
type Config struct {
	CodexBinary     string
	ClaudeBinary    string
	ModuBinary      string
	ModuIntegration string
	ModuConfigPath  string
	ModuAgentDir    string
	PiBinary        string
	GrokBinary      string
	DshBinary       string
	// DshSessionRoot is where OneCatch keeps DeepSeek Harness session logs. The
	// adapter reads the harness's own log to recover its event stream, so it
	// needs a directory it controls rather than the harness's shared default.
	DshSessionRoot string
}

// NewEngine builds an engine with all standard local runtime runners.
func NewEngine(cfg Config) *Engine {
	return NewEngineWithRunners(
		NewCodexRunner(cfg.CodexBinary),
		NewClaudeRunner(cfg.ClaudeBinary),
		newModuRuntimeRunner(cfg),
		NewPiRunner(cfg.PiBinary),
		NewGrokRunner(cfg.GrokBinary),
		NewDshRunner(cfg.DshBinary, cfg.DshSessionRoot),
	)
}

// NewEngineWithRunners builds an engine from an explicit set of runners. Tests
// use it to inject stub runners.
func NewEngineWithRunners(runners ...Runner) *Engine {
	m := make(map[Runtime]Runner, len(runners))
	for _, r := range runners {
		m[r.Runtime()] = r
	}
	return &Engine{runners: m}
}

// Runner returns the runner for a runtime, or nil if none is registered.
func (e *Engine) Runner(rt Runtime) Runner {
	return e.runners[rt]
}

// Available reports whether the named runtime is installed and runnable.
func (e *Engine) Available(rt Runtime) bool {
	r := e.runners[rt]
	return r != nil && r.Available()
}

// AvailableRuntimes lists every runtime whose CLI is installed, so the product
// can show users which local agents they can actually run.
func (e *Engine) AvailableRuntimes() []Runtime {
	// Stable order so the UI does not reshuffle between calls.
	order := []Runtime{RuntimeCodex, RuntimeClaude, RuntimeModu, RuntimePi, RuntimeGrok, RuntimeDsh}
	var out []Runtime
	for _, rt := range order {
		if e.Available(rt) {
			out = append(out, rt)
		}
	}
	return out
}

// SupportsInteractivePermissions reports whether a run of this runtime and
// sandbox will ask the host to approve tool calls. Hosts use it to decide
// whether to install a PermissionHandler; a runtime that cannot ask would
// otherwise be given a handler that is never called.
func (e *Engine) SupportsInteractivePermissions(rt Runtime, sandbox Sandbox) bool {
	runner, ok := e.runners[rt].(InteractivePermissionRunner)
	return ok && runner.SupportsInteractivePermissions(sandbox)
}

// Run validates the request, selects the runner, and executes it. It returns
// ErrUnknownRuntime or ErrRuntimeUnavailable before spawning anything so
// callers can fail fast with a clear reason.
func (e *Engine) Run(ctx context.Context, req Request, sink Sink) (Result, error) {
	if !req.Runtime.Valid() {
		return Result{}, ErrUnknownRuntime{Runtime: req.Runtime}
	}
	runner := e.runners[req.Runtime]
	if runner == nil {
		return Result{}, ErrUnknownRuntime{Runtime: req.Runtime}
	}
	if !runner.Available() {
		return Result{}, ErrRuntimeUnavailable{Runtime: req.Runtime}
	}
	if req.Sandbox == "" {
		req.Sandbox = SandboxWorkspaceWrite
	}
	return runner.Run(ctx, req, sink)
}
