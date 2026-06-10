package data

import (
	"github.com/openmodu/oneshot/internal/repo/agents"
	"github.com/openmodu/oneshot/internal/repo/artifacts"
	"github.com/openmodu/oneshot/internal/repo/billing"
	"github.com/openmodu/oneshot/internal/repo/orders"
	"github.com/openmodu/oneshot/internal/repo/users"
)

type OneShotRepo struct {
	Agents    agents.AgentsRepo
	Artifacts artifacts.ArtifactsRepo
	Billing   billing.BillingRepo
	Orders    orders.OrdersRepo
	Users     users.UsersRepo
}

func NewOneShotRepo(
	agents agents.AgentsRepo,
	artifacts artifacts.ArtifactsRepo,
	billing billing.BillingRepo,
	orders orders.OrdersRepo,
	users users.UsersRepo,
) *OneShotRepo {
	return &OneShotRepo{
		Agents:    agents,
		Artifacts: artifacts,
		Billing:   billing,
		Orders:    orders,
		Users:     users,
	}
}
