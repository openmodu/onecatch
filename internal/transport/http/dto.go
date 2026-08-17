package httptransport

import (
	"time"

	domainartifacts "github.com/openmodu/onecatch/internal/domain/artifacts"
	domainauth "github.com/openmodu/onecatch/internal/domain/auth"
	domainbilling "github.com/openmodu/onecatch/internal/domain/billing"
	domainconversations "github.com/openmodu/onecatch/internal/domain/conversations"
	domainorders "github.com/openmodu/onecatch/internal/domain/orders"
	"github.com/openmodu/onecatch/internal/domain/users"
	usecaseexecution "github.com/openmodu/onecatch/internal/usecase/execution"
)

// This file defines the user-facing response DTOs. They expose only the fields
// the desktop UI renders and deliberately omit internal/privacy fields:
//   - userId            (the caller already owns every resource it can read)
//   - paymentId         (internal payment reference)
//   - providerSubject   (OAuth identity mapping — never serialized here)
//   - artifact preview / storage internals
// Admin responses use separate DTOs under admin.go and may include more.

type userDTO struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	Email       string `json:"email,omitempty"`
	AvatarURL   string `json:"avatarUrl,omitempty"`
}

func toUserDTO(u users.User) userDTO {
	return userDTO{
		ID:          u.ID,
		DisplayName: u.DisplayName,
		Email:       u.Email,
		AvatarURL:   u.AvatarURL,
	}
}

type sessionDTO struct {
	Token     string    `json:"token"`
	Provider  string    `json:"provider"`
	User      userDTO   `json:"user"`
	ExpiresAt time.Time `json:"expiresAt,omitempty"`
}

func toSessionDTO(s domainauth.Session) sessionDTO {
	return sessionDTO{
		Token:     s.Token,
		Provider:  s.Provider,
		User:      toUserDTO(s.User),
		ExpiresAt: s.ExpiresAt,
	}
}

type balanceDTO struct {
	Remaining int `json:"remaining"`
}

func toBalanceDTO(b domainbilling.Balance) balanceDTO {
	return balanceDTO{Remaining: b.Remaining}
}

type ledgerEntryDTO struct {
	ID           string    `json:"id"`
	Type         string    `json:"type"`
	OrderID      string    `json:"orderId,omitempty"`
	Delta        int       `json:"delta"`
	BalanceAfter int       `json:"balanceAfter"`
	CreatedAt    time.Time `json:"createdAt"`
}

func toLedgerEntryDTO(e domainbilling.LedgerEntry) ledgerEntryDTO {
	return ledgerEntryDTO{
		ID:           e.ID,
		Type:         string(e.Type),
		OrderID:      e.OrderID,
		Delta:        e.Delta,
		BalanceAfter: e.BalanceAfter,
		CreatedAt:    e.CreatedAt,
	}
}

func toLedgerDTOs(entries []domainbilling.LedgerEntry) []ledgerEntryDTO {
	out := make([]ledgerEntryDTO, 0, len(entries))
	for _, e := range entries {
		out = append(out, toLedgerEntryDTO(e))
	}
	return out
}

type purchaseDTO struct {
	ID           string    `json:"id"`
	PlanID       string    `json:"planId"`
	Uses         int       `json:"uses"`
	AmountCents  int       `json:"amountCents"`
	BalanceAfter int       `json:"balanceAfter"`
	CreatedAt    time.Time `json:"createdAt"`
}

func toPurchaseDTO(p domainbilling.Purchase) purchaseDTO {
	return purchaseDTO{
		ID:           p.ID,
		PlanID:       p.PlanID,
		Uses:         p.Uses,
		AmountCents:  p.AmountCents,
		BalanceAfter: p.BalanceAfter,
		CreatedAt:    p.CreatedAt,
	}
}

type orderDTO struct {
	ID                    string                      `json:"id"`
	AgentID               string                      `json:"agentId"`
	AgentName             string                      `json:"agentName"`
	Requirement           domainorders.Requirement    `json:"requirement"`
	Status                string                      `json:"status"`
	UsageCost             int                         `json:"usageCost"`
	AmountCents           int                         `json:"amountCents"`
	EstimatedCompletionAt time.Time                   `json:"estimatedCompletionAt,omitempty"`
	FailureReason         string                      `json:"failureReason,omitempty"`
	Progress              []domainorders.ProgressStep `json:"progress"`
	CreatedAt             time.Time                   `json:"createdAt"`
	UpdatedAt             time.Time                   `json:"updatedAt"`
}

func toOrderDTO(o domainorders.Order) orderDTO {
	return orderDTO{
		ID:                    o.ID,
		AgentID:               o.AgentID,
		AgentName:             o.AgentName,
		Requirement:           o.Requirement,
		Status:                string(o.Status),
		UsageCost:             o.UsageCost,
		AmountCents:           o.AmountCents,
		EstimatedCompletionAt: o.EstimatedCompletionAt,
		FailureReason:         o.FailureReason,
		Progress:              o.Progress,
		CreatedAt:             o.CreatedAt,
		UpdatedAt:             o.UpdatedAt,
	}
}

