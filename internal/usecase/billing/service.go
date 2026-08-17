package billing

import (
	"context"
	"fmt"
	"time"

	"github.com/openmodu/onecatch/internal/domain/billing"
)

type Repository interface {
	GetBalance(context.Context, string) (billing.Balance, error)
	ListLedger(context.Context, string) ([]billing.LedgerEntry, error)
	AddLedgerEntry(context.Context, billing.LedgerEntry) error
	SavePurchase(context.Context, billing.Purchase) (billing.Purchase, bool, error)
	DebitForOrder(ctx context.Context, userID string, orderID string, ledgerID string, uses int, at time.Time) (billing.LedgerEntry, error)
	RefundForOrder(ctx context.Context, userID string, orderID string, ledgerID string, uses int, at time.Time) (billing.LedgerEntry, error)
	NextLedgerID(context.Context) (string, error)
	NextPurchaseID(context.Context) (string, error)
}

type Usecase struct {
	repo Repository
	now  func() time.Time
}

func NewUsecase(repo Repository) *Usecase {
	return &Usecase{repo: repo, now: time.Now}
}

type PurchaseInput struct {
	UserID    string `json:"userId"`
	PlanID    string `json:"planId"`
	PaymentID string `json:"paymentId"`
}

var purchasePlans = map[string]billing.PurchasePlan{
	"uses_10":  {ID: "uses_10", Uses: 10, AmountCents: 1990},
	"uses_30":  {ID: "uses_30", Uses: 30, AmountCents: 4990},
	"uses_100": {ID: "uses_100", Uses: 100, AmountCents: 12900},
}

func (s *Usecase) GetBalance(ctx context.Context, userID string) (billing.Balance, error) {
	return s.repo.GetBalance(ctx, userID)
}

func (s *Usecase) ListLedger(ctx context.Context, userID string) ([]billing.LedgerEntry, error) {
	return s.repo.ListLedger(ctx, userID)
}

func (s *Usecase) StartPurchase(ctx context.Context, input PurchaseInput) (billing.Purchase, error) {
	if input.UserID == "" {
		return billing.Purchase{}, fmt.Errorf("user id is required")
	}
	plan, ok := purchasePlans[input.PlanID]
	if !ok {
		return billing.Purchase{}, fmt.Errorf("unknown purchase plan %q", input.PlanID)
	}

	balance, err := s.repo.GetBalance(ctx, input.UserID)
	if err != nil {
		return billing.Purchase{}, err
	}
	id, err := s.repo.NextPurchaseID(ctx)
	if err != nil {
		return billing.Purchase{}, err
	}
	if input.PaymentID == "" {
		input.PaymentID = "dev-payment-" + id
	}
	purchase := billing.Purchase{
		ID:           id,
		UserID:       input.UserID,
		PlanID:       plan.ID,
		PaymentID:    input.PaymentID,
		Uses:         plan.Uses,
		AmountCents:  plan.AmountCents,
		BalanceAfter: balance.Remaining + plan.Uses,
		CreatedAt:    s.now(),
	}
	purchase, duplicated, err := s.repo.SavePurchase(ctx, purchase)
	if err != nil {
		return billing.Purchase{}, err
	}
	if duplicated {
		return purchase, nil
	}

	ledgerID, err := s.repo.NextLedgerID(ctx)
	if err != nil {
		return billing.Purchase{}, err
	}
	if err := s.repo.AddLedgerEntry(ctx, billing.LedgerEntry{
		ID:           ledgerID,
		UserID:       input.UserID,
		Type:         billing.LedgerTypePurchase,
		PaymentID:    purchase.PaymentID,
		Delta:        plan.Uses,
		BalanceAfter: purchase.BalanceAfter,
		CreatedAt:    purchase.CreatedAt,
	}); err != nil {
		return billing.Purchase{}, err
	}
	return purchase, nil
}

func (s *Usecase) DebitForOrder(ctx context.Context, userID string, orderID string, uses int) error {
	if uses <= 0 {
		return fmt.Errorf("uses must be positive")
	}
	id, err := s.repo.NextLedgerID(ctx)
	if err != nil {
		return err
	}
	_, err = s.repo.DebitForOrder(ctx, userID, orderID, id, uses, s.now())
	return err
}

// RefundForOrder returns the uses debited for an order. It is idempotent per
// order: repeated calls return the original refund entry.
func (s *Usecase) RefundForOrder(ctx context.Context, userID string, orderID string, uses int) error {
	if uses <= 0 {
		return fmt.Errorf("uses must be positive")
	}
	id, err := s.repo.NextLedgerID(ctx)
	if err != nil {
		return err
	}
	_, err = s.repo.RefundForOrder(ctx, userID, orderID, id, uses, s.now())
	return err
}
