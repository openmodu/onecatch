package desktop

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	domaintasks "github.com/openmodu/onecatch/internal/domain/tasks"
	domainworkspaces "github.com/openmodu/onecatch/internal/domain/workspaces"
)

func (a *Service) GitListWorktrees(ctx context.Context, workspaceID string) ([]domainworkspaces.Worktree, error) {
	workspace, err := a.store.Repos.Tasks.GetWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	if workspace.RemoteFS != nil {
		return nil, coded("worktree_local_only", "worktree selection is currently available for local projects")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	items, err := a.git.ListWorktrees(ctx, workspace.Path)
	if err != nil {
		return nil, err
	}
	for n := range items {
		items[n].ContextID = fmt.Sprintf("worktree:%s:%x", workspace.ID, sha256.Sum256([]byte(items[n].Path)))
	}
	return items, nil
}

func (a *Service) resolveWorktreeContext(ctx context.Context, id string) (domainworkspaces.Workspace, error) {
	parts := strings.Split(id, ":")
	if len(parts) != 3 {
		return domainworkspaces.Workspace{}, coded("worktree_invalid", "invalid worktree context")
	}
	items, err := a.GitListWorktrees(ctx, parts[1])
	if err != nil {
		return domainworkspaces.Workspace{}, err
	}
	for _, item := range items {
		if item.ContextID == id && !item.Unavailable {
			return a.resolveTaskWorkspace(ctx, domaintasks.Task{WorkspaceID: parts[1], Worktree: &item})
		}
	}
	return domainworkspaces.Workspace{}, coded("worktree_unavailable", "worktree is no longer available")
}

func (a *Service) resolveTaskWorkspace(ctx context.Context, task domaintasks.Task) (domainworkspaces.Workspace, error) {
	workspace, err := a.store.Repos.Tasks.GetWorkspace(ctx, task.WorkspaceID)
	if err != nil {
		return domainworkspaces.Workspace{}, err
	}
	if task.Worktree == nil {
		if workspace.RemoteFS == nil {
			canonical, err := filepath.EvalSymlinks(workspace.Path)
			if err != nil {
				return domainworkspaces.Workspace{}, err
			}
			probeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			defer cancel()
			if identity, err := a.git.WorktreeIdentity(probeCtx, canonical); err == nil {
				canonical = identity.Root
			}
			workspace.LockID = fmt.Sprintf("workdir_%x", sha256.Sum256([]byte(canonical)))
		}
		return workspace, nil
	}
	if workspace.RemoteFS != nil {
		return domainworkspaces.Workspace{}, coded("worktree_local_only", "worktree target must be local")
	}
	binding := task.Worktree
	info, err := os.Stat(binding.Path)
	if err != nil || !info.IsDir() {
		return domainworkspaces.Workspace{}, coded("worktree_missing", "the task's worktree directory is missing")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	identity, err := a.git.WorktreeIdentity(ctx, binding.Path)
	if err != nil || identity.GitDir != binding.GitDir || identity.CommonDir != binding.CommonDir || identity.Root != binding.Root {
		return domainworkspaces.Workspace{}, coded("worktree_changed", "the task's worktree identity has changed")
	}
	workspace.Path = filepath.Clean(binding.Path)
	workspace.LockID = fmt.Sprintf("workdir_%x", sha256.Sum256([]byte(binding.Root)))
	workspace.ID = binding.ContextID
	// Tools use the validated directory context; locks use physical checkout identity.
	return workspace, nil
}

// Empty mode follows the saved project preference. An explicit checkout or
// project-directory override applies only to the task being created.
func resolveWorktreeMode(auto bool, mode, id string) (string, error) {
	switch mode {
	case "", "default":
		if id != "" {
			return "existing", nil
		}
		if auto {
			return "new", nil
		}
		return "project", nil
	case "project", "new":
		if id != "" {
			return "", coded("worktree_invalid", "worktree mode conflicts with a selected directory")
		}
		return mode, nil
	case "existing":
		if id != "" {
			return mode, nil
		}
	}
	return "", coded("worktree_invalid", "invalid worktree selection")
}

func (a *Service) createTaskWorktree(ctx context.Context, workspace domainworkspaces.Workspace, taskID string) (domainworkspaces.Worktree, error) {
	if workspace.RemoteFS != nil {
		return domainworkspaces.Worktree{}, coded("worktree_local_only", "automatic worktrees currently require a local Git project")
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	destination := filepath.Join(a.store.Data.Paths.Root, "worktrees", workspace.ID, taskID)
	branch := "onecatch/" + taskID
	binding, err := a.git.CreateWorktree(ctx, workspace.Path, destination, branch)
	if err != nil {
		return domainworkspaces.Worktree{}, coded("worktree_create_failed", fmt.Sprintf("could not create worktree at %s (branch %s): %v; the project directory was not used as a fallback", destination, branch, err))
	}
	binding.ContextID = fmt.Sprintf("worktree:%s:%x", workspace.ID, sha256.Sum256([]byte(binding.Path)))
	binding.Managed = true
	return binding, nil
}

func worktreeTaskError(task domaintasks.Task, err error) error {
	if task.Worktree == nil || !task.Worktree.Managed {
		return err
	}
	return coded("worktree_task_failed", fmt.Sprintf("%v; worktree preserved at %s on branch %s; select it under existing worktrees to retry", err, task.Worktree.Root, task.Worktree.Branch))
}
