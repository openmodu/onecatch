package orders

import (
	"context"
	"fmt"
	"time"

	"github.com/openmodu/oneshot/internal/domain/agents"
	"github.com/openmodu/oneshot/internal/domain/orders"
	usecasebilling "github.com/openmodu/oneshot/internal/usecase/billing"
)

type AgentRepository interface {
	GetAgent(context.Context, string) (agents.Agent, error)
}

type Repository interface {
	NextOrderID(context.Context) (string, error)
	SaveOrder(context.Context, orders.Order) error
	ListOrders(context.Context, string) ([]orders.Order, error)
	GetOrder(context.Context, string, string) (orders.Order, error)
}

type Usecase struct {
	agents  AgentRepository
	orders  Repository
	billing *usecasebilling.Usecase
	now     func() time.Time
}

type CreateInput struct {
	UserID      string             `json:"userId"`
	AgentID     string             `json:"agentId"`
	Requirement orders.Requirement `json:"requirement"`
}

func NewUsecase(agentRepo AgentRepository, orderRepo Repository, billing *usecasebilling.Usecase) *Usecase {
	return &Usecase{
		agents:  agentRepo,
		orders:  orderRepo,
		billing: billing,
		now:     time.Now,
	}
}

func (s *Usecase) Create(ctx context.Context, input CreateInput) (orders.Order, error) {
	if input.UserID == "" {
		return orders.Order{}, fmt.Errorf("user id is required")
	}
	if input.AgentID == "" {
		return orders.Order{}, fmt.Errorf("agent id is required")
	}

	agent, err := s.agents.GetAgent(ctx, input.AgentID)
	if err != nil {
		return orders.Order{}, err
	}

	id, err := s.orders.NextOrderID(ctx)
	if err != nil {
		return orders.Order{}, err
	}

	if err := s.billing.DebitForOrder(ctx, input.UserID, id, agent.PriceUses); err != nil {
		return orders.Order{}, err
	}

	now := s.now()
	order := orders.Order{
		ID:          id,
		UserID:      input.UserID,
		AgentID:     input.AgentID,
		Requirement: input.Requirement,
		Status:      orders.StatusRunning,
		UsageCost:   agent.PriceUses,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	return order, s.orders.SaveOrder(ctx, order)
}

func (s *Usecase) List(ctx context.Context, userID string) ([]orders.Order, error) {
	return s.orders.ListOrders(ctx, userID)
}

func (s *Usecase) Get(ctx context.Context, userID string, orderID string) (orders.Order, error) {
	return s.orders.GetOrder(ctx, userID, orderID)
}

func (s *Usecase) Cancel(ctx context.Context, userID string, orderID string) (orders.Order, error) {
	order, err := s.orders.GetOrder(ctx, userID, orderID)
	if err != nil {
		return orders.Order{}, err
	}
	if !order.CanCancel() {
		return orders.Order{}, fmt.Errorf("order %s cannot be cancelled from status %s", order.ID, order.Status)
	}

	order.Status = orders.StatusCancelled
	order.UpdatedAt = s.now()
	return order, s.orders.SaveOrder(ctx, order)
}
