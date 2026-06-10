package data

import (
	"github.com/openmodu/oneshot/internal/repo/agents"
	"github.com/openmodu/oneshot/internal/repo/billing"
	"github.com/openmodu/oneshot/internal/repo/orders"
)

type OneShotRepo struct {
	Agents  agents.AgentsRepo
	Billing billing.BillingRepo
	Orders  orders.OrdersRepo
}

func NewOneShotRepo(
	agents agents.AgentsRepo,
	billing billing.BillingRepo,
	orders orders.OrdersRepo,
) *OneShotRepo {
	return &OneShotRepo{
		Agents:  agents,
		Billing: billing,
		Orders:  orders,
	}
}
