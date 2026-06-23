package conversations

import (
	"context"
	"errors"
	"testing"

	domainconversations "github.com/openmodu/oneshot/internal/domain/conversations"
	"github.com/openmodu/oneshot/internal/domain/users"
	repoagents "github.com/openmodu/oneshot/internal/repo/agents"
	repobilling "github.com/openmodu/oneshot/internal/repo/billing"
	repoconversations "github.com/openmodu/oneshot/internal/repo/conversations"
	repoorders "github.com/openmodu/oneshot/internal/repo/orders"
	usecaseagents "github.com/openmodu/oneshot/internal/usecase/agents"
	usecasebilling "github.com/openmodu/oneshot/internal/usecase/billing"
	usecaseorders "github.com/openmodu/oneshot/internal/usecase/orders"
)

// noResume satisfies orders.RunSessionReader; the conversation tests never
// resume, so it always reports no session.
type noResume struct{}

func (noResume) SessionID(string) (string, bool) { return "", false }

func newFixture() (*Usecase, *usecasebilling.Usecase) {
	agentRepo := repoagents.NewAgentsRepo(nil)
	billingUsecase := usecasebilling.NewUsecase(repobilling.NewBillingRepo(nil))
	ordersUsecase := usecaseorders.NewUsecase(agentRepo, repoorders.NewOrdersRepo(nil), billingUsecase, noResume{})
	usecase := NewUsecase(repoconversations.NewConversationsRepo(nil), usecaseagents.NewUsecase(agentRepo), ordersUsecase)
	return usecase, billingUsecase
}

func TestConversationFlowDebitsOnlyOnConfirm(t *testing.T) {
	ctx := context.Background()
	usecase, billing := newFixture()

	before, _ := billing.GetBalance(ctx, users.DevUserID)

	conv, err := usecase.Start(ctx, users.DevUserID, "research-analyst", "")
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if conv.Status != domainconversations.StatusActive || len(conv.Messages) != 1 {
		t.Fatalf("unexpected start state: %+v", conv)
	}

	conv, err = usecase.PostMessage(ctx, users.DevUserID, conv.ID, "帮我研究 2026 AI Agent 市场")
	if err != nil {
		t.Fatalf("PostMessage() error = %v", err)
	}
	if conv.Status != domainconversations.StatusAwaitingConfirm {
		t.Fatalf("status = %s, want awaiting_confirm", conv.Status)
	}
	last := conv.Messages[len(conv.Messages)-1]
	if last.Kind != domainconversations.KindCheckout {
		t.Fatalf("last message kind = %s, want checkout", last.Kind)
	}

	// No debit until confirmation.
	mid, _ := billing.GetBalance(ctx, users.DevUserID)
	if mid.Remaining != before.Remaining {
		t.Fatalf("balance changed before confirm: %d -> %d", before.Remaining, mid.Remaining)
	}

	conv, err = usecase.Confirm(ctx, users.DevUserID, conv.ID)
	if err != nil {
		t.Fatalf("Confirm() error = %v", err)
	}
	if conv.Status != domainconversations.StatusRunning || conv.OrderID == "" {
		t.Fatalf("unexpected confirmed state: %+v", conv)
	}
	after, _ := billing.GetBalance(ctx, users.DevUserID)
	if after.Remaining != before.Remaining-1 {
		t.Fatalf("balance after confirm = %d, want %d", after.Remaining, before.Remaining-1)
	}

	// Nothing left to confirm.
	if _, err := usecase.Confirm(ctx, users.DevUserID, conv.ID); !errors.Is(err, domainconversations.ErrNothingToConfirm) {
		t.Fatalf("second Confirm() err = %v, want ErrNothingToConfirm", err)
	}
}

func TestConversationCrossUserIsolation(t *testing.T) {
	ctx := context.Background()
	usecase, _ := newFixture()

	conv, err := usecase.Start(ctx, users.DevUserID, "research-analyst", "")
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if _, err := usecase.Get(ctx, "another-user", conv.ID); !errors.Is(err, domainconversations.ErrNotFound) {
		t.Fatalf("cross-user Get() err = %v, want ErrNotFound", err)
	}
	if _, err := usecase.PostMessage(ctx, "another-user", conv.ID, "偷看"); !errors.Is(err, domainconversations.ErrNotFound) {
		t.Fatalf("cross-user PostMessage() err = %v, want ErrNotFound", err)
	}
}

func TestPostMessageRejectsEmpty(t *testing.T) {
	ctx := context.Background()
	usecase, _ := newFixture()

	conv, err := usecase.Start(ctx, users.DevUserID, "research-analyst", "")
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if _, err := usecase.PostMessage(ctx, users.DevUserID, conv.ID, "   "); !errors.Is(err, domainconversations.ErrEmptyMessage) {
		t.Fatalf("empty PostMessage() err = %v, want ErrEmptyMessage", err)
	}
}
