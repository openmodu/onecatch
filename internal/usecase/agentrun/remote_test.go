package agentrun

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openmodu/onecatch/internal/usecase/agentrun/seam"
)

func TestSetupRemoteCodexCreatesFailClosedEnvironment(t *testing.T) {
	dir := t.TempDir()
	sourceHome := filepath.Join(dir, "source-codex")
	workspace := filepath.Join(dir, "workspace")
	target := filepath.Join(dir, "target")
	for _, path := range []string{sourceHome, workspace, target} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(sourceHome, "config.toml"), []byte("model = \"test\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(seam.DirEnv, filepath.Join(dir, "seams"))
	t.Setenv(ShellBinaryEnv, "/bin/sh")

	setup, err := setupRemoteCodex(Request{
		RunID: "remote-test", Workspace: workspace, Sandbox: SandboxReadOnly,
		Remote:      &seam.Target{Root: target},
		Environment: []string{"PATH=/usr/bin", "CODEX_HOME=" + sourceHome},
	})
	if err != nil {
		t.Fatal(err)
	}
	home := environmentValue(setup.env, "CODEX_HOME")
	if home == "" || home == sourceHome {
		t.Fatalf("managed CODEX_HOME = %q", home)
	}
	document, err := os.ReadFile(filepath.Join(home, "environments.toml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"include_local = false", `args = ["exec-server"]`, `ONECATCH_SEAM_SESSION = "remote-test"`} {
		if !strings.Contains(string(document), expected) {
			t.Errorf("environments.toml missing %q:\n%s", expected, document)
		}
	}
	session, err := seam.LoadSession("remote-test")
	if err != nil {
		t.Fatal(err)
	}
	if !session.ReadOnly {
		t.Error("read-only sandbox was not persisted at the target boundary")
	}

	setup.cleanup()
	if _, err := seam.LoadSession("remote-test"); err == nil {
		t.Error("cleanup left an active seam session")
	}
	if _, err := os.Stat(home); err != nil {
		t.Fatalf("managed home needed for thread resume was removed: %v", err)
	}
}

func TestMergeEnvironmentReplacesKeys(t *testing.T) {
	merged := mergeEnvironment(
		[]string{"PATH=/bin", "CODEX_HOME=/real", "KEEP=yes"},
		[]string{"CODEX_HOME=/managed", "NEW=value"},
	)
	if got := environmentValue(merged, "CODEX_HOME"); got != "/managed" {
		t.Fatalf("CODEX_HOME = %q", got)
	}
	if got := environmentValue(merged, "KEEP"); got != "yes" {
		t.Fatalf("KEEP = %q", got)
	}
	var count int
	for _, value := range merged {
		if strings.HasPrefix(value, "CODEX_HOME=") {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("environment has %d CODEX_HOME entries: %v", count, merged)
	}
}

func TestShellBinaryCandidatesFindDevelopmentBuild(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	executable := filepath.Join(root, "bin", "OneCatch.dev.app", "Contents", "MacOS", "onecatch")
	want := filepath.Join(root, "bin", "onecatchsh")
	candidates := shellBinaryCandidates(executable)
	if candidates[len(candidates)-1] != want {
		t.Fatalf("development shell candidate = %q, want %q", candidates[len(candidates)-1], want)
	}

	production := filepath.Join(root, "Applications", "OneCatch.app", "Contents", "MacOS", "onecatch")
	for _, candidate := range shellBinaryCandidates(production) {
		if candidate == filepath.Join(root, "Applications", "onecatchsh") {
			t.Fatalf("production bundle searched an unrelated parent directory: %v", candidate)
		}
	}
}

func TestPrepareRemoteRequestUsesIsolatedLocalWorkspace(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(seam.DirEnv, filepath.Join(dir, "seams"))
	req, err := prepareRemoteRequest(Request{
		Workspace: "/srv/project",
		Sandbox:   SandboxWorkspaceWrite,
		Remote:    &seam.Target{Host: "devbox", Root: "/srv/project"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if req.Workspace == "/srv/project" || !strings.HasPrefix(req.Workspace, filepath.Join(dir, "seams")) {
		t.Fatalf("isolated workspace = %q", req.Workspace)
	}
	if info, err := os.Stat(req.Workspace); err != nil || !info.IsDir() {
		t.Fatalf("isolated workspace is unavailable: %v", err)
	}
	again, err := prepareRemoteRequest(Request{
		Workspace: "/some/other/local/path",
		Sandbox:   SandboxWorkspaceWrite,
		Remote:    &seam.Target{Host: "devbox", Root: "/srv/project"},
	})
	if err != nil || again.Workspace != req.Workspace {
		t.Fatalf("stable workspace = %q, %v; want %q", again.Workspace, err, req.Workspace)
	}
}

func TestPrepareRemoteRequestRejectsFullSandbox(t *testing.T) {
	_, err := prepareRemoteRequest(Request{Sandbox: SandboxFull, Remote: &seam.Target{Host: "devbox", Root: "/srv/project"}})
	if err == nil {
		t.Fatal("full sandbox was accepted for a remote FS run")
	}
}
