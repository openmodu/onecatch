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

	domainauth "github.com/openmodu/oneshot/internal/domain/auth"
	"github.com/openmodu/oneshot/internal/domain/users"
)

type Repository interface {
	FindOrCreateByIdentity(context.Context, users.AuthIdentity) (users.User, error)
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
	mu       sync.RWMutex
	repo     Repository
	session  *domainauth.Session
	sessions map[string]domainauth.Session
	states   map[string]string
	now      func() time.Time
}

func NewUsecase(repo Repository) *Usecase {
	return &Usecase{
		repo:     repo,
		sessions: make(map[string]domainauth.Session),
		states:   make(map[string]string),
		now:      time.Now,
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

func (s *Usecase) CurrentUserByToken(_ context.Context, token string) (users.User, error) {
	session, err := s.SessionByToken(token)
	if err != nil {
		return users.User{}, err
	}
	return session.User, nil
}

func (s *Usecase) SessionByToken(token string) (domainauth.Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	session, ok := s.sessions[token]
	if !ok || session.ExpiresAt.Before(s.now()) {
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
		Email:           "dev@oneshot.local",
	})
}

func (s *Usecase) LoginWithGoogleCallback(ctx context.Context, input OAuthCallbackInput) (domainauth.Session, error) {
	return s.loginWithOAuth(ctx, "google", input)
}

func (s *Usecase) Logout(context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.session != nil {
		delete(s.sessions, s.session.Token)
	}
	s.session = nil
	return nil
}

func (s *Usecase) LogoutToken(_ context.Context, token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.sessions, token)
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
	if input.State != "" {
		if err := s.consumeState(provider, input.State); err != nil {
			return domainauth.Session{}, err
		}
	}
	if input.Code == "" && input.ProviderSubject == "" {
		if provider == "google" {
			return s.LoginWithGoogle(ctx)
		}
		return s.LoginWithWechat(ctx)
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

	session := domainauth.Session{
		Token:     token,
		Provider:  identity.Provider,
		User:      user,
		ExpiresAt: s.now().Add(30 * 24 * time.Hour),
	}
	s.session = &session
	s.sessions[token] = session
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
		values.Set("client_id", defaultString(os.Getenv("ONESHOT_GOOGLE_CLIENT_ID"), "oneshot-desktop"))
		values.Set("redirect_uri", defaultString(os.Getenv("ONESHOT_GOOGLE_REDIRECT_URI"), "oneshot://oauth/google"))
		values.Set("scope", "openid email profile")
		return "https://accounts.google.com/o/oauth2/v2/auth", values
	case "wechat":
		values.Set("appid", defaultString(os.Getenv("ONESHOT_WECHAT_APP_ID"), "oneshot-desktop"))
		values.Set("redirect_uri", defaultString(os.Getenv("ONESHOT_WECHAT_REDIRECT_URI"), "oneshot://oauth/wechat"))
		values.Set("scope", "snsapi_login")
		return "https://open.weixin.qq.com/connect/qrconnect", values
	default:
		values.Set("client_id", "oneshot-desktop")
		values.Set("redirect_uri", "oneshot://oauth/"+provider)
		return "https://auth.oneshot.local/" + provider + "/authorize", values
	}
}
