package auth

import (
	"errors"
	"time"

	"github.com/openmodu/onecatch/internal/domain/users"
)

var ErrUnauthenticated = errors.New("unauthenticated")
var ErrInvalidOAuthState = errors.New("invalid oauth state")
var ErrVerifierRequired = errors.New("oauth identity verification is required")

type Session struct {
	Token     string     `json:"token"`
	Provider  string     `json:"provider"`
	User      users.User `json:"user"`
	ExpiresAt time.Time  `json:"expiresAt,omitempty"`
}

type OAuthStart struct {
	Provider string `json:"provider"`
	AuthURL  string `json:"authUrl"`
	State    string `json:"state"`
}
