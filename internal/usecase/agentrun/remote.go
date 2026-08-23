package agentrun

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/openmodu/onecatch/internal/usecase/agentrun/seam"
)

// ShellBinaryEnv points at the onecatchsh binary, for development builds where
// it does not sit beside the running executable.
const ShellBinaryEnv = "ONECATCH_SHELL_BINARY"

// deniedFileTools are Claude Code's native file tools.
//
// Denying them is a safety property, not a preference. They call straight into
// Node's fs with no seam to redirect, so on a remote run they would keep
// reading and writing *this* machine while the agent believes it is operating
// on the target. An agent that reads a local file it thinks is remote reports
// confident nonsense; one that writes a local file it thinks is remote destroys
// the user's own work. The conformance suite asserts that a deny list still
// removes them from the model's tool surface; if that ever stops being true, a
// remote run must not be started at all.
var deniedFileTools = []string{"Read", "Edit", "Write", "NotebookEdit", "Glob", "Grep"}

const remoteGuidance = `This session operates on a REMOTE target through OneCatch.

Your Bash tool runs on the remote target, not on this machine. The native file
tools (Read, Edit, Write, Glob, Grep) are disabled because they would act on
this machine instead of the target, silently giving you the wrong file.

Use shell commands for all file access; they run on the target:
  read      cat -- FILE        (or sed -n 'A,Bp' FILE for a range)
  search    rg PATTERN DIR     (falls back to grep if ripgrep is absent)
  list      ls -la DIR ; find DIR -name PATTERN
  write     cat > FILE <<'EOF' ... EOF
  edit      apply a patch, or use sed -i / python3 for in-place edits

Paths are the target's own absolute paths. Do not translate them, and do not
use paths from this machine in a shell command — they do not exist there.`

// remoteSetup is everything a remote run adds to a harness launch.
type remoteSetup struct {
	env     []string
	args    []string
	cleanup func()
}

// prepareRemoteRequest gives the local harness an isolated, real working
// directory. A remote root may not exist on this machine and must never be
// created here: doing so would make a failed redirect look like a successful
// local file operation.
func prepareRemoteRequest(req Request) (Request, error) {
	if req.Remote == nil {
		return req, nil
	}
	if req.Sandbox == SandboxFull {
		return Request{}, fmt.Errorf("remote FS runs do not support the full sandbox; use read-only or workspace-write")
	}
	base, err := seam.SessionDir()
	if err != nil {
		return Request{}, err
	}
	encoded, err := json.Marshal(req.Remote)
	if err != nil {
		return Request{}, fmt.Errorf("encode remote target: %w", err)
	}
	digest := sha256.Sum256(encoded)
	workspace := filepath.Join(base, "harness-workspaces", hex.EncodeToString(digest[:12]))
	if err := os.MkdirAll(workspace, 0o700); err != nil {
		return Request{}, fmt.Errorf("create remote harness workspace: %w", err)
	}
	req.Workspace = workspace
	return req, nil
}

