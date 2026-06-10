package main

import (
	"context"
	"os"

	oneshot "github.com/openmodu/oneshot/clients/oneshot"
	domainagents "github.com/openmodu/oneshot/internal/domain/agents"
	domainartifacts "github.com/openmodu/oneshot/internal/domain/artifacts"
	domainauth "github.com/openmodu/oneshot/internal/domain/auth"
	domainbilling "github.com/openmodu/oneshot/internal/domain/billing"
	domainorders "github.com/openmodu/oneshot/internal/domain/orders"
	"github.com/openmodu/oneshot/internal/domain/users"
	repoagents "github.com/openmodu/oneshot/internal/repo/agents"
	repoartifacts "github.com/openmodu/oneshot/internal/repo/artifacts"
	repobilling "github.com/openmodu/oneshot/internal/repo/billing"
	repoorders "github.com/openmodu/oneshot/internal/repo/orders"
	repousers "github.com/openmodu/oneshot/internal/repo/users"
	"github.com/openmodu/oneshot/internal/service"
	usecaseagents "github.com/openmodu/oneshot/internal/usecase/agents"
	usecaseartifacts "github.com/openmodu/oneshot/internal/usecase/artifacts"
	usecaseauth "github.com/openmodu/oneshot/internal/usecase/auth"
	usecasebilling "github.com/openmodu/oneshot/internal/usecase/billing"
	usecaseorders "github.com/openmodu/oneshot/internal/usecase/orders"
)

type localClient struct {
	services *service.Services
}

func newDesktopClient() oneshot.Client {
	if baseURL := os.Getenv("ONESHOT_API_BASE_URL"); baseURL != "" {
		return oneshot.NewHTTPClient(baseURL)
	}

	agentRepo := repoagents.NewAgentsRepo(nil)
	artifactRepo := repoartifacts.NewArtifactsRepo(nil)
	billingRepo := repobilling.NewBillingRepo(nil)
	orderRepo := repoorders.NewOrdersRepo(nil)
	userRepo := repousers.NewUsersRepo(nil)
	billingUsecase := usecasebilling.NewUsecase(billingRepo)

	return &localClient{
		services: service.NewServices(
			usecaseauth.NewUsecase(userRepo),
			usecaseagents.NewUsecase(agentRepo),
			usecaseartifacts.NewUsecase(artifactRepo, orderRepo),
			billingUsecase,
			usecaseorders.NewUsecase(agentRepo, orderRepo, billingUsecase),
		),
	}
}

func (c *localClient) ListArtifacts(ctx context.Context, orderID string) ([]oneshot.Artifact, error) {
	user, err := c.services.Auth.CurrentUser(ctx)
	if err != nil {
		return nil, err
	}
	artifacts, err := c.services.Artifacts.ListForOrder(ctx, user.ID, orderID)
	if err != nil {
		return nil, err
	}
	out := make([]oneshot.Artifact, 0, len(artifacts))
	for _, artifact := range artifacts {
		out = append(out, toClientArtifact(artifact))
	}
	return out, nil
}

func (c *localClient) DownloadArtifact(ctx context.Context, artifactID string) (oneshot.ArtifactDownload, error) {
	user, err := c.services.Auth.CurrentUser(ctx)
	if err != nil {
		return oneshot.ArtifactDownload{}, err
	}
	download, err := c.services.Artifacts.Download(ctx, user.ID, artifactID)
	if err != nil {
		return oneshot.ArtifactDownload{}, err
	}
	path, err := oneshot.SaveDownload(download.Artifact.FileName, download.Content)
	if err != nil {
		return oneshot.ArtifactDownload{}, err
	}
	return oneshot.ArtifactDownload{
		ArtifactID:  download.Artifact.ID,
		FileName:    download.Artifact.FileName,
		FilePath:    path,
		ContentType: download.ContentType,
	}, nil
}

func (c *localClient) ShareArtifact(ctx context.Context, artifactID string) (oneshot.ArtifactShare, error) {
	user, err := c.services.Auth.CurrentUser(ctx)
	if err != nil {
		return oneshot.ArtifactShare{}, err
	}
	share, err := c.services.Artifacts.Share(ctx, user.ID, artifactID)
	if err != nil {
		return oneshot.ArtifactShare{}, err
	}
	return toClientArtifactShare(share), nil
}

func (c *localClient) CurrentUser(ctx context.Context) (oneshot.User, error) {
	user, err := c.services.Auth.CurrentUser(ctx)
	if err != nil {
		return oneshot.User{}, err
	}
	return toClientUser(user), nil
}

