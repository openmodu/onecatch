package sessions

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	domainauth "github.com/openmodu/onecatch/internal/domain/auth"
	domainusers "github.com/openmodu/onecatch/internal/domain/users"
	pkgsql "github.com/openmodu/onecatch/pkg/sql"
	"gorm.io/gorm"
)

type SessionsRepo interface {
	SaveSession(context.Context, domainauth.Session) error
	// FindSessionByToken returns domainauth.ErrUnauthenticated when the token
	// is unknown. Expiry is checked by the caller.
	FindSessionByToken(ctx context.Context, token string) (domainauth.Session, error)
	DeleteSessionByToken(ctx context.Context, token string) error
	DeleteExpiredSessions(ctx context.Context, now time.Time) error
}

type sessionsImpl struct {
	sql *pkgsql.Sql
	mu  sync.RWMutex

	sessions map[string]domainauth.Session
}

// sessionRecord stores a hash of the bearer token, never the token itself,
// plus a snapshot of the user so lookups need no join.
type sessionRecord struct {
	TokenHash   string    `gorm:"primaryKey;size:64"`
	Provider    string    `gorm:"size:32;not null"`
	UserID      string    `gorm:"size:64;not null;index"`
	DisplayName string    `gorm:"size:255"`
	Email       string    `gorm:"size:255"`
	AvatarURL   string    `gorm:"size:512"`
	ExpiresAt   time.Time `gorm:"index"`
	CreatedAt   time.Time
}

func (sessionRecord) TableName() string {
	return "auth_sessions"
}

func NewSessionsRepo(sql *pkgsql.Sql) SessionsRepo {
	return &sessionsImpl{
		sql:      sql,
		sessions: make(map[string]domainauth.Session),
	}
}

func (r *sessionsImpl) SaveSession(ctx context.Context, session domainauth.Session) error {
	if r.sql != nil && r.sql.Gorm() != nil {
		return r.sql.Gorm().WithContext(ctx).Save(&sessionRecord{
			TokenHash:   hashToken(session.Token),
			Provider:    session.Provider,
			UserID:      session.User.ID,
			DisplayName: session.User.DisplayName,
			Email:       session.User.Email,
			AvatarURL:   session.User.AvatarURL,
			ExpiresAt:   session.ExpiresAt,
			CreatedAt:   time.Now(),
		}).Error
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.sessions[hashToken(session.Token)] = session
	return nil
}

func (r *sessionsImpl) FindSessionByToken(ctx context.Context, token string) (domainauth.Session, error) {
	key := hashToken(token)
	if r.sql != nil && r.sql.Gorm() != nil {
		var record sessionRecord
		err := r.sql.Gorm().WithContext(ctx).Where("token_hash = ?", key).First(&record).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domainauth.Session{}, domainauth.ErrUnauthenticated
		}
		if err != nil {
			return domainauth.Session{}, err
		}
		return domainauth.Session{
			Token:    token,
			Provider: record.Provider,
			User: domainusers.User{
				ID:          record.UserID,
				DisplayName: record.DisplayName,
				Email:       record.Email,
				AvatarURL:   record.AvatarURL,
			},
			ExpiresAt: record.ExpiresAt,
		}, nil
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	session, ok := r.sessions[key]
	if !ok {
		return domainauth.Session{}, domainauth.ErrUnauthenticated
	}
	return session, nil
}

func (r *sessionsImpl) DeleteSessionByToken(ctx context.Context, token string) error {
	key := hashToken(token)
	if r.sql != nil && r.sql.Gorm() != nil {
		return r.sql.Gorm().WithContext(ctx).Where("token_hash = ?", key).Delete(&sessionRecord{}).Error
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.sessions, key)
	return nil
}

func (r *sessionsImpl) DeleteExpiredSessions(ctx context.Context, now time.Time) error {
	if r.sql != nil && r.sql.Gorm() != nil {
		return r.sql.Gorm().WithContext(ctx).Where("expires_at < ?", now).Delete(&sessionRecord{}).Error
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for key, session := range r.sessions {
		if session.ExpiresAt.Before(now) {
			delete(r.sessions, key)
		}
	}
	return nil
}

// Migrate creates or updates this repo's tables. Called once at startup.
func Migrate(db *gorm.DB) error {
	return db.AutoMigrate(&sessionRecord{})
}
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
