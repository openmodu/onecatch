package execution

import (
	"context"
	"testing"
	"time"

	domainorders "github.com/openmodu/oneshot/internal/domain/orders"
	"github.com/openmodu/oneshot/internal/domain/users"
	repoartifacts "github.com/openmodu/oneshot/internal/repo/artifacts"
	repoorders "github.com/openmodu/oneshot/internal/repo/orders"
	usecaseartifacts "github.com/openmodu/oneshot/internal/usecase/artifacts"
)

func TestRunOnceDeliversRunningOrder(t *testing.T) {
	ctx := context.Background()
	orderRepo := repoorders.NewOrdersRepo(nil)
	artifactRepo := repoartifacts.NewArtifactsRepo(nil)
	artifactUsecase := usecaseartifacts.NewUsecase(artifactRepo, orderRepo)
	executionUsecase := NewUsecase(orderRepo, artifactUsecase)

	order := domainorders.Order{
		ID:          "order_worker_1",
		UserID:      users.DevUserID,
		AgentID:     "research-analyst",
		AgentName:   "行业研究分析师",
		Requirement: domainorders.Requirement{Prompt: "测试 worker"},
		Status:      domainorders.StatusRunning,
		UsageCost:   1,
		AmountCents: 1990,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	if err := orderRepo.SaveOrder(ctx, order); err != nil {
		t.Fatalf("save order: %v", err)
	}

	if err := executionUsecase.RunOnce(ctx); err != nil {
		t.Fatalf("run once: %v", err)
	}

	delivered, err := orderRepo.GetOrder(ctx, users.DevUserID, order.ID)
	if err != nil {
		t.Fatalf("get delivered order: %v", err)
	}
	if delivered.Status != domainorders.StatusDelivered {
		t.Fatalf("status = %s, want %s", delivered.Status, domainorders.StatusDelivered)
	}

	artifacts, err := artifactUsecase.ListForOrder(ctx, users.DevUserID, order.ID)
	if err != nil {
		t.Fatalf("list artifacts: %v", err)
	}
	if len(artifacts) != 1 || artifacts[0].OrderID != order.ID {
		t.Fatalf("unexpected artifacts: %+v", artifacts)
	}
}
