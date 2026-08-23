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

// Config configures the engine's adapters. An empty binary falls back to the
// harness's catalogued command, resolved from PATH.
type Config struct {
	// Binaries overrides a harness's executable, keyed by runtime id. A map
	// rather than one named field per harness, so adding a harness does not
	// mean editing this struct and every site that populates it.
	Binaries map[string]string
	// Modu chooses between a native SDK and its CLI, and the SDK needs to be
	// told where its configuration lives.
	ModuIntegration string
	ModuConfigPath  string
	ModuAgentDir    string
	// DshSessionRoot is where OneCatch keeps DeepSeek Harness session logs. The
	// adapter reads the harness's own log to recover its event stream, so it
	// needs a directory it controls rather than the harness's shared default.
	DshSessionRoot string
}

// Binary returns the configured executable for a runtime, or empty to let the
// adapter fall back to the catalogued command.
func (c Config) Binary(runtime Runtime) string { return c.Binaries[string(runtime)] }

// descriptor binds a catalogued harness to the code that drives it.
//
// The catalog in domain/harnesses says which harnesses exist and what is true
// about them; this says how to build one. Registering here is the single edit
// that makes a new adapter reachable — the engine, the desktop probe list, and
// the worker's availability report all iterate this rather than naming runtimes.
type descriptor struct {
	runtime Runtime
	build   func(Config) Runner
}

var descriptors = []descriptor{
	{RuntimeCodex, func(c Config) Runner { return NewCodexRunner(c.Binary(RuntimeCodex)) }},
	{RuntimeClaude, func(c Config) Runner { return NewClaudeRunner(c.Binary(RuntimeClaude)) }},
	{RuntimeModu, newModuRuntimeRunner},
	{RuntimePi, func(c Config) Runner { return NewPiRunner(c.Binary(RuntimePi)) }},
	{RuntimeGrok, func(c Config) Runner { return NewGrokRunner(c.Binary(RuntimeGrok)) }},
	{RuntimeDsh, func(c Config) Runner { return NewDshRunner(c.Binary(RuntimeDsh), c.DshSessionRoot) }},
}

// NewEngine builds an engine with every registered runtime runner.
func NewEngine(cfg Config) *Engine {
	runners := make([]Runner, 0, len(descriptors))
	for _, item := range descriptors {
		runners = append(runners, item.build(cfg))
	}
	return NewEngineWithRunners(runners...)
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
// can show users which local agents they can actually run. Registration order
// is stable, so the UI does not reshuffle between calls.
func (e *Engine) AvailableRuntimes() []Runtime {
	var out []Runtime
	for _, item := range descriptors {
		if e.Available(item.runtime) {
			out = append(out, item.runtime)
		}
	}
	return out
}

// Runtimes lists every registered runtime, installed or not.
func (e *Engine) Runtimes() []Runtime {
	out := make([]Runtime, 0, len(descriptors))
	for _, item := range descriptors {
		out = append(out, item.runtime)
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

// HarnessModel is one model a harness advertises.
type HarnessModel struct {
	Model       string `json:"model"`
	DisplayName string `json:"displayName"`
	Description string `json:"description,omitempty"`
	// Efforts is this model's own reasoning-effort vocabulary. Harnesses vary
	// it per model — Grok offers xhigh on 4.6 but not on 4.5 — so a single
	// harness-wide list would offer a level the model rejects.
	Efforts []string `json:"efforts,omitempty"`
	// DefaultEffort is the level this model uses when none is chosen.
	DefaultEffort string `json:"defaultEffort,omitempty"`
}

// HarnessConfiguration is what a harness reports about itself when asked.
type HarnessConfiguration struct {
	// Model is the harness's own current selection, when it reports one.
	Model  string         `json:"model,omitempty"`
	Models []HarnessModel `json:"models,omitempty"`
	// Efforts applies when a harness has one vocabulary for every model. A
	// model's own Efforts take precedence over this.
	Efforts []string `json:"efforts,omitempty"`
}

// ConfigurationInspector is implemented by runners that can report their models
// without starting a session or spending model quota.
type ConfigurationInspector interface {
	InspectConfiguration(ctx context.Context, cwd string, environment []string) (HarnessConfiguration, error)
}

// ErrInspectionUnsupported is returned for a runtime that cannot report its
// configuration, so callers can fall back to free-text entry.
type ErrInspectionUnsupported struct{ Runtime Runtime }

func (e ErrInspectionUnsupported) Error() string {
	return fmt.Sprintf("agentrun: runtime %q cannot report its configuration", e.Runtime)
}

// InspectConfiguration asks a runtime what models and reasoning levels it
// offers. One entry point rather than one method per harness, so the desktop
// and its bindings do not grow a copy for every new adapter.
func (e *Engine) InspectConfiguration(ctx context.Context, rt Runtime, cwd string, environment []string) (HarnessConfiguration, error) {
	runner := e.runners[rt]
	if runner == nil {
		return HarnessConfiguration{}, ErrUnknownRuntime{Runtime: rt}
	}
	if !runner.Available() {
		return HarnessConfiguration{}, ErrRuntimeUnavailable{Runtime: rt}
	}
	inspector, ok := runner.(ConfigurationInspector)
	if !ok {
		return HarnessConfiguration{}, ErrInspectionUnsupported{Runtime: rt}
	}
	return inspector.InspectConfiguration(ctx, cwd, environment)
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
