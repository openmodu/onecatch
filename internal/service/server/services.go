package server

import (
	usecaseagents "github.com/openmodu/oneshot/internal/usecase/agents"
	usecaseartifacts "github.com/openmodu/oneshot/internal/usecase/artifacts"
	usecaseauth "github.com/openmodu/oneshot/internal/usecase/auth"
	usecasebilling "github.com/openmodu/oneshot/internal/usecase/billing"
	usecaseconversations "github.com/openmodu/oneshot/internal/usecase/conversations"
	usecaseexecution "github.com/openmodu/oneshot/internal/usecase/execution"
	usecaseorders "github.com/openmodu/oneshot/internal/usecase/orders"
)

type Services struct {
	Auth          *usecaseauth.Usecase
	Agents        *usecaseagents.Usecase
	Artifacts     *usecaseartifacts.Usecase
	Billing       *usecasebilling.Usecase
	Conversations *usecaseconversations.Usecase
	Execution     *usecaseexecution.Usecase
	Orders        *usecaseorders.Usecase
}

func NewServices(
	auth *usecaseauth.Usecase,
	agents *usecaseagents.Usecase,
	artifacts *usecaseartifacts.Usecase,
	billing *usecasebilling.Usecase,
	conversations *usecaseconversations.Usecase,
	execution *usecaseexecution.Usecase,
	orders *usecaseorders.Usecase,
) *Services {
	return &Services{
		Auth:          auth,
		Agents:        agents,
		Artifacts:     artifacts,
		Billing:       billing,
		Conversations: conversations,
		Execution:     execution,
		Orders:        orders,
	}
}
