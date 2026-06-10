package orders

import (
	"errors"
	"time"
)

var ErrNotFound = errors.New("order not found")

type Status string

const (
	StatusDraft          Status = "draft"
	StatusPendingPayment Status = "pending_payment"
	StatusPaid           Status = "paid"
	StatusRunning        Status = "running"
	StatusDelivering     Status = "delivering"
	StatusDelivered      Status = "delivered"
	StatusFailed         Status = "failed"
	StatusCancelled      Status = "cancelled"
)

type Requirement struct {
	Prompt string `json:"prompt"`
}

type Order struct {
	ID          string      `json:"id"`
	UserID      string      `json:"userId"`
	AgentID     string      `json:"agentId"`
	Requirement Requirement `json:"requirement"`
	Status      Status      `json:"status"`
	UsageCost   int         `json:"usageCost"`
	CreatedAt   time.Time   `json:"createdAt"`
	UpdatedAt   time.Time   `json:"updatedAt"`
}

func (o Order) CanCancel() bool {
	return o.Status == StatusDraft ||
		o.Status == StatusPendingPayment ||
		o.Status == StatusPaid ||
		o.Status == StatusRunning
}
