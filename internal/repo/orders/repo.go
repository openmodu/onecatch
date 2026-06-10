package orders

import (
	"context"
	"fmt"
	"sync"

	domainorders "github.com/openmodu/oneshot/internal/domain/orders"
	pkgsql "github.com/openmodu/oneshot/pkg/sql"
)

type OrdersRepo interface {
	NextOrderID(context.Context) (string, error)
	SaveOrder(context.Context, domainorders.Order) error
	ListOrders(context.Context, string) ([]domainorders.Order, error)
	GetOrder(context.Context, string, string) (domainorders.Order, error)
}

type ordersImpl struct {
	sql *pkgsql.Sql
	mu  sync.RWMutex

	orders    map[string]domainorders.Order
	nextOrder int
}

func NewOrdersRepo(sql *pkgsql.Sql) OrdersRepo {
	return &ordersImpl{
		sql:    sql,
		orders: make(map[string]domainorders.Order),
	}
}

func (r *ordersImpl) NextOrderID(context.Context) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.nextOrder++
	return fmt.Sprintf("order_%06d", r.nextOrder), nil
}

func (r *ordersImpl) SaveOrder(_ context.Context, order domainorders.Order) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.orders[order.ID] = order
	return nil
}

func (r *ordersImpl) ListOrders(_ context.Context, userID string) ([]domainorders.Order, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]domainorders.Order, 0, len(r.orders))
	for _, order := range r.orders {
		if order.UserID == userID {
			out = append(out, order)
		}
	}
	return out, nil
}

func (r *ordersImpl) GetOrder(_ context.Context, userID string, orderID string) (domainorders.Order, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	order, ok := r.orders[orderID]
	if !ok || order.UserID != userID {
		return domainorders.Order{}, domainorders.ErrNotFound
	}
	return order, nil
}
