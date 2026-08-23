package wailstransport

import (
	"context"
	"strings"

	desktopservice "github.com/openmodu/onecatch/internal/service/desktop"
	terminalservice "github.com/openmodu/onecatch/internal/service/terminal"
)

type TerminalCreateInput struct {
	WorkspaceID string   `json:"workspaceId"`
	Shell       string   `json:"shell,omitempty"`
	Arguments   []string `json:"arguments,omitempty"`
	Rows        uint16   `json:"rows,omitempty"`
	Cols        uint16   `json:"cols,omitempty"`
}

type TerminalBinding struct {
	service *terminalservice.Service
	desktop *desktopservice.Service
}

func NewTerminalBinding(service *terminalservice.Service, desktop *desktopservice.Service) *TerminalBinding {
	return &TerminalBinding{service: service, desktop: desktop}
}

func (b *TerminalBinding) CreateTerminal(input TerminalCreateInput) (terminalservice.Session, error) {
	workspace, err := b.desktop.GetWorkspace(context.Background(), strings.TrimSpace(input.WorkspaceID))
	if err != nil {
		return terminalservice.Session{}, err
	}
	return b.service.Create(terminalservice.CreateInput{
		Workspace: workspace.Path,
		Shell:     input.Shell, Arguments: input.Arguments,
		RemoteFS: workspace.RemoteFS,
		Rows:     input.Rows, Cols: input.Cols,
	})
}

func (b *TerminalBinding) WriteTerminal(sessionID, data string) error {
	return b.service.Write(sessionID, data)
}

func (b *TerminalBinding) ResizeTerminal(sessionID string, rows, cols uint16) error {
	return b.service.Resize(sessionID, rows, cols)
}

func (b *TerminalBinding) CloseTerminal(sessionID string) error {
	return b.service.CloseSession(sessionID)
}

func (b *TerminalBinding) ListTerminals() []terminalservice.Session {
	return b.service.List()
}
