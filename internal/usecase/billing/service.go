package billing

import (
	"context"
	"fmt"
	"time"

	"github.com/openmodu/oneshot/internal/domain/billing"
)

type Repository interface {
	GetBalance(context.Context, string) (billing.Balance, error)
	ListLedger(context.Context, string) ([]billing.LedgerEntry, error)
	AddLedgerEntry(context.Context, billing.LedgerEntry) error
	NextLedgerID(context.Context) (string, error)
}

type Usecase struct {
	repo Repository
	now  func() time.Time
}

func NewUsecase(repo Repository) *Usecase {
	return &Usecase{repo: repo, now: time.Now}
}

func (s *Usecase) GetBalance(ctx context.Context, userID string) (billing.Balance, error) {
	return s.repo.GetBalance(ctx, userID)
}

func (s *Usecase) ListLedger(ctx context.Context, userID string) ([]billing.LedgerEntry, error) {
	return s.repo.ListLedger(ctx, userID)
}

func (s *Usecase) DebitForOrder(ctx context.Context, userID string, orderID string, uses int) error {
	if uses <= 0 {
		return fmt.Errorf("uses must be positive")
	}

	balance, err := s.repo.GetBalance(ctx, userID)
	if err != nil {
		return err
	}
	if balance.Remaining < uses {
		return billing.ErrInsufficientBalance
	}

	id, err := s.repo.NextLedgerID(ctx)
	if err != nil {
		return err
	}

	return s.repo.AddLedgerEntry(ctx, billing.LedgerEntry{
		ID:           id,
		UserID:       userID,
		Type:         billing.LedgerTypeDebit,
		OrderID:      orderID,
		Delta:        -uses,
		BalanceAfter: balance.Remaining - uses,
		CreatedAt:    s.now(),
	})
}
