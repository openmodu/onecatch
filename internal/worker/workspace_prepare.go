package worker

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func prepareGitWorkspace(ctx context.Context, path, remoteURL, revision string) *RemoteError {
	path = filepath.Clean(strings.TrimSpace(path))
	remoteURL = strings.TrimSpace(remoteURL)
	revision = strings.TrimSpace(revision)
	if !filepath.IsAbs(path) || revision == "" {
		return &RemoteError{Code: "worker_workspace_prepare_invalid", Message: "an absolute path and Git revision are required"}
	}
	if remoteURL == "" {
		return &RemoteError{Code: "worker_workspace_remote_missing", Message: "the project has no origin URL for cloning or fetching"}
	}
	cloned := false
	info, statErr := os.Stat(path)
	switch {
	case errors.Is(statErr, os.ErrNotExist):
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return &RemoteError{Code: "worker_workspace_prepare_failed", Message: "could not create the workspace parent directory"}
		}
		temporary, err := os.MkdirTemp(filepath.Dir(path), ".oneshot-clone-")
		if err != nil {
			return &RemoteError{Code: "worker_workspace_prepare_failed", Message: "could not create a temporary clone directory"}
		}
		defer os.RemoveAll(temporary)
		command := exec.CommandContext(ctx, "git", "clone", "--quiet", "--no-checkout", "--", remoteURL, temporary)
		if output, err := command.CombinedOutput(); err != nil {
			return &RemoteError{Code: "worker_workspace_clone_failed", Message: commandMessage(output, "could not clone the project on the worker")}
		}
		if err := os.Rename(temporary, path); err != nil {
			return &RemoteError{Code: "worker_workspace_prepare_failed", Message: "could not install the cloned workspace"}
		}
		cloned = true
	case statErr != nil || !info.IsDir():
		return &RemoteError{Code: "worker_workspace_prepare_invalid", Message: "the worker workspace path is not a directory"}
	}

	inside, err := gitOutput(ctx, path, "rev-parse", "--is-inside-work-tree")
	if err != nil || strings.TrimSpace(string(inside)) != "true" {
		return &RemoteError{Code: "worker_workspace_git_required", Message: "the worker workspace path is not a Git worktree"}
	}
	if !cloned {
		currentRemote, err := WorkspaceRemoteURL(ctx, path)
		if err != nil || normalizeRemoteURL(currentRemote) != normalizeRemoteURL(remoteURL) {
			return &RemoteError{Code: "worker_workspace_remote_mismatch", Message: "the existing worker workspace belongs to a different Git remote"}
		}
		status, err := gitOutput(ctx, path, "status", "--porcelain=v1", "-z", "--untracked-files=all")
		if err != nil {
			return &RemoteError{Code: "worker_workspace_git_failed", Message: "could not inspect the worker workspace"}
		}
		if len(status) != 0 {
			return &RemoteError{Code: "worker_workspace_dirty", Message: "the worker workspace has uncommitted changes"}
		}
	}
	if !gitRevisionExists(ctx, path, revision) {
		if output, fetchErr := gitCombinedOutput(ctx, path, "fetch", "--all", "--prune"); fetchErr != nil {
			return &RemoteError{Code: "worker_workspace_fetch_failed", Message: commandMessage(output, "could not fetch the requested revision on the worker")}
		}
	}
	if !gitRevisionExists(ctx, path, revision) {
		return &RemoteError{Code: "worker_workspace_revision_missing", Message: "the requested Git revision is unavailable on the worker"}
	}
	if output, err := gitCombinedOutput(ctx, path, "checkout", "--quiet", "--detach", revision); err != nil {
		return &RemoteError{Code: "worker_workspace_checkout_failed", Message: commandMessage(output, "could not check out the requested revision on the worker")}
	}
	if baselineErr := validateWorkspaceBaseline(ctx, path, revision); baselineErr != nil {
		return baselineErr
	}
	return nil
}

func WorkspaceRemoteURL(ctx context.Context, workspace string) (string, error) {
	output, err := gitOutput(ctx, workspace, "config", "--get", "remote.origin.url")
	if err != nil || strings.TrimSpace(string(output)) == "" {
		return "", RemoteError{Code: "worker_workspace_remote_missing", Message: "the local project has no origin URL"}
	}
	return strings.TrimSpace(string(output)), nil
}

func normalizeRemoteURL(value string) string {
	value = strings.TrimRight(strings.TrimSpace(value), "/")
	return strings.TrimSuffix(value, ".git")
}

func gitRevisionExists(ctx context.Context, workspace, revision string) bool {
	command := exec.CommandContext(ctx, "git", "cat-file", "-e", revision+"^{commit}")
	command.Dir = workspace
	return command.Run() == nil
}

func gitCombinedOutput(ctx context.Context, workspace string, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, "git", args...)
	command.Dir = workspace
	return command.CombinedOutput()
}

func commandMessage(output []byte, fallback string) string {
	if message := strings.TrimSpace(string(output)); message != "" {
		return message
	}
	return fallback
}