// setupRemoteClaude binds a run to its target and returns what Claude Code
// must be launched with to reach it.
func setupRemoteClaude(req Request) (*remoteSetup, error) {
	if req.Sandbox == SandboxReadOnly {
		// Read-only disallows Bash, and a remote run denies the file tools;
		// together they leave the agent with no way to reach the target at
		// all. Refusing beats starting a run that can do nothing and reports
		// no reason why.
		return nil, fmt.Errorf("a remote run cannot be read-only: read-only disallows Bash, " +
			"which is the only tool that reaches the target")
	}
	if runtime.GOOS == "windows" {
		// The prefix contract, the shim and the POSIX quoting all assume a
		// Unix host. A half-working seam is the worst outcome available here:
		// the harness cannot tell its shell was not redirected, so the model's
		// commands run on this machine while the agent believes otherwise.
		return nil, fmt.Errorf("remote runs are not supported from Windows yet")
	}

	shell, err := shellBinaryPath()
	if err != nil {
		return nil, err
	}

	name, err := remoteSessionName(req)
	if err != nil {
		return nil, err
	}
	session, err := seam.NewSession(name, *req.Remote)
	if err != nil {
		return nil, err
	}

	settings, err := writeDenySettings(name)
	if err != nil {
		_ = session.Remove()
		return nil, err
	}

	return &remoteSetup{
		env: []string{
			seam.SessionEnv + "=" + name,
			"CLAUDE_CODE_SHELL_PREFIX=" + shell,
		},
		args: []string{
			"--settings", settings,
			// The target's root is declared as an allowed directory so the
			// harness accepts the working directory reported back to it.
			// Without this it rejects every remote path as outside the
			// workspace and appends "Shell cwd was reset to <local dir>" to
			// every tool result — telling the model, on every single call,
			// something that is not true. A path that does not exist on this
			// machine is accepted here, which is the whole point.
			"--add-dir", req.Remote.Root,
			"--append-system-prompt", remoteGuidance,
		},
		cleanup: func() {
			_ = os.Remove(settings)
			_ = session.Remove()
			// The ssh control master is deliberately left alive. It is keyed
			// on the host and shared by every run against it, so tearing it
			// down here would drop a connection another run is using.
			// ControlPersist expires it when it is genuinely idle.
		},
	}, nil
}

// setupRemoteCodex installs a private CODEX_HOME whose environments.toml
// names onecatchsh's exec-server mode. Unlike Claude's shell-only seam this
// redirects Codex's complete tool surface, including native fs/* operations.
func setupRemoteCodex(req Request) (*remoteSetup, error) {
	if runtime.GOOS == "windows" {
		return nil, fmt.Errorf("remote Codex runs are not supported from Windows yet")
	}
	shell, err := shellBinaryPath()
	if err != nil {
		return nil, err
	}
	name, err := remoteSessionName(req)
	if err != nil {
		return nil, err
	}
	session, err := seam.NewSession(name, *req.Remote)
	if err != nil {
		return nil, err
	}
	if err := session.SetReadOnly(req.Sandbox == SandboxReadOnly); err != nil {
		_ = session.Remove()
		return nil, fmt.Errorf("configure remote sandbox: %w", err)
	}
	codexHome, err := writeRemoteCodexHome(req, name, shell)
	if err != nil {
		_ = session.Remove()
		return nil, err
	}
	return &remoteSetup{
		env: []string{
			"CODEX_HOME=" + codexHome,
			seam.SessionEnv + "=" + name,
			"ONECATCH_EXEC_WORKSPACE=" + req.Workspace,
		},
		// Remote commands are outside the local Codex sandbox. Keeping network
		// enabled avoids asking Codex for a local network proxy which this
		// stdio environment deliberately does not advertise.
		args: []string{"-c", "sandbox_workspace_write.network_access=true"},
		cleanup: func() {
			_ = session.Remove()
			// codexHome is retained under ~/.onecatch so thread/resume can read
			// the session transcript on a later workflow turn. It contains copies
			// of auth/config, never links into the user's real CODEX_HOME.
		},
	}, nil
}

func remoteSessionName(req Request) (string, error) {
	if req.RunID != "" {
		return req.RunID, nil
	}
	var data [8]byte
	if _, err := rand.Read(data[:]); err != nil {
		return "", fmt.Errorf("generate a session name: %w", err)
	}
	return "run-" + hex.EncodeToString(data[:]), nil
}

