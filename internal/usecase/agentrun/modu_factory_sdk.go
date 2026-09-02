//go:build !onecatch_worker

package agentrun

func newModuRuntimeRunner(cfg Config) Runner {
	if cfg.ModuIntegration == "cli" {
		runner := NewModuRunner(cfg.Binary(RuntimeModu))
		runner.agentDir = cfg.ModuAgentDir
		// The standalone CLI cannot accept a caller-provided tool manager. For
		// Remote FS runs, route through the equivalent in-process SDK adapter so
		// workspace tools can be replaced safely while credentials remain local.
		runner.remoteRunner = NewModuSDKRunner(ModuSDKOptions{ConfigPath: cfg.ModuConfigPath, AgentDir: cfg.ModuAgentDir})
		return runner
	}
	return NewModuSDKRunner(ModuSDKOptions{ConfigPath: cfg.ModuConfigPath, AgentDir: cfg.ModuAgentDir})
}
