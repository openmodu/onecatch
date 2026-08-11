// Package settings defines the versioned, local application settings model.
package settings

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

const CurrentSchemaVersion = 1

const (
	SectionRuntime      = "runtime"
	SectionTerminal     = "terminal"
	SectionExecution    = "execution"
	SectionSecurity     = "security"
	SectionStorage      = "storage"
	SectionExperimental = "experimental"
)

type Settings struct {
	SchemaVersion int                        `json:"schemaVersion"`
	Revision      int64                      `json:"revision"`
	Runtimes      map[string]RuntimeSettings `json:"runtimes"`
	Terminal      TerminalSettings           `json:"terminal"`
	Execution     ExecutionSettings          `json:"execution"`
	Security      SecuritySettings           `json:"security"`
	Storage       StorageSettings            `json:"storage"`
	Experimental  ExperimentalSettings       `json:"experimental"`
	UpdatedAt     time.Time                  `json:"updatedAt"`
}

type RuntimeSettings struct {
	Binary               string   `json:"binary,omitempty"`
	DefaultModel         string   `json:"defaultModel,omitempty"`
	ReasoningEffort      string   `json:"reasoningEffort,omitempty"`
	ServiceTier          string   `json:"serviceTier,omitempty"`
	Provider             string   `json:"provider,omitempty"`
	EnvironmentAllowlist []string `json:"environmentAllowlist,omitempty"`
}

type TerminalSettings struct {
	Shell     string   `json:"shell,omitempty"`
	Arguments []string `json:"arguments,omitempty"`
	Theme     string   `json:"theme"`
}

type ExecutionSettings struct {
	MaxTransitions         int    `json:"maxTransitions"`
	MaxConsecutiveFailures int    `json:"maxConsecutiveFailures"`
	StepTimeoutSeconds     int    `json:"stepTimeoutSeconds"`
	MaxLocalDAGConcurrency int    `json:"maxLocalDAGConcurrency"`
	InterruptGraceSeconds  int    `json:"interruptGraceSeconds"`
	DefaultSandbox         string `json:"defaultSandbox"`
}

type SecuritySettings struct {
	AllowFullSandbox            bool `json:"allowFullSandbox"`
	ConfirmFullSandboxEveryRun  bool `json:"confirmFullSandboxEveryRun"`
	DiagnosticsIncludePrompt    bool `json:"diagnosticsIncludePrompt"`
	DiagnosticsIncludeRawEvents bool `json:"diagnosticsIncludeRawEvents"`
}

type StorageSettings struct {
	CompletedRunRetentionDays int    `json:"completedRunRetentionDays"`
	LogLevel                  string `json:"logLevel"`
	LogMaxSizeMB              int    `json:"logMaxSizeMB"`
	LogMaxBackups             int    `json:"logMaxBackups"`
	LogMaxAgeDays             int    `json:"logMaxAgeDays"`
}

type ExperimentalSettings struct {
	RemoteWorkersEnabled bool `json:"remoteWorkersEnabled"`
}

func Defaults() Settings {
	return Settings{
		SchemaVersion: CurrentSchemaVersion,
		Revision:      1,
		Runtimes: map[string]RuntimeSettings{
			"codex":  {},
			"claude": {},
			"modu":   {},
		},
		Terminal: TerminalSettings{Theme: "system"},
		Execution: ExecutionSettings{
			MaxTransitions: 20, MaxConsecutiveFailures: 3, StepTimeoutSeconds: 1800,
			MaxLocalDAGConcurrency: 4, InterruptGraceSeconds: 10, DefaultSandbox: "workspace-write",
		},
		Security: SecuritySettings{ConfirmFullSandboxEveryRun: true},
		Storage: StorageSettings{
			CompletedRunRetentionDays: 0, LogLevel: "info", LogMaxSizeMB: 20, LogMaxBackups: 5, LogMaxAgeDays: 14,
		},
		Experimental: ExperimentalSettings{},
	}
}

