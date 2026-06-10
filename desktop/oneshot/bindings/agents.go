package bindings

import (
	"context"

	oneshot "github.com/openmodu/oneshot/clients/oneshot"
)

type AgentBinding struct {
	client oneshot.Client
}

func NewAgentBinding(client oneshot.Client) *AgentBinding {
	return &AgentBinding{client: client}
}

func (b *AgentBinding) ListAgents() ([]oneshot.Agent, error) {
	return b.client.ListAgents(context.Background())
}

func (b *AgentBinding) GetAgent(agentID string) (oneshot.Agent, error) {
	return b.client.GetAgent(context.Background(), agentID)
}
