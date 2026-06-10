package orders

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	domainorders "github.com/openmodu/oneshot/internal/domain/orders"
	pkgsql "github.com/openmodu/oneshot/pkg/sql"
	"gorm.io/gorm"
)

type OrdersRepo interface {
	NextOrderID(context.Context) (string, error)
	SaveOrder(context.Context, domainorders.Order) error
	ListOrders(context.Context, string) ([]domainorders.Order, error)
	ListOrdersByStatus(context.Context, domainorders.Status, int) ([]domainorders.Order, error)
	GetOrder(context.Context, string, string) (domainorders.Order, error)
}

type ordersImpl struct {
	sql *pkgsql.Sql
	mu  sync.RWMutex

	orders    map[string]domainorders.Order
	nextOrder int

	migrateOnce sync.Once
	migrateErr  error
}

type orderRecord struct {
	ID                    string `gorm:"primaryKey;size:64"`
	UserID                string `gorm:"size:64;not null;index"`
	AgentID               string `gorm:"size:64;not null;index"`
	AgentName             string `gorm:"size:255;not null"`
	RequirementPrompt     string `gorm:"type:text"`
	Status                string `gorm:"size:32;not null;index"`
	UsageCost             int
	AmountCents           int
	EstimatedCompletionAt time.Time
	FailureReason         string `gorm:"type:text"`
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

func NewOrdersRepo(sql *pkgsql.Sql) OrdersRepo {
	return &ordersImpl{
		sql:    sql,
		orders: make(map[string]domainorders.Order),
	}
}

func (orderRecord) TableName() string {
	return "orders"
}

func (r *ordersImpl) NextOrderID(context.Context) (string, error) {
	if r.sql != nil && r.sql.Gorm() != nil {
		return fmt.Sprintf("order_%d", time.Now().UnixNano()), nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.nextOrder++
	return fmt.Sprintf("order_%06d", r.nextOrder), nil
}

func (r *ordersImpl) SaveOrder(ctx context.Context, order domainorders.Order) error {
	if r.sql != nil && r.sql.Gorm() != nil {
		return r.saveOrderSQL(ctx, order)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.orders[order.ID] = order
	return nil
}

func (r *ordersImpl) ListOrders(ctx context.Context, userID string) ([]domainorders.Order, error) {
	if r.sql != nil && r.sql.Gorm() != nil {
		return r.listOrdersSQL(ctx, userID)
	}

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

func (r *ordersImpl) ListOrdersByStatus(ctx context.Context, status domainorders.Status, limit int) ([]domainorders.Order, error) {
	if r.sql != nil && r.sql.Gorm() != nil {
		return r.listOrdersByStatusSQL(ctx, status, limit)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]domainorders.Order, 0)
	for _, order := range r.orders {
		if order.Status == status {
			out = append(out, order)
			if limit > 0 && len(out) >= limit {
				break
			}
		}
	}
	return out, nil
}

func (r *ordersImpl) GetOrder(ctx context.Context, userID string, orderID string) (domainorders.Order, error) {
	if r.sql != nil && r.sql.Gorm() != nil {
		return r.getOrderSQL(ctx, userID, orderID)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	order, ok := r.orders[orderID]
	if !ok || order.UserID != userID {
		return domainorders.Order{}, domainorders.ErrNotFound
	}
	return order, nil
}

func (r *ordersImpl) saveOrderSQL(ctx context.Context, order domainorders.Order) error {
	if err := r.ensureSchema(); err != nil {
		return err
	}
	record := fromDomainOrder(order)
	return r.sql.Gorm().WithContext(ctx).Save(&record).Error
}

func (r *ordersImpl) listOrdersSQL(ctx context.Context, userID string) ([]domainorders.Order, error) {
	if err := r.ensureSchema(); err != nil {
		return nil, err
	}
	var records []orderRecord
	if err := r.sql.Gorm().WithContext(ctx).Where("user_id = ?", userID).Order("created_at DESC").Find(&records).Error; err != nil {
		return nil, err
	}
	return toDomainOrders(records), nil
}

func (r *ordersImpl) listOrdersByStatusSQL(ctx context.Context, status domainorders.Status, limit int) ([]domainorders.Order, error) {
	if err := r.ensureSchema(); err != nil {
		return nil, err
	}
	db := r.sql.Gorm().WithContext(ctx).Where("status = ?", string(status)).Order("created_at ASC")
	if limit > 0 {
		db = db.Limit(limit)
	}
	var records []orderRecord
	if err := db.Find(&records).Error; err != nil {
		return nil, err
	}
	return toDomainOrders(records), nil
}

func (r *ordersImpl) getOrderSQL(ctx context.Context, userID string, orderID string) (domainorders.Order, error) {
	if err := r.ensureSchema(); err != nil {
		return domainorders.Order{}, err
	}
	var record orderRecord
	err := r.sql.Gorm().WithContext(ctx).Where("id = ? AND user_id = ?", orderID, userID).First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domainorders.Order{}, domainorders.ErrNotFound
	}
	if err != nil {
		return domainorders.Order{}, err
	}
	return toDomainOrder(record), nil
}

func (r *ordersImpl) ensureSchema() error {
	r.migrateOnce.Do(func() {
		r.migrateErr = r.sql.Gorm().AutoMigrate(&orderRecord{})
	})
	return r.migrateErr
}

func toDomainOrders(records []orderRecord) []domainorders.Order {
	out := make([]domainorders.Order, 0, len(records))
	for _, record := range records {
		out = append(out, toDomainOrder(record))
	}
	return out
}

func toDomainOrder(record orderRecord) domainorders.Order {
	return domainorders.Order{
		ID:                    record.ID,
		UserID:                record.UserID,
		AgentID:               record.AgentID,
		AgentName:             record.AgentName,
		Requirement:           domainorders.Requirement{Prompt: record.RequirementPrompt},
		Status:                domainorders.Status(record.Status),
		UsageCost:             record.UsageCost,
		AmountCents:           record.AmountCents,
		EstimatedCompletionAt: record.EstimatedCompletionAt,
		FailureReason:         record.FailureReason,
		CreatedAt:             record.CreatedAt,
		UpdatedAt:             record.UpdatedAt,
	}
}

func fromDomainOrder(order domainorders.Order) orderRecord {
	return orderRecord{
		ID:                    order.ID,
		UserID:                order.UserID,
		AgentID:               order.AgentID,
		AgentName:             order.AgentName,
		RequirementPrompt:     order.Requirement.Prompt,
		Status:                string(order.Status),
		UsageCost:             order.UsageCost,
		AmountCents:           order.AmountCents,
		EstimatedCompletionAt: order.EstimatedCompletionAt,
		FailureReason:         order.FailureReason,
		CreatedAt:             order.CreatedAt,
		UpdatedAt:             order.UpdatedAt,
	}
}
