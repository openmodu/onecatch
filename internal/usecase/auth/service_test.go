package auth

import (
	"context"
	"errors"
	"testing"

	domainauth "github.com/openmodu/oneshot/internal/domain/auth"
	"github.com/openmodu/oneshot/internal/domain/users"
	repousers "github.com/openmodu/oneshot/internal/repo/users"
)

func TestUsecaseSessionLifecycle(t *testing.T) {
	ctx := context.Background()
	usecase := NewUsecase(repousers.NewUsersRepo(nil))

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
	usecase := NewUsecase(repousers.NewUsersRepo(nil))

	start, err := usecase.StartWechat(ctx)
	if err != nil {
		t.Fatalf("StartWechat() error = %v", err)
	}
	if start.Provider != "wechat" || start.State == "" || start.AuthURL == "" {
		t.Fatalf("unexpected oauth start = %+v", start)
	}

	session, err := usecase.LoginWithWechatCallback(ctx, OAuthCallbackInput{
		State:           start.State,
		Code:            "wechat-code",
		ProviderSubject: "wechat-openid-1",
		DisplayName:     "Wechat User",
	})
	if err != nil {
		t.Fatalf("LoginWithWechatCallback() error = %v", err)
	}
	if session.Provider != "wechat" {
		t.Fatalf("provider = %q, want wechat", session.Provider)
	}
	if session.User.ID == "" || session.User.ID == users.DevUserID {
		t.Fatalf("user id = %q, want provider-derived user", session.User.ID)
	}

	repeated, err := usecase.LoginWithWechatCallback(ctx, OAuthCallbackInput{
		ProviderSubject: "wechat-openid-1",
		DisplayName:     "Wechat User",
	})
	if err != nil {
		t.Fatalf("LoginWithWechatCallback() repeat error = %v", err)
	}
	if repeated.User.ID != session.User.ID {
		t.Fatalf("repeat login user id = %q, want %q", repeated.User.ID, session.User.ID)
	}
}

func TestOAuthStateRejectsInvalidCallback(t *testing.T) {
	ctx := context.Background()
	usecase := NewUsecase(repousers.NewUsersRepo(nil))

	if _, err := usecase.LoginWithGoogleCallback(ctx, OAuthCallbackInput{
		State: "missing",
		Code:  "google-code",
	}); !errors.Is(err, domainauth.ErrInvalidOAuthState) {
		t.Fatalf("LoginWithGoogleCallback() err = %v, want ErrInvalidOAuthState", err)
	}
}