// Normalize fills fields introduced by schema v1 without changing explicit
// boolean values. It is intentionally schema-aware so false remains meaningful.
func Normalize(input Settings) (Settings, error) {
	if input.SchemaVersion > CurrentSchemaVersion {
		return Settings{}, fmt.Errorf("unsupported settings schema version %d", input.SchemaVersion)
	}
	defaults := Defaults()
	if input.SchemaVersion == 0 {
		input.SchemaVersion = CurrentSchemaVersion
	}
	if input.Revision < 1 {
		input.Revision = 1
	}
	if input.Runtimes == nil {
		input.Runtimes = defaults.Runtimes
	}
	for _, id := range []string{"codex", "claude", "modu"} {
		if _, ok := input.Runtimes[id]; !ok {
			input.Runtimes[id] = RuntimeSettings{}
		}
	}
	if input.Execution.MaxTransitions == 0 {
		input.Execution.MaxTransitions = defaults.Execution.MaxTransitions
	}
	input.Terminal.Shell = strings.TrimSpace(input.Terminal.Shell)
	input.Terminal.Theme = strings.ToLower(strings.TrimSpace(input.Terminal.Theme))
	if input.Terminal.Theme == "" {
		input.Terminal.Theme = defaults.Terminal.Theme
	}
	arguments := make([]string, 0, len(input.Terminal.Arguments))
	for _, argument := range input.Terminal.Arguments {
		if argument = strings.TrimSpace(argument); argument != "" {
			arguments = append(arguments, argument)
		}
	}
	input.Terminal.Arguments = arguments
	if input.Execution.MaxConsecutiveFailures == 0 {
		input.Execution.MaxConsecutiveFailures = defaults.Execution.MaxConsecutiveFailures
	}
	if input.Execution.StepTimeoutSeconds == 0 {
		input.Execution.StepTimeoutSeconds = defaults.Execution.StepTimeoutSeconds
	}
	if input.Execution.MaxLocalDAGConcurrency == 0 {
		input.Execution.MaxLocalDAGConcurrency = defaults.Execution.MaxLocalDAGConcurrency
	}
	if input.Execution.InterruptGraceSeconds == 0 {
		input.Execution.InterruptGraceSeconds = defaults.Execution.InterruptGraceSeconds
	}
	if input.Execution.DefaultSandbox == "" {
		input.Execution.DefaultSandbox = defaults.Execution.DefaultSandbox
	}
	if input.Storage.LogLevel == "" {
		input.Storage.LogLevel = defaults.Storage.LogLevel
	}
	if input.Storage.LogMaxSizeMB == 0 {
		input.Storage.LogMaxSizeMB = defaults.Storage.LogMaxSizeMB
	}
	if input.Storage.LogMaxBackups == 0 {
		input.Storage.LogMaxBackups = defaults.Storage.LogMaxBackups
	}
	if input.Storage.LogMaxAgeDays == 0 {
		input.Storage.LogMaxAgeDays = defaults.Storage.LogMaxAgeDays
	}
	for id, runtime := range input.Runtimes {
		runtime.Binary = strings.TrimSpace(runtime.Binary)
		runtime.DefaultModel = strings.TrimSpace(runtime.DefaultModel)
		runtime.ReasoningEffort = strings.ToLower(strings.TrimSpace(runtime.ReasoningEffort))
		runtime.ServiceTier = strings.ToLower(strings.TrimSpace(runtime.ServiceTier))
		runtime.Provider = strings.ToLower(strings.TrimSpace(runtime.Provider))
		runtime.EnvironmentAllowlist = normalizeKeys(runtime.EnvironmentAllowlist)
		input.Runtimes[id] = runtime
	}
	return input, nil
}

var (
	environmentKey = regexp.MustCompile(`^[A-Z_][A-Z0-9_]{0,127}$`)
	serviceTierKey = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)
)

