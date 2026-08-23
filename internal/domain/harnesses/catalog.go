// Package harnesses is the catalog of coding harnesses OneCatch can drive.
//
// It holds what is true about a harness itself — its name, its default command,
// which reasoning levels and providers it accepts, whether it can resume — as
// opposed to what a user chose for it, which lives in the settings model.
//
// One catalog exists because the alternative was tried and failed. The same
// facts used to be spelled out separately in task validation, in settings
// validation, in the engine's registry, in the desktop probe list, and again in
// the frontend. Adding a harness meant finding all of them, and a runtime added
// to the engine but missed in task validation shipped selectable but unusable.
//
// The catalog is deliberately declarative and dependency-free so every layer can
// read it: the domain validates against it, the agentrun engine builds runners
// from it, and the desktop publishes it to the UI instead of the UI keeping a
// second copy. Anything that needs code rather than data — how to spawn the
// harness, how to parse its events — belongs to its adapter, not here.
package harnesses

import "slices"

// Integration is how OneCatch talks to a harness.
const (
	// IntegrationCLI drives a child process.
	IntegrationCLI = "cli"
	// IntegrationSDK links the harness in-process.
	IntegrationSDK = "sdk"
)

// Harness is one entry in the catalog.
type Harness struct {
	// ID is the stable identifier used in settings, tasks, and workflow steps.
	ID string `json:"id"`
	// Name is the human label shown in the UI.
	Name string `json:"name"`
	// Command is the default executable, resolved from PATH when the user has
	// not configured a path.
	Command string `json:"command"`
	// Efforts is the reasoning-effort vocabulary this harness accepts. Empty
	// means it exposes no reasoning control, and configuring one is an error
	// rather than a silently ignored setting.
	Efforts []string `json:"efforts,omitempty"`
	// Providers is the provider vocabulary. Empty means the harness has a fixed
	// provider and does not offer a choice.
	Providers []string `json:"providers,omitempty"`
	// ServiceTiers reports whether the harness has a speed/processing tier.
	ServiceTiers bool `json:"serviceTiers,omitempty"`
	// Integrations lists the supported integrations, most preferred first. The
	// first entry is the default for a harness the user has not configured.
	Integrations []string `json:"integrations"`
	// EnvironmentHint is the placeholder shown for the environment allowlist:
	// the variables this harness typically needs. Empty means the UI should
	// describe the field generically, because the harness reads its credentials
	// from its own configuration rather than from the environment.
	EnvironmentHint string `json:"environmentHint,omitempty"`
	// CanResume reports whether a completed run can be continued in the same
	// session. A harness that cannot must reject a resume rather than silently
	// starting a fresh conversation.
	CanResume bool `json:"canResume"`
}

// DefaultIntegration is the integration used for a harness the user has not
// configured.
func (h Harness) DefaultIntegration() string {
	if len(h.Integrations) == 0 {
		return IntegrationCLI
	}
	return h.Integrations[0]
}

// SupportsIntegration reports whether this harness can run through integration.
func (h Harness) SupportsIntegration(integration string) bool {
	return slices.Contains(h.Integrations, integration)
}

// SupportsEffort reports whether effort is a level this harness accepts. An
// empty effort always passes: it means "leave the harness's own default".
func (h Harness) SupportsEffort(effort string) bool {
	return effort == "" || slices.Contains(h.Efforts, effort)
}

// SupportsProvider reports whether provider is one this harness can route to.
// An empty provider always passes.
func (h Harness) SupportsProvider(provider string) bool {
	return provider == "" || slices.Contains(h.Providers, provider)
}

// catalog is the ordered list every consumer derives from. The order is the one
// the desktop lists harnesses in.
var catalog = []Harness{
	{
		ID: "codex", Name: "Codex", Command: "codex",
		Efforts:      []string{"minimal", "low", "medium", "high", "xhigh", "max", "ultra"},
		ServiceTiers: true,
		Integrations: []string{IntegrationCLI},
		// Codex is the only harness with a speed/processing tier.
		EnvironmentHint: "OPENAI_API_KEY, HTTPS_PROXY",
		CanResume:       true,
	},
	{
		ID: "claude", Name: "Claude Code", Command: "claude",
		Efforts:         []string{"low", "medium", "high", "xhigh", "max"},
		Integrations:    []string{IntegrationCLI},
		EnvironmentHint: "ANTHROPIC_API_KEY, HTTPS_PROXY",
		CanResume:       true,
	},
	{
		ID: "modu", Name: "Modu Code", Command: "modu_code",
		Providers: []string{"auto", "openai", "anthropic", "gemini"},
		// The native SDK is preferred; the CLI remains for standalone installs.
		Integrations: []string{IntegrationSDK, IntegrationCLI},
		// Credentials come from the harness's own TOML rather than the
		// environment, so there is no useful variable to suggest.
		CanResume: true,
	},
	{
		ID: "pi", Name: "Pi", Command: "pi",
		// Pi spells this --thinking, and unlike the others it can be turned off
		// outright rather than only left at the model's default.
		Efforts:         []string{"off", "minimal", "low", "medium", "high", "xhigh"},
		Integrations:    []string{IntegrationCLI},
		EnvironmentHint: "ANTHROPIC_API_KEY, OPENAI_API_KEY, GEMINI_API_KEY",
		CanResume:       true,
	},
	{
		ID: "grok", Name: "Grok Build", Command: "grok",
		// Read from Grok's own model catalog during the ACP handshake.
		Efforts:         []string{"low", "medium", "high", "xhigh"},
		Integrations:    []string{IntegrationCLI},
		EnvironmentHint: "XAI_API_KEY, HTTPS_PROXY",
		CanResume:       true,
	},
	{
		ID: "dsh", Name: "DeepSeek Harness", Command: "dsh",
		// The harness routes through named provider entries in its own plugin
		// composition rather than through a generic provider name.
		Providers:       []string{"deepseek-official", "pi-ai"},
		Integrations:    []string{IntegrationCLI},
		EnvironmentHint: "DEEPSEEK_API_KEY, HTTPS_PROXY",
		// Its one-shot headless profile creates a fresh agent per invocation
		// and exposes no resume flag.
		CanResume: false,
	},
}

// Catalog returns every known harness, in display order. The slice is a copy so
// a caller cannot reorder or mutate the shared catalog.
func Catalog() []Harness {
	out := make([]Harness, len(catalog))
	copy(out, catalog)
	return out
}

// Find returns the harness with this id.
func Find(id string) (Harness, bool) {
	for _, harness := range catalog {
		if harness.ID == id {
			return harness, true
		}
	}
	return Harness{}, false
}

// IDs returns every harness identifier, in display order.
func IDs() []string {
	out := make([]string, 0, len(catalog))
	for _, harness := range catalog {
		out = append(out, harness.ID)
	}
	return out
}

// IsKnown reports whether id names a harness the product can run.
func IsKnown(id string) bool {
	_, ok := Find(id)
	return ok
}
