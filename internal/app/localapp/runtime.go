package localapp

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/openmodu/oneshot/internal/agentrun"
	domainsettings "github.com/openmodu/oneshot/internal/domain/settings"
)

type RuntimeConfig struct {
	CodexBinary  string `json:"codexBinary,omitempty"`
	ClaudeBinary string `json:"claudeBinary,omitempty"`
	ModuBinary   string `json:"moduBinary,omitempty"`
}

type RuntimeConfigInput struct {
	Runtime string `json:"runtime"`
	Binary  string `json:"binary"`
}

type RuntimeInfo struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Available bool      `json:"available"`
	Version   string    `json:"version,omitempty"`
	CheckedAt time.Time `json:"checkedAt"`
}

// RuntimeRegistry is a hot-swappable Engine. The orchestrator keeps this
// pointer while users update local CLI paths from the desktop application.
type RuntimeRegistry struct {
	mu             sync.RWMutex
	config         RuntimeConfig
	settings       map[string]domainsettings.RuntimeSettings
	interruptGrace time.Duration
	engine         *agentrun.Engine
}

func NewRuntimeRegistry(root string) (*RuntimeRegistry, error) {
	_ = root
	registry := &RuntimeRegistry{}
	registry.replace(RuntimeConfig{})
	return registry, nil
}

func (r *RuntimeRegistry) Available(runtime agentrun.Runtime) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.engine.Available(runtime)
}

func (r *RuntimeRegistry) Run(ctx context.Context, request agentrun.Request, sink agentrun.Sink) (agentrun.Result, error) {
	r.mu.RLock()
	engine := r.engine
	runtimeSettings := r.settings[string(request.Runtime)]
	interruptGrace := r.interruptGrace
	r.mu.RUnlock()
	if request.Model == "" {
		request.Model = runtimeSettings.DefaultModel
	}
	allowlist := runtimeSettings.EnvironmentAllowlist
	if request.EnvironmentAllowlist != nil {
		allowlist = request.EnvironmentAllowlist
	}
	if request.Environment == nil {
		request.Environment = allowedEnvironment(allowlist)
	}
	if request.Runtime == agentrun.RuntimeModu {
		provider := runtimeSettings.Provider
		if request.Provider != "" {
			provider = request.Provider
		}
		request.Environment = configureModuProvider(request.Environment, provider)
	}
	if request.InterruptGrace <= 0 {
		request.InterruptGrace = interruptGrace
	}
	return engine.Run(ctx, request, sink)
}

func (r *RuntimeRegistry) List() []RuntimeInfo {
	r.mu.RLock()
	config := r.config
	engine := r.engine
	r.mu.RUnlock()
	return []RuntimeInfo{
		runtimeInfo(engine, agentrun.RuntimeCodex, "Codex", config.CodexBinary),
		runtimeInfo(engine, agentrun.RuntimeClaude, "Claude Code", config.ClaudeBinary),
		runtimeInfo(engine, agentrun.RuntimeModu, "Modu Code", config.ModuBinary),
	}
}

func (r *RuntimeRegistry) Check(runtime string) (RuntimeInfo, error) {
	for _, item := range r.List() {
		if item.ID == runtime {
			return item, nil
		}
	}
	return RuntimeInfo{}, coded("runtime_unknown", "unknown runtime")
}

func (r *RuntimeRegistry) Update(input RuntimeConfigInput) (RuntimeInfo, error) {
	input.Runtime = strings.TrimSpace(input.Runtime)
	input.Binary = strings.TrimSpace(input.Binary)
	if input.Binary != "" {
		if _, err := exec.LookPath(input.Binary); err != nil {
			return RuntimeInfo{}, coded("runtime_unavailable", "binary is not executable")
		}
	}
	r.mu.Lock()
	config := r.config
	switch input.Runtime {
	case string(agentrun.RuntimeCodex):
		config.CodexBinary = input.Binary
	case string(agentrun.RuntimeClaude):
		config.ClaudeBinary = input.Binary
	case string(agentrun.RuntimeModu):
		config.ModuBinary = input.Binary
	default:
		r.mu.Unlock()
		return RuntimeInfo{}, coded("runtime_unknown", "unknown runtime")
	}
	r.config = config
	r.engine = newRuntimeEngine(config)
	r.mu.Unlock()
	return r.Check(input.Runtime)
}