func Validate(input Settings) error {
	if input.SchemaVersion != CurrentSchemaVersion {
		return errors.New("schemaVersion must be 1")
	}
	if input.Revision < 1 {
		return errors.New("revision must be positive")
	}
	for id, runtime := range input.Runtimes {
		if id != "codex" && id != "claude" && id != "modu" {
			return fmt.Errorf("unknown runtime %q", id)
		}
		if strings.ContainsAny(runtime.Binary+runtime.DefaultModel+runtime.ReasoningEffort+runtime.ServiceTier+runtime.Provider, "\r\n\x00") {
			return fmt.Errorf("runtime %s contains control characters", id)
		}
		if id == "codex" {
			if !contains([]string{"", "minimal", "low", "medium", "high", "xhigh", "max", "ultra"}, runtime.ReasoningEffort) {
				return errors.New("codex reasoning effort is invalid")
			}
			if runtime.ServiceTier != "" && !serviceTierKey.MatchString(runtime.ServiceTier) {
				return errors.New("codex service tier is invalid")
			}
		} else if id == "claude" {
			if !contains([]string{"", "low", "medium", "high", "xhigh", "max"}, runtime.ReasoningEffort) {
				return errors.New("Claude Code reasoning effort is invalid")
			}
			if runtime.ServiceTier != "" {
				return errors.New("Claude Code does not support service tier settings")
			}
		} else if runtime.ReasoningEffort != "" || runtime.ServiceTier != "" {
			return fmt.Errorf("runtime %s does not support reasoning or service tier settings", id)
		}
		if id == "modu" {
			if !contains([]string{"", "auto", "openai", "anthropic", "gemini"}, runtime.Provider) {
				return errors.New("modu provider must be auto, openai, anthropic, or gemini")
			}
		} else if runtime.Provider != "" {
			return fmt.Errorf("runtime %s does not support provider selection", id)
		}
		for _, key := range runtime.EnvironmentAllowlist {
			if !environmentKey.MatchString(key) || forbiddenEnvironmentKey(key) {
				return fmt.Errorf("environment key %q is not allowed", key)
			}
		}
	}
	if strings.ContainsAny(input.Terminal.Shell, "\r\n\x00") {
		return errors.New("terminal shell contains control characters")
	}
	for _, argument := range input.Terminal.Arguments {
		if strings.ContainsAny(argument, "\r\n\x00") {
			return errors.New("terminal argument contains control characters")
		}
	}
	if !contains([]string{"system", "paper", "midnight", "contrast"}, input.Terminal.Theme) {
		return errors.New("terminal theme is invalid")
	}
	e := input.Execution
	if e.MaxTransitions < 1 || e.MaxTransitions > 10000 {
		return errors.New("maxTransitions must be between 1 and 10000")
	}
	if e.MaxConsecutiveFailures < 1 || e.MaxConsecutiveFailures > 100 {
		return errors.New("maxConsecutiveFailures must be between 1 and 100")
	}
	if e.StepTimeoutSeconds < 30 || e.StepTimeoutSeconds > 86400 {
		return errors.New("stepTimeoutSeconds must be between 30 and 86400")
	}
	if e.MaxLocalDAGConcurrency < 1 || e.MaxLocalDAGConcurrency > 16 {
		return errors.New("maxLocalDAGConcurrency must be between 1 and 16")
	}
	if e.InterruptGraceSeconds < 1 || e.InterruptGraceSeconds > 60 {
		return errors.New("interruptGraceSeconds must be between 1 and 60")
	}
	if e.DefaultSandbox != "read-only" && e.DefaultSandbox != "workspace-write" {
		return errors.New("defaultSandbox must be read-only or workspace-write")
	}
	if input.Storage.CompletedRunRetentionDays != 0 && input.Storage.CompletedRunRetentionDays != 30 && input.Storage.CompletedRunRetentionDays != 90 && input.Storage.CompletedRunRetentionDays != 180 {
		return errors.New("completedRunRetentionDays must be 0, 30, 90, or 180")
	}
	if !contains([]string{"error", "warn", "info", "debug"}, input.Storage.LogLevel) {
		return errors.New("logLevel is invalid")
	}
	if input.Storage.LogMaxSizeMB < 1 || input.Storage.LogMaxSizeMB > 1024 {
		return errors.New("logMaxSizeMB must be between 1 and 1024")
	}
	if input.Storage.LogMaxBackups < 1 || input.Storage.LogMaxBackups > 50 {
		return errors.New("logMaxBackups must be between 1 and 50")
	}
	if input.Storage.LogMaxAgeDays < 1 || input.Storage.LogMaxAgeDays > 365 {
		return errors.New("logMaxAgeDays must be between 1 and 365")
	}
	return nil
}

func DefaultSection(section string) (any, error) {
	d := Defaults()
	switch section {
	case SectionRuntime:
		return d.Runtimes, nil
	case SectionTerminal:
		return d.Terminal, nil
	case SectionExecution:
		return d.Execution, nil
	case SectionSecurity:
		return d.Security, nil
	case SectionStorage:
		return d.Storage, nil
	case SectionExperimental:
		return d.Experimental, nil
	default:
		return nil, fmt.Errorf("unknown settings section %q", section)
	}
}

func forbiddenEnvironmentKey(key string) bool {
	return contains([]string{"PATH", "HOME", "SHELL", "BASH_ENV", "ENV", "ZDOTDIR"}, key) || strings.HasPrefix(key, "DYLD_") || strings.HasPrefix(key, "LD_")
}

func normalizeKeys(keys []string) []string {
	seen := make(map[string]struct{}, len(keys))
	out := make([]string, 0, len(keys))
	for _, raw := range keys {
		key := strings.ToUpper(strings.TrimSpace(raw))
		if key == "" {
			continue
		}
		if _, ok := seen[key]; !ok {
			seen[key] = struct{}{}
			out = append(out, key)
		}
	}
	sort.Strings(out)
	return out
}

func contains(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}
