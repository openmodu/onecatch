package bindings

import (
	"context"

	oneshot "github.com/openmodu/oneshot/clients/oneshot"
)

type AuthBinding struct {
	client oneshot.Client
}

func NewAuthBinding(client oneshot.Client) *AuthBinding {
	return &AuthBinding{client: client}
}

func (b *AuthBinding) CurrentUser() (oneshot.User, error) {
	return b.client.CurrentUser(context.Background())
}

func (b *AuthBinding) LoginWithGoogle() (oneshot.Session, error) {
	return b.client.LoginWithGoogle(context.Background())
}

func (b *AuthBinding) Logout() error {
	return b.client.Logout(context.Background())
}
