package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"sync"
	"time"

	domainauth "github.com/openmodu/onecatch/internal/domain/auth"
	"github.com/openmodu/onecatch/internal/domain/users"
)

type Repository interface {
	FindOrCreateByIdentity(context.Context, users.AuthIdentity) (users.User, error)
}

// IdentityVerifier exchanges an OAuth authorization code with the provider
// and returns the verified identity. Callbacks that carry a code MUST go
// through a verifier unless insecure callbacks are explicitly enabled.
type IdentityVerifier interface {
	Verify(ctx context.Context, provider string, code string) (users.AuthIdentity, error)
}

// SessionStore persists issued sessions so logins survive server restarts.
// Lookups receive the raw bearer token; implementations are expected to hash
// it before storage.
type SessionStore interface {
	SaveSession(context.Context, domainauth.Session) error
	FindSessionByToken(ctx context.Context, token string) (domainauth.Session, error)
	DeleteSessionByToken(ctx context.Context, token string) error
	DeleteExpiredSessions(ctx context.Context, now time.Time) error
}

type Options struct {
	// Verifier validates authorization codes against the real provider.
	Verifier IdentityVerifier
	// Sessions persists issued sessions. Defaults to an in-process store.
	Sessions SessionStore
	// AllowInsecureCallbacks lets callbacks be trusted without provider-side
	// verification. For local development and tests only.
	AllowInsecureCallbacks bool
}

type OAuthCallbackInput struct {
	State           string `json:"state"`
	Code            string `json:"code"`
	ProviderSubject string `json:"providerSubject"`
	Email           string `json:"email"`
	DisplayName     string `json:"displayName"`
	AvatarURL       string `json:"avatarUrl"`
}

type Usecase struct {
	mu            sync.RWMutex
	repo          Repository
	session       *domainauth.Session
	store         SessionStore
	states        map[string]string
	verifier      IdentityVerifier
	allowInsecure bool
	now           func() time.Time
}

// NewUsecase builds an auth usecase with secure defaults: a verifier is
// configured from environment credentials when present, and insecure
// callbacks require ONECATCH_AUTH_INSECURE_CALLBACKS=1.
func NewUsecase(repo Repository, sessions SessionStore) *Usecase {
	return NewUsecaseWithOptions(repo, Options{
		Verifier:               NewHTTPVerifierFromEnv(),
		Sessions:               sessions,
		AllowInsecureCallbacks: os.Getenv("ONECATCH_AUTH_INSECURE_CALLBACKS") == "1",
	})
}

func NewUsecaseWithOptions(repo Repository, opts Options) *Usecase {
	store := opts.Sessions
	if store == nil {
		store = newMemorySessionStore()
	}
	return &Usecase{
		repo:          repo,
		store:         store,
		states:        make(map[string]string),
		verifier:      opts.Verifier,
		allowInsecure: opts.AllowInsecureCallbacks,
		now:           time.Now,
	}
}

func (s *Usecase) CurrentUser(context.Context) (users.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.session == nil || s.session.ExpiresAt.Before(s.now()) {
		return users.User{}, domainauth.ErrUnauthenticated
	}
	return s.session.User, nil
}

func (s *Usecase) CurrentUserByToken(ctx context.Context, token string) (users.User, error) {
	session, err := s.SessionByToken(ctx, token)
	if err != nil {
		return users.User{}, err
	}
	return session.User, nil
}

func (s *Usecase) SessionByToken(ctx context.Context, token string) (domainauth.Session, error) {
	session, err := s.store.FindSessionByToken(ctx, token)
	if err != nil {
		return domainauth.Session{}, err
	}
	if session.ExpiresAt.Before(s.now()) {
		return domainauth.Session{}, domainauth.ErrUnauthenticated
	}
	return session, nil
}

func (s *Usecase) StartWechat(context.Context) (domainauth.OAuthStart, error) {
	return s.startOAuth("wechat")
}

func (s *Usecase) LoginWithWechat(ctx context.Context) (domainauth.Session, error) {
	return s.login(ctx, devIdentity("wechat"))
}

func (s *Usecase) LoginWithWechatCallback(ctx context.Context, input OAuthCallbackInput) (domainauth.Session, error) {
	return s.loginWithOAuth(ctx, "wechat", input)
}

func (s *Usecase) LoginWithGoogle(ctx context.Context) (domainauth.Session, error) {
	return s.login(ctx, users.AuthIdentity{
		UserID:          users.DevUserID,
		Provider:        "google",
		ProviderSubject: "local-dev",
		DisplayName:     "Local Developer",
		Email:           "dev@onecatch.local",
	})
}

func (s *Usecase) LoginWithGoogleCallback(ctx context.Context, input OAuthCallbackInput) (domainauth.Session, error) {
	return s.loginWithOAuth(ctx, "google", input)
}

