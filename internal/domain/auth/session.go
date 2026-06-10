package auth

import "github.com/openmodu/oneshot/internal/domain/users"

type Session struct {
	Token string     `json:"token"`
	User  users.User `json:"user"`
}
