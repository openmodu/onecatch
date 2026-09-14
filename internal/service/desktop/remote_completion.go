package desktop

import (
	"context"
	"strings"
	"time"

	"github.com/openmodu/onecatch/internal/remotefs"
	"github.com/openmodu/onecatch/internal/sshcredentials"
	"github.com/openmodu/onecatch/internal/sshendpoint"
)

type RemoteDirectoryCompletionInput struct {
	Host        string `json:"host"`
	Username    string `json:"username"`
	Password    string `json:"password,omitempty"`
	Path        string `json:"path"`
	WorkspaceID string `json:"workspaceId,omitempty"`
}

func (a *Service) CompleteRemoteDirectories(ctx context.Context, input RemoteDirectoryCompletionInput) ([]string, error) {
	endpoint, err := sshendpoint.Parse(strings.TrimSpace(input.Host))
	if err != nil {
		return nil, coded("remote_fs_invalid", err.Error())
	}
	username := strings.TrimSpace(input.Username)
	if strings.ContainsAny(username, "\x00\r\n") || strings.IndexByte(input.Path, 0) >= 0 {
		return nil, coded("remote_fs_invalid", "invalid SSH username or path")
	}
	config := remotefs.SFTPConfig{Host: endpoint.String(), Username: username}
	if input.WorkspaceID != "" {
		workspace, err := a.GetWorkspace(ctx, input.WorkspaceID)
		if err != nil {
			return nil, err
		}
		if remote := workspace.RemoteFS; remote != nil && remote.Host == config.Host && remote.Username == username {
			config.CredentialID = remote.CredentialID
			config.SSHOptions = remote.SSHOptions
		}
	}
	if input.Password != "" {
		if username == "" || len(input.Password) > 2048 || strings.ContainsAny(input.Password, "\x00\r\n") {
			return nil, coded("remote_fs_credentials_invalid", "valid SSH username and password are required")
		}
		credentials := a.remoteCredentials
		if credentials == nil {
			credentials = sshcredentials.KeyringStore{}
		}
		id, err := sshcredentials.NewID()
		if err != nil {
			return nil, coded("remote_fs_credentials_unavailable", err.Error())
		}
		if err := credentials.Set(id, input.Password); err != nil {
			return nil, coded("remote_fs_credentials_unavailable", err.Error())
		}
		defer credentials.Delete(id)
		config.CredentialID = id
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	results, err := remotefs.CompleteDirectories(ctx, config, input.Path)
	if err != nil {
		return nil, coded("remote_fs_unavailable", err.Error())
	}
	return results, nil
}
