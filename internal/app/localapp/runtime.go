package localapp

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/openmodu/oneshot/internal/agentrun"
	"github.com/openmodu/oneshot/pkg/localfile"
)

type RuntimeConfig struct {
	CodexBinary  string `json:"codexBinary,omitempty"`
	ClaudeBinary string `json:"claudeBinary,omitempty"`
}

type RuntimeConfigInput struct {
	Runtime string `json:"runtime"`
	Binary  string `json:"binary"`
}

type RuntimeInfo struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Available bool   `json:"available"`
	Version   string `json:"version,omitempty"`
}

// RuntimeRegistry is a hot-swappable Engine. The orchestrator keeps this
// pointer while users update local CLI paths from the desktop application.
type RuntimeRegistry struct {
	mu         sync.RWMutex
	configPath string
	config     RuntimeConfig
	engine     *agentrun.Engine
}

func NewRuntimeRegistry(root string) (*RuntimeRegistry, error) {
	registry := &RuntimeRegistry{configPath: filepath.Join(root, "runtime.json")}
	var config RuntimeConfig
	if err := localfile.ReadJSON(registry.configPath, &config); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	registry.replace(config)
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
	r.mu.RUnlock()
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
	default:
		r.mu.Unlock()
		return RuntimeInfo{}, coded("runtime_unknown", "unknown runtime")
	}
	if err := localfile.WriteJSONAtomic(r.configPath, config); err != nil {
		r.mu.Unlock()
		return RuntimeInfo{}, err
	}
	r.config = config
	r.engine = agentrun.NewEngine(agentrun.Config{CodexBinary: config.CodexBinary, ClaudeBinary: config.ClaudeBinary})
	r.mu.Unlock()
	return r.Check(input.Runtime)
}

func (r *RuntimeRegistry) replace(config RuntimeConfig) {
	r.config = config
	r.engine = agentrun.NewEngine(agentrun.Config{CodexBinary: config.CodexBinary, ClaudeBinary: config.ClaudeBinary})
}

func runtimeInfo(engine *agentrun.Engine, runtime agentrun.Runtime, name, configured string) RuntimeInfo {
	available := engine.Available(runtime)
	binary := configured
	if binary == "" {
		binary = string(runtime)
		if runtime == agentrun.RuntimeClaude {
			binary = "claude"
		}
	}
	version := ""
	if available {
		if output, err := exec.Command(binary, "--version").CombinedOutput(); err == nil {
			version = strings.TrimSpace(string(output))
			if index := strings.IndexByte(version, '\n'); index >= 0 {
				version = version[:index]
			}
		}
	}
	return RuntimeInfo{ID: string(runtime), Name: name, Available: available, Version: version}
}
