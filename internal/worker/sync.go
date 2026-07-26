package worker

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"os/exec"
	"strings"
)

const maxPatchBytes = 24 * 1024 * 1024

// WorkspaceBaseline verifies that the coordinator worktree can safely receive
// a remote patch and returns the exact commit both machines must share.
func WorkspaceBaseline(ctx context.Context, workspace string) (string, error) {
	head, remoteErr := cleanGitHead(ctx, workspace)
	if remoteErr != nil {
		return "", *remoteErr
	}
	return head, nil
}

func validateWorkspaceBaseline(ctx context.Context, workspace, expected string) *RemoteError {
	head, err := cleanGitHead(ctx, workspace)
	if err != nil {
		return err
	}
	if head != expected {
		return &RemoteError{Code: "worker_workspace_revision_mismatch", Message: "local and remote workspaces are not at the same Git revision"}
	}
	return nil
}

func cleanGitHead(ctx context.Context, workspace string) (string, *RemoteError) {
	head, err := gitOutput(ctx, workspace, "rev-parse", "--verify", "HEAD")
	if err != nil {
		return "", &RemoteError{Code: "worker_workspace_git_required", Message: "writable remote runs require a Git worktree with an initial commit"}
	}
	index, err := gitOutput(ctx, workspace, "ls-files", "--stage")
	if err != nil {
		return "", &RemoteError{Code: "worker_workspace_git_failed", Message: "could not inspect the workspace index"}
	}
	for _, line := range bytes.Split(index, []byte{'\n'}) {
		if bytes.HasPrefix(line, []byte("160000 ")) {
			return "", &RemoteError{Code: "worker_workspace_submodules_unsupported", Message: "writable remote runs do not support repositories with Git submodules"}
		}
	}
	status, err := gitOutput(ctx, workspace, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		return "", &RemoteError{Code: "worker_workspace_git_failed", Message: "could not inspect workspace Git state"}
	}
	if len(status) != 0 {
		return "", &RemoteError{Code: "worker_workspace_dirty", Message: "writable remote runs require a clean workspace on both machines"}
	}
	return strings.TrimSpace(string(head)), nil
}

func buildWorkspacePatch(ctx context.Context, workspace, baseRevision string) (*WorkspacePatch, *RemoteError) {
	head, err := gitOutput(ctx, workspace, "rev-parse", "--verify", "HEAD")
	if err != nil || strings.TrimSpace(string(head)) != baseRevision {
		return nil, &RemoteError{Code: "worker_workspace_revision_changed", Message: "remote agent changed the Git revision; changes were preserved on the worker"}
	}
	tracked, err := gitOutput(ctx, workspace, "diff", "--binary", "--full-index", "--no-ext-diff", baseRevision, "--")
	if err != nil {
		return nil, &RemoteError{Code: "worker_patch_failed", Message: "could not create the remote workspace patch"}
	}
	var data bytes.Buffer
	data.Write(tracked)
	if data.Len() > maxPatchBytes {
		return nil, &RemoteError{Code: "worker_patch_too_large", Message: "remote changes exceed the 24 MiB synchronization limit; changes were preserved on the worker"}
	}
	untracked, err := gitOutput(ctx, workspace, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return nil, &RemoteError{Code: "worker_patch_failed", Message: "could not list new remote files"}
	}
	var untrackedPaths []string
	for _, rawPath := range bytes.Split(untracked, []byte{0}) {
		if len(rawPath) == 0 {
			continue
		}
		path := string(rawPath)
		untrackedPaths = append(untrackedPaths, path)
		command := exec.CommandContext(ctx, "git", "diff", "--binary", "--full-index", "--no-index", "--", "/dev/null", path)
		command.Dir = workspace
		output, commandErr := command.Output()
		var exitErr *exec.ExitError
		if commandErr != nil && (!errors.As(commandErr, &exitErr) || exitErr.ExitCode() != 1) {
			return nil, &RemoteError{Code: "worker_patch_failed", Message: "could not include a new remote file in the patch"}
		}
		data.Write(output)
		if data.Len() > maxPatchBytes {
			return nil, &RemoteError{Code: "worker_patch_too_large", Message: "remote changes exceed the 24 MiB synchronization limit; changes were preserved on the worker"}
		}
	}
	if data.Len() == 0 {
		return nil, nil
	}
	digestBytes := sha256.Sum256(data.Bytes())
	return &WorkspacePatch{
		BaseRevision: baseRevision, Digest: hex.EncodeToString(digestBytes[:]),
		Encoding: "base64", Data: base64.StdEncoding.EncodeToString(data.Bytes()),
		UntrackedPaths: untrackedPaths,
	}, nil
}

