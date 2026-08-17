package server

import (
	"github.com/google/wire"
	"github.com/openmodu/onecatch/internal/repo/store"
	usecaseagents "github.com/openmodu/onecatch/internal/usecase/agents"
	usecaseartifacts "github.com/openmodu/onecatch/internal/usecase/artifacts"
	usecaseauth "github.com/openmodu/onecatch/internal/usecase/auth"
	usecasebilling "github.com/openmodu/onecatch/internal/usecase/billing"
	usecaseconversations "github.com/openmodu/onecatch/internal/usecase/conversations"
	usecaseexecution "github.com/openmodu/onecatch/internal/usecase/execution"
	usecaseorders "github.com/openmodu/onecatch/internal/usecase/orders"
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

func ProvideAgentRepository(repos *store.OneCatchRepo) usecaseagents.Repository {
	return repos.Agents
}

func ProvideArtifactRepository(repos *store.OneCatchRepo) usecaseartifacts.Repository {
	return repos.Artifacts
}

func ProvideArtifactOrderRepository(repos *store.OneCatchRepo) usecaseartifacts.OrderRepository {
	return repos.Orders
}

func ProvideAuthRepository(repos *store.OneCatchRepo) usecaseauth.Repository {
	return repos.Users
}

func ProvideAuthSessionStore(repos *store.OneCatchRepo) usecaseauth.SessionStore {
	return repos.Sessions
}

func ProvideBillingRepository(repos *store.OneCatchRepo) usecasebilling.Repository {
	return repos.Billing
}

func ProvideExecutionOrderRepository(repos *store.OneCatchRepo) usecaseexecution.OrderRepository {
	return repos.Orders
}

func ProvideExecutionArtifactRecorder(artifacts *usecaseartifacts.Usecase) usecaseexecution.ArtifactRecorder {
	return artifacts
}

func ProvideExecutionAgentResolver(repos *store.OneCatchRepo) usecaseexecution.AgentResolver {
	return repos.Agents
}

func ProvideOrderAgentRepository(repos *store.OneCatchRepo) usecaseorders.AgentRepository {
	return repos.Agents
}

func ProvideOrderRepository(repos *store.OneCatchRepo) usecaseorders.Repository {
	return repos.Orders
}

func ProvideOrderRunSessionReader(execution *usecaseexecution.Usecase) usecaseorders.RunSessionReader {
	return execution
}

func ProvideConversationRepository(repos *store.OneCatchRepo) usecaseconversations.Repository {
	return repos.Conversations
}

func ProvideConversationAgentGetter(agents *usecaseagents.Usecase) usecaseconversations.AgentGetter {
	return agents
}

func ProvideConversationOrderCreator(orders *usecaseorders.Usecase) usecaseconversations.OrderCreator {
	return orders
}
