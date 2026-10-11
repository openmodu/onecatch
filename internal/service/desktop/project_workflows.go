package desktop

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	domaintasks "github.com/openmodu/onecatch/internal/domain/tasks"
	domainworkflows "github.com/openmodu/onecatch/internal/domain/workflows"
	repoworkflows "github.com/openmodu/onecatch/internal/repo/workflows"
	"github.com/openmodu/onecatch/pkg/localfile"
)

// ProjectWorkflow is a workflow file kept in a project's repository. A file
// that cannot be used carries Error and ErrorLine and an empty Definition.
type ProjectWorkflow struct {
	Definition domainworkflows.Definition `json:"definition"`
	Path       string                     `json:"path"`
	File       string                     `json:"file"`
	Error      string                     `json:"error,omitempty"`
	ErrorLine  int                        `json:"errorLine,omitempty"`
	Revision   string                     `json:"revision"`
}

// ListProjectWorkflows reads the workflow files of a project. Remote FS
// projects have none: their files are not on this machine.
func (a *Service) ListProjectWorkflows(ctx context.Context, workspaceID string) ([]ProjectWorkflow, error) {
	workspace, err := a.store.Repos.Tasks.GetWorkspace(ctx, strings.TrimSpace(workspaceID))
	if err != nil {
		return nil, coded("workspace_not_found", "workspace was not found")
	}
	if workspace.RemoteFS != nil {
		return nil, nil
	}
	items, err := repoworkflows.ListProjectWorkflows(workspace.Path)
	if err != nil {
		return nil, err
	}
	out := make([]ProjectWorkflow, 0, len(items))
	for _, item := range items {
		view := ProjectWorkflow{Definition: item.Definition, Path: item.Path, File: filepath.ToSlash(strings.TrimPrefix(item.Path, workspace.Path+string(filepath.Separator))), Revision: item.ModTime.UTC().Format("20060102150405.000000000")}
		if item.Err != nil {
			view.Error, view.ErrorLine = item.Err.Message, item.Err.Line
		}
		out = append(out, view)
	}
	return out, nil
}

// definitionFor resolves a workflow ID for a task in workspaceID. Project
// workflows are read from the project root, not a task worktree, so a file
// that is not committed yet still runs in a new worktree.
func (a *Service) definitionFor(ctx context.Context, id, workspaceID string) (domainworkflows.Definition, error) {
	id = strings.TrimSpace(id)
	if !repoworkflows.IsProjectWorkflowID(id) {
		return a.GetDefinition(ctx, id)
	}
	workspace, err := a.store.Repos.Tasks.GetWorkspace(ctx, workspaceID)
	if err != nil {
		return domainworkflows.Definition{}, coded("workspace_not_found", "workspace was not found")
	}
	if workspace.RemoteFS != nil {
		return domainworkflows.Definition{}, coded("workflow_not_found", "project workflows are only available for local projects")
	}
	item, err := repoworkflows.GetProjectWorkflow(workspace.Path, id)
	if err != nil {
		return domainworkflows.Definition{}, coded("workflow_not_found", "the workflow file was not found in "+repoworkflows.ProjectWorkflowDir)
	}
	if item.Err != nil {
		return domainworkflows.Definition{}, coded("workflow_file_invalid", filepath.Base(item.Path)+": "+item.Err.Error())
	}
	return item.Definition, nil
}

func (a *Service) taskDefinition(ctx context.Context, task domaintasks.Task) (domainworkflows.Definition, error) {
	return a.definitionFor(ctx, task.WorkflowID, task.WorkspaceID)
}

// ExportWorkflowToProject writes a workflow into the project as a YAML file
// and returns the file path. An existing file is never overwritten.
func (a *Service) ExportWorkflowToProject(ctx context.Context, workspaceID, definitionID string) (string, error) {
	workspace, err := a.store.Repos.Tasks.GetWorkspace(ctx, strings.TrimSpace(workspaceID))
	if err != nil {
		return "", coded("workspace_not_found", "workspace was not found")
	}
	if workspace.RemoteFS != nil {
		return "", coded("workflow_export_local_only", "workflow files can only be written to local projects")
	}
	definition, err := a.definitionFor(ctx, definitionID, workspace.ID)
	if err != nil {
		return "", err
	}
	stem := strings.TrimPrefix(definition.ID, repoworkflows.ProjectWorkflowPrefix)
	data, err := repoworkflows.MarshalWorkflowFile(definition)
	if err != nil {
		return "", err
	}
	path := filepath.Join(workspace.Path, repoworkflows.ProjectWorkflowDir, stem+".yaml")
	if _, err := os.Stat(path); err == nil {
		return path, coded("workflow_file_exists", "a workflow file with this name already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := localfile.WriteTextAtomic(path, string(data)); err != nil {
		return "", err
	}
	return path, nil
}
