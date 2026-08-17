package desktop

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	domainsettings "github.com/openmodu/onecatch/internal/domain/settings"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
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
	mu                 sync.RWMutex
	probeMu            sync.Mutex
	config             RuntimeConfig
	settings           map[string]domainsettings.RuntimeSettings
	statusCache        map[string]runtimeStatusCacheEntry
	interruptGrace     time.Duration
	engine             *agentrun.Engine
	permissionMu       sync.Mutex
	pendingPermissions map[string]*pendingPermission
}

type pendingPermission struct {
	runID     string
	stepRunID string
	request   agentrun.PermissionRequest
	response  chan agentrun.PermissionDecision
}

const runtimeStatusCacheTTL = 5 * time.Minute

type runtimeStatusCacheEntry struct {
	configured string
	info       RuntimeInfo
}

func NewRuntimeRegistry(root string) (*RuntimeRegistry, error) {
	_ = root
	registry := &RuntimeRegistry{pendingPermissions: make(map[string]*pendingPermission), statusCache: make(map[string]runtimeStatusCacheEntry)}
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
	if !request.RuntimeDefaultsResolved {
		if request.Model == "" {
			request.Model = runtimeSettings.DefaultModel
		}
		if request.ReasoningEffort == "" {
			request.ReasoningEffort = runtimeSettings.ReasoningEffort
		}
		if request.ServiceTier == "" {
			request.ServiceTier = runtimeSettings.ServiceTier
		}
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
	if request.Runtime == agentrun.RuntimeClaude && request.RunID != "" && request.PermissionHandler == nil {
		runID, stepRunID := request.RunID, request.StepRunID
		request.PermissionHandler = func(ctx context.Context, permission agentrun.PermissionRequest) (agentrun.PermissionDecision, error) {
			return r.awaitPermission(ctx, runID, stepRunID, permission)
		}
	}
	return engine.Run(ctx, request, sink)
}

func (r *RuntimeRegistry) awaitPermission(ctx context.Context, runID, stepRunID string, request agentrun.PermissionRequest) (agentrun.PermissionDecision, error) {
	pending := &pendingPermission{runID: runID, stepRunID: stepRunID, request: request, response: make(chan agentrun.PermissionDecision, 1)}
	r.permissionMu.Lock()
	r.pendingPermissions[request.ID] = pending
	r.permissionMu.Unlock()
	defer func() {
		r.permissionMu.Lock()
		if r.pendingPermissions[request.ID] == pending {
			delete(r.pendingPermissions, request.ID)
		}
		r.permissionMu.Unlock()
	}()
	select {
	case decision := <-pending.response:
		return decision, nil
	case <-ctx.Done():
		return agentrun.PermissionDecision{}, ctx.Err()
	}
}

// ResolvePermission delivers a desktop decision to the Claude process that is
// blocked on requestID. The run check prevents one task card from answering a
// request belonging to another concurrently running workflow.
func (r *RuntimeRegistry) ResolvePermission(runID, requestID, decision string) error {
	r.permissionMu.Lock()
	pending := r.pendingPermissions[requestID]
	if pending == nil || pending.runID != runID {
		r.permissionMu.Unlock()
		return coded("permission_not_pending", "permission request is no longer pending")
	}
	var response agentrun.PermissionDecision
	switch decision {
	case "allow_once":
		response = agentrun.PermissionDecision{Behavior: "allow", DecisionClassification: "user_temporary"}
	case "allow_always":
		if pending.request.SuppressAlwaysAllow || len(pending.request.Suggestions) == 0 {
			r.permissionMu.Unlock()
			return coded("permission_persistent_unavailable", "this request cannot be permanently allowed")
		}
		response = agentrun.PermissionDecision{Behavior: "allow", UpdatedPermissions: pending.request.Suggestions, DecisionClassification: "user_permanent"}
	case "deny":
		response = agentrun.PermissionDecision{Behavior: "deny", Message: "Permission denied by user", DecisionClassification: "user_reject"}
	default:
		r.permissionMu.Unlock()
		return coded("permission_decision_invalid", "permission decision must be allow_once, allow_always or deny")
	}
	select {
	case pending.response <- response:
		r.permissionMu.Unlock()
		return nil
	default:
		r.permissionMu.Unlock()
		return coded("permission_not_pending", "permission request was already answered")
	}
}

func (r *RuntimeRegistry) List() []RuntimeInfo {
	r.probeMu.Lock()
	defer r.probeMu.Unlock()
	if r.statusCache == nil {
		r.statusCache = make(map[string]runtimeStatusCacheEntry)
	}
	r.mu.RLock()
	config := r.config
	engine := r.engine
	r.mu.RUnlock()
	type runtimeSpec struct {
		runtime    agentrun.Runtime
		name       string
		configured string
	}
	specs := []runtimeSpec{
		{agentrun.RuntimeCodex, "Codex", config.CodexBinary},
		{agentrun.RuntimeClaude, "Claude Code", config.ClaudeBinary},
		{agentrun.RuntimeModu, "Modu Code", config.ModuBinary},
	}
	items := make([]RuntimeInfo, len(specs))
	type probeResult struct {
		index int
		info  RuntimeInfo
	}
	results := make(chan probeResult, len(specs))
	pending := 0
	for index, spec := range specs {
		cached, ok := r.statusCache[string(spec.runtime)]
		if ok && cached.configured == spec.configured && time.Since(cached.info.CheckedAt) < runtimeStatusCacheTTL {
			items[index] = cached.info
			continue
		}
		pending++
		go func(index int, spec runtimeSpec) {
			results <- probeResult{index: index, info: runtimeInfo(engine, spec.runtime, spec.name, spec.configured)}
		}(index, spec)
	}
	for range pending {
		result := <-results
		items[result.index] = result.info
		r.statusCache[string(specs[result.index].runtime)] = runtimeStatusCacheEntry{configured: specs[result.index].configured, info: result.info}
	}
	return items
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
	r.invalidateStatusCache()
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
	r.invalidateStatusCache()
}

func (r *RuntimeRegistry) invalidateStatusCache() {
	r.probeMu.Lock()
	r.statusCache = make(map[string]runtimeStatusCacheEntry)
	r.probeMu.Unlock()
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
			version = "Print · NDJSON"
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
