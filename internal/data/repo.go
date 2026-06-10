package data

import (
	"github.com/openmodu/oneshot/internal/repo/agents"
	"github.com/openmodu/oneshot/internal/repo/billing"
	"github.com/openmodu/oneshot/internal/repo/orders"
	"github.com/openmodu/oneshot/internal/repo/users"
)

type OneShotRepo struct {
	Agents  agents.AgentsRepo
	Billing billing.BillingRepo
	Orders  orders.OrdersRepo
	Users   users.UsersRepo
}

func NewOneShotRepo(
	agents agents.AgentsRepo,
	billing billing.BillingRepo,
	orders orders.OrdersRepo,
	users users.UsersRepo,
) *OneShotRepo {
	return &OneShotRepo{
		Agents:  agents,
		Billing: billing,
		Orders:  orders,
		Users:   users,
	}
}
