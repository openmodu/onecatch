//go:build conformance

package agentrun

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/openmodu/onecatch/internal/usecase/agentrun/seam/seamtest"
)

// The real CLI enforces the sandbox and performs Git operations in a throwaway
// repository. A localhost scripted model asks for escalation; no account or
// model credits are used. This catches protocol drift that a JSON fixture cannot.
func TestCodexGitCommitApprovalConformance(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires a configured native sandbox on Windows")
	}
	binary, err := exec.LookPath("codex")
	if err != nil {
		t.Skip("codex CLI not installed")
	}
	for _, allow := range []bool{true, false} {
		t.Run(fmt.Sprintf("allow=%t", allow), func(t *testing.T) {
			workspace := t.TempDir()
			codexHome := t.TempDir()
			if out, err := exec.Command("git", "init", "-q", workspace).CombinedOutput(); err != nil {
				t.Fatalf("git init: %s (%v)", out, err)
			}
			if err := os.WriteFile(filepath.Join(workspace, "approval.txt"), []byte("approved commit\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			mock := seamtest.StartMockWithTool(seamtest.DialectResponses, "exec_command", map[string]any{
				"cmd":     "git add approval.txt && git -c user.name=OneCatch -c user.email=test@example.invalid -c commit.gpgsign=false -c core.hooksPath=/dev/null commit -m 'approval test'",
				"workdir": workspace, "sandbox_permissions": "require_escalated",
				"yield_time_ms": 10000,
				"justification": "Allow the test to commit approval.txt in this temporary repository?",
			})
			defer mock.Close()
			config := fmt.Sprintf(`model = "seam-mock"
model_provider = "seam"
approvals_reviewer = "user"
[model_providers.seam]
name = "seam"
base_url = %q
wire_api = "responses"
requires_openai_auth = false
supports_websockets = false
`, mock.BaseURL())
			if err := os.WriteFile(filepath.Join(codexHome, "config.toml"), []byte(config), 0o600); err != nil {
				t.Fatal(err)
			}
			approvals := 0
			ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			defer cancel()
			runner := NewCodexRunner(binary)
			defer runner.Close()
			result, err := runner.Run(ctx, Request{
				Workspace: workspace, Prompt: "Commit approval.txt using the requested permission flow.", Sandbox: SandboxWorkspaceWrite,
				Environment: harnessEnvironment(map[string]string{"CODEX_HOME": codexHome}),
				PermissionHandler: func(_ context.Context, request PermissionRequest) (PermissionDecision, error) {
					approvals++
					if !strings.Contains(request.Title, "git") || !request.SuppressAlwaysAllow {
						t.Errorf("unexpected approval: %+v", request)
					}
					decision := "deny"
					if allow {
						decision = "allow"
					}
					return PermissionDecision{Behavior: decision}, nil
				},
			}, nil)
			out, _ := mock.Result()
			// A CLI may interrupt the turn when the only offered negative
			// decision is cancel. That is a valid outcome after denial.
			if err != nil || (allow && !result.Succeeded) || approvals != 1 {
				t.Fatalf("run = %+v, %v; approvals = %d; tool output = %s", result, err, approvals, out)
			}
			committed, gitErr := exec.Command("git", "-C", workspace, "show", "HEAD:approval.txt").CombinedOutput()
			if allow && (gitErr != nil || string(committed) != "approved commit\n") {
				t.Fatalf("approved Git commit failed: %s (%v); tool output = %s", committed, gitErr, out)
			}
			if !allow && gitErr == nil {
				t.Fatal("Git commit ran despite the user denying permission")
			}
			if !allow {
				if out, err := exec.Command("git", "-C", workspace, "diff", "--cached", "--exit-code").CombinedOutput(); err != nil {
					t.Fatalf("denied command staged files: %s (%v)", out, err)
				}
			}
		})
	}
}
