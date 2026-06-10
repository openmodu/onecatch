package billing

import (
	"errors"
	"time"
)

var ErrInsufficientBalance = errors.New("insufficient usage balance")
var ErrDuplicatePayment = errors.New("duplicate payment")

type Balance struct {
	UserID    string `json:"userId"`
	Remaining int    `json:"remaining"`
}

type LedgerType string

const (
	LedgerTypePurchase LedgerType = "purchase"
	LedgerTypeDebit    LedgerType = "debit"
	LedgerTypeRefund   LedgerType = "refund"
	LedgerTypeAdjust   LedgerType = "adjust"
)

type LedgerEntry struct {
	ID           string     `json:"id"`
	UserID       string     `json:"userId"`
	Type         LedgerType `json:"type"`
	OrderID      string     `json:"orderId,omitempty"`
	PaymentID    string     `json:"paymentId,omitempty"`
	Delta        int        `json:"delta"`
	BalanceAfter int        `json:"balanceAfter"`
	CreatedAt    time.Time  `json:"createdAt"`
}

type PurchasePlan struct {
	ID          string `json:"id"`
	Uses        int    `json:"uses"`
	AmountCents int    `json:"amountCents"`
}

type Purchase struct {
	ID           string    `json:"id"`
	UserID       string    `json:"userId"`
	PlanID       string    `json:"planId"`
	PaymentID    string    `json:"paymentId"`
	Uses         int       `json:"uses"`
	AmountCents  int       `json:"amountCents"`
	BalanceAfter int       `json:"balanceAfter"`
	CreatedAt    time.Time `json:"createdAt"`
}
