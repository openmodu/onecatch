package usecase

import (
	"github.com/google/wire"
	"github.com/openmodu/oneshot/internal/data"
	usecaseagents "github.com/openmodu/oneshot/internal/usecase/agents"
	usecaseartifacts "github.com/openmodu/oneshot/internal/usecase/artifacts"
	usecaseauth "github.com/openmodu/oneshot/internal/usecase/auth"
	usecasebilling "github.com/openmodu/oneshot/internal/usecase/billing"
	usecaseconversations "github.com/openmodu/oneshot/internal/usecase/conversations"
	usecaseexecution "github.com/openmodu/oneshot/internal/usecase/execution"
	usecaseorders "github.com/openmodu/oneshot/internal/usecase/orders"
)

var ProviderSet = wire.NewSet(
	ProvideAgentRepository,
	ProvideArtifactOrderRepository,
	ProvideArtifactRepository,
	ProvideAuthRepository,
	ProvideAuthSessionStore,
	ProvideBillingRepository,
	ProvideExecutionArtifactRecorder,
	ProvideExecutionAgentResolver,
	ProvideExecutionOrderRepository,
	ProvideOrderAgentRepository,
	ProvideOrderRepository,
	ProvideConversationRepository,
	ProvideConversationAgentGetter,
	ProvideConversationOrderCreator,
	usecaseauth.NewUsecase,
	usecaseagents.NewUsecase,
	usecaseartifacts.NewUsecase,
	usecasebilling.NewUsecase,
	usecaseconversations.NewUsecase,
	usecaseexecution.NewUsecase,
	usecaseorders.NewUsecase,
)

func ProvideAgentRepository(repos *data.OneShotRepo) usecaseagents.Repository {
	return repos.Agents
}

func ProvideArtifactRepository(repos *data.OneShotRepo) usecaseartifacts.Repository {
	return repos.Artifacts
}

func ProvideArtifactOrderRepository(repos *data.OneShotRepo) usecaseartifacts.OrderRepository {
	return repos.Orders
}

func ProvideAuthRepository(repos *data.OneShotRepo) usecaseauth.Repository {
	return repos.Users
}

func ProvideAuthSessionStore(repos *data.OneShotRepo) usecaseauth.SessionStore {
	return repos.Sessions
}

func ProvideBillingRepository(repos *data.OneShotRepo) usecasebilling.Repository {
	return repos.Billing
}

func ProvideExecutionOrderRepository(repos *data.OneShotRepo) usecaseexecution.OrderRepository {
	return repos.Orders
}

func ProvideExecutionArtifactRecorder(artifacts *usecaseartifacts.Usecase) usecaseexecution.ArtifactRecorder {
	return artifacts
}

func ProvideExecutionAgentResolver(repos *data.OneShotRepo) usecaseexecution.AgentResolver {
	return repos.Agents
}

func ProvideOrderAgentRepository(repos *data.OneShotRepo) usecaseorders.AgentRepository {
	return repos.Agents
}

func ProvideOrderRepository(repos *data.OneShotRepo) usecaseorders.Repository {
	return repos.Orders
}

func ProvideConversationRepository(repos *data.OneShotRepo) usecaseconversations.Repository {
	return repos.Conversations
}

func ProvideConversationAgentGetter(agents *usecaseagents.Usecase) usecaseconversations.AgentGetter {
	return agents
}

func ProvideConversationOrderCreator(orders *usecaseorders.Usecase) usecaseconversations.OrderCreator {
	return orders
}
