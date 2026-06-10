package auth

import (
	"context"
	"sync"

	domainauth "github.com/openmodu/oneshot/internal/domain/auth"
	"github.com/openmodu/oneshot/internal/domain/users"
)

type Repository interface {
	FindOrCreateByIdentity(context.Context, users.AuthIdentity) (users.User, error)
}

type Usecase struct {
	mu      sync.RWMutex
	repo    Repository
	session *domainauth.Session
}

func NewUsecase(repo Repository) *Usecase {
	return &Usecase{repo: repo}
}

func (s *Usecase) CurrentUser(context.Context) (users.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.session == nil {
		return users.User{}, domainauth.ErrUnauthenticated
	}
	return s.session.User, nil
}

func (s *Usecase) StartWechat(ctx context.Context) (domainauth.Session, error) {
	return s.login(ctx, devIdentity("wechat"))
}

func (s *Usecase) LoginWithWechat(ctx context.Context) (domainauth.Session, error) {
	return s.login(ctx, devIdentity("wechat"))
}

func (s *Usecase) LoginWithGoogle(ctx context.Context) (domainauth.Session, error) {
	return s.login(ctx, users.AuthIdentity{
		UserID:          users.DevUserID,
		Provider:        "google",
		ProviderSubject: "local-dev",
		DisplayName:     "Local Developer",
		Email:           "dev@oneshot.local",
	})
}

func (s *Usecase) Logout(context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.session = nil
	return nil
}

func (s *Usecase) login(ctx context.Context, identity users.AuthIdentity) (domainauth.Session, error) {
	user, err := s.repo.FindOrCreateByIdentity(ctx, identity)
	if err != nil {
		return domainauth.Session{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	session := domainauth.Session{
		Token:    "dev-" + identity.Provider + "-token",
		Provider: identity.Provider,
		User:     user,
	}
	s.session = &session
	return session, nil
}

func devIdentity(provider string) users.AuthIdentity {
	return users.AuthIdentity{
		UserID:          users.DevUserID,
		Provider:        provider,
		ProviderSubject: "local-dev",
		DisplayName:     "Local Developer",
	}
}
