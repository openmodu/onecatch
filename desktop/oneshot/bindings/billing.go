package bindings

import (
	"context"

	oneshot "github.com/openmodu/oneshot/clients/oneshot"
)

type BillingBinding struct {
	client oneshot.Client
}

func NewBillingBinding(client oneshot.Client) *BillingBinding {
	return &BillingBinding{client: client}
}

func (b *BillingBinding) GetBalance() (oneshot.Balance, error) {
	return b.client.GetBalance(context.Background())
}

func (b *BillingBinding) ListLedger() ([]oneshot.LedgerEntry, error) {
	return b.client.ListLedger(context.Background())
}

func (b *BillingBinding) StartPurchase(input oneshot.StartPurchaseRequest) (oneshot.Purchase, error) {
	return b.client.StartPurchase(context.Background(), input)
}
