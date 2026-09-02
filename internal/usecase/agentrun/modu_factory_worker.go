//go:build onecatch_worker

package agentrun

// Remote workers use the CLI adapter so the SDK dependency graph is not
// duplicated into the application package's bundled worker binary.
func newModuRuntimeRunner(cfg Config) Runner {
	runner := NewModuRunner(cfg.Binary(RuntimeModu))
	runner.agentDir = cfg.ModuAgentDir
	return runner
}
