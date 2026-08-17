package agents

import (
	"context"
	"errors"
	"testing"

	domainagents "github.com/openmodu/onecatch/internal/domain/agents"
)

func TestAgentsRepoSeedCatalog(t *testing.T) {
	repo := NewAgentsRepo(nil)

	items, err := repo.ListAgents(context.Background())
	if err != nil {
		t.Fatalf("ListAgents() error = %v", err)
	}
	if len(items) != 6 {
		t.Fatalf("agent count = %d, want 6", len(items))
	}

	agent, err := repo.GetAgent(context.Background(), "research-analyst")
	if err != nil {
		t.Fatalf("GetAgent() error = %v", err)
	}
	if agent.Name != "行业研究分析师" {
		t.Fatalf("agent name = %q, want 行业研究分析师", agent.Name)
	}
	if agent.PriceCents != 1990 {
		t.Fatalf("price cents = %d, want 1990", agent.PriceCents)
	}
	if agent.Rating == "" || agent.DealCount == 0 || agent.Deliverable == "" {
		t.Fatalf("agent missing catalog fields: %+v", agent)
	}
}

func TestAgentsRepoNotFound(t *testing.T) {
	repo := NewAgentsRepo(nil)

	_, err := repo.GetAgent(context.Background(), "missing-agent")
	if !errors.Is(err, domainagents.ErrNotFound) {
		t.Fatalf("GetAgent() error = %v, want ErrNotFound", err)
	}
}
