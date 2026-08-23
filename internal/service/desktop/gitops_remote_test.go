package desktop

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	domainworkspaces "github.com/openmodu/onecatch/internal/domain/workspaces"
	"github.com/openmodu/onecatch/internal/usecase/agentrun/seam"
)

func TestRemoteFSGitControlsRunAgainstTargetWorkspace(t *testing.T) {
	ctx := context.Background()
	app, store := newLocalTestApp(t, completingEngine{})
	repo := filepath.Join(t.TempDir(), "remote repo")
	if err := os.MkdirAll(repo, 0o750); err != nil {
		t.Fatal(err)
	}
	runGitTestCommand(t, "", "init", repo)
	runGitTestCommand(t, repo, "config", "user.name", "OneCatch Test")
	runGitTestCommand(t, repo, "config", "user.email", "onecatch@example.test")
	tracked := filepath.Join(repo, "review.txt")
	if err := os.WriteFile(tracked, []byte("first\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGitTestCommand(t, repo, "add", "--all")
	runGitTestCommand(t, repo, "commit", "-m", "chore: seed remote review")
	initialBranch := strings.TrimSpace(runGitTestCommand(t, repo, "branch", "--show-current"))
	bareRemote := filepath.Join(t.TempDir(), "remote origin.git")
	runGitTestCommand(t, "", "init", "--bare", bareRemote)
	runGitTestCommand(t, repo, "remote", "add", "origin", bareRemote)

	if err := os.WriteFile(tracked, []byte("first\nsecond\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "未跟踪.txt"), []byte("new\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	workspace := domainworkspaces.Workspace{
		ID: "remote-git-workspace", Name: "Remote Git", Path: repo,
		RemoteFS: &domainworkspaces.RemoteFS{
			Host: "devbox:2222", Root: repo, Username: "deploy",
			CredentialID: "sshcred_0123456789abcdef0123456789abcdef",
			SSHOptions:   []string{"ProxyJump=bastion"},
		},
		DefaultSandbox: "workspace-write", CreatedAt: now, LastOpenedAt: now,
	}
	if err := store.Repos.Tasks.SaveWorkspace(ctx, workspace); err != nil {
		t.Fatal(err)
	}
	var captured domainworkspaces.RemoteFS
	app.remoteGitExecutor = func(target domainworkspaces.RemoteFS) seam.Executor {
		captured = target
		// The local executor gives the test a real Git repository while exercising
		// exactly the command construction used by SSH-backed remote workspaces.
		return seam.NewExecutor(seam.Target{})
	}

	snapshot, err := app.GitStatus(ctx, workspace.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !snapshot.IsRepo || len(snapshot.Files) != 2 || !strings.Contains(snapshot.Status, "未跟踪.txt") {
		t.Fatalf("remote snapshot = %+v", snapshot)
	}
	if captured.Host != workspace.RemoteFS.Host || captured.Username != workspace.RemoteFS.Username || captured.CredentialID != workspace.RemoteFS.CredentialID || len(captured.SSHOptions) != 1 {
		t.Fatalf("remote target = %+v", captured)
	}
	diff, err := app.GitDiff(ctx, workspace.ID, false)
	if err != nil || !strings.Contains(diff, "+second") {
		t.Fatalf("remote diff = %q, %v", diff, err)
	}
	branches, err := app.GitListBranches(ctx, workspace.ID)
	if err != nil || len(branches) != 1 || !branches[0].Current {
		t.Fatalf("remote branches = %+v, %v", branches, err)
	}
	branchName := "feature/review's-output"
	created, err := app.GitCreateBranch(ctx, workspace.ID, branchName)
	if err != nil || created.Branch != branchName {
		t.Fatalf("created branch = %+v, %v", created, err)
	}
	switched, err := app.GitSwitchBranch(ctx, workspace.ID, initialBranch)
	if err != nil || switched.Branch != initialBranch {
		t.Fatalf("switched branch = %+v, %v", switched, err)
	}
	result, err := app.GitCommitAndPush(ctx, GitCommitInput{
		WorkspaceID: workspace.ID,
		Message:     "feat: review user's remote changes",
		Push:        true,
	})
	if err != nil || len(result.CommitHash) < 7 || !result.Pushed {
		t.Fatalf("remote commit = %+v, %v", result, err)
	}
	refs := runGitTestCommand(t, "", "--git-dir", bareRemote, "show-ref")
	if !strings.Contains(refs, result.CommitHash) {
		t.Fatalf("remote refs %q do not contain %q", refs, result.CommitHash)
	}
}

func runGitTestCommand(t *testing.T, workspace string, args ...string) string {
	t.Helper()
	if workspace != "" {
		args = append([]string{"-C", workspace}, args...)
	}
	command := exec.Command("git", args...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, output)
	}
	return string(output)
}
