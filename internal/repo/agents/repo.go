package agents

import (
	"context"
	"sync"

	domainagents "github.com/openmodu/oneshot/internal/domain/agents"
	pkgsql "github.com/openmodu/oneshot/pkg/sql"
)

type AgentsRepo interface {
	ListAgents(context.Context) ([]domainagents.Agent, error)
	GetAgent(context.Context, string) (domainagents.Agent, error)
}

type agentsImpl struct {
	sql *pkgsql.Sql
	mu  sync.RWMutex

	agents []domainagents.Agent
}

func NewAgentsRepo(sql *pkgsql.Sql) AgentsRepo {
	return &agentsImpl{
		sql:    sql,
		agents: domainagents.SeedCatalog(),
	}
}

func (r *agentsImpl) ListAgents(context.Context) ([]domainagents.Agent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]domainagents.Agent, len(r.agents))
	copy(out, r.agents)
	return out, nil
}

func (r *agentsImpl) GetAgent(_ context.Context, id string) (domainagents.Agent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, agent := range r.agents {
		if agent.ID == id {
			return agent, nil
		}
	}
	return domainagents.Agent{}, domainagents.ErrNotFound
}
