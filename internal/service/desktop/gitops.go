package desktop

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	domainworkspaces "github.com/openmodu/onecatch/internal/domain/workspaces"
	gitrepo "github.com/openmodu/onecatch/internal/repo/git"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
	"github.com/openmodu/onecatch/internal/usecase/agentrun/seam"
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

type remoteGitCommandRunner struct {
	executor seam.Executor
}

type remoteGitExitError struct {
	args       []string
	exitCode   int
	diagnostic string
}

func (e *remoteGitExitError) Error() string {
	message := fmt.Sprintf("remote git %s exited with status %d", strings.Join(e.args, " "), e.exitCode)
	if e.diagnostic != "" {
		message += ": " + e.diagnostic
	}
	return message
}

func (e *remoteGitExitError) ExitCode() int { return e.exitCode }

func (r *remoteGitCommandRunner) Run(ctx context.Context, workspace string, args ...string) (string, error) {
	var command strings.Builder
	command.WriteString("git")
	for _, arg := range args {
		command.WriteByte(' ')
		command.WriteString(posixShellWord(arg))
	}
	var stdout, stderr bytes.Buffer
	outcome, err := r.executor.Run(ctx, seam.Command{
		Command: command.String(),
		Dir:     workspace,
		Env: map[string]string{
			"GIT_TERMINAL_PROMPT": "0",
			"GCM_INTERACTIVE":     "Never",
		},
		Stdout: &stdout,
		Stderr: &stderr,
	})
	if err != nil {
		return "", coded("remote_fs_git_unavailable", err.Error())
	}
	if outcome.ExitCode != 0 {
		diagnostic := strings.TrimSpace(strings.Join([]string{stderr.String(), stdout.String()}, "\n"))
		if outcome.ExitCode == 126 || outcome.ExitCode == 127 {
			if diagnostic == "" {
				diagnostic = "git is not installed on the remote host"
			}
			return "", coded("remote_fs_git_unavailable", diagnostic)
		}
		return "", &remoteGitExitError{args: append([]string{}, args...), exitCode: outcome.ExitCode, diagnostic: diagnostic}
	}
	return stdout.String(), nil
}

func posixShellWord(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}

func newRemoteGitExecutor(target domainworkspaces.RemoteFS) seam.Executor {
	return seam.NewExecutor(seam.Target{
		Host:         target.Host,
		Root:         target.Root,
		Username:     target.Username,
		CredentialID: target.CredentialID,
		SSHOptions:   append([]string{}, target.SSHOptions...),
	})
}

func (a *Service) gitForWorkspace(workspace domainworkspaces.Workspace) *gitrepo.Inspector {
	if workspace.RemoteFS == nil {
		return a.git
	}
	factory := a.remoteGitExecutor
	if factory == nil {
		factory = newRemoteGitExecutor
	}
	return gitrepo.NewWithRunner(&remoteGitCommandRunner{executor: factory(*workspace.RemoteFS)})
}

func (a *Service) GitStatus(ctx context.Context, workspaceID string) (domainworkspaces.GitSnapshot, error) {
	workspace, err := a.GetWorkspace(ctx, strings.TrimSpace(workspaceID))
	if err != nil {
		return domainworkspaces.GitSnapshot{}, err
	}
	return a.gitForWorkspace(workspace).Inspect(ctx, workspace.Path)
}

func (a *Service) GitDiff(ctx context.Context, workspaceID string, staged bool) (string, error) {
	workspace, err := a.GetWorkspace(ctx, strings.TrimSpace(workspaceID))
	if err != nil {
		return "", err
	}
	return a.gitForWorkspace(workspace).Diff(ctx, workspace.Path, staged)
}

func (a *Service) GitStageAll(ctx context.Context, workspaceID string) error {
	workspace, err := a.GetWorkspace(ctx, strings.TrimSpace(workspaceID))
	if err != nil {
		return err
	}
	return a.gitForWorkspace(workspace).StageAll(ctx, workspace.Path)
}

func (a *Service) GitListBranches(ctx context.Context, workspaceID string) ([]domainworkspaces.GitBranch, error) {
	workspace, err := a.GetWorkspace(ctx, strings.TrimSpace(workspaceID))
	if err != nil {
		return nil, err
	}
	return a.gitForWorkspace(workspace).ListBranches(ctx, workspace.Path)
}

func (a *Service) GitSwitchBranch(ctx context.Context, workspaceID, name string) (domainworkspaces.GitSnapshot, error) {
	workspace, err := a.GetWorkspace(ctx, strings.TrimSpace(workspaceID))
	if err != nil {
		return domainworkspaces.GitSnapshot{}, err
	}
	git := a.gitForWorkspace(workspace)
	if err := git.SwitchBranch(ctx, workspace.Path, name); err != nil {
		return domainworkspaces.GitSnapshot{}, err
	}
	return git.Inspect(ctx, workspace.Path)
}

func (a *Service) GitCreateBranch(ctx context.Context, workspaceID, name string) (domainworkspaces.GitSnapshot, error) {
	workspace, err := a.GetWorkspace(ctx, strings.TrimSpace(workspaceID))
	if err != nil {
		return domainworkspaces.GitSnapshot{}, err
	}
	git := a.gitForWorkspace(workspace)
	if err := git.CreateBranch(ctx, workspace.Path, name); err != nil {
		return domainworkspaces.GitSnapshot{}, err
	}
	return git.Inspect(ctx, workspace.Path)
}

func (a *Service) GenerateCommitMessage(ctx context.Context, workspaceID, requestedRuntime string) (string, error) {
	workspace, err := a.GetWorkspace(ctx, strings.TrimSpace(workspaceID))
	if err != nil {
		return "", err
	}
	git := a.gitForWorkspace(workspace)
	diff, err := git.Diff(ctx, workspace.Path, false)
	if err != nil {
		return "", err
	}
	staged, err := git.Diff(ctx, workspace.Path, true)
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
	runWorkspace := workspace.Path
	if workspace.RemoteFS != nil {
		// The complete diff is already in the prompt. Run the message-only Agent
		// in an empty local directory so it neither needs nor gains workspace
		// access, and so Claude can generate a message even though its remote
		// read-only mode intentionally has no Bash access.
		temporary, tempErr := os.MkdirTemp("", "onecatch-commit-message-")
		if tempErr != nil {
			return "", fmt.Errorf("create commit-message workspace: %w", tempErr)
		}
		defer os.RemoveAll(temporary)
		runWorkspace = temporary
	}
	runCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	result, err := a.runtimes.Run(runCtx, agentrun.Request{Runtime: runtime, Workspace: runWorkspace, Prompt: prompt, Sandbox: agentrun.SandboxReadOnly}, nil)
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
	git := a.gitForWorkspace(workspace)
	if err := git.StageAll(ctx, workspace.Path); err != nil {
		return GitCommitResult{}, err
	}
	hash, err := git.Commit(ctx, workspace.Path, input.Message)
	if err != nil {
		return GitCommitResult{}, err
	}
	result := GitCommitResult{CommitHash: hash}
	if input.Push {
		if err := git.Push(ctx, workspace.Path, input.Remote); err != nil {
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
