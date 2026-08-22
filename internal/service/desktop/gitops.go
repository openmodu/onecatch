package desktop

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	domainworkspaces "github.com/openmodu/onecatch/internal/domain/workspaces"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
)

type GitCommitInput struct {
	WorkspaceID string `json:"workspaceId"`
	Message     string `json:"message"`
	Remote      string `json:"remote,omitempty"`
	Push        bool   `json:"push,omitempty"`
}

type GitCommitResult struct {
	CommitHash string `json:"commitHash"`
	Pushed     bool   `json:"pushed"`
}

func requireLocalGitWorkspace(workspace domainworkspaces.Workspace) error {
	if workspace.RemoteFS != nil {
		return coded("remote_fs_git_unavailable", "Git controls are not available for remote FS workspaces")
	}
	return nil
}

func (a *Service) GitStatus(ctx context.Context, workspaceID string) (domainworkspaces.GitSnapshot, error) {
	workspace, err := a.GetWorkspace(ctx, strings.TrimSpace(workspaceID))
	if err != nil {
		return domainworkspaces.GitSnapshot{}, err
	}
	if err := requireLocalGitWorkspace(workspace); err != nil {
		return domainworkspaces.GitSnapshot{}, err
	}
	return a.git.Inspect(ctx, workspace.Path)
}

func (a *Service) GitDiff(ctx context.Context, workspaceID string, staged bool) (string, error) {
	workspace, err := a.GetWorkspace(ctx, strings.TrimSpace(workspaceID))
	if err != nil {
		return "", err
	}
	if err := requireLocalGitWorkspace(workspace); err != nil {
		return "", err
	}
	return a.git.Diff(ctx, workspace.Path, staged)
}

func (a *Service) GitStageAll(ctx context.Context, workspaceID string) error {
	workspace, err := a.GetWorkspace(ctx, strings.TrimSpace(workspaceID))
	if err != nil {
		return err
	}
	if err := requireLocalGitWorkspace(workspace); err != nil {
		return err
	}
	return a.git.StageAll(ctx, workspace.Path)
}

func (a *Service) GitListBranches(ctx context.Context, workspaceID string) ([]domainworkspaces.GitBranch, error) {
	workspace, err := a.GetWorkspace(ctx, strings.TrimSpace(workspaceID))
	if err != nil {
		return nil, err
	}
	if err := requireLocalGitWorkspace(workspace); err != nil {
		return nil, err
	}
	return a.git.ListBranches(ctx, workspace.Path)
}

func (a *Service) GitSwitchBranch(ctx context.Context, workspaceID, name string) (domainworkspaces.GitSnapshot, error) {
	workspace, err := a.GetWorkspace(ctx, strings.TrimSpace(workspaceID))
	if err != nil {
		return domainworkspaces.GitSnapshot{}, err
	}
	if err := requireLocalGitWorkspace(workspace); err != nil {
		return domainworkspaces.GitSnapshot{}, err
	}
	if err := a.git.SwitchBranch(ctx, workspace.Path, name); err != nil {
		return domainworkspaces.GitSnapshot{}, err
	}
	return a.git.Inspect(ctx, workspace.Path)
}

func (a *Service) GitCreateBranch(ctx context.Context, workspaceID, name string) (domainworkspaces.GitSnapshot, error) {
	workspace, err := a.GetWorkspace(ctx, strings.TrimSpace(workspaceID))
	if err != nil {
		return domainworkspaces.GitSnapshot{}, err
	}
	if err := requireLocalGitWorkspace(workspace); err != nil {
		return domainworkspaces.GitSnapshot{}, err
	}
	if err := a.git.CreateBranch(ctx, workspace.Path, name); err != nil {
		return domainworkspaces.GitSnapshot{}, err
	}
	return a.git.Inspect(ctx, workspace.Path)
}

func (a *Service) GenerateCommitMessage(ctx context.Context, workspaceID, requestedRuntime string) (string, error) {
	workspace, err := a.GetWorkspace(ctx, strings.TrimSpace(workspaceID))
	if err != nil {
		return "", err
	}
	if err := requireLocalGitWorkspace(workspace); err != nil {
		return "", err
	}
	diff, err := a.git.Diff(ctx, workspace.Path, false)
	if err != nil {
		return "", err
	}
	staged, err := a.git.Diff(ctx, workspace.Path, true)
	if err != nil {
		return "", err
	}
	combined := strings.TrimSpace(staged + "\n" + diff)
	if combined == "" {
		return "", coded("git_no_changes", "there are no changes to describe")
	}
	if len(combined) > 80_000 {
		combined = combined[:80_000]
	}
	runtime := agentrun.Runtime(strings.TrimSpace(requestedRuntime))
	if runtime == "" || !runtime.Valid() || !a.runtimes.Available(runtime) {
		for _, candidate := range []agentrun.Runtime{agentrun.RuntimeCodex, agentrun.RuntimeClaude, agentrun.RuntimeModu} {
			if a.runtimes.Available(candidate) {
				runtime = candidate
				break
			}
		}
	}
	if !runtime.Valid() || !a.runtimes.Available(runtime) {
		return "", coded("runtime_unavailable", "no local Agent is available to generate a commit message")
	}
	prompt := "Write one concise Conventional Commit subject for the following diff. Return only the subject line, without quotes, Markdown, body, or explanation.\n\n" + combined
	runCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	result, err := a.runtimes.Run(runCtx, agentrun.Request{Runtime: runtime, Workspace: workspace.Path, Prompt: prompt, Sandbox: agentrun.SandboxReadOnly}, nil)
	if err != nil {
		return "", err
	}
	if !result.Succeeded {
		return "", errors.New("Agent did not generate a commit message")
	}
	return normalizeCommitMessage(result.FinalMessage), nil
}

func (a *Service) GitCommitAndPush(ctx context.Context, input GitCommitInput) (GitCommitResult, error) {
	workspace, err := a.GetWorkspace(ctx, strings.TrimSpace(input.WorkspaceID))
	if err != nil {
		return GitCommitResult{}, err
	}
	if err := requireLocalGitWorkspace(workspace); err != nil {
		return GitCommitResult{}, err
	}
	if err := a.git.StageAll(ctx, workspace.Path); err != nil {
		return GitCommitResult{}, err
	}
	hash, err := a.git.Commit(ctx, workspace.Path, input.Message)
	if err != nil {
		return GitCommitResult{}, err
	}
	result := GitCommitResult{CommitHash: hash}
	if input.Push {
		if err := a.git.Push(ctx, workspace.Path, input.Remote); err != nil {
			return result, err
		}
		result.Pushed = true
	}
	return result, nil
}

var conventionalCommit = regexp.MustCompile(`^(build|chore|ci|docs|feat|fix|perf|refactor|revert|style|test)(\([^)]+\))?!?:\s+`)

func normalizeCommitMessage(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "```") {
		lines := strings.Split(value, "\n")
		if len(lines) > 1 {
			lines = lines[1:]
		}
		if len(lines) > 0 && strings.HasPrefix(strings.TrimSpace(lines[len(lines)-1]), "```") {
			lines = lines[:len(lines)-1]
		}
		value = strings.TrimSpace(strings.Join(lines, "\n"))
	}
	value = strings.Trim(value, "`\"'")
	if index := strings.IndexAny(value, "\r\n"); index >= 0 {
		value = value[:index]
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "chore: update workspace"
	}
	if !conventionalCommit.MatchString(value) {
		value = "chore: " + strings.ToLower(strings.TrimRight(value, "."))
	}
	if len(value) > 120 {
		value = strings.TrimSpace(value[:120])
	}
	return value
}
