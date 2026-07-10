package bindings

import (
	"context"

	"github.com/openmodu/oneshot/internal/app/localapp"
	domainworkspaces "github.com/openmodu/oneshot/internal/domain/workspaces"
	"github.com/wailsapp/wails/v3/pkg/application"
)

type WorkspaceBinding struct {
	app               *localapp.App
	applicationSource func() *application.App
}

func NewWorkspaceBinding(app *localapp.App, applicationSource func() *application.App) *WorkspaceBinding {
	return &WorkspaceBinding{app: app, applicationSource: applicationSource}
}

func (b *WorkspaceBinding) ChooseDirectory() (string, error) {
	if b.applicationSource == nil || b.applicationSource() == nil {
		return "", nil
	}
	return b.applicationSource().Dialog.OpenFile().
		CanChooseFiles(false).
		CanChooseDirectories(true).
		CanCreateDirectories(false).
		SetTitle("选择 Agent 工作目录").
		PromptForSingleSelection()
}

func (b *WorkspaceBinding) AddWorkspace(input localapp.AddWorkspaceInput) (domainworkspaces.Workspace, error) {
	return b.app.AddWorkspace(context.Background(), input)
}
func (b *WorkspaceBinding) ListWorkspaces() ([]domainworkspaces.Workspace, error) {
	return b.app.ListWorkspaces(context.Background())
}
func (b *WorkspaceBinding) GetWorkspace(id string) (domainworkspaces.Workspace, error) {
	return b.app.GetWorkspace(context.Background(), id)
}
func (b *WorkspaceBinding) GetWorkspaceStatus(id string) (localapp.WorkspaceStatus, error) {
	return b.app.GetWorkspaceStatus(context.Background(), id)
}
