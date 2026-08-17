package auth

import (
	"context"
	"errors"
	"testing"

	domainauth "github.com/openmodu/onecatch/internal/domain/auth"
	"github.com/openmodu/onecatch/internal/domain/users"
	repousers "github.com/openmodu/onecatch/internal/repo/users"
)

func TestUsecaseSessionLifecycle(t *testing.T) {
	ctx := context.Background()
	usecase := NewUsecaseWithOptions(repousers.NewUsersRepo(nil), Options{})

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
	usecase := NewUsecaseWithOptions(repousers.NewUsersRepo(nil), Options{AllowInsecureCallbacks: true})

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

	// Replaying without a fresh state must fail: state is mandatory for
	// identity-bearing callbacks.
	if _, err := usecase.LoginWithWechatCallback(ctx, OAuthCallbackInput{
		ProviderSubject: "wechat-openid-1",
		DisplayName:     "Wechat User",
	}); !errors.Is(err, domainauth.ErrInvalidOAuthState) {
		t.Fatalf("stateless repeat callback err = %v, want ErrInvalidOAuthState", err)
	}

	restart, err := usecase.StartWechat(ctx)
	if err != nil {
		t.Fatalf("StartWechat() restart error = %v", err)
	}
	repeated, err := usecase.LoginWithWechatCallback(ctx, OAuthCallbackInput{
		State:           restart.State,
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

func TestSessionSurvivesUsecaseRestart(t *testing.T) {
	ctx := context.Background()
	repo := repousers.NewUsersRepo(nil)
	store := newMemorySessionStore()

	first := NewUsecaseWithOptions(repo, Options{Sessions: store})
	session, err := first.LoginWithGoogle(ctx)
	if err != nil {
		t.Fatalf("LoginWithGoogle() error = %v", err)
	}

	// A new usecase over the same store (= process restart with a persistent
	// store) must still resolve the token.
	second := NewUsecaseWithOptions(repo, Options{Sessions: store})
	user, err := second.CurrentUserByToken(ctx, session.Token)
	if err != nil {
		t.Fatalf("CurrentUserByToken() after restart error = %v", err)
	}
	if user.ID != session.User.ID {
		t.Fatalf("user id = %q, want %q", user.ID, session.User.ID)
	}

	if err := second.LogoutToken(ctx, session.Token); err != nil {
		t.Fatalf("LogoutToken() error = %v", err)
	}
	if _, err := second.CurrentUserByToken(ctx, session.Token); !errors.Is(err, domainauth.ErrUnauthenticated) {
		t.Fatalf("token after logout err = %v, want ErrUnauthenticated", err)
	}
}

func TestSecureDefaultRejectsUnverifiedCallback(t *testing.T) {
	ctx := context.Background()
	usecase := NewUsecaseWithOptions(repousers.NewUsersRepo(nil), Options{})

	// Without a verifier and without insecure mode, identity-bearing and
	// empty callbacks must both be rejected.
	start, err := usecase.StartWechat(ctx)
	if err != nil {
		t.Fatalf("StartWechat() error = %v", err)
	}
	if _, err := usecase.LoginWithWechatCallback(ctx, OAuthCallbackInput{
		State:           start.State,
		ProviderSubject: "forged-openid",
	}); !errors.Is(err, domainauth.ErrVerifierRequired) {
		t.Fatalf("unverified callback err = %v, want ErrVerifierRequired", err)
	}
	if _, err := usecase.LoginWithGoogleCallback(ctx, OAuthCallbackInput{}); !errors.Is(err, domainauth.ErrVerifierRequired) {
		t.Fatalf("empty callback err = %v, want ErrVerifierRequired", err)
	}
}

func TestOAuthStateRejectsInvalidCallback(t *testing.T) {
	ctx := context.Background()
	usecase := NewUsecaseWithOptions(repousers.NewUsersRepo(nil), Options{})

	if _, err := usecase.LoginWithGoogleCallback(ctx, OAuthCallbackInput{
		State: "missing",
		Code:  "google-code",
	}); !errors.Is(err, domainauth.ErrInvalidOAuthState) {
		t.Fatalf("LoginWithGoogleCallback() err = %v, want ErrInvalidOAuthState", err)
	}
}
