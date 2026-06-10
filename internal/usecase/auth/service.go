package auth

import (
	"context"
	"sync"

	domainauth "github.com/openmodu/oneshot/internal/domain/auth"
	"github.com/openmodu/oneshot/internal/domain/users"
)

type Usecase struct {
	mu      sync.RWMutex
	devUser users.User
	session *domainauth.Session
}

func NewUsecase() *Usecase {
	return &Usecase{
		devUser: users.User{
			ID:          users.DevUserID,
			DisplayName: "Local Developer",
			Email:       "dev@oneshot.local",
		},
	}
}

func (s *Usecase) CurrentUser(context.Context) (users.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.session == nil {
		return users.User{}, domainauth.ErrUnauthenticated
	}
	return s.session.User, nil
}

func (s *Usecase) StartWechat(context.Context) (domainauth.Session, error) {
	return s.login("wechat"), nil
}

func (s *Usecase) LoginWithWechat(context.Context) (domainauth.Session, error) {
	return s.login("wechat"), nil
}

func (s *Usecase) LoginWithGoogle(context.Context) (domainauth.Session, error) {
	return s.login("google"), nil
}

func (s *Usecase) Logout(context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.session = nil
	return nil
}

func (s *Usecase) login(provider string) domainauth.Session {
	s.mu.Lock()
	defer s.mu.Unlock()

	session := domainauth.Session{
		Token:    "dev-" + provider + "-token",
		Provider: provider,
		User:     s.devUser,
	}
	s.session = &session
	return session
}
