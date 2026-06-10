package service

import (
	usecaseagents "github.com/openmodu/oneshot/internal/usecase/agents"
	usecaseauth "github.com/openmodu/oneshot/internal/usecase/auth"
	usecasebilling "github.com/openmodu/oneshot/internal/usecase/billing"
	usecaseorders "github.com/openmodu/oneshot/internal/usecase/orders"
)

type Services struct {
	Auth    *usecaseauth.Usecase
	Agents  *usecaseagents.Usecase
	Billing *usecasebilling.Usecase
	Orders  *usecaseorders.Usecase
}

func NewServices(
	auth *usecaseauth.Usecase,
	agents *usecaseagents.Usecase,
	billing *usecasebilling.Usecase,
	orders *usecaseorders.Usecase,
) *Services {
	return &Services{
		Auth:    auth,
		Agents:  agents,
		Billing: billing,
		Orders:  orders,
	}
}
