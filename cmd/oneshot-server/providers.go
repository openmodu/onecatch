package main

import (
	"github.com/openmodu/oneshot/internal/agentrun"
	"github.com/openmodu/oneshot/internal/app/config"
	"github.com/openmodu/oneshot/internal/data"
	usecaseexecution "github.com/openmodu/oneshot/internal/usecase/execution"
)

func provideMySQLDSN(cfg config.Config) data.MySQLDSN {
	return data.MySQLDSN(cfg.MySQLDSN)
}

// provideAgentEngine builds the local-agent execution engine from configured
// CLI overrides (empty paths resolve "codex"/"claude" from PATH).
func provideAgentEngine(cfg config.Config) usecaseexecution.Engine {
	return agentrun.NewEngine(agentrun.Config{
		CodexBinary:  cfg.CodexBinary,
		ClaudeBinary: cfg.ClaudeBinary,
	})
}

func provideExecutionConfig(cfg config.Config) usecaseexecution.Config {
	return usecaseexecution.Config{
		WorkspaceRoot: cfg.WorkspaceRoot,
	}
}
