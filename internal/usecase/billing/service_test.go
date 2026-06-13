package billing

import (
	"context"
	"errors"
	"sync"
	"testing"

	domainbilling "github.com/openmodu/oneshot/internal/domain/billing"
	"github.com/openmodu/oneshot/internal/domain/users"
	repobilling "github.com/openmodu/oneshot/internal/repo/billing"
)

func TestConcurrentDebitNeverOverdraws(t *testing.T) {
	ctx := context.Background()
	usecase := NewUsecase(repobilling.NewBillingRepo(nil))

	// Dev user starts with 10 uses; 30 concurrent debits of 1 must allow
	// exactly 10 to succeed.
	const attempts = 30
	var wg sync.WaitGroup
	results := make(chan error, attempts)
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			results <- usecase.DebitForOrder(ctx, users.DevUserID, "order_race", 1)
		}(i)
	}
	wg.Wait()
	close(results)

	succeeded := 0
	for err := range results {
		if err == nil {
			succeeded++
			continue
		}
		if !errors.Is(err, domainbilling.ErrInsufficientBalance) {
			t.Fatalf("unexpected debit error: %v", err)
		}
	}
	if succeeded != 10 {
		t.Fatalf("successful debits = %d, want 10", succeeded)
	}

	balance, err := usecase.GetBalance(ctx, users.DevUserID)
	if err != nil {
		t.Fatalf("GetBalance() error = %v", err)
	}
	if balance.Remaining != 0 {
		t.Fatalf("remaining = %d, want 0", balance.Remaining)
	}
}

func TestRefundForOrderIsIdempotent(t *testing.T) {
	ctx := context.Background()
	usecase := NewUsecase(repobilling.NewBillingRepo(nil))

	if err := usecase.DebitForOrder(ctx, users.DevUserID, "order_refund", 3); err != nil {
		t.Fatalf("DebitForOrder() error = %v", err)
	}
	if err := usecase.RefundForOrder(ctx, users.DevUserID, "order_refund", 3); err != nil {
		t.Fatalf("RefundForOrder() error = %v", err)
	}
	if err := usecase.RefundForOrder(ctx, users.DevUserID, "order_refund", 3); err != nil {
		t.Fatalf("RefundForOrder() repeat error = %v", err)
	}

	balance, err := usecase.GetBalance(ctx, users.DevUserID)
	if err != nil {
		t.Fatalf("GetBalance() error = %v", err)
	}
	if balance.Remaining != 10 {
		t.Fatalf("remaining after refund = %d, want 10 (no double refund)", balance.Remaining)
	}

	ledger, err := usecase.ListLedger(ctx, users.DevUserID)
	if err != nil {
		t.Fatalf("ListLedger() error = %v", err)
	}
	refunds := 0
	for _, entry := range ledger {
		if entry.Type == domainbilling.LedgerTypeRefund {
			refunds++
		}
	}
	if refunds != 1 {
		t.Fatalf("refund entries = %d, want 1", refunds)
	}
}
