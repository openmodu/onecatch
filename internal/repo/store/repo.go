package store

import (
	"github.com/openmodu/onecatch/internal/repo/agents"
	"github.com/openmodu/onecatch/internal/repo/artifacts"
	"github.com/openmodu/onecatch/internal/repo/billing"
	"github.com/openmodu/onecatch/internal/repo/conversations"
	"github.com/openmodu/onecatch/internal/repo/orders"
	"github.com/openmodu/onecatch/internal/repo/sessions"
	"github.com/openmodu/onecatch/internal/repo/users"
)

type OneCatchRepo struct {
	Agents        agents.AgentsRepo
	Artifacts     artifacts.ArtifactsRepo
	Billing       billing.BillingRepo
	Conversations conversations.ConversationsRepo
	Orders        orders.OrdersRepo
	Sessions      sessions.SessionsRepo
	Users         users.UsersRepo
}

func NewOneCatchRepo(
	agents agents.AgentsRepo,
	artifacts artifacts.ArtifactsRepo,
	billing billing.BillingRepo,
	conversations conversations.ConversationsRepo,
	orders orders.OrdersRepo,
	sessions sessions.SessionsRepo,
	users users.UsersRepo,
) *OneCatchRepo {
	return &OneCatchRepo{
		Agents:        agents,
		Artifacts:     artifacts,
		Billing:       billing,
		Conversations: conversations,
		Orders:        orders,
		Sessions:      sessions,
		Users:         users,
	}
}
