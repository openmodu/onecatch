package orders

import (
	"context"
	"errors"
	"testing"

	domainorders "github.com/openmodu/oneshot/internal/domain/orders"
	"github.com/openmodu/oneshot/internal/domain/users"
	repoagents "github.com/openmodu/oneshot/internal/repo/agents"
	repobilling "github.com/openmodu/oneshot/internal/repo/billing"
	repoorders "github.com/openmodu/oneshot/internal/repo/orders"
	usecasebilling "github.com/openmodu/oneshot/internal/usecase/billing"
)

func newOrderFixture() (*Usecase, *usecasebilling.Usecase, repoorders.OrdersRepo) {
	agentRepo := repoagents.NewAgentsRepo(nil)
	orderRepo := repoorders.NewOrdersRepo(nil)
	billingUsecase := usecasebilling.NewUsecase(repobilling.NewBillingRepo(nil))
	return NewUsecase(agentRepo, orderRepo, billingUsecase), billingUsecase, orderRepo
}

func TestCancelRefundsDebitedUses(t *testing.T) {
	ctx := context.Background()
	usecase, billing, _ := newOrderFixture()

	order, err := usecase.Create(ctx, CreateInput{
		UserID:      users.DevUserID,
		AgentID:     "research-analyst",
		Requirement: domainorders.Requirement{Prompt: "测试取消退次"},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	afterDebit, err := billing.GetBalance(ctx, users.DevUserID)
	if err != nil {
		t.Fatalf("GetBalance() error = %v", err)
	}

	cancelled, err := usecase.Cancel(ctx, users.DevUserID, order.ID)
	if err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}
	if cancelled.Status != domainorders.StatusCancelled {
		t.Fatalf("status = %s, want cancelled", cancelled.Status)
	}

	afterRefund, err := billing.GetBalance(ctx, users.DevUserID)
	if err != nil {
		t.Fatalf("GetBalance() after refund error = %v", err)
	}
	if afterRefund.Remaining != afterDebit.Remaining+order.UsageCost {
		t.Fatalf("balance after cancel = %d, want %d", afterRefund.Remaining, afterDebit.Remaining+order.UsageCost)
	}
}

func TestCancelConflictsWithConcurrentDelivery(t *testing.T) {
	ctx := context.Background()
	usecase, _, orderRepo := newOrderFixture()

	order, err := usecase.Create(ctx, CreateInput{
		UserID:      users.DevUserID,
		AgentID:     "research-analyst",
		Requirement: domainorders.Requirement{Prompt: "测试取消竞态"},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Simulate the worker moving the order to delivering first.
	moved := order
	moved.Status = domainorders.StatusDelivering
	if err := orderRepo.TransitionOrder(ctx, moved, domainorders.StatusRunning); err != nil {
		t.Fatalf("TransitionOrder() error = %v", err)
	}

	// A cancel attempt that read the order as running before the worker's
	// transition must fail the conditional update instead of overwriting it.
	stale := order
	stale.Status = domainorders.StatusCancelled
	if err := orderRepo.TransitionOrder(ctx, stale, domainorders.StatusRunning); !errors.Is(err, domainorders.ErrStatusConflict) {
		t.Fatalf("stale TransitionOrder() err = %v, want ErrStatusConflict", err)
	}

	// And a fresh cancel sees the delivering status and is rejected outright.
	if _, err := usecase.Cancel(ctx, users.DevUserID, order.ID); err == nil {
		t.Fatal("Cancel() on delivering order succeeded, want error")
	}
}
