//go:build !onecatch_worker

package agentrun

func newModuRuntimeRunner(cfg Config) Runner {
	if cfg.ModuIntegration == "cli" {
		return NewModuRunner(cfg.ModuBinary)
	}
	return NewModuSDKRunner(ModuSDKOptions{ConfigPath: cfg.ModuConfigPath, AgentDir: cfg.ModuAgentDir})
}