func toOrderDTOs(orders []domainorders.Order) []orderDTO {
	out := make([]orderDTO, 0, len(orders))
	for _, o := range orders {
		out = append(out, toOrderDTO(o))
	}
	return out
}

type artifactDTO struct {
	ID        string    `json:"id"`
	OrderID   string    `json:"orderId"`
	FileName  string    `json:"fileName"`
	FileType  string    `json:"fileType"`
	SizeBytes int64     `json:"sizeBytes"`
	CreatedAt time.Time `json:"createdAt"`
}

func toArtifactDTO(a domainartifacts.Artifact) artifactDTO {
	return artifactDTO{
		ID:        a.ID,
		OrderID:   a.OrderID,
		FileName:  a.FileName,
		FileType:  a.FileType,
		SizeBytes: a.SizeBytes,
		CreatedAt: a.CreatedAt,
	}
}

func toArtifactDTOs(items []domainartifacts.Artifact) []artifactDTO {
	out := make([]artifactDTO, 0, len(items))
	for _, a := range items {
		out = append(out, toArtifactDTO(a))
	}
	return out
}

type messageDTO struct {
	ID        string    `json:"id"`
	Role      string    `json:"role"`
	Kind      string    `json:"kind"`
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"createdAt"`
}

type conversationDTO struct {
	ID        string       `json:"id"`
	AgentID   string       `json:"agentId"`
	AgentName string       `json:"agentName"`
	Status    string       `json:"status"`
	OrderID   string       `json:"orderId,omitempty"`
	Messages  []messageDTO `json:"messages"`
	CreatedAt time.Time    `json:"createdAt"`
	UpdatedAt time.Time    `json:"updatedAt"`
}

// toConversationDTO omits userId and the internal pendingRequirement (the task
// staged before confirmation is reflected by the visible user message instead).
func toConversationDTO(c domainconversations.Conversation) conversationDTO {
	messages := make([]messageDTO, 0, len(c.Messages))
	for _, m := range c.Messages {
		messages = append(messages, messageDTO{
			ID:        m.ID,
			Role:      string(m.Role),
			Kind:      string(m.Kind),
			Text:      m.Text,
			CreatedAt: m.CreatedAt,
		})
	}
	return conversationDTO{
		ID:        c.ID,
		AgentID:   c.AgentID,
		AgentName: c.AgentName,
		Status:    string(c.Status),
		OrderID:   c.OrderID,
		Messages:  messages,
		CreatedAt: c.CreatedAt,
		UpdatedAt: c.UpdatedAt,
	}
}

type runEventDTO struct {
	Kind string    `json:"kind"`
	Text string    `json:"text,omitempty"`
	At   time.Time `json:"at"`
}

type runDTO struct {
	OrderID      string        `json:"orderId"`
	Runtime      string        `json:"runtime,omitempty"`
	Status       string        `json:"status"`
	Events       []runEventDTO `json:"events"`
	FinalMessage string        `json:"finalMessage,omitempty"`
	Error        string        `json:"error,omitempty"`
	StartedAt    time.Time     `json:"startedAt,omitempty"`
	UpdatedAt    time.Time     `json:"updatedAt,omitempty"`
}

// toRunDTO projects the worker's live run log for the desktop. The raw JSONL
// from the runtime is intentionally dropped — only the normalized kind/text the
// UI renders is exposed.
func toRunDTO(orderID string, log usecaseexecution.RunLog, found bool) runDTO {
	if !found {
		// No run yet (e.g. order still pending): report a stable empty shape.
		return runDTO{OrderID: orderID, Status: "pending", Events: []runEventDTO{}}
	}
	events := make([]runEventDTO, 0, len(log.Events))
	for _, e := range log.Events {
		events = append(events, runEventDTO{
			Kind: string(e.Kind),
			Text: e.Text,
			At:   e.At,
		})
	}
	return runDTO{
		OrderID:      orderID,
		Runtime:      log.Runtime,
		Status:       string(log.Status),
		Events:       events,
		FinalMessage: log.FinalMessage,
		Error:        log.Error,
		StartedAt:    log.StartedAt,
		UpdatedAt:    log.UpdatedAt,
	}
}

type shareDTO struct {
	ArtifactID string    `json:"artifactId"`
	URL        string    `json:"url"`
	CreatedAt  time.Time `json:"createdAt"`
}

func toShareDTO(s domainartifacts.Share) shareDTO {
	return shareDTO{
		ArtifactID: s.ArtifactID,
		URL:        s.URL,
		CreatedAt:  s.CreatedAt,
	}
}
