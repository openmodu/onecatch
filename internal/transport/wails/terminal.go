package wailstransport

import terminalservice "github.com/openmodu/oneshot/internal/service/terminal"

type TerminalBinding struct{ service *terminalservice.Service }

func NewTerminalBinding(service *terminalservice.Service) *TerminalBinding {
	return &TerminalBinding{service: service}
}

func (b *TerminalBinding) CreateTerminal(input terminalservice.CreateInput) (terminalservice.Session, error) {
	return b.service.Create(input)
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
