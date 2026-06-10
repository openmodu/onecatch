package auth

import (
	"context"
	"errors"
	"testing"

	domainauth "github.com/openmodu/oneshot/internal/domain/auth"
	"github.com/openmodu/oneshot/internal/domain/users"
)

func TestUsecaseSessionLifecycle(t *testing.T) {
	ctx := context.Background()
	usecase := NewUsecase()

	if _, err := usecase.CurrentUser(ctx); !errors.Is(err, domainauth.ErrUnauthenticated) {
		t.Fatalf("CurrentUser() err = %v, want ErrUnauthenticated", err)
	}

	session, err := usecase.LoginWithGoogle(ctx)
	if err != nil {
		t.Fatalf("LoginWithGoogle() error = %v", err)
	}
	if session.Provider != "google" {
		t.Fatalf("provider = %q, want google", session.Provider)
	}
	if session.Token == "" {
		t.Fatal("token is empty")
	}
	if session.User.ID != users.DevUserID {
		t.Fatalf("user id = %q, want %q", session.User.ID, users.DevUserID)
	}

	user, err := usecase.CurrentUser(ctx)
	if err != nil {
		t.Fatalf("CurrentUser() after login error = %v", err)
	}
	if user.ID != users.DevUserID {
		t.Fatalf("current user id = %q, want %q", user.ID, users.DevUserID)
	}

	if err := usecase.Logout(ctx); err != nil {
		t.Fatalf("Logout() error = %v", err)
	}
	if _, err := usecase.CurrentUser(ctx); !errors.Is(err, domainauth.ErrUnauthenticated) {
		t.Fatalf("CurrentUser() after logout err = %v, want ErrUnauthenticated", err)
	}
}

func TestUsecaseWechatLogin(t *testing.T) {
	ctx := context.Background()
	usecase := NewUsecase()

	session, err := usecase.StartWechat(ctx)
	if err != nil {
		t.Fatalf("StartWechat() error = %v", err)
	}
	if session.Provider != "wechat" {
		t.Fatalf("provider = %q, want wechat", session.Provider)
	}

	if err := usecase.Logout(ctx); err != nil {
		t.Fatalf("Logout() error = %v", err)
	}

	session, err = usecase.LoginWithWechat(ctx)
	if err != nil {
		t.Fatalf("LoginWithWechat() error = %v", err)
	}
	if session.Provider != "wechat" {
		t.Fatalf("provider = %q, want wechat", session.Provider)
	}
}
