package conversations

import (
	"errors"
	"time"
)

var ErrNotFound = errors.New("conversation not found")
var ErrEmptyMessage = errors.New("message text is required")
var ErrNothingToConfirm = errors.New("no task awaiting confirmation")

type Status string

const (
	StatusActive          Status = "active"
	StatusAwaitingConfirm Status = "awaiting_confirm"
	StatusRunning         Status = "running"
)

type Role string

const (
	RoleUser   Role = "user"
	RoleAgent  Role = "agent"
	RoleSystem Role = "system"
)

type MessageKind string

const (
	KindText     MessageKind = "text"
	KindCheckout MessageKind = "checkout"
)

type Message struct {
	ID             string      `json:"id"`
	ConversationID string      `json:"conversationId"`
	Role           Role        `json:"role"`
	Kind           MessageKind `json:"kind"`
	Text           string      `json:"text"`
	CreatedAt      time.Time   `json:"createdAt"`
}

type Conversation struct {
	ID                 string    `json:"id"`
	UserID             string    `json:"userId"`
	AgentID            string    `json:"agentId"`
	AgentName          string    `json:"agentName"`
	Status             Status    `json:"status"`
	OrderID            string    `json:"orderId,omitempty"`
	PendingRequirement string    `json:"pendingRequirement,omitempty"`
	Messages           []Message `json:"messages"`
	CreatedAt          time.Time `json:"createdAt"`
	UpdatedAt          time.Time `json:"updatedAt"`
}
