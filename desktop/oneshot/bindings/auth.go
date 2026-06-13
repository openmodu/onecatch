package bindings

import (
	"context"
	"errors"

	oneshot "github.com/openmodu/oneshot/clients/oneshot"
)

type AuthBinding struct {
	client oneshot.Client
}

func NewAuthBinding(client oneshot.Client) *AuthBinding {
	return &AuthBinding{client: client}
}

func (b *AuthBinding) CurrentUser() (oneshot.User, error) {
	user, err := b.client.CurrentUser(context.Background())
	if errors.Is(err, oneshot.ErrUnauthenticated) {
		return oneshot.User{}, nil
	}
	return user, err
}

func (b *AuthBinding) LoginWithWechat() (oneshot.Session, error) {
	return b.client.LoginWithWechat(context.Background())
}

func (b *AuthBinding) LoginWithGoogle() (oneshot.Session, error) {
	return b.client.LoginWithGoogle(context.Background())
}

func (b *AuthBinding) Logout() error {
	return b.client.Logout(context.Background())
}
