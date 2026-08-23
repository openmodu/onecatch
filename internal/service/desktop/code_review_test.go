package desktop

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	domainworkspaces "github.com/openmodu/onecatch/internal/domain/workspaces"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
	"github.com/openmodu/onecatch/internal/usecase/agentrun/seam"
)

type codeReviewTestRunner struct {
	runtime         agentrun.Runtime
	final           string
	request         agentrun.Request
	workspaceExists bool
}

func (r *codeReviewTestRunner) Runtime() agentrun.Runtime { return r.runtime }
func (r *codeReviewTestRunner) Available() bool           { return true }
func (r *codeReviewTestRunner) Run(_ context.Context, request agentrun.Request, _ agentrun.Sink) (agentrun.Result, error) {
	r.request = request
	info, err := os.Stat(request.Workspace)
	r.workspaceExists = err == nil && info.IsDir()
	return agentrun.Result{Succeeded: true, FinalMessage: r.final}, nil
}

func TestReviewChangesUsesSelectedAgentForRemoteGitDiffWithoutWorkspaceMutation(t *testing.T) {
	ctx := context.Background()
	app, store := newLocalTestApp(t, completingEngine{})
	repo := filepath.Join(t.TempDir(), "remote review")
	if err := os.MkdirAll(repo, 0o750); err != nil {
		t.Fatal(err)
	}
	runGitTestCommand(t, "", "init", repo)
	runGitTestCommand(t, repo, "config", "user.name", "OneCatch Test")
	runGitTestCommand(t, repo, "config", "user.email", "onecatch@example.test")
	file := filepath.Join(repo, "review.go")
	if err := os.WriteFile(file, []byte("package review\n\nfunc enabled() bool { return true }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGitTestCommand(t, repo, "add", "--all")
	runGitTestCommand(t, repo, "commit", "-m", "chore: seed review")
	changed := "package review\n\nfunc enabled() bool { return false }\n"
	if err := os.WriteFile(file, []byte(changed), 0o600); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	workspace := domainworkspaces.Workspace{
		ID: "remote-code-review", Name: "Remote Review", Path: repo,
		RemoteFS:       &domainworkspaces.RemoteFS{Host: "devbox", Root: repo, Username: "deploy"},
		DefaultSandbox: "workspace-write", CreatedAt: now, LastOpenedAt: now,
	}
	if err := store.Repos.Tasks.SaveWorkspace(ctx, workspace); err != nil {
		t.Fatal(err)
	}
	app.remoteGitExecutor = func(domainworkspaces.RemoteFS) seam.Executor {
		return seam.NewExecutor(seam.Target{})
	}
	runner := &codeReviewTestRunner{runtime: agentrun.RuntimeClaude, final: "Here is the result:\n```json\n" +
		`{"summary":"发现一处回归。","findings":[{"priority":"P1","title":"恢复启用状态","body":"该变更会让所有调用方始终得到 false。","file":"review.go","startLine":3,"endLine":3}]}` +
		"\n```"}
	app.runtimes.mu.Lock()
	app.runtimes.engine = agentrun.NewEngineWithRunners(runner)
	app.runtimes.mu.Unlock()

	result, err := app.ReviewChanges(ctx, CodeReviewInput{WorkspaceID: workspace.ID, Runtime: "claude", Language: "zh-CN"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Runtime != "claude" || result.Summary != "发现一处回归。" || len(result.Findings) != 1 || result.Findings[0].Priority != 1 || result.Findings[0].File != "review.go" || len(result.ChangeHash) != 64 {
		t.Fatalf("review result = %+v", result)
	}
	if runner.request.Runtime != agentrun.RuntimeClaude || runner.request.Sandbox != agentrun.SandboxReadOnly || runner.request.Remote != nil || runner.request.Workspace == repo || !runner.workspaceExists {
		t.Fatalf("review request = %+v, workspaceExists=%v", runner.request, runner.workspaceExists)
	}
	if !strings.Contains(runner.request.Prompt, "+func enabled() bool { return false }") || !strings.Contains(runner.request.Prompt, "Simplified Chinese") {
		t.Fatalf("review prompt did not contain the remote diff and language: %s", runner.request.Prompt)
	}
	if _, err := os.Stat(runner.request.Workspace); !os.IsNotExist(err) {
		t.Fatalf("temporary review workspace still exists: %v", err)
	}
	content, err := os.ReadFile(file)
	if err != nil || string(content) != changed {
		t.Fatalf("workspace changed during review: %q, %v", content, err)
	}
}

func TestParseCodeReviewResultNormalizesPrioritiesAndRejectsUnchangedPaths(t *testing.T) {
	message := `{"signal":"completed","content":"{\"summary\":\"Two findings\",\"findings\":[{\"priority\":\"high\",\"title\":\"Valid\",\"body\":\"Concrete bug\",\"file\":\"b/src/main.go\",\"start_line\":8},{\"priority\":0,\"title\":\"Hallucinated\",\"body\":\"Not changed\",\"file\":\"secrets.go\",\"startLine\":1},{\"priority\":\"P3\",\"title\":\"Low\",\"body\":\"Minor bug\",\"file\":\"src/main.go\",\"startLine\":20,\"endLine\":99}]}"}`
	summary, findings, err := parseCodeReviewResult(message, map[string]struct{}{"src/main.go": {}}, "en")
	if err != nil {
		t.Fatal(err)
	}
	if summary != "Two findings" || len(findings) != 2 {
		t.Fatalf("summary=%q findings=%+v", summary, findings)
	}
	if findings[0].Priority != 1 || findings[0].File != "src/main.go" || findings[0].EndLine != 8 {
		t.Fatalf("first finding = %+v", findings[0])
	}
	if findings[1].Priority != 3 || findings[1].EndLine != 40 {
		t.Fatalf("second finding = %+v", findings[1])
	}
}

func TestTruncateCodeReviewComponentsPreservesUTF8AndSharesBudget(t *testing.T) {
	first, second := strings.Repeat("界", 100), strings.Repeat("b", 300)
	if !truncateCodeReviewComponents([]*string{&first, &second}, 120) {
		t.Fatal("large review input was not marked truncated")
	}
	if !strings.Contains(first, "truncated") || !strings.Contains(second, "truncated") || !utf8.ValidString(first) || !utf8.ValidString(second) {
		t.Fatalf("truncated components are invalid: %q / %q", first, second)
	}
}
