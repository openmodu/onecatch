package agents

import (
	"context"

	"github.com/openmodu/onecatch/internal/domain/agents"
)

type Repository interface {
	ListAgents(context.Context) ([]agents.Agent, error)
	GetAgent(context.Context, string) (agents.Agent, error)
}

type Usecase struct {
	repo Repository
}

func NewUsecase(repo Repository) *Usecase {
	return &Usecase{repo: repo}
}

func (s *Usecase) List(ctx context.Context) ([]agents.Agent, error) {
	return s.repo.ListAgents(ctx)
}

func (s *Usecase) Get(ctx context.Context, id string) (agents.Agent, error) {
	return s.repo.GetAgent(ctx, id)
}
