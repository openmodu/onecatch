package seam

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSSHExecutorUsesPOSIXShellBehindNonPOSIXLoginShell(t *testing.T) {
	stub := filepath.Join(t.TempDir(), "ssh")
	script := `#!/bin/sh
for last do :; done
case "$last" in
  "exec /bin/sh -c "*) exec /bin/sh -c "$last" ;;
  *) printf '%s\n' "fish: Unsupported use of '='" >&2; exit 0 ;;
esac
`
	if err := os.WriteFile(stub, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	outcome, err := NewExecutor(Target{Host: "devbox", SSHBinary: stub}).Run(context.Background(), Command{
		Command: `printf "%s\n" "it's remote"`,
		Dir:     "/",
		Stdout:  &stdout,
		Stderr:  &stderr,
	})
	if err != nil {
		t.Fatalf("Run() error = %v; stderr = %q", err, stderr.String())
	}
	if outcome.ExitCode != 0 || strings.TrimSpace(stdout.String()) != "it's remote" {
		t.Fatalf("Run() = %#v, stdout %q, stderr %q", outcome, stdout.String(), stderr.String())
	}
}

func TestRunShellIncludesStderrInTransportFailure(t *testing.T) {
	var stderr bytes.Buffer
	_, err := runShell(context.Background(), Command{Stderr: &stderr}, func(ctx context.Context) *exec.Cmd {
		return exec.CommandContext(ctx, "/bin/sh", "-c", `printf 'permission denied by ssh\n' >&2; exit 255`)
	}, "missing-status", "missing-cwd", "ssh devbox")
	if err == nil {
		t.Fatal("Run() unexpectedly succeeded")
	}
	for _, expected := range []string{"exit status 255", "permission denied by ssh"} {
		if !strings.Contains(err.Error(), expected) {
			t.Errorf("transport error %q does not contain %q", err, expected)
		}
	}
}
