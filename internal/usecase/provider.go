package usecase

import (
	"github.com/google/wire"
	"github.com/openmodu/oneshot/internal/data"
	usecaseagents "github.com/openmodu/oneshot/internal/usecase/agents"
	usecaseauth "github.com/openmodu/oneshot/internal/usecase/auth"
	usecasebilling "github.com/openmodu/oneshot/internal/usecase/billing"
	usecaseorders "github.com/openmodu/oneshot/internal/usecase/orders"
)

var ProviderSet = wire.NewSet(
	ProvideAgentRepository,
	ProvideAuthRepository,
	ProvideBillingRepository,
	ProvideOrderAgentRepository,
	ProvideOrderRepository,
	usecaseauth.NewUsecase,
	usecaseagents.NewUsecase,
	usecasebilling.NewUsecase,
	usecaseorders.NewUsecase,
)

func ProvideAgentRepository(repos *data.OneShotRepo) usecaseagents.Repository {
	return repos.Agents
}

func ProvideAuthRepository(repos *data.OneShotRepo) usecaseauth.Repository {
	return repos.Users
}

func ProvideBillingRepository(repos *data.OneShotRepo) usecasebilling.Repository {
	return repos.Billing
}

func ProvideOrderAgentRepository(repos *data.OneShotRepo) usecaseorders.AgentRepository {
	return repos.Agents
}

func ProvideOrderRepository(repos *data.OneShotRepo) usecaseorders.Repository {
	return repos.Orders
}
