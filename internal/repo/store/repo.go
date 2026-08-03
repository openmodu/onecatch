package store

import (
	"github.com/openmodu/oneshot/internal/repo/agents"
	"github.com/openmodu/oneshot/internal/repo/artifacts"
	"github.com/openmodu/oneshot/internal/repo/billing"
	"github.com/openmodu/oneshot/internal/repo/conversations"
	"github.com/openmodu/oneshot/internal/repo/orders"
	"github.com/openmodu/oneshot/internal/repo/sessions"
	"github.com/openmodu/oneshot/internal/repo/users"
)

type OneShotRepo struct {
	Agents        agents.AgentsRepo
	Artifacts     artifacts.ArtifactsRepo
	Billing       billing.BillingRepo
	Conversations conversations.ConversationsRepo
	Orders        orders.OrdersRepo
	Sessions      sessions.SessionsRepo
	Users         users.UsersRepo
}

func NewOneShotRepo(
	agents agents.AgentsRepo,
	artifacts artifacts.ArtifactsRepo,
	billing billing.BillingRepo,
	conversations conversations.ConversationsRepo,
	orders orders.OrdersRepo,
	sessions sessions.SessionsRepo,
	users users.UsersRepo,
) *OneShotRepo {
	return &OneShotRepo{
		Agents:        agents,
		Artifacts:     artifacts,
		Billing:       billing,
		Conversations: conversations,
		Orders:        orders,
		Sessions:      sessions,
		Users:         users,
	}
}
