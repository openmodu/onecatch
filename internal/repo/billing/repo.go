package billing

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	domainbilling "github.com/openmodu/oneshot/internal/domain/billing"
	"github.com/openmodu/oneshot/internal/domain/users"
	pkgsql "github.com/openmodu/oneshot/pkg/sql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type BillingRepo interface {
	GetBalance(context.Context, string) (domainbilling.Balance, error)
	ListLedger(context.Context, string) ([]domainbilling.LedgerEntry, error)
	AddLedgerEntry(context.Context, domainbilling.LedgerEntry) error
	SavePurchase(context.Context, domainbilling.Purchase) (domainbilling.Purchase, bool, error)
	NextLedgerID(context.Context) (string, error)
	NextPurchaseID(context.Context) (string, error)
}

type billingImpl struct {
	sql *pkgsql.Sql
	mu  sync.RWMutex

	balances     map[string]int
	ledger       []domainbilling.LedgerEntry
	purchases    map[string]domainbilling.Purchase
	nextLedger   int
	nextPurchase int

	migrateOnce sync.Once
	migrateErr  error
}

type balanceRecord struct {
	UserID    string `gorm:"primaryKey;size:64"`
	Remaining int
	CreatedAt time.Time
	UpdatedAt time.Time
}

type ledgerRecord struct {
	ID           string `gorm:"primaryKey;size:64"`
	UserID       string `gorm:"size:64;not null;index"`
	Type         string `gorm:"size:32;not null;index"`
	OrderID      string `gorm:"size:64;index"`
	PaymentID    string `gorm:"size:128;index"`
	Delta        int
	BalanceAfter int
	CreatedAt    time.Time
}

type purchaseRecord struct {
	ID           string `gorm:"primaryKey;size:64"`
	UserID       string `gorm:"size:64;not null;index"`
	PlanID       string `gorm:"size:64;not null"`
	PaymentID    string `gorm:"size:128;not null;uniqueIndex"`
	Uses         int
	AmountCents  int
	BalanceAfter int
	CreatedAt    time.Time
}

func NewBillingRepo(sql *pkgsql.Sql) BillingRepo {
	return &billingImpl{
		sql: sql,
		balances: map[string]int{
			users.DevUserID: 10,
		},
		purchases: make(map[string]domainbilling.Purchase),
	}
}

func (balanceRecord) TableName() string {
	return "user_balances"
}

func (ledgerRecord) TableName() string {
	return "billing_ledger"
}

func (purchaseRecord) TableName() string {
	return "billing_purchases"
}

func (r *billingImpl) GetBalance(ctx context.Context, userID string) (domainbilling.Balance, error) {
	if r.sql != nil && r.sql.Gorm() != nil {
		return r.getBalanceSQL(ctx, userID)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	return domainbilling.Balance{UserID: userID, Remaining: r.balances[userID]}, nil
}

func (r *billingImpl) ListLedger(ctx context.Context, userID string) ([]domainbilling.LedgerEntry, error) {
	if r.sql != nil && r.sql.Gorm() != nil {
		return r.listLedgerSQL(ctx, userID)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]domainbilling.LedgerEntry, 0, len(r.ledger))
	for _, entry := range r.ledger {
		if entry.UserID == userID {
			out = append(out, entry)
		}
	}
	return out, nil
}

func (r *billingImpl) AddLedgerEntry(ctx context.Context, entry domainbilling.LedgerEntry) error {
	if r.sql != nil && r.sql.Gorm() != nil {
		return r.addLedgerEntrySQL(ctx, entry)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.balances[entry.UserID] = entry.BalanceAfter
	r.ledger = append(r.ledger, entry)
	return nil
}

func (r *billingImpl) SavePurchase(ctx context.Context, purchase domainbilling.Purchase) (domainbilling.Purchase, bool, error) {
	if r.sql != nil && r.sql.Gorm() != nil {
		return r.savePurchaseSQL(ctx, purchase)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if existing, ok := r.purchases[purchase.PaymentID]; ok {
		return existing, true, nil
	}
	r.balances[purchase.UserID] = purchase.BalanceAfter
	r.purchases[purchase.PaymentID] = purchase
	return purchase, false, nil
}

func (r *billingImpl) NextLedgerID(context.Context) (string, error) {
	if r.sql != nil && r.sql.Gorm() != nil {
		return fmt.Sprintf("ledger_%d", time.Now().UnixNano()), nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.nextLedger++
	return fmt.Sprintf("ledger_%06d", r.nextLedger), nil
}

func (r *billingImpl) NextPurchaseID(context.Context) (string, error) {
	if r.sql != nil && r.sql.Gorm() != nil {
		return fmt.Sprintf("purchase_%d", time.Now().UnixNano()), nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.nextPurchase++
	return fmt.Sprintf("purchase_%06d", r.nextPurchase), nil
}

func (r *billingImpl) getBalanceSQL(ctx context.Context, userID string) (domainbilling.Balance, error) {
	if err := r.ensureSchema(); err != nil {
		return domainbilling.Balance{}, err
	}
	record, err := r.findOrCreateBalance(ctx, r.sql.Gorm(), userID)
	if err != nil {
		return domainbilling.Balance{}, err
	}
	return domainbilling.Balance{UserID: record.UserID, Remaining: record.Remaining}, nil
}

func (r *billingImpl) listLedgerSQL(ctx context.Context, userID string) ([]domainbilling.LedgerEntry, error) {
	if err := r.ensureSchema(); err != nil {
		return nil, err
	}
	var records []ledgerRecord
	if err := r.sql.Gorm().WithContext(ctx).Where("user_id = ?", userID).Order("created_at ASC").Find(&records).Error; err != nil {
		return nil, err
	}
	out := make([]domainbilling.LedgerEntry, 0, len(records))
	for _, record := range records {
		out = append(out, toDomainLedger(record))
	}
	return out, nil
}

func (r *billingImpl) addLedgerEntrySQL(ctx context.Context, entry domainbilling.LedgerEntry) error {
	if err := r.ensureSchema(); err != nil {
		return err
	}
	return r.sql.Gorm().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "user_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"remaining", "updated_at"}),
		}).Create(&balanceRecord{
			UserID:    entry.UserID,
			Remaining: entry.BalanceAfter,
			CreatedAt: entry.CreatedAt,
			UpdatedAt: entry.CreatedAt,
		}).Error; err != nil {
			return err
		}
		return tx.Create(&ledgerRecord{
			ID:           entry.ID,
			UserID:       entry.UserID,
			Type:         string(entry.Type),
			OrderID:      entry.OrderID,
			PaymentID:    entry.PaymentID,
			Delta:        entry.Delta,
			BalanceAfter: entry.BalanceAfter,
			CreatedAt:    entry.CreatedAt,
		}).Error
	})
}