func (r *RuntimeRegistry) ApplySettings(runtimes map[string]domainsettings.RuntimeSettings, interruptGraceSeconds int) {
	r.mu.Lock()
	copySettings := make(map[string]domainsettings.RuntimeSettings, len(runtimes))
	for id, item := range runtimes {
		copySettings[id] = item
	}
	r.settings = copySettings
	r.interruptGrace = time.Duration(interruptGraceSeconds) * time.Second
	config := RuntimeConfig{CodexBinary: runtimes["codex"].Binary, ClaudeBinary: runtimes["claude"].Binary, ModuBinary: runtimes["modu"].Binary}
	r.replace(config)
	r.mu.Unlock()
}

func (r *RuntimeRegistry) CheckDraft(runtime string, input domainsettings.RuntimeSettings) (RuntimeInfo, error) {
	name := map[string]string{"codex": "Codex", "claude": "Claude Code", "modu": "Modu Code"}[runtime]
	if name == "" {
		return RuntimeInfo{}, coded("runtime_unknown", "unknown runtime")
	}
	engine := agentrun.NewEngine(agentrun.Config{CodexBinary: func() string {
		if runtime == "codex" {
			return input.Binary
		}
		return ""
	}(), ClaudeBinary: func() string {
		if runtime == "claude" {
			return input.Binary
		}
		return ""
	}(), ModuBinary: func() string {
		if runtime == "modu" {
			return input.Binary
		}
		return ""
	}()})
	return runtimeInfo(engine, agentrun.Runtime(runtime), name, input.Binary), nil
}

func (r *RuntimeRegistry) replace(config RuntimeConfig) {
	r.config = config
	r.engine = newRuntimeEngine(config)
}

func newRuntimeEngine(config RuntimeConfig) *agentrun.Engine {
	return agentrun.NewEngine(agentrun.Config{CodexBinary: config.CodexBinary, ClaudeBinary: config.ClaudeBinary, ModuBinary: config.ModuBinary})
}

func allowedEnvironment(allowlist []string) []string {
	keys := append([]string{"PATH", "HOME", "TMPDIR", "USER", "LANG", "LC_ALL"}, allowlist...)
	seen := make(map[string]struct{}, len(keys))
	environment := make([]string, 0, len(keys))
	for _, key := range keys {
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		if value, ok := os.LookupEnv(key); ok {
			environment = append(environment, key+"="+value)
		}
	}
	return environment
}

func configureModuProvider(environment []string, provider string) []string {
	provider = strings.ToLower(strings.TrimSpace(provider))
	filtered := make([]string, 0, len(environment)+1)
	for _, item := range environment {
		if !strings.HasPrefix(item, "MODU_CODE_PROVIDER=") {
			filtered = append(filtered, item)
		}
	}
	if provider != "" && provider != "auto" {
		filtered = append(filtered, "MODU_CODE_PROVIDER="+provider)
	}
	return filtered
}

func runtimeInfo(engine *agentrun.Engine, runtime agentrun.Runtime, name, configured string) RuntimeInfo {
	available := engine.Available(runtime)
	binary := configured
	if binary == "" {
		binary = string(runtime)
		if runtime == agentrun.RuntimeClaude {
			binary = "claude"
		} else if runtime == agentrun.RuntimeModu {
			binary = "modu_code"
		}
	}
	version := ""
	if available {
		if runtime == agentrun.RuntimeModu {
			version = "ACP · executable"
			return RuntimeInfo{ID: string(runtime), Name: name, Available: true, Version: version, CheckedAt: time.Now().UTC()}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if output, err := exec.CommandContext(ctx, binary, "--version").CombinedOutput(); err == nil {
			version = strings.TrimSpace(string(output))
			if index := strings.IndexByte(version, '\n'); index >= 0 {
				version = version[:index]
			}
		}
	}
	return RuntimeInfo{ID: string(runtime), Name: name, Available: available, Version: version, CheckedAt: time.Now().UTC()}
}
