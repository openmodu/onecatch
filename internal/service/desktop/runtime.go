package desktop

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	domainsettings "github.com/openmodu/onecatch/internal/domain/settings"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
	"github.com/openmodu/onecatch/pkg/localfile"
)

type RuntimeConfig struct {
	CodexBinary     string `json:"codexBinary,omitempty"`
	ClaudeBinary    string `json:"claudeBinary,omitempty"`
	ModuBinary      string `json:"moduBinary,omitempty"`
	ModuIntegration string `json:"moduIntegration,omitempty"`
	ModuConfigPath  string `json:"moduConfigPath,omitempty"`
	ModuAgentDir    string `json:"moduAgentDir,omitempty"`
	PiBinary        string `json:"piBinary,omitempty"`
	GrokBinary      string `json:"grokBinary,omitempty"`
	DshBinary       string `json:"dshBinary,omitempty"`
	DshSessionRoot  string `json:"dshSessionRoot,omitempty"`
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
	statusPath         string
	dataRoot           string
	statusRefreshing   bool
	interruptGrace     time.Duration
	engine             *agentrun.Engine
	permissionMu       sync.Mutex
	pendingPermissions map[string]*pendingPermission
	changedMu          sync.RWMutex
	changed            func([]RuntimeInfo)
}

type pendingPermission struct {
	runID     string
	stepRunID string
	request   agentrun.PermissionRequest
	response  chan agentrun.PermissionDecision
}

const runtimeStatusCacheTTL = 5 * time.Minute

// runtimeStatusFile persists the last probe result across restarts. Probing
// spawns `<binary> --version` per runtime — hundreds of milliseconds of process
// startup for the Node-based CLIs — and the in-memory cache alone means every
// cold launch pays it again before the runtime list can be shown.
const runtimeStatusFile = "runtime-status.json"

type runtimeStatusCacheEntry struct {
	Configured string      `json:"configured"`
	Info       RuntimeInfo `json:"info"`
}

type runtimeStatusSnapshot struct {
	Version int                                `json:"version"`
	Entries map[string]runtimeStatusCacheEntry `json:"entries"`
}

func NewRuntimeRegistry(root string) (*RuntimeRegistry, error) {
	registry := &RuntimeRegistry{dataRoot: strings.TrimSpace(root), pendingPermissions: make(map[string]*pendingPermission), statusCache: make(map[string]runtimeStatusCacheEntry)}
	if strings.TrimSpace(root) != "" {
		registry.statusPath = filepath.Join(root, runtimeStatusFile)
		registry.loadStatusCache()
	}
	registry.replace(RuntimeConfig{})
	return registry, nil
}

// SetRuntimesChanged registers the sink for background probe results. It is
// wired once during startup, before any window exists, so a plain guarded field
// is enough.
func (r *RuntimeRegistry) SetRuntimesChanged(notify func([]RuntimeInfo)) {
	r.changedMu.Lock()
	r.changed = notify
	r.changedMu.Unlock()
}

func (r *RuntimeRegistry) notifyRuntimesChanged(items []RuntimeInfo) {
	r.changedMu.RLock()
	notify := r.changed
	r.changedMu.RUnlock()
	if notify != nil {
		notify(items)
	}
}

// loadStatusCache seeds the in-memory cache from disk. A missing or unreadable
// snapshot is not an error: it only means the next List() probes for real.
func (r *RuntimeRegistry) loadStatusCache() {
	var snapshot runtimeStatusSnapshot
	if err := localfile.ReadJSON(r.statusPath, &snapshot); err != nil || snapshot.Version != 1 {
		return
	}
	for id, entry := range snapshot.Entries {
		if entry.Info.ID == id {
			r.statusCache[id] = entry
		}
	}
}

