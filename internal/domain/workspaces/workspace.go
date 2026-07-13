package workspaces

import (
	"errors"
	"path/filepath"
	"strings"
	"time"
)

var (
	ErrNotFound = errors.New("workspace not found")
	ErrInvalid  = errors.New("workspace is invalid")
)

type Workspace struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Path           string    `json:"path"`
	DefaultSandbox string    `json:"defaultSandbox"`
	Pinned         bool      `json:"pinned,omitempty"`
	Hidden         bool      `json:"hidden,omitempty"`
	CreatedAt      time.Time `json:"createdAt"`
	LastOpenedAt   time.Time `json:"lastOpenedAt"`
}

type GitSnapshot struct {
	IsRepo   bool   `json:"isRepo"`
	Head     string `json:"head,omitempty"`
	Status   string `json:"status,omitempty"`
	DiffStat string `json:"diffStat,omitempty"`
}

func Validate(workspace Workspace) error {
	if strings.TrimSpace(workspace.ID) == "" || strings.TrimSpace(workspace.Name) == "" || !filepath.IsAbs(workspace.Path) {
		return ErrInvalid
	}
	switch workspace.DefaultSandbox {
	case "", "read-only", "workspace-write", "full":
		return nil
	default:
		return ErrInvalid
	}
}
