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

// stubSessions is a RunSessionReader returning a fixed resumable session.
type stubSessions struct {
	id string
	ok bool
}

func (s stubSessions) SessionID(string) (string, bool) { return s.id, s.ok }

func newOrderFixture() (*Usecase, *usecasebilling.Usecase, repoorders.OrdersRepo) {
	return newOrderFixtureWithSessions(stubSessions{})
}

func newOrderFixtureWithSessions(runs RunSessionReader) (*Usecase, *usecasebilling.Usecase, repoorders.OrdersRepo) {
	agentRepo := repoagents.NewAgentsRepo(nil)
	orderRepo := repoorders.NewOrdersRepo(nil)
	billingUsecase := usecasebilling.NewUsecase(repobilling.NewBillingRepo(nil))
	return NewUsecase(agentRepo, orderRepo, billingUsecase, runs), billingUsecase, orderRepo
}

func TestContinueResumesDeliveredOrder(t *testing.T) {
	ctx := context.Background()
	usecase, _, orderRepo := newOrderFixtureWithSessions(stubSessions{id: "sess-1", ok: true})

	order, err := usecase.Create(ctx, CreateInput{
		UserID:      users.DevUserID,
		AgentID:     "research-analyst",
		Requirement: domainorders.Requirement{Prompt: "首轮任务"},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	// Settle the order so it can be continued.
	order.Status = domainorders.StatusDelivered
	if err := orderRepo.TransitionOrder(ctx, order, domainorders.StatusRunning); err != nil {
		t.Fatalf("settle: %v", err)
	}

	resumed, err := usecase.Continue(ctx, users.DevUserID, order.ID, "继续做第二步")
	if err != nil {
		t.Fatalf("continue: %v", err)
	}
	if resumed.Status != domainorders.StatusRunning {
		t.Fatalf("status = %s, want running", resumed.Status)
	}
	if resumed.ResumeSessionID != "sess-1" {
		t.Fatalf("resume session = %q, want sess-1", resumed.ResumeSessionID)
	}
	if resumed.Requirement.Prompt != "继续做第二步" {
		t.Fatalf("requirement = %q", resumed.Requirement.Prompt)
	}
}

func TestContinueRejectsWhenNoSession(t *testing.T) {
	ctx := context.Background()
	usecase, _, orderRepo := newOrderFixtureWithSessions(stubSessions{ok: false})
	order, err := usecase.Create(ctx, CreateInput{
		UserID:      users.DevUserID,
		AgentID:     "research-analyst",
		Requirement: domainorders.Requirement{Prompt: "任务"},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	order.Status = domainorders.StatusDelivered
	_ = orderRepo.TransitionOrder(ctx, order, domainorders.StatusRunning)

	if _, err := usecase.Continue(ctx, users.DevUserID, order.ID, "继续"); err == nil {
		t.Fatal("expected error when no resumable session")
	}
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