// persistStatusCacheLocked must be called with probeMu held.
func (r *RuntimeRegistry) persistStatusCacheLocked() {
	if r.statusPath == "" {
		return
	}
	entries := make(map[string]runtimeStatusCacheEntry, len(r.statusCache))
	for id, entry := range r.statusCache {
		entries[id] = entry
	}
	// A failed write only costs the next launch one probe round.
	_ = localfile.WriteJSONAtomic(r.statusPath, runtimeStatusSnapshot{Version: 1, Entries: entries})
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
	if engine.SupportsInteractivePermissions(request.Runtime, request.Sandbox) && request.RunID != "" && request.PermissionHandler == nil {
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

// ResolvePermission delivers a desktop decision to the harness process that is
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
		// Whether a harness can remember a decision is the adapter's call:
		// Claude needs provider-authored rules to persist, while an ACP agent
		// simply offers the option and keeps the memory itself. Both express
		// the answer through SuppressAlwaysAllow.
		if pending.request.SuppressAlwaysAllow {
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

type runtimeSpec struct {
	runtime    agentrun.Runtime
	name       string
	configured string
	cacheKey   string
}

func runtimeSpecs(config RuntimeConfig) []runtimeSpec {
	return []runtimeSpec{
		{agentrun.RuntimeCodex, "Codex", config.CodexBinary, config.CodexBinary},
		{agentrun.RuntimeClaude, "Claude Code", config.ClaudeBinary, config.ClaudeBinary},
		{agentrun.RuntimeModu, "Modu Code", config.ModuBinary, strings.Join([]string{config.ModuIntegration, config.ModuBinary, config.ModuConfigPath, config.ModuAgentDir}, "\x00")},
		{agentrun.RuntimePi, "Pi", config.PiBinary, config.PiBinary},
		{agentrun.RuntimeGrok, "Grok Build", config.GrokBinary, config.GrokBinary},
		{agentrun.RuntimeDsh, "DeepSeek Harness", config.DshBinary, config.DshBinary},
	}
}

// probeRuntimes runs the given probes concurrently and writes each result into
// items at its own index.
func probeRuntimes(engine *agentrun.Engine, specs []runtimeSpec, indexes []int, items []RuntimeInfo) {
	type probeResult struct {
		index int
		info  RuntimeInfo
	}
	results := make(chan probeResult, len(indexes))
	for _, index := range indexes {
		go func(index int, spec runtimeSpec) {
			results <- probeResult{index: index, info: runtimeInfo(engine, spec.runtime, spec.name, spec.configured)}
		}(index, specs[index])
	}
	for range indexes {
		result := <-results
		items[result.index] = result.info
	}
}

// List reports the status of every known runtime.
//
// A cached entry is returned even once it has aged past the TTL, because the
// only way to refresh it is to spawn `<binary> --version` — half a second of
// process startup that must never sit in front of a window's first paint. The
// stale entry goes out immediately and a background refresh pushes the
// corrected list through SetRuntimesChanged. Only a runtime with no usable
// entry at all (first ever launch, or a binary path the user just changed) is
// probed synchronously, since there is nothing else to show for it.
func (r *RuntimeRegistry) List() []RuntimeInfo {
	r.probeMu.Lock()
	if r.statusCache == nil {
		r.statusCache = make(map[string]runtimeStatusCacheEntry)
	}
	r.mu.RLock()
	config := r.config
	engine := r.engine
	r.mu.RUnlock()
	specs := runtimeSpecs(config)
	items := make([]RuntimeInfo, len(specs))
	var uncached []int
	stale := false
	for index, spec := range specs {
		cached, ok := r.statusCache[string(spec.runtime)]
		if !ok || cached.Configured != spec.cacheKey {
			uncached = append(uncached, index)
			continue
		}
		items[index] = cached.Info
		if time.Since(cached.Info.CheckedAt) >= runtimeStatusCacheTTL {
			stale = true
		}
	}
	if len(uncached) > 0 {
		probeRuntimes(engine, specs, uncached, items)
		for _, index := range uncached {
			r.statusCache[string(specs[index].runtime)] = runtimeStatusCacheEntry{Configured: specs[index].cacheKey, Info: items[index]}
		}
		r.persistStatusCacheLocked()
	}
	refresh := stale && !r.statusRefreshing
	if refresh {
		r.statusRefreshing = true
	}
	r.probeMu.Unlock()
	if refresh {
		go r.refreshStatus()
	}
	return items
}

// refreshStatus re-probes every runtime off the caller's path and announces the
// result only when it differs from what the UI was already shown.
func (r *RuntimeRegistry) refreshStatus() {
	r.probeMu.Lock()
	r.mu.RLock()
	config := r.config
	engine := r.engine
	r.mu.RUnlock()
	specs := runtimeSpecs(config)
	items := make([]RuntimeInfo, len(specs))
	indexes := make([]int, len(specs))
	previous := make([]RuntimeInfo, len(specs))
	for index, spec := range specs {
		indexes[index] = index
		previous[index] = r.statusCache[string(spec.runtime)].Info
	}
	probeRuntimes(engine, specs, indexes, items)
	for index, spec := range specs {
		r.statusCache[string(spec.runtime)] = runtimeStatusCacheEntry{Configured: spec.cacheKey, Info: items[index]}
	}
	r.persistStatusCacheLocked()
	r.statusRefreshing = false
	r.probeMu.Unlock()

	for index := range items {
		// CheckedAt moves on every probe; only a real status change is worth a
		// re-render in every open window.
		if previous[index].Available != items[index].Available || previous[index].Version != items[index].Version {
			r.notifyRuntimesChanged(items)
			return
		}
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
	case string(agentrun.RuntimePi):
		config.PiBinary = input.Binary
	case string(agentrun.RuntimeGrok):
		config.GrokBinary = input.Binary
	case string(agentrun.RuntimeDsh):
		config.DshBinary = input.Binary
	default:
		r.mu.Unlock()
		return RuntimeInfo{}, coded("runtime_unknown", "unknown runtime")
	}
	r.config = config
	r.engine = newRuntimeEngine(config)
	r.mu.Unlock()
	r.invalidateRuntimeStatus(input.Runtime)
	return r.Check(input.Runtime)
}

// ApplySettings deliberately does not drop the status cache. It runs on every
// startup and after every settings save, including saves that touch nothing a
// runtime probe depends on. Entries are keyed by the configured binary path, so
// a path that actually changed is re-probed by List() on its own; a path that
// did not is refreshed in the background once its TTL expires.
func (r *RuntimeRegistry) ApplySettings(runtimes map[string]domainsettings.RuntimeSettings, interruptGraceSeconds int) {
	r.ensureIsolatedModuConfig(runtimes["modu"])
	r.mu.Lock()
	copySettings := make(map[string]domainsettings.RuntimeSettings, len(runtimes))
	for id, item := range runtimes {
		copySettings[id] = item
	}
	r.settings = copySettings
	r.interruptGrace = time.Duration(interruptGraceSeconds) * time.Second
	config := RuntimeConfig{
		CodexBinary: runtimes["codex"].Binary, ClaudeBinary: runtimes["claude"].Binary,
		ModuBinary: runtimes["modu"].Binary, ModuIntegration: runtimes["modu"].Integration,
		PiBinary: runtimes["pi"].Binary, GrokBinary: runtimes["grok"].Binary,
		DshBinary: runtimes["dsh"].Binary, DshSessionRoot: r.dshSessionRoot(),
	}
	config.ModuConfigPath, config.ModuAgentDir = r.moduSDKPaths(runtimes["modu"])
	r.replace(config)
	r.mu.Unlock()
}

func (r *RuntimeRegistry) ensureIsolatedModuConfig(settings domainsettings.RuntimeSettings) {
	if settings.Integration == "cli" || settings.ConfigSource != "onecatch" || strings.TrimSpace(settings.ConfigPath) != "" {
		return
	}
	target, _ := r.moduSDKPaths(settings)
	if target == "" {
		return
	}
	if _, err := os.Stat(target); err == nil || !os.IsNotExist(err) {
		return
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	data, err := os.ReadFile(filepath.Join(home, ".modu", "config.toml"))
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return
	}
	file, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return
	}
	if _, err = file.Write(data); err != nil {
		_ = file.Close()
		_ = os.Remove(target)
		return
	}
	_ = file.Close()
}

// invalidateRuntimeStatus forces the next List() to probe one runtime for real.
// It exists for the explicit "check this runtime" actions, where the path may be
// unchanged while its reality is not — the user just installed or removed the
// CLI — and a stale-serving List() would otherwise answer from the cache.
func (r *RuntimeRegistry) invalidateRuntimeStatus(id string) {
	r.probeMu.Lock()
	delete(r.statusCache, id)
	r.persistStatusCacheLocked()
	r.probeMu.Unlock()
}

func (r *RuntimeRegistry) CheckDraft(runtime string, input domainsettings.RuntimeSettings) (RuntimeInfo, error) {
	name := runtimeDisplayName(agentrun.Runtime(runtime))
	if name == "" {
		return RuntimeInfo{}, coded("runtime_unknown", "unknown runtime")
	}
	// Probe only the runtime being edited: every other binary is left empty so
	// the draft engine cannot report a neighbour's installed state as this
	// one's.
	config := agentrun.Config{}
	switch agentrun.Runtime(runtime) {
	case agentrun.RuntimeCodex:
		config.CodexBinary = input.Binary
	case agentrun.RuntimeClaude:
		config.ClaudeBinary = input.Binary
	case agentrun.RuntimeModu:
		config.ModuBinary = input.Binary
		config.ModuIntegration = input.Integration
		config.ModuConfigPath, config.ModuAgentDir = r.moduSDKPaths(input)
	case agentrun.RuntimePi:
		config.PiBinary = input.Binary
	case agentrun.RuntimeGrok:
		config.GrokBinary = input.Binary
	case agentrun.RuntimeDsh:
		config.DshBinary = input.Binary
		config.DshSessionRoot = r.dshSessionRoot()
	}
	return runtimeInfo(agentrun.NewEngine(config), agentrun.Runtime(runtime), name, input.Binary), nil
}

// runtimeDisplayName is the single source of each harness's human name, shared
// by the probe list and the settings draft check so the two cannot drift.
func runtimeDisplayName(runtime agentrun.Runtime) string {
	for _, spec := range runtimeSpecs(RuntimeConfig{}) {
		if spec.runtime == runtime {
			return spec.name
		}
	}
	return ""
}

func (r *RuntimeRegistry) replace(config RuntimeConfig) {
	r.config = config
	r.engine = newRuntimeEngine(config)
}

func newRuntimeEngine(config RuntimeConfig) *agentrun.Engine {
	return agentrun.NewEngine(agentrun.Config{
		CodexBinary: config.CodexBinary, ClaudeBinary: config.ClaudeBinary,
		ModuBinary: config.ModuBinary, ModuIntegration: config.ModuIntegration,
		ModuConfigPath: config.ModuConfigPath, ModuAgentDir: config.ModuAgentDir,
		PiBinary: config.PiBinary, GrokBinary: config.GrokBinary,
		DshBinary: config.DshBinary, DshSessionRoot: config.DshSessionRoot,
	})
}

// dshSessionRoot keeps DeepSeek Harness session logs inside OneCatch's own data
// directory. The adapter recovers its event stream by reading that log, so the
// harness must write somewhere OneCatch controls rather than into its shared
// default, where another dsh client's sessions would be mixed in.
func (r *RuntimeRegistry) dshSessionRoot() string {
	if r.dataRoot == "" {
		return ""
	}
	return filepath.Join(r.dataRoot, "harnesses", "dsh", "sessions")
}

func (r *RuntimeRegistry) moduSDKPaths(settings domainsettings.RuntimeSettings) (string, string) {
	if settings.Integration == "cli" || settings.ConfigSource != "onecatch" {
		return "", ""
	}
	configPath := expandHomePath(settings.ConfigPath)
	if configPath == "" {
		configPath = filepath.Join(r.dataRoot, "harnesses", "modu", "config.toml")
	}
	return configPath, filepath.Dir(configPath)
}

func expandHomePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "~" || strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(home, strings.TrimPrefix(path, "~/"))
		}
	}
	if path == "" {
		return ""
	}
	return filepath.Clean(path)
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
			integration, _ := engine.Runner(runtime).(interface{ ModuIntegration() string })
			if integration != nil && integration.ModuIntegration() == "sdk" {
				version = "Native Go SDK"
			} else {
				version = "Print · NDJSON"
			}
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