func (r *billingImpl) savePurchaseSQL(ctx context.Context, purchase domainbilling.Purchase) (domainbilling.Purchase, bool, error) {
	if err := r.ensureSchema(); err != nil {
		return domainbilling.Purchase{}, false, err
	}
	var out domainbilling.Purchase
	duplicated := false
	err := r.sql.Gorm().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing purchaseRecord
		err := tx.Where("payment_id = ?", purchase.PaymentID).First(&existing).Error
		if err == nil {
			out = toDomainPurchase(existing)
			duplicated = true
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err := tx.Create(fromDomainPurchase(purchase)).Error; err != nil {
			return err
		}
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "user_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"remaining", "updated_at"}),
		}).Create(&balanceRecord{
			UserID:    purchase.UserID,
			Remaining: purchase.BalanceAfter,
			CreatedAt: purchase.CreatedAt,
			UpdatedAt: purchase.CreatedAt,
		}).Error; err != nil {
			return err
		}
		out = purchase
		return nil
	})
	return out, duplicated, err
}

func (r *billingImpl) findOrCreateBalance(ctx context.Context, db *gorm.DB, userID string) (balanceRecord, error) {
	var record balanceRecord
	err := db.WithContext(ctx).Where("user_id = ?", userID).First(&record).Error
	if err == nil {
		return record, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return balanceRecord{}, err
	}
	now := time.Now()
	record = balanceRecord{
		UserID:    userID,
		Remaining: defaultBalance(userID),
		CreatedAt: now,
		UpdatedAt: now,
	}
	return record, db.WithContext(ctx).Create(&record).Error
}

func (r *billingImpl) ensureSchema() error {
	r.migrateOnce.Do(func() {
		r.migrateErr = r.sql.Gorm().AutoMigrate(&balanceRecord{}, &ledgerRecord{}, &purchaseRecord{})
	})
	return r.migrateErr
}

func defaultBalance(userID string) int {
	if userID == users.DevUserID {
		return 10
	}
	return 0
}

func toDomainLedger(record ledgerRecord) domainbilling.LedgerEntry {
	return domainbilling.LedgerEntry{
		ID:           record.ID,
		UserID:       record.UserID,
		Type:         domainbilling.LedgerType(record.Type),
		OrderID:      record.OrderID,
		PaymentID:    record.PaymentID,
		Delta:        record.Delta,
		BalanceAfter: record.BalanceAfter,
		CreatedAt:    record.CreatedAt,
	}
}

func fromDomainPurchase(purchase domainbilling.Purchase) *purchaseRecord {
	return &purchaseRecord{
		ID:           purchase.ID,
		UserID:       purchase.UserID,
		PlanID:       purchase.PlanID,
		PaymentID:    purchase.PaymentID,
		Uses:         purchase.Uses,
		AmountCents:  purchase.AmountCents,
		BalanceAfter: purchase.BalanceAfter,
		CreatedAt:    purchase.CreatedAt,
	}
}

func toDomainPurchase(record purchaseRecord) domainbilling.Purchase {
	return domainbilling.Purchase{
		ID:           record.ID,
		UserID:       record.UserID,
		PlanID:       record.PlanID,
		PaymentID:    record.PaymentID,
		Uses:         record.Uses,
		AmountCents:  record.AmountCents,
		BalanceAfter: record.BalanceAfter,
		CreatedAt:    record.CreatedAt,
	}
}