func (s *Usecase) Logout(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.session != nil {
		if err := s.store.DeleteSessionByToken(ctx, s.session.Token); err != nil {
			return err
		}
	}
	s.session = nil
	return nil
}

func (s *Usecase) LogoutToken(ctx context.Context, token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.store.DeleteSessionByToken(ctx, token); err != nil {
		return err
	}
	if s.session != nil && s.session.Token == token {
		s.session = nil
	}
	return nil
}

func (s *Usecase) startOAuth(provider string) (domainauth.OAuthStart, error) {
	state, err := randomToken(24)
	if err != nil {
		return domainauth.OAuthStart{}, err
	}
	s.mu.Lock()
	s.states[state] = provider
	s.mu.Unlock()

	endpoint, values := oauthEndpoint(provider)
	values.Set("state", state)
	return domainauth.OAuthStart{
		Provider: provider,
		AuthURL:  endpoint + "?" + values.Encode(),
		State:    state,
	}, nil
}

func (s *Usecase) loginWithOAuth(ctx context.Context, provider string, input OAuthCallbackInput) (domainauth.Session, error) {
	if input.Code == "" && input.ProviderSubject == "" {
		// Empty callback: development-only shortcut login.
		if !s.allowInsecure {
			return domainauth.Session{}, domainauth.ErrVerifierRequired
		}
		if provider == "google" {
			return s.LoginWithGoogle(ctx)
		}
		return s.LoginWithWechat(ctx)
	}

	// State is mandatory for identity-bearing callbacks (CSRF protection).
	if err := s.consumeState(provider, input.State); err != nil {
		return domainauth.Session{}, err
	}

	if s.verifier != nil && input.Code != "" {
		identity, err := s.verifier.Verify(ctx, provider, input.Code)
		if err != nil {
			return domainauth.Session{}, err
		}
		return s.login(ctx, identity)
	}

	// No verifier: never trust client-supplied identity unless explicitly
	// running in insecure development mode.
	if !s.allowInsecure {
		return domainauth.Session{}, domainauth.ErrVerifierRequired
	}

	subject := input.ProviderSubject
	if subject == "" {
		subject = "oauth_" + stableSuffix(provider+"_"+input.Code)
	}
	return s.login(ctx, users.AuthIdentity{
		Provider:        provider,
		ProviderSubject: subject,
		DisplayName:     defaultString(input.DisplayName, provider+" user"),
		Email:           input.Email,
		AvatarURL:       input.AvatarURL,
	})
}

func (s *Usecase) consumeState(provider string, state string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	actual, ok := s.states[state]
	if !ok || actual != provider {
		return domainauth.ErrInvalidOAuthState
	}
	delete(s.states, state)
	return nil
}

func (s *Usecase) login(ctx context.Context, identity users.AuthIdentity) (domainauth.Session, error) {
	user, err := s.repo.FindOrCreateByIdentity(ctx, identity)
	if err != nil {
		return domainauth.Session{}, err
	}
	token, err := randomToken(32)
	if err != nil {
		return domainauth.Session{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()
	_ = s.store.DeleteExpiredSessions(ctx, now)

	session := domainauth.Session{
		Token:     token,
		Provider:  identity.Provider,
		User:      user,
		ExpiresAt: now.Add(30 * 24 * time.Hour),
	}
	if err := s.store.SaveSession(ctx, session); err != nil {
		return domainauth.Session{}, err
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

func randomToken(size int) (string, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

func stableSuffix(value string) string {
	sum := sha1.Sum([]byte(value))
	return hex.EncodeToString(sum[:])[:16]
}

func defaultString(value string, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

func oauthEndpoint(provider string) (string, url.Values) {
	values := url.Values{}
	values.Set("response_type", "code")
	switch provider {
	case "google":
		values.Set("client_id", defaultString(os.Getenv("ONECATCH_GOOGLE_CLIENT_ID"), "onecatch-desktop"))
		values.Set("redirect_uri", defaultString(os.Getenv("ONECATCH_GOOGLE_REDIRECT_URI"), "onecatch://oauth/google"))
		values.Set("scope", "openid email profile")
		return "https://accounts.google.com/o/oauth2/v2/auth", values
	case "wechat":
		values.Set("appid", defaultString(os.Getenv("ONECATCH_WECHAT_APP_ID"), "onecatch-desktop"))
		values.Set("redirect_uri", defaultString(os.Getenv("ONECATCH_WECHAT_REDIRECT_URI"), "onecatch://oauth/wechat"))
		values.Set("scope", "snsapi_login")
		return "https://open.weixin.qq.com/connect/qrconnect", values
	default:
		values.Set("client_id", "onecatch-desktop")
		values.Set("redirect_uri", "onecatch://oauth/"+provider)
		return "https://auth.onecatch.local/" + provider + "/authorize", values
	}
}
