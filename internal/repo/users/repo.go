package users

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	domainusers "github.com/openmodu/onecatch/internal/domain/users"
	pkgsql "github.com/openmodu/onecatch/pkg/sql"
	"gorm.io/gorm"
)

const activeStatus = "active"

type UsersRepo interface {
	FindOrCreateByIdentity(context.Context, domainusers.AuthIdentity) (domainusers.User, error)
}

type usersImpl struct {
	sql *pkgsql.Sql

	mu         sync.RWMutex
	users      map[string]domainusers.User
	identities map[string]domainusers.AuthIdentity
}

type userRecord struct {
	ID          string `gorm:"primaryKey;size:64"`
	DisplayName string `gorm:"size:255;not null"`
	Email       string `gorm:"size:320;index"`
	AvatarURL   string `gorm:"size:1024"`
	Status      string `gorm:"size:32;not null"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type authIdentityRecord struct {
	ID              string `gorm:"primaryKey;size:128"`
	UserID          string `gorm:"size:64;not null;index"`
	Provider        string `gorm:"size:32;not null;uniqueIndex:idx_auth_provider_subject"`
	ProviderSubject string `gorm:"size:255;not null;uniqueIndex:idx_auth_provider_subject"`
	Email           string `gorm:"size:320;index"`
	DisplayName     string `gorm:"size:255"`
	AvatarURL       string `gorm:"size:1024"`
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func NewUsersRepo(sql *pkgsql.Sql) UsersRepo {
	return &usersImpl{
		sql:        sql,
		users:      make(map[string]domainusers.User),
		identities: make(map[string]domainusers.AuthIdentity),
	}
}

func (userRecord) TableName() string {
	return "users"
}

func (authIdentityRecord) TableName() string {
	return "auth_identities"
}

func (r *usersImpl) FindOrCreateByIdentity(ctx context.Context, identity domainusers.AuthIdentity) (domainusers.User, error) {
	if err := validateIdentity(identity); err != nil {
		return domainusers.User{}, err
	}
	if r.sql == nil || r.sql.Gorm() == nil {
		return r.findOrCreateInMemory(identity), nil
	}
	return r.findOrCreateInSQL(ctx, identity)
}

func (r *usersImpl) findOrCreateInMemory(identity domainusers.AuthIdentity) domainusers.User {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := identityKey(identity.Provider, identity.ProviderSubject)
	if existing, ok := r.identities[key]; ok {
		return r.users[existing.UserID]
	}

	now := time.Now()
	userID := candidateUserID(identity)
	user, ok := r.users[userID]
	if !ok && identity.Email != "" {
		for _, existing := range r.users {
			if existing.Email == identity.Email {
				user = existing
				ok = true
				break
			}
		}
	}
	if !ok {
		user = domainusers.User{
			ID:          userID,
			DisplayName: defaultString(identity.DisplayName, identity.Provider),
			Email:       identity.Email,
			AvatarURL:   identity.AvatarURL,
			Status:      activeStatus,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		r.users[user.ID] = user
	}

	identity.ID = candidateIdentityID(identity)
	identity.UserID = user.ID
	identity.CreatedAt = now
	identity.UpdatedAt = now
	r.identities[key] = identity
	return user
}

func (r *usersImpl) findOrCreateInSQL(ctx context.Context, identity domainusers.AuthIdentity) (domainusers.User, error) {
	var out domainusers.User
	err := r.sql.Gorm().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existingIdentity authIdentityRecord
		err := tx.Where("provider = ? AND provider_subject = ?", identity.Provider, identity.ProviderSubject).
			First(&existingIdentity).Error
		if err == nil {
			user, err := findUserRecord(tx, existingIdentity.UserID)
			if err != nil {
				return err
			}
			out = toDomainUser(user)
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		user, err := r.resolveUserForIdentity(tx, identity)
		if err != nil {
			return err
		}

		record := fromDomainIdentity(identity)
		record.ID = candidateIdentityID(identity)
		record.UserID = user.ID
		if err := tx.Create(&record).Error; err != nil {
			return err
		}
		out = toDomainUser(user)
		return nil
	})
	return out, err
}

func (r *usersImpl) resolveUserForIdentity(tx *gorm.DB, identity domainusers.AuthIdentity) (userRecord, error) {
	userID := candidateUserID(identity)
	if user, err := findUserRecord(tx, userID); err == nil {
		return user, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return userRecord{}, err
	}

	if identity.Email != "" {
		var existing userRecord
		err := tx.Where("email = ?", identity.Email).First(&existing).Error
		if err == nil {
			return existing, nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return userRecord{}, err
		}
	}

	now := time.Now()
	user := userRecord{
		ID:          userID,
		DisplayName: defaultString(identity.DisplayName, identity.Provider),
		Email:       identity.Email,
		AvatarURL:   identity.AvatarURL,
		Status:      activeStatus,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	return user, tx.Create(&user).Error
}

// Migrate creates or updates this repo's tables. Called once at startup.
func Migrate(db *gorm.DB) error {
	return db.AutoMigrate(&userRecord{}, &authIdentityRecord{})
}
func findUserRecord(tx *gorm.DB, userID string) (userRecord, error) {
	var user userRecord
	err := tx.Where("id = ?", userID).First(&user).Error
	return user, err
}

func validateIdentity(identity domainusers.AuthIdentity) error {
	if identity.Provider == "" {
		return fmt.Errorf("provider is required")
	}
	if identity.ProviderSubject == "" {
		return fmt.Errorf("provider subject is required")
	}
	return nil
}

func candidateUserID(identity domainusers.AuthIdentity) string {
	if identity.UserID != "" {
		return identity.UserID
	}
	return "user_" + stableSuffix(identity.Provider+"_"+identity.ProviderSubject)
}

func candidateIdentityID(identity domainusers.AuthIdentity) string {
	if identity.ID != "" {
		return identity.ID
	}
	return "identity_" + stableSuffix(identity.Provider+"_"+identity.ProviderSubject)
}

func stableSuffix(value string) string {
	sum := sha1.Sum([]byte(value))
	return hex.EncodeToString(sum[:])[:16]
}

func identityKey(provider string, subject string) string {
	return provider + ":" + subject
}

func defaultString(value string, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

func toDomainUser(record userRecord) domainusers.User {
	return domainusers.User{
		ID:          record.ID,
		DisplayName: record.DisplayName,
		Email:       record.Email,
		AvatarURL:   record.AvatarURL,
		Status:      record.Status,
		CreatedAt:   record.CreatedAt,
		UpdatedAt:   record.UpdatedAt,
	}
}

func fromDomainIdentity(identity domainusers.AuthIdentity) authIdentityRecord {
	return authIdentityRecord{
		ID:              identity.ID,
		UserID:          identity.UserID,
		Provider:        identity.Provider,
		ProviderSubject: identity.ProviderSubject,
		Email:           identity.Email,
		DisplayName:     identity.DisplayName,
		AvatarURL:       identity.AvatarURL,
		CreatedAt:       identity.CreatedAt,
		UpdatedAt:       identity.UpdatedAt,
	}
}
