package conversations

import (
	"context"
	"fmt"
	"strings"
	"time"

	domainagents "github.com/openmodu/oneshot/internal/domain/agents"
	domainconversations "github.com/openmodu/oneshot/internal/domain/conversations"
	domainorders "github.com/openmodu/oneshot/internal/domain/orders"
	usecaseorders "github.com/openmodu/oneshot/internal/usecase/orders"
)

type Repository interface {
	NextConversationID(context.Context) (string, error)
	NextMessageID(context.Context) (string, error)
	Create(context.Context, domainconversations.Conversation) error
	Get(ctx context.Context, userID string, id string) (domainconversations.Conversation, error)
	AppendMessage(context.Context, domainconversations.Message) error
	UpdateMeta(context.Context, domainconversations.Conversation) error
}

type AgentGetter interface {
	Get(ctx context.Context, id string) (domainagents.Agent, error)
}

type OrderCreator interface {
	Create(ctx context.Context, input usecaseorders.CreateInput) (domainorders.Order, error)
}

type Usecase struct {
	repo   Repository
	agents AgentGetter
	orders OrderCreator
	now    func() time.Time
}

func NewUsecase(repo Repository, agents AgentGetter, orders OrderCreator) *Usecase {
	return &Usecase{repo: repo, agents: agents, orders: orders, now: time.Now}
}

// Start opens a conversation with an agent and seeds the greeting message.
func (s *Usecase) Start(ctx context.Context, userID string, agentID string) (domainconversations.Conversation, error) {
	if userID == "" {
		return domainconversations.Conversation{}, fmt.Errorf("user id is required")
	}
	agent, err := s.agents.Get(ctx, agentID)
	if err != nil {
		return domainconversations.Conversation{}, err
	}

	convID, err := s.repo.NextConversationID(ctx)
	if err != nil {
		return domainconversations.Conversation{}, err
	}
	now := s.now()
	greeting, err := s.buildMessage(ctx, convID, domainconversations.RoleAgent, domainconversations.KindText, greetingText(agent), now)
	if err != nil {
		return domainconversations.Conversation{}, err
	}

	conv := domainconversations.Conversation{
		ID:        convID,
		UserID:    userID,
		AgentID:   agent.ID,
		AgentName: agent.Name,
		Status:    domainconversations.StatusActive,
		Messages:  []domainconversations.Message{greeting},
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.repo.Create(ctx, conv); err != nil {
		return domainconversations.Conversation{}, err
	}
	return conv, nil
}

func (s *Usecase) Get(ctx context.Context, userID string, convID string) (domainconversations.Conversation, error) {
	return s.repo.Get(ctx, userID, convID)
}

// PostMessage records the user's message and replies with an agent checkout
// proposal. Before confirmation the user may post again to revise the task;
// the latest message becomes the pending requirement.
func (s *Usecase) PostMessage(ctx context.Context, userID string, convID string, text string) (domainconversations.Conversation, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return domainconversations.Conversation{}, domainconversations.ErrEmptyMessage
	}
	conv, err := s.repo.Get(ctx, userID, convID)
	if err != nil {
		return domainconversations.Conversation{}, err
	}

	now := s.now()
	if err := s.appendMessage(ctx, convID, domainconversations.RoleUser, domainconversations.KindText, text, now); err != nil {
		return domainconversations.Conversation{}, err
	}

	agent, err := s.agents.Get(ctx, conv.AgentID)
	if err != nil {
		return domainconversations.Conversation{}, err
	}
	if err := s.appendMessage(ctx, convID, domainconversations.RoleAgent, domainconversations.KindCheckout, checkoutText(agent), s.now()); err != nil {
		return domainconversations.Conversation{}, err
	}

	conv.Status = domainconversations.StatusAwaitingConfirm
	conv.PendingRequirement = text
	conv.UpdatedAt = s.now()
	if err := s.repo.UpdateMeta(ctx, conv); err != nil {
		return domainconversations.Conversation{}, err
	}
	return s.repo.Get(ctx, userID, convID)
}

// Confirm creates the order for the pending requirement (the real billing
// debit happens here) and moves the conversation into the running state.
func (s *Usecase) Confirm(ctx context.Context, userID string, convID string) (domainconversations.Conversation, error) {
	conv, err := s.repo.Get(ctx, userID, convID)
	if err != nil {
		return domainconversations.Conversation{}, err
	}
	if conv.Status != domainconversations.StatusAwaitingConfirm || strings.TrimSpace(conv.PendingRequirement) == "" {
		return domainconversations.Conversation{}, domainconversations.ErrNothingToConfirm
	}

	order, err := s.orders.Create(ctx, usecaseorders.CreateInput{
		UserID:      userID,
		AgentID:     conv.AgentID,
		Requirement: domainorders.Requirement{Prompt: conv.PendingRequirement},
	})
	if err != nil {
		return domainconversations.Conversation{}, err
	}

	if err := s.appendMessage(ctx, convID, domainconversations.RoleSystem, domainconversations.KindText, runningText(order), s.now()); err != nil {
		return domainconversations.Conversation{}, err
	}

	conv.Status = domainconversations.StatusRunning
	conv.OrderID = order.ID
	conv.PendingRequirement = ""
	conv.UpdatedAt = s.now()
	if err := s.repo.UpdateMeta(ctx, conv); err != nil {
		return domainconversations.Conversation{}, err
	}
	return s.repo.Get(ctx, userID, convID)
}

// buildMessage mints a message with a fresh ID but does not persist it; used
// for the greeting, which is written as part of the conversation's Create.
func (s *Usecase) buildMessage(ctx context.Context, convID string, role domainconversations.Role, kind domainconversations.MessageKind, text string, at time.Time) (domainconversations.Message, error) {
	id, err := s.repo.NextMessageID(ctx)
	if err != nil {
		return domainconversations.Message{}, err
	}
	return domainconversations.Message{
		ID:             id,
		ConversationID: convID,
		Role:           role,
		Kind:           kind,
		Text:           text,
		CreatedAt:      at,
	}, nil
}

// appendMessage mints and persists a message onto an existing conversation.
func (s *Usecase) appendMessage(ctx context.Context, convID string, role domainconversations.Role, kind domainconversations.MessageKind, text string, at time.Time) error {
	msg, err := s.buildMessage(ctx, convID, role, kind, text, at)
	if err != nil {
		return err
	}
	return s.repo.AppendMessage(ctx, msg)
}

func greetingText(agent domainagents.Agent) string {
	return fmt.Sprintf("你好，我是%s。%s\n请描述你的任务，我会先给出扣次确认。", agent.Name, agent.Description)
}

func checkoutText(agent domainagents.Agent) string {
	return fmt.Sprintf("我已理解你的任务。本次执行将扣减 %d 次，确认后开始。", maxInt(agent.PriceUses, 1))
}

func runningText(order domainorders.Order) string {
	return fmt.Sprintf("已开始执行，订单号 %s。进度和交付物会在这里更新。", order.ID)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
