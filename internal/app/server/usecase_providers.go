package server

import (
	"github.com/google/wire"
	"github.com/openmodu/oneshot/internal/repo/store"
	usecaseagents "github.com/openmodu/oneshot/internal/usecase/agents"
	usecaseartifacts "github.com/openmodu/oneshot/internal/usecase/artifacts"
	usecaseauth "github.com/openmodu/oneshot/internal/usecase/auth"
	usecasebilling "github.com/openmodu/oneshot/internal/usecase/billing"
	usecaseconversations "github.com/openmodu/oneshot/internal/usecase/conversations"
	usecaseexecution "github.com/openmodu/oneshot/internal/usecase/execution"
	usecaseorders "github.com/openmodu/oneshot/internal/usecase/orders"
)

var UsecaseProviderSet = wire.NewSet(
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
	ProvideOrderRunSessionReader,
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

func ProvideAgentRepository(repos *store.OneShotRepo) usecaseagents.Repository {
	return repos.Agents
}

func ProvideArtifactRepository(repos *store.OneShotRepo) usecaseartifacts.Repository {
	return repos.Artifacts
}

func ProvideArtifactOrderRepository(repos *store.OneShotRepo) usecaseartifacts.OrderRepository {
	return repos.Orders
}

func ProvideAuthRepository(repos *store.OneShotRepo) usecaseauth.Repository {
	return repos.Users
}

func ProvideAuthSessionStore(repos *store.OneShotRepo) usecaseauth.SessionStore {
	return repos.Sessions
}

func ProvideBillingRepository(repos *store.OneShotRepo) usecasebilling.Repository {
	return repos.Billing
}

func ProvideExecutionOrderRepository(repos *store.OneShotRepo) usecaseexecution.OrderRepository {
	return repos.Orders
}

func ProvideExecutionArtifactRecorder(artifacts *usecaseartifacts.Usecase) usecaseexecution.ArtifactRecorder {
	return artifacts
}

func ProvideExecutionAgentResolver(repos *store.OneShotRepo) usecaseexecution.AgentResolver {
	return repos.Agents
}

func ProvideOrderAgentRepository(repos *store.OneShotRepo) usecaseorders.AgentRepository {
	return repos.Agents
}

func ProvideOrderRepository(repos *store.OneShotRepo) usecaseorders.Repository {
	return repos.Orders
}

func ProvideOrderRunSessionReader(execution *usecaseexecution.Usecase) usecaseorders.RunSessionReader {
	return execution
}

func ProvideConversationRepository(repos *store.OneShotRepo) usecaseconversations.Repository {
	return repos.Conversations
}

func ProvideConversationAgentGetter(agents *usecaseagents.Usecase) usecaseconversations.AgentGetter {
	return agents
}

func ProvideConversationOrderCreator(orders *usecaseorders.Usecase) usecaseconversations.OrderCreator {
	return orders
}
