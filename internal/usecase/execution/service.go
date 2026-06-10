package execution

import (
	"context"
	"fmt"
	"time"

	domainartifacts "github.com/openmodu/oneshot/internal/domain/artifacts"
	domainorders "github.com/openmodu/oneshot/internal/domain/orders"
)

type OrderRepository interface {
	ListOrdersByStatus(context.Context, domainorders.Status, int) ([]domainorders.Order, error)
	SaveOrder(context.Context, domainorders.Order) error
}

type ArtifactCreator interface {
	CreateForOrder(context.Context, domainorders.Order) (domainartifacts.Artifact, error)
}

type Usecase struct {
	orders    OrderRepository
	artifacts ArtifactCreator
	now       func() time.Time
}

func NewUsecase(orderRepo OrderRepository, artifacts ArtifactCreator) *Usecase {
	return &Usecase{
		orders:    orderRepo,
		artifacts: artifacts,
		now:       time.Now,
	}
}

func (s *Usecase) RunOnce(ctx context.Context) error {
	items, err := s.orders.ListOrdersByStatus(ctx, domainorders.StatusRunning, 10)
	if err != nil {
		return err
	}
	for _, order := range items {
		if err := s.completeOrder(ctx, order); err != nil {
			return err
		}
	}
	return nil
}

func (s *Usecase) Start(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = time.Second
	}
	go func() {
		_ = s.RunOnce(ctx)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = s.RunOnce(ctx)
			}
		}
	}()
}

func (s *Usecase) completeOrder(ctx context.Context, order domainorders.Order) error {
	now := s.now()
	order.Status = domainorders.StatusDelivering
	order.UpdatedAt = now
	order.Progress = domainorders.BuildProgress(order)
	if err := s.orders.SaveOrder(ctx, order); err != nil {
		return err
	}

	order.Status = domainorders.StatusDelivered
	order.UpdatedAt = s.now()
	order.Progress = domainorders.BuildProgress(order)
	if err := s.orders.SaveOrder(ctx, order); err != nil {
		return err
	}
	if _, err := s.artifacts.CreateForOrder(ctx, order); err != nil {
		failed := order
		failed.Status = domainorders.StatusFailed
		failed.FailureReason = fmt.Sprintf("generate artifact: %v", err)
		failed.UpdatedAt = s.now()
		failed.Progress = domainorders.BuildProgress(failed)
		_ = s.orders.SaveOrder(ctx, failed)
		return err
	}
	return nil
}
