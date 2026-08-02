//go:build darwin

package desktop

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
)

func TestExtractLoginShellPath(t *testing.T) {
	output := "startup message\n" + loginPathStart + "/custom/bin:/usr/bin" + loginPathEnd + "\n"
	if got, want := extractLoginShellPath(output), "/custom/bin:/usr/bin"; got != want {
		t.Fatalf("extractLoginShellPath() = %q, want %q", got, want)
	}
}

func TestExtractLoginShellPathRejectsIncompleteOutput(t *testing.T) {
	if got := extractLoginShellPath(loginPathStart + "/custom/bin"); got != "" {
		t.Fatalf("extractLoginShellPath() = %q, want empty", got)
	}
}

func TestMergeCommandPathsPreservesOrderAndRemovesDuplicates(t *testing.T) {
	got := filepath.SplitList(mergeCommandPaths("/user/bin:/usr/bin", "/usr/bin:/bin", ""))
	want := []string{"/user/bin", "/usr/bin", "/bin"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("mergeCommandPaths() = %#v, want %#v", got, want)
	}
}

func TestMacOSFallbackCommandPaths(t *testing.T) {
	got := macOSFallbackCommandPaths("/Users/tester", "tester")
	want := []string{
		"/Users/tester/.local/bin",
		"/Users/tester/.npm-global/bin",
		"/Users/tester/.cargo/bin",
		"/Users/tester/go/bin",
		"/Users/tester/.nix-profile/bin",
		"/opt/homebrew/bin",
		"/opt/homebrew/sbin",
		"/usr/local/bin",
		"/etc/profiles/per-user/tester/bin",
		"/run/current-system/sw/bin",
		"/nix/var/nix/profiles/default/bin",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("macOSFallbackCommandPaths() = %#v, want %#v", got, want)
	}
}

func TestPrepareCommandEnvironmentFindsUserInstalledRuntime(t *testing.T) {
	home := t.TempDir()
	binDir := filepath.Join(home, ".npm-global", "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	codex := filepath.Join(binDir, "codex")
	if err := os.WriteFile(codex, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	shell := filepath.Join(home, "login-shell")
	shellScript := "#!/bin/sh\nprintf '\\n" + loginPathStart + "/usr/bin:/bin" + loginPathEnd + "\\n'\n"
	if err := os.WriteFile(shell, []byte(shellScript), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("HOME", home)
	t.Setenv("USER", "tester")
	t.Setenv("SHELL", shell)
	t.Setenv("PATH", "/usr/bin:/bin")
	prepareCommandEnvironment()

	got, err := exec.LookPath("codex")
	if err != nil {
		t.Fatalf("LookPath(codex): %v", err)
	}
	if got != codex {
		t.Fatalf("LookPath(codex) = %q, want %q", got, codex)
	}
}

func TestPrepareCommandEnvironmentUsesCustomLoginShellPath(t *testing.T) {
	home := t.TempDir()
	customBin := filepath.Join(home, "tools", "current", "bin")
	if err := os.MkdirAll(customBin, 0o755); err != nil {
		t.Fatal(err)
	}
	codex := filepath.Join(customBin, "codex")
	if err := os.WriteFile(codex, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	shell := filepath.Join(home, "login-shell")
	shellScript := "#!/bin/sh\nprintf '\\n" + loginPathStart + customBin + ":/usr/bin:/bin" + loginPathEnd + "\\n'\n"
	if err := os.WriteFile(shell, []byte(shellScript), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("HOME", home)
	t.Setenv("USER", "tester")
	t.Setenv("SHELL", shell)
	t.Setenv("PATH", "/usr/bin:/bin")
	prepareCommandEnvironment()

	got, err := exec.LookPath("codex")
	if err != nil {
		t.Fatalf("LookPath(codex): %v", err)
	}
	if got != codex {
		t.Fatalf("LookPath(codex) = %q, want %q", got, codex)
	}
}
