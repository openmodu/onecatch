package orders

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
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
	TransitionOrder(ctx context.Context, order orders.Order, from orders.Status) error
	ListOrders(context.Context, string) ([]orders.Order, error)
	ListOrdersByStatus(context.Context, orders.Status, int) ([]orders.Order, error)
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

type ListInput struct {
	UserID string        `json:"userId"`
	Status orders.Status `json:"status,omitempty"`
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
	if err := orders.ValidateRequirement(input.Requirement); err != nil {
		return orders.Order{}, err
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
	estimated := now.Add(parseDuration(agent.EstimatedDuration))
	order := orders.Order{
		ID:                    id,
		UserID:                input.UserID,
		AgentID:               input.AgentID,
		AgentName:             agent.Name,
		Requirement:           input.Requirement,
		Status:                orders.StatusRunning,
		UsageCost:             agent.PriceUses,
		AmountCents:           agent.PriceCents,
		EstimatedCompletionAt: estimated,
		CreatedAt:             now,
		UpdatedAt:             now,
	}
	order.Progress = orders.BuildProgress(order)
	if err := s.orders.SaveOrder(ctx, order); err != nil {
		// Compensate the debit so a failed save does not strand the user's uses.
		if refundErr := s.billing.RefundForOrder(ctx, input.UserID, id, agent.PriceUses); refundErr != nil {
			return orders.Order{}, fmt.Errorf("save order: %w (refund failed: %v)", err, refundErr)
		}
		return orders.Order{}, fmt.Errorf("save order: %w", err)
	}
	return order, nil
}

func (s *Usecase) List(ctx context.Context, input ListInput) ([]orders.Order, error) {
	items, err := s.orders.ListOrders(ctx, input.UserID)
	if err != nil {
		return nil, err
	}
	out := make([]orders.Order, 0, len(items))
	for _, order := range items {
		if input.Status == "" || order.Status == input.Status {
			order.Progress = orders.BuildProgress(order)
			out = append(out, order)
		}
	}
	return out, nil
}

func (s *Usecase) Get(ctx context.Context, userID string, orderID string) (orders.Order, error) {
	order, err := s.orders.GetOrder(ctx, userID, orderID)
	if err != nil {
		return orders.Order{}, err
	}
	order.Progress = orders.BuildProgress(order)
	return order, nil
}

func (s *Usecase) Cancel(ctx context.Context, userID string, orderID string) (orders.Order, error) {
	order, err := s.orders.GetOrder(ctx, userID, orderID)
	if err != nil {
		return orders.Order{}, err
	}
	if !order.CanCancel() {
		return orders.Order{}, fmt.Errorf("order %s cannot be cancelled from status %s", order.ID, order.Status)
	}

	from := order.Status
	order.Status = orders.StatusCancelled
	order.UpdatedAt = s.now()
	if err := s.orders.TransitionOrder(ctx, order, from); err != nil {
		return orders.Order{}, err
	}
	if err := s.billing.RefundForOrder(ctx, userID, order.ID, order.UsageCost); err != nil {
		return orders.Order{}, fmt.Errorf("order cancelled but refund failed: %w", err)
	}
	order.Progress = orders.BuildProgress(order)
	return order, nil
}

var durationNumbers = regexp.MustCompile(`\d+`)

// parseDuration reads estimates like "30-60 分钟" or "2-4 小时" and returns the
// upper bound. Unrecognized values fall back to one hour.
func parseDuration(value string) time.Duration {
	matches := durationNumbers.FindAllString(value, -1)
	if len(matches) == 0 {
		return time.Hour
	}
	upper, err := strconv.Atoi(matches[len(matches)-1])
	if err != nil || upper <= 0 {
		return time.Hour
	}
	if strings.Contains(value, "分钟") {
		return time.Duration(upper) * time.Minute
	}
	return time.Duration(upper) * time.Hour
}
