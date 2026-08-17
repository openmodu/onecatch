package wailstransport

import (
	"context"

	domainworkspaces "github.com/openmodu/onecatch/internal/domain/workspaces"
	desktopservice "github.com/openmodu/onecatch/internal/service/desktop"
	"github.com/wailsapp/wails/v3/pkg/application"
)

type WorkspaceBinding struct {
	service           *desktopservice.Service
	applicationSource func() *application.App
}

func NewWorkspaceBinding(service *desktopservice.Service, applicationSource func() *application.App) *WorkspaceBinding {
	return &WorkspaceBinding{service: service, applicationSource: applicationSource}
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

func (b *WorkspaceBinding) ChooseAttachments() ([]string, error) {
	if b.applicationSource == nil || b.applicationSource() == nil {
		return []string{}, nil
	}
	return b.applicationSource().Dialog.OpenFile().
		CanChooseFiles(true).
		CanChooseDirectories(false).
		CanCreateDirectories(false).
		SetTitle("选择任务附件").
		PromptForMultipleSelection()
}

func (b *WorkspaceBinding) AddWorkspace(input desktopservice.AddWorkspaceInput) (domainworkspaces.Workspace, error) {
	return b.service.AddWorkspace(context.Background(), input)
}
func (b *WorkspaceBinding) ListWorkspaces() ([]domainworkspaces.Workspace, error) {
	return b.service.ListWorkspaces(context.Background())
}
func (b *WorkspaceBinding) OpenWorkspace(id string) (domainworkspaces.Workspace, error) {
	return b.service.OpenWorkspace(context.Background(), id)
}
func (b *WorkspaceBinding) SetWorkspacePinned(id string, pinned bool) (domainworkspaces.Workspace, error) {
	return b.service.SetWorkspacePinned(context.Background(), id, pinned)
}
func (b *WorkspaceBinding) RemoveWorkspace(id string) error {
	return b.service.RemoveWorkspace(context.Background(), id)
}
func (b *WorkspaceBinding) GetWorkspace(id string) (domainworkspaces.Workspace, error) {
	return b.service.GetWorkspace(context.Background(), id)
}
func (b *WorkspaceBinding) GetWorkspaceStatus(id string) (desktopservice.WorkspaceStatus, error) {
	return b.service.GetWorkspaceStatus(context.Background(), id)
}
func (b *WorkspaceBinding) ListWorkspaceFiles(workspaceID, directory string) ([]desktopservice.WorkspaceFileEntry, error) {
	return b.service.ListWorkspaceFiles(context.Background(), workspaceID, directory)
}
func (b *WorkspaceBinding) ReadWorkspaceFile(workspaceID, path string) (desktopservice.WorkspaceFileDocument, error) {
	return b.service.ReadWorkspaceFile(context.Background(), workspaceID, path)
}
func (b *WorkspaceBinding) WriteWorkspaceFile(input desktopservice.WriteWorkspaceFileInput) (desktopservice.WorkspaceFileDocument, error) {
	return b.service.WriteWorkspaceFile(context.Background(), input)
}
