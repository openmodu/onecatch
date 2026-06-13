package execution

import (
	"context"
	"errors"
	"fmt"
	"time"

	domainartifacts "github.com/openmodu/oneshot/internal/domain/artifacts"
	domainorders "github.com/openmodu/oneshot/internal/domain/orders"
)

type OrderRepository interface {
	ListOrdersByStatus(context.Context, domainorders.Status, int) ([]domainorders.Order, error)
	TransitionOrder(ctx context.Context, order domainorders.Order, from domainorders.Status) error
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
	var errs []error
	for _, order := range items {
		err := s.completeOrder(ctx, order)
		if err == nil || errors.Is(err, domainorders.ErrStatusConflict) {
			// A status conflict means the order moved on concurrently
			// (e.g. the user cancelled it); skip without failing the batch.
			continue
		}
		errs = append(errs, fmt.Errorf("order %s: %w", order.ID, err))
	}
	return errors.Join(errs...)
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
	order.Status = domainorders.StatusDelivering
	order.UpdatedAt = s.now()
	if err := s.orders.TransitionOrder(ctx, order, domainorders.StatusRunning); err != nil {
		return err
	}

	// Generate the artifact before marking the order delivered, so clients
	// never observe a delivered order without its deliverable.
	if _, err := s.artifacts.CreateForOrder(ctx, order); err != nil {
		failed := order
		failed.Status = domainorders.StatusFailed
		failed.FailureReason = fmt.Sprintf("generate artifact: %v", err)
		failed.UpdatedAt = s.now()
		_ = s.orders.TransitionOrder(ctx, failed, domainorders.StatusDelivering)
		return err
	}

	delivered := order
	delivered.Status = domainorders.StatusDelivered
	delivered.UpdatedAt = s.now()
	return s.orders.TransitionOrder(ctx, delivered, domainorders.StatusDelivering)
}
