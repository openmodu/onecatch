package wailstransport

import (
	"context"

	lspservice "github.com/openmodu/onecatch/internal/service/lsp"
)

type LSPBinding struct {
	service *lspservice.Service
}

func NewLSPBinding(service *lspservice.Service) *LSPBinding {
	return &LSPBinding{service: service}
}

func (b *LSPBinding) Detect(input lspservice.DetectInput) (lspservice.Capability, error) {
	return b.service.Detect(context.Background(), input)
}

func (b *LSPBinding) SyncDocument(input lspservice.SyncDocumentInput) error {
	return b.service.SyncDocument(context.Background(), input)
}

func (b *LSPBinding) Definition(input lspservice.DefinitionInput) ([]lspservice.Location, error) {
	return b.service.Definition(context.Background(), input)
}

func (b *LSPBinding) CloseDocument(input lspservice.CloseDocumentInput) error {
	return b.service.CloseDocument(context.Background(), input)
}

func (b *LSPBinding) CloseWorkspace(input lspservice.CloseWorkspaceInput) error {
	return b.service.CloseWorkspace(context.Background(), input)
}
