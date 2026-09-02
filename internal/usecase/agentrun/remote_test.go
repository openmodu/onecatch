package agentrun

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/openmodu/onecatch/internal/usecase/agentrun/seam"
)

func TestSetupRemoteCodexCreatesFailClosedEnvironment(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("remote Codex is not supported on Windows")
	}
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
	skillScript := filepath.Join(sourceHome, "skills", "a-stock-data", "scripts", "query.sh")
	if err := os.MkdirAll(filepath.Dir(skillScript), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceHome, "skills", "a-stock-data", "SKILL.md"), []byte("stock instructions"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(skillScript, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	sharedReference := filepath.Join(sourceHome, "shared-reference.txt")
	if err := os.WriteFile(sharedReference, []byte("reference data"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(sharedReference, filepath.Join(sourceHome, "skills", "a-stock-data", "reference.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(sourceHome, "skills", ".system", "do-not-copy"), 0o700); err != nil {
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
	copiedScript := filepath.Join(home, "skills", "a-stock-data", "scripts", "query.sh")
	data, err := os.ReadFile(copiedScript)
	if err != nil || string(data) != "#!/bin/sh\n" {
		t.Fatalf("copied Skill script = %q, %v", data, err)
	}
	if info, err := os.Stat(copiedScript); err != nil || info.Mode().Perm() != 0o755 {
		t.Fatalf("copied Skill script mode = %v, %v", info, err)
	}
	copiedReference := filepath.Join(home, "skills", "a-stock-data", "reference.txt")
	data, err = os.ReadFile(copiedReference)
	if err != nil || string(data) != "reference data" {
		t.Fatalf("copied Skill reference = %q, %v", data, err)
	}
	if info, err := os.Lstat(copiedReference); err != nil || info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("copied Skill reference remained a symlink: %v, %v", info, err)
	}
	if _, err := os.Stat(filepath.Join(home, "skills", ".system", "do-not-copy")); !os.IsNotExist(err) {
		t.Fatalf("user system Skills were copied into managed home: %v", err)
	}
}

func TestRemoteCodexScansOriginalUserSkills(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("stub binary uses a POSIX shell script")
	}
	dir := t.TempDir()
	sourceHome := filepath.Join(dir, "source-codex")
	skillPath := filepath.Join(sourceHome, "skills", "a-stock-data", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(skillPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(skillPath, []byte("---\nname: a-stock-data\n---\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	bin := filepath.Join(dir, "codex")
	capture := filepath.Join(dir, "requests.jsonl")
	script := `#!/bin/sh
while IFS= read -r line; do
  printf '%s\n' "$line" >> "$ONECATCH_CODEX_CAPTURE"
  case "$line" in
    *'"id":1'*) printf '%s\n' '{"id":1,"result":{}}' ;;
    *'"method":"skills/list"'*)
      if [ -f "$CODEX_HOME/skills/a-stock-data/SKILL.md" ]; then
        printf '{"id":2,"result":{"data":[{"cwd":"remote","skills":[{"name":"a-stock-data","description":"A stock data","path":"%s","scope":"user","enabled":true}],"errors":[]}]}}\n' "$CODEX_HOME/skills/a-stock-data/SKILL.md"
      else
        printf '%s\n' '{"id":2,"result":{"data":[{"cwd":"remote","skills":[],"errors":[]}]}}'
      fi
      ;;
    *'"id":3'*) printf '%s\n' '{"id":3,"result":{"thread":{"id":"thread-remote-skills"}}}' ;;
    *'"id":4'*)
      printf '%s\n' '{"id":4,"result":{"turn":{"id":"turn-remote-skills","status":"inProgress"}}}'
      printf '%s\n' '{"method":"item/completed","params":{"threadId":"thread-remote-skills","turnId":"turn-remote-skills","item":{"id":"message-1","type":"agentMessage","text":"ok"}}}'
      printf '%s\n' '{"method":"turn/completed","params":{"threadId":"thread-remote-skills","turn":{"id":"turn-remote-skills","status":"completed"}}}'
      ;;
  esac
done
`
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv(seam.DirEnv, filepath.Join(dir, "seams"))
	t.Setenv(ShellBinaryEnv, "/bin/sh")
	result, err := NewCodexRunner(bin).Run(context.Background(), Request{
		RunID: "remote-skills", Workspace: dir, Prompt: "$a-stock-data summarize yesterday",
		Sandbox: SandboxWorkspaceWrite,
		Remote:  &seam.Target{Host: "devbox", Root: "/srv/project"},
		Environment: append(os.Environ(),
			"CODEX_HOME="+sourceHome,
			"ONECATCH_CODEX_CAPTURE="+capture,
		),
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.FinalMessage != "ok" {
		t.Fatalf("result = %+v", result)
	}
	payload, err := os.ReadFile(capture)
	if err != nil {
		t.Fatal(err)
	}
	value := string(payload)
	syncedSkillPath := filepath.Join(dir, "seams", "codex-home", "remote-skills", "skills", "a-stock-data", "SKILL.md")
	for _, expected := range []string{
		`"forceReload":true`,
		`"developerInstructions":"This Codex session operates on a REMOTE target through OneCatch.`,
		`Treat that injected content as the required full read of\nthe Skill. Do not call a shell or filesystem tool to re-read its SKILL.md path.`,
		`{"name":"a-stock-data","path":"` + syncedSkillPath + `","type":"skill"}`,
	} {
		if !strings.Contains(value, expected) {
			t.Fatalf("app-server requests missing %s: %s", expected, value)
		}
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
