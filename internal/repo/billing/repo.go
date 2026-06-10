package billing

import (
	"context"
	"fmt"
	"sync"

	domainbilling "github.com/openmodu/oneshot/internal/domain/billing"
	"github.com/openmodu/oneshot/internal/domain/users"
	pkgsql "github.com/openmodu/oneshot/pkg/sql"
)

type BillingRepo interface {
	GetBalance(context.Context, string) (domainbilling.Balance, error)
	ListLedger(context.Context, string) ([]domainbilling.LedgerEntry, error)
	AddLedgerEntry(context.Context, domainbilling.LedgerEntry) error
	SavePurchase(context.Context, domainbilling.Purchase) (domainbilling.Purchase, bool, error)
	NextLedgerID(context.Context) (string, error)
	NextPurchaseID(context.Context) (string, error)
}

type billingImpl struct {
	sql *pkgsql.Sql
	mu  sync.RWMutex

	balances     map[string]int
	ledger       []domainbilling.LedgerEntry
	purchases    map[string]domainbilling.Purchase
	nextLedger   int
	nextPurchase int
}

func NewBillingRepo(sql *pkgsql.Sql) BillingRepo {
	return &billingImpl{
		sql: sql,
		balances: map[string]int{
			users.DevUserID: 10,
		},
		purchases: make(map[string]domainbilling.Purchase),
	}
}

func (r *billingImpl) GetBalance(_ context.Context, userID string) (domainbilling.Balance, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return domainbilling.Balance{UserID: userID, Remaining: r.balances[userID]}, nil
}

func (r *billingImpl) ListLedger(_ context.Context, userID string) ([]domainbilling.LedgerEntry, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]domainbilling.LedgerEntry, 0, len(r.ledger))
	for _, entry := range r.ledger {
		if entry.UserID == userID {
			out = append(out, entry)
		}
	}
	return out, nil
}

func (r *billingImpl) AddLedgerEntry(_ context.Context, entry domainbilling.LedgerEntry) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.balances[entry.UserID] = entry.BalanceAfter
	r.ledger = append(r.ledger, entry)
	return nil
}

func (r *billingImpl) SavePurchase(_ context.Context, purchase domainbilling.Purchase) (domainbilling.Purchase, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if existing, ok := r.purchases[purchase.PaymentID]; ok {
		return existing, true, nil
	}
	r.balances[purchase.UserID] = purchase.BalanceAfter
	r.purchases[purchase.PaymentID] = purchase
	return purchase, false, nil
}

func (r *billingImpl) NextLedgerID(context.Context) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.nextLedger++
	return fmt.Sprintf("ledger_%06d", r.nextLedger), nil
}

func (r *billingImpl) NextPurchaseID(context.Context) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.nextPurchase++
	return fmt.Sprintf("purchase_%06d", r.nextPurchase), nil
}
