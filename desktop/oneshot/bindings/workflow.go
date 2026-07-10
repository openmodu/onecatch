package bindings

import (
	"context"

	"github.com/openmodu/oneshot/internal/app/localapp"
	domainworkflows "github.com/openmodu/oneshot/internal/domain/workflows"
)

type WorkflowBinding struct{ app *localapp.App }

func NewWorkflowBinding(app *localapp.App) *WorkflowBinding { return &WorkflowBinding{app: app} }

func (b *WorkflowBinding) ValidateDefinition(input domainworkflows.Definition) []domainworkflows.ValidationIssue {
	return b.app.ValidateDefinition(input)
}
func (b *WorkflowBinding) CreateDefinition(input domainworkflows.Definition) (domainworkflows.Definition, error) {
	return b.app.SaveDefinition(context.Background(), input)
}
func (b *WorkflowBinding) UpdateDefinition(id string, input domainworkflows.Definition) (domainworkflows.Definition, error) {
	input.ID = id
	return b.app.SaveDefinition(context.Background(), input)
}
func (b *WorkflowBinding) ListDefinitions() ([]domainworkflows.Definition, error) {
	return b.app.ListDefinitions(context.Background())
}
func (b *WorkflowBinding) GetDefinition(id string) (domainworkflows.Definition, error) {
	return b.app.GetDefinition(context.Background(), id)
}
