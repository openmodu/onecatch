package wailstransport

import (
	"context"

	domainworkflows "github.com/openmodu/onecatch/internal/domain/workflows"
	desktopservice "github.com/openmodu/onecatch/internal/service/desktop"
)

type WorkflowBinding struct{ service *desktopservice.Service }

func NewWorkflowBinding(service *desktopservice.Service) *WorkflowBinding {
	return &WorkflowBinding{service: service}
}

func (b *WorkflowBinding) ValidateDefinition(input domainworkflows.Definition) []domainworkflows.ValidationIssue {
	return b.service.ValidateDefinition(input)
}
func (b *WorkflowBinding) CreateDefinition(input domainworkflows.Definition) (domainworkflows.Definition, error) {
	return b.service.CreateDefinition(context.Background(), input)
}
func (b *WorkflowBinding) UpdateDefinition(id string, input domainworkflows.Definition) (domainworkflows.Definition, error) {
	return b.service.UpdateDefinition(context.Background(), id, input)
}
func (b *WorkflowBinding) DeleteDefinition(id string) error {
	return b.service.DeleteDefinition(context.Background(), id)
}
func (b *WorkflowBinding) ListDefinitions() ([]domainworkflows.Definition, error) {
	return b.service.ListDefinitions(context.Background())
}
func (b *WorkflowBinding) GetDefinition(id string) (domainworkflows.Definition, error) {
	return b.service.GetDefinition(context.Background(), id)
}
