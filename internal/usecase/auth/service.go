package auth

import (
	"context"

	domainauth "github.com/openmodu/oneshot/internal/domain/auth"
	"github.com/openmodu/oneshot/internal/domain/users"
)

type Usecase struct {
	devUser users.User
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
	return s.devUser, nil
}

func (s *Usecase) LoginWithGoogle(context.Context) (domainauth.Session, error) {
	return domainauth.Session{Token: "dev-token", User: s.devUser}, nil
}

func (s *Usecase) Logout(context.Context) error {
	return nil
}