// ApplyWorkspacePatch verifies the untouched coordinator worktree and applies
// the worker delta without staging it. git apply is atomic unless explicitly
// asked to reject hunks, which this protocol never does.
func ApplyWorkspacePatch(ctx context.Context, workspace string, patch WorkspacePatch) error {
	data := []byte(patch.Data)
	if patch.Encoding != "" {
		if patch.Encoding != "base64" {
			return RemoteError{Code: "worker_patch_encoding_unsupported", Message: "remote patch uses an unsupported encoding"}
		}
		decoded, err := base64.StdEncoding.DecodeString(patch.Data)
		if err != nil {
			return RemoteError{Code: "worker_patch_encoding_invalid", Message: "remote patch is not valid base64"}
		}
		data = decoded
	}
	if len(data) > maxPatchBytes {
		return RemoteError{Code: "worker_patch_too_large", Message: "remote patch exceeds the synchronization limit"}
	}
	digestBytes := sha256.Sum256(data)
	if hex.EncodeToString(digestBytes[:]) != patch.Digest {
		return RemoteError{Code: "worker_patch_digest_mismatch", Message: "remote patch failed its integrity check"}
	}
	if baselineErr := validateWorkspaceBaseline(ctx, workspace, patch.BaseRevision); baselineErr != nil {
		return *baselineErr
	}
	command := exec.CommandContext(ctx, "git", "apply", "--binary", "--whitespace=nowarn", "-")
	command.Dir = workspace
	command.Stdin = bytes.NewReader(data)
	if output, err := command.CombinedOutput(); err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = "remote patch could not be applied; changes remain on the worker"
		}
		return RemoteError{Code: "worker_patch_apply_failed", Message: message}
	}
	return nil
}

func cleanWorkspace(ctx context.Context, workspace string, patch WorkspacePatch) *RemoteError {
	head, err := gitOutput(ctx, workspace, "rev-parse", "--verify", "HEAD")
	if err != nil || strings.TrimSpace(string(head)) != patch.BaseRevision {
		return &RemoteError{Code: "worker_workspace_revision_changed", Message: "remote workspace revision changed; changes were preserved"}
	}
	if _, err := gitOutput(ctx, workspace, "reset", "--hard", patch.BaseRevision); err != nil {
		return &RemoteError{Code: "worker_patch_cleanup_failed", Message: "could not reset tracked remote changes"}
	}
	for start := 0; start < len(patch.UntrackedPaths); start += 128 {
		end := min(start+128, len(patch.UntrackedPaths))
		args := append([]string{"clean", "-f", "--"}, patch.UntrackedPaths[start:end]...)
		if _, err := gitOutput(ctx, workspace, args...); err != nil {
			return &RemoteError{Code: "worker_patch_cleanup_failed", Message: "could not remove synchronized new remote files"}
		}
	}
	status, err := gitOutput(ctx, workspace, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil || len(status) != 0 {
		return &RemoteError{Code: "worker_patch_cleanup_incomplete", Message: "remote workspace contains additional changes; they were preserved for manual recovery"}
	}
	return nil
}

func gitOutput(ctx context.Context, workspace string, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, "git", args...)
	command.Dir = workspace
	return command.Output()
}
