package billing

import (
	"errors"
	"time"
)

var ErrInsufficientBalance = errors.New("insufficient usage balance")

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
	Delta        int        `json:"delta"`
	BalanceAfter int        `json:"balanceAfter"`
	CreatedAt    time.Time  `json:"createdAt"`
}