func (c *localClient) StartWechat(ctx context.Context) (oneshot.Session, error) {
	session, err := c.services.Auth.StartWechat(ctx)
	if err != nil {
		return oneshot.Session{}, err
	}
	return toClientSession(session), nil
}

func (c *localClient) LoginWithWechat(ctx context.Context) (oneshot.Session, error) {
	session, err := c.services.Auth.LoginWithWechat(ctx)
	if err != nil {
		return oneshot.Session{}, err
	}
	return toClientSession(session), nil
}

func (c *localClient) LoginWithGoogle(ctx context.Context) (oneshot.Session, error) {
	session, err := c.services.Auth.LoginWithGoogle(ctx)
	if err != nil {
		return oneshot.Session{}, err
	}
	return toClientSession(session), nil
}

func (c *localClient) Logout(ctx context.Context) error {
	return c.services.Auth.Logout(ctx)
}

func (c *localClient) ListAgents(ctx context.Context) ([]oneshot.Agent, error) {
	agents, err := c.services.Agents.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]oneshot.Agent, 0, len(agents))
	for _, agent := range agents {
		out = append(out, toClientAgent(agent))
	}
	return out, nil
}

func (c *localClient) GetAgent(ctx context.Context, agentID string) (oneshot.Agent, error) {
	agent, err := c.services.Agents.Get(ctx, agentID)
	if err != nil {
		return oneshot.Agent{}, err
	}
	return toClientAgent(agent), nil
}

func (c *localClient) GetBalance(ctx context.Context) (oneshot.Balance, error) {
	user, err := c.services.Auth.CurrentUser(ctx)
	if err != nil {
		return oneshot.Balance{}, err
	}
	balance, err := c.services.Billing.GetBalance(ctx, user.ID)
	if err != nil {
		return oneshot.Balance{}, err
	}
	return toClientBalance(balance), nil
}

