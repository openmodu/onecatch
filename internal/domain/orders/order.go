package orders

import (
	"errors"
	"strings"
	"time"
)

var ErrNotFound = errors.New("order not found")
var ErrInvalidRequirement = errors.New("order requirement is required")

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
	ID                    string         `json:"id"`
	UserID                string         `json:"userId"`
	AgentID               string         `json:"agentId"`
	AgentName             string         `json:"agentName"`
	Requirement           Requirement    `json:"requirement"`
	Status                Status         `json:"status"`
	UsageCost             int            `json:"usageCost"`
	AmountCents           int            `json:"amountCents"`
	EstimatedCompletionAt time.Time      `json:"estimatedCompletionAt,omitempty"`
	FailureReason         string         `json:"failureReason,omitempty"`
	Progress              []ProgressStep `json:"progress"`
	CreatedAt             time.Time      `json:"createdAt"`
	UpdatedAt             time.Time      `json:"updatedAt"`
}

type ProgressStep struct {
	Key       string    `json:"key"`
	Label     string    `json:"label"`
	State     string    `json:"state"`
	Timestamp time.Time `json:"timestamp,omitempty"`
}

func (o Order) CanCancel() bool {
	return o.Status == StatusDraft ||
		o.Status == StatusPendingPayment ||
		o.Status == StatusPaid ||
		o.Status == StatusRunning
}

func ValidateRequirement(requirement Requirement) error {
	if strings.TrimSpace(requirement.Prompt) == "" {
		return ErrInvalidRequirement
	}
	return nil
}

func BuildProgress(order Order) []ProgressStep {
	steps := []ProgressStep{
		{Key: "submitted", Label: "需求已提交"},
		{Key: "charged", Label: "扣次确认"},
		{Key: "running", Label: "执行中"},
		{Key: "delivering", Label: "交付物生成中"},
		{Key: "delivered", Label: "已交付"},
	}
	current := 0
	switch order.Status {
	case StatusRunning:
		current = 2
	case StatusDelivering:
		current = 3
	case StatusDelivered:
		current = 4
	case StatusFailed, StatusCancelled:
		current = 2
	default:
		current = 1
	}
	for i := range steps {
		switch {
		case i < current:
			steps[i].State = "done"
			steps[i].Timestamp = order.CreatedAt
		case i == current:
			steps[i].State = "current"
			if order.Status == StatusDelivered {
				steps[i].Timestamp = order.UpdatedAt
			}
		default:
			steps[i].State = "next"
		}
	}
	if order.Status == StatusFailed {
		steps[current].Label = "执行失败"
	}
	if order.Status == StatusCancelled {
		steps[current].Label = "已取消"
	}
	return steps
}
