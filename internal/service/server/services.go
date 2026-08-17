package server

import (
	usecaseagents "github.com/openmodu/onecatch/internal/usecase/agents"
	usecaseartifacts "github.com/openmodu/onecatch/internal/usecase/artifacts"
	usecaseauth "github.com/openmodu/onecatch/internal/usecase/auth"
	usecasebilling "github.com/openmodu/onecatch/internal/usecase/billing"
	usecaseconversations "github.com/openmodu/onecatch/internal/usecase/conversations"
	usecaseexecution "github.com/openmodu/onecatch/internal/usecase/execution"
	usecaseorders "github.com/openmodu/onecatch/internal/usecase/orders"
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