func (c *localClient) ListLedger(ctx context.Context) ([]oneshot.LedgerEntry, error) {
	user, err := c.services.Auth.CurrentUser(ctx)
	if err != nil {
		return nil, err
	}
	ledger, err := c.services.Billing.ListLedger(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	out := make([]oneshot.LedgerEntry, 0, len(ledger))
	for _, entry := range ledger {
		out = append(out, toClientLedgerEntry(entry))
	}
	return out, nil
}

func (c *localClient) StartPurchase(ctx context.Context, input oneshot.StartPurchaseRequest) (oneshot.Purchase, error) {
	user, err := c.services.Auth.CurrentUser(ctx)
	if err != nil {
		return oneshot.Purchase{}, err
	}
	purchase, err := c.services.Billing.StartPurchase(ctx, usecasebilling.PurchaseInput{
		UserID:    user.ID,
		PlanID:    input.PlanID,
		PaymentID: input.PaymentID,
	})
	if err != nil {
		return oneshot.Purchase{}, err
	}
	return toClientPurchase(purchase), nil
}

func (c *localClient) CreateOrder(ctx context.Context, input oneshot.CreateOrderRequest) (oneshot.Order, error) {
	user, err := c.services.Auth.CurrentUser(ctx)
	if err != nil {
		return oneshot.Order{}, err
	}
	order, err := c.services.Orders.Create(ctx, usecaseorders.CreateInput{
		UserID:  user.ID,
		AgentID: input.AgentID,
		Requirement: domainorders.Requirement{
			Prompt: input.Requirement.Prompt,
		},
	})
	if err != nil {
		return oneshot.Order{}, err
	}
	return toClientOrder(order), nil
}

func (c *localClient) ListOrders(ctx context.Context) ([]oneshot.Order, error) {
	user, err := c.services.Auth.CurrentUser(ctx)
	if err != nil {
		return nil, err
	}
	orders, err := c.services.Orders.List(ctx, usecaseorders.ListInput{UserID: user.ID})
	if err != nil {
		return nil, err
	}
	out := make([]oneshot.Order, 0, len(orders))
	for _, order := range orders {
		out = append(out, toClientOrder(order))
	}
	return out, nil
}

func (c *localClient) GetOrder(ctx context.Context, orderID string) (oneshot.Order, error) {
	user, err := c.services.Auth.CurrentUser(ctx)
	if err != nil {
		return oneshot.Order{}, err
	}
	order, err := c.services.Orders.Get(ctx, user.ID, orderID)
	if err != nil {
		return oneshot.Order{}, err
	}
	return toClientOrder(order), nil
}

func (c *localClient) CancelOrder(ctx context.Context, orderID string) (oneshot.Order, error) {
	user, err := c.services.Auth.CurrentUser(ctx)
	if err != nil {
		return oneshot.Order{}, err
	}
	order, err := c.services.Orders.Cancel(ctx, user.ID, orderID)
	if err != nil {
		return oneshot.Order{}, err
	}
	return toClientOrder(order), nil
}

func toClientSession(session domainauth.Session) oneshot.Session {
	return oneshot.Session{
		Token:    session.Token,
		Provider: session.Provider,
		User:     toClientUser(session.User),
	}
}

func toClientUser(user users.User) oneshot.User {
	return oneshot.User{
		ID:          user.ID,
		DisplayName: user.DisplayName,
		Email:       user.Email,
		AvatarURL:   user.AvatarURL,
	}
}

func toClientAgent(agent domainagents.Agent) oneshot.Agent {
	return oneshot.Agent{
		ID:                agent.ID,
		Name:              agent.Name,
		Category:          agent.Category,
		Tags:              agent.Tags,
		Description:       agent.Description,
		PriceUses:         agent.PriceUses,
		PriceCents:        agent.PriceCents,
		Rating:            agent.Rating,
		DealCount:         agent.DealCount,
		EstimatedDuration: agent.EstimatedDuration,
		Deliverable:       agent.Deliverable,
		ArtifactTypes:     agent.ArtifactTypes,
	}
}

func toClientBalance(balance domainbilling.Balance) oneshot.Balance {
	return oneshot.Balance{
		UserID:    balance.UserID,
		Remaining: balance.Remaining,
	}
}

func toClientLedgerEntry(entry domainbilling.LedgerEntry) oneshot.LedgerEntry {
	return oneshot.LedgerEntry{
		ID:           entry.ID,
		UserID:       entry.UserID,
		Type:         string(entry.Type),
		OrderID:      entry.OrderID,
		PaymentID:    entry.PaymentID,
		Delta:        entry.Delta,
		BalanceAfter: entry.BalanceAfter,
		CreatedAt:    entry.CreatedAt,
	}
}

func toClientPurchase(purchase domainbilling.Purchase) oneshot.Purchase {
	return oneshot.Purchase{
		ID:           purchase.ID,
		UserID:       purchase.UserID,
		PlanID:       purchase.PlanID,
		PaymentID:    purchase.PaymentID,
		Uses:         purchase.Uses,
		AmountCents:  purchase.AmountCents,
		BalanceAfter: purchase.BalanceAfter,
		CreatedAt:    purchase.CreatedAt,
	}
}

func toClientOrder(order domainorders.Order) oneshot.Order {
	return oneshot.Order{
		ID:        order.ID,
		UserID:    order.UserID,
		AgentID:   order.AgentID,
		AgentName: order.AgentName,
		Requirement: oneshot.Requirement{
			Prompt: order.Requirement.Prompt,
		},
		Status:                string(order.Status),
		UsageCost:             order.UsageCost,
		AmountCents:           order.AmountCents,
		EstimatedCompletionAt: order.EstimatedCompletionAt,
		FailureReason:         order.FailureReason,
		Progress:              toClientProgress(order.Progress),
		CreatedAt:             order.CreatedAt,
		UpdatedAt:             order.UpdatedAt,
	}
}

func toClientProgress(steps []domainorders.ProgressStep) []oneshot.ProgressStep {
	out := make([]oneshot.ProgressStep, 0, len(steps))
	for _, step := range steps {
		out = append(out, oneshot.ProgressStep{
			Key:       step.Key,
			Label:     step.Label,
			State:     step.State,
			Timestamp: step.Timestamp,
		})
	}
	return out
}

func toClientArtifact(artifact domainartifacts.Artifact) oneshot.Artifact {
	return oneshot.Artifact{
		ID:        artifact.ID,
		OrderID:   artifact.OrderID,
		UserID:    artifact.UserID,
		FileName:  artifact.FileName,
		FileType:  artifact.FileType,
		SizeBytes: artifact.SizeBytes,
		Preview:   artifact.Preview,
		CreatedAt: artifact.CreatedAt,
	}
}

func toClientArtifactShare(share domainartifacts.Share) oneshot.ArtifactShare {
	return oneshot.ArtifactShare{
		ArtifactID: share.ArtifactID,
		Token:      share.Token,
		URL:        share.URL,
		CreatedAt:  share.CreatedAt,
	}
}
