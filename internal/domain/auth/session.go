package auth

import (
	"errors"

	"github.com/openmodu/oneshot/internal/domain/users"
)

var ErrUnauthenticated = errors.New("unauthenticated")

type Session struct {
	Token    string     `json:"token"`
	Provider string     `json:"provider"`
	User     users.User `json:"user"`
}