func writeRemoteCodexHome(req Request, name, shell string) (string, error) {
	base, err := seam.SessionDir()
	if err != nil {
		return "", err
	}
	destination := filepath.Join(base, "codex-home", name)
	if err := os.MkdirAll(destination, 0o700); err != nil {
		return "", fmt.Errorf("create remote Codex home: %w", err)
	}
	source := environmentValue(req.Environment, "CODEX_HOME")
	if source == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("locate Codex home: %w", err)
		}
		source = filepath.Join(home, ".codex")
	}
	if filepath.Clean(source) != filepath.Clean(destination) {
		for _, filename := range []string{"auth.json", "config.toml"} {
			data, readErr := os.ReadFile(filepath.Join(source, filename))
			if readErr != nil {
				if os.IsNotExist(readErr) {
					continue
				}
				return "", fmt.Errorf("read Codex %s: %w", filename, readErr)
			}
			if err := os.WriteFile(filepath.Join(destination, filename), data, 0o600); err != nil {
				return "", fmt.Errorf("copy Codex %s: %w", filename, err)
			}
		}
	}
	toml := seam.EnvironmentsTOML(shell, name)
	if err := os.WriteFile(filepath.Join(destination, "environments.toml"), []byte(toml), 0o600); err != nil {
		return "", fmt.Errorf("write Codex remote environment: %w", err)
	}
	return destination, nil
}

func environmentValue(environment []string, name string) string {
	if environment == nil {
		return os.Getenv(name)
	}
	for index := len(environment) - 1; index >= 0; index-- {
		key, value, found := strings.Cut(environment[index], "=")
		if found && key == name {
			return value
		}
	}
	return ""
}

// mergeEnvironment applies overrides without leaving duplicate keys in the
// child environment. When base is nil it first materializes normal inheritance.
func mergeEnvironment(base, overrides []string) []string {
	if base == nil {
		base = os.Environ()
	}
	replaced := make(map[string]bool, len(overrides))
	for _, value := range overrides {
		key, _, found := strings.Cut(value, "=")
		if found {
			replaced[key] = true
		}
	}
	result := make([]string, 0, len(base)+len(overrides))
	for _, value := range base {
		key, _, found := strings.Cut(value, "=")
		if found && replaced[key] {
			continue
		}
		result = append(result, value)
	}
	return append(result, overrides...)
}

// shellBinaryPath locates onecatchsh, which must be a real path on disk
// because the harness stats it rather than running it through a shell.
func shellBinaryPath() (string, error) {
	if p := strings.TrimSpace(os.Getenv(ShellBinaryEnv)); p != "" {
		if _, err := os.Stat(p); err != nil {
			return "", fmt.Errorf("%s=%s: %w", ShellBinaryEnv, p, err)
		}
		return p, nil
	}
	self, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locate this executable: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(self); err == nil {
		self = resolved
	}
	for _, candidate := range shellBinaryCandidates(self) {
		if info, statErr := os.Stat(candidate); statErr == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("onecatchsh was not found for %s; build it with "+
		"`go tool wails3 task build:shell`, or set %s", self, ShellBinaryEnv)
}

func shellBinaryCandidates(executable string) []string {
	dir := filepath.Dir(executable)
	candidates := []string{
		filepath.Join(dir, "onecatchsh"),
		// Packaged macOS builds keep helper executables under Resources/bin.
		filepath.Clean(filepath.Join(dir, "..", "Resources", "bin", "onecatchsh")),
	}
	// Wails places development executables under
	// bin/OneCatch.dev.app/Contents/MacOS while task build:shell writes the
	// helper directly to bin. Keep this package-external fallback dev-only.
	if strings.HasSuffix(filepath.ToSlash(dir), ".dev.app/Contents/MacOS") {
		candidates = append(candidates, filepath.Clean(filepath.Join(dir, "..", "..", "..", "onecatchsh")))
	}
	return candidates
}

// writeDenySettings emits the settings document that removes the local file
// tools from the model's surface, and returns its path.
func writeDenySettings(name string) (string, error) {
	dir, err := seam.SessionDir()
	if err != nil {
		return "", err
	}
	doc := map[string]any{
		"permissions": map[string]any{"deny": deniedFileTools},
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, name+".claude-settings.json")
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		return "", err
	}
	return path, nil
}
