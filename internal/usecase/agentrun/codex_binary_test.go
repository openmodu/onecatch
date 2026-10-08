package agentrun

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCodexDefaultUsesNewerManagedRuntime(t *testing.T) {
	oldCLI := stubBinary(t, "codex-cli 0.100.0\n", "", 0)
	newCLI := stubBinary(t, "codex-cli 0.101.0\n", "", 0)
	if got := newerCodexBinary(oldCLI, newCLI); got != newCLI {
		t.Fatalf("selected %q, want newer managed CLI", got)
	}
	if got := newerCodexBinary(newCLI, oldCLI); got != newCLI {
		t.Fatalf("downgraded PATH CLI to %q", got)
	}
	if got := newerCodexBinary(oldCLI, filepath.Join(t.TempDir(), "missing")); got != oldCLI {
		t.Fatalf("missing managed CLI: %q", got)
	}
	broken := stubBinary(t, "", "broken", 1)
	if got := newerCodexBinary(oldCLI, broken); got != oldCLI {
		t.Fatalf("broken managed CLI: %q", got)
	}
	if got := newerCodexBinary(filepath.Join(t.TempDir(), "missing"), newCLI); got != newCLI {
		t.Fatalf("missing PATH CLI: %q", got)
	}
}

func TestExplicitCodexBinaryIsNotReplaced(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	dir := filepath.Join(home, "packages", "app-server-daemon", "current", "bin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	explicit := stubBinary(t, "codex-cli 0.100.0\n", "", 0)
	if got := NewCodexRunner(explicit).Executable(); got != explicit {
		t.Fatalf("explicit executable replaced: %q", got)
	}
}
