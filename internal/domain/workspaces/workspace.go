package workspaces

import (
	"errors"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/openmodu/onecatch/internal/sshendpoint"
)

var (
	ErrNotFound = errors.New("workspace not found")
	ErrInvalid  = errors.New("workspace is invalid")
)

type Workspace struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Path           string    `json:"path"`
	RemoteFS       *RemoteFS `json:"remoteFs,omitempty"`
	DefaultSandbox string    `json:"defaultSandbox"`
	Pinned         bool      `json:"pinned,omitempty"`
	Hidden         bool      `json:"hidden,omitempty"`
	CreatedAt      time.Time `json:"createdAt"`
	LastOpenedAt   time.Time `json:"lastOpenedAt"`
}

// RemoteFS identifies a directory reached through the user's OpenSSH
// configuration. The local Agent keeps running on this machine; only its
// command and filesystem operations are redirected to this target.
type RemoteFS struct {
	Host         string   `json:"host"`
	Root         string   `json:"root"`
	Username     string   `json:"username,omitempty"`
	CredentialID string   `json:"credentialId,omitempty"`
	SSHOptions   []string `json:"sshOptions,omitempty"`
}

type GitSnapshot struct {
	IsRepo     bool      `json:"isRepo"`
	Head       string    `json:"head,omitempty"`
	Branch     string    `json:"branch,omitempty"`
	Ahead      int       `json:"ahead,omitempty"`
	Behind     int       `json:"behind,omitempty"`
	Status     string    `json:"status,omitempty"`
	DiffStat   string    `json:"diffStat,omitempty"`
	StagedStat string    `json:"stagedStat,omitempty"`
	Files      []GitFile `json:"files,omitempty"`
}

type GitFile struct {
	Path     string `json:"path"`
	Index    string `json:"index"`
	Worktree string `json:"worktree"`
}

type GitBranch struct {
	Name     string `json:"name"`
	Current  bool   `json:"current"`
	Upstream string `json:"upstream,omitempty"`
}

func Validate(workspace Workspace) error {
	if strings.TrimSpace(workspace.ID) == "" || strings.TrimSpace(workspace.Name) == "" {
		return ErrInvalid
	}
	if workspace.RemoteFS == nil {
		if !filepath.IsAbs(workspace.Path) {
			return ErrInvalid
		}
	} else {
		if _, err := sshendpoint.Parse(workspace.RemoteFS.Host); err != nil || !path.IsAbs(strings.TrimSpace(workspace.RemoteFS.Root)) {
			return ErrInvalid
		}
		if workspace.RemoteFS.CredentialID != "" && strings.TrimSpace(workspace.RemoteFS.Username) == "" {
			return ErrInvalid
		}
		if !path.IsAbs(filepath.ToSlash(workspace.Path)) || path.Clean(workspace.RemoteFS.Root) != path.Clean(filepath.ToSlash(workspace.Path)) {
			return ErrInvalid
		}
	}
	switch workspace.DefaultSandbox {
	case "", "read-only", "workspace-write", "full":
		return nil
	default:
		return ErrInvalid
	}
}
