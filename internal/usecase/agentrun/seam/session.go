package seam

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// DirEnv overrides where session files live, for tests.
const DirEnv = "ONECATCH_SEAM_DIR"

// Target describes the machine a run's shell commands execute on.
type Target struct {
	// Host is anything OpenSSH understands, including a ~/.ssh/config alias.
	// Destination parsing is deliberately delegated to ssh so ProxyJump,
	// IdentityFile, Match blocks and hardware tokens keep working untouched —
	// the same reason the SFTP backend shells out rather than speaking SSH
	// itself.
	Host string `json:"host"`
	// Root is the absolute directory on the target the run works in.
	Root string `json:"root"`
	// Username overrides the user resolved from OpenSSH configuration.
	Username string `json:"username,omitempty"`
	// CredentialID is an opaque reference into the operating system credential
	// store. The password itself is never persisted in the session.
	CredentialID string `json:"credential_id,omitempty"`
	// SSHOptions are extra -o options, matching remotefs.SFTPConfig.
	SSHOptions []string `json:"ssh_options,omitempty"`
	// SSHBinary overrides the ssh executable. Empty means "ssh".
	SSHBinary string `json:"ssh_binary,omitempty"`
	// AskPassBinary overrides onecatch-askpass discovery for tests.
	AskPassBinary string `json:"askpass_binary,omitempty"`
}

func (t Target) String() string {
	if t.Host == "" {
		return "(local)"
	}
	destination := t.Host
	if t.Username != "" {
		destination = t.Username + "@" + destination
	}
	return "ssh://" + destination + t.Root
}

// Session is the state one agent run's shell invocations share.
//
// It lives in a file rather than in memory because there is no long-lived
// process to hold it: the harness starts a fresh shell-prefix process for
// every single tool call, and the only thing connecting them is this file.
// The working directory in particular has nowhere else to live — without it,
// `cd` stops persisting between the agent's commands and every command runs
// in the run's root again.
type Session struct {
	// Name identifies the session; it is the run id in practice.
	Name string `json:"name"`
	// Target is where the model's commands execute.
	Target Target `json:"target"`
	// Cwd is the working directory on the target for the next command.
	Cwd string `json:"cwd"`
	// ReadOnly is enforced by the exec-server itself. Relying on a harness-side
	// sandbox would be unsafe because the target process is outside that local
	// sandbox's kernel boundary.
	ReadOnly bool `json:"read_only,omitempty"`

	path string
}

// SetReadOnly persists the target-side permission boundary for Codex.
func (s *Session) SetReadOnly(readOnly bool) error {
	if s.ReadOnly == readOnly {
		return nil
	}
	s.ReadOnly = readOnly
	return s.save()
}

var sessionNameRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

// SessionDir is where session files live.
func SessionDir() (string, error) {
	base := os.Getenv(DirEnv)
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("locate home directory: %w", err)
		}
		base = filepath.Join(home, ".onecatch", "seam")
	}
	if err := os.MkdirAll(base, 0o700); err != nil {
		return "", fmt.Errorf("create %s: %w", base, err)
	}
	return base, nil
}

// NewSession creates and persists a session bound to a target.
func NewSession(name string, target Target) (*Session, error) {
	if !sessionNameRE.MatchString(name) {
		return nil, fmt.Errorf("session name %q must be alphanumeric with . _ -", name)
	}
	if target.Host != "" && !strings.HasPrefix(target.Root, "/") {
		return nil, fmt.Errorf("target root %q must be absolute", target.Root)
	}
	dir, err := SessionDir()
	if err != nil {
		return nil, err
	}
	s := &Session{
		Name:   name,
		Target: target,
		Cwd:    target.Root,
		path:   filepath.Join(dir, name+".json"),
	}
	return s, s.save()
}

// LoadSession reads a session by name.
func LoadSession(name string) (*Session, error) {
	if !sessionNameRE.MatchString(name) {
		return nil, fmt.Errorf("session name %q must be alphanumeric with . _ -", name)
	}
	dir, err := SessionDir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, name+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read session %s: %w", name, err)
	}
	var s Session
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse session %s: %w", name, err)
	}
	s.path = path
	return &s, nil
}

// Remove deletes the session file.
func (s *Session) Remove() error {
	if s.path == "" {
		return nil
	}
	if err := os.Remove(s.path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// SetCwd records the working directory the next command starts in.
//
// The write is atomic. Tool calls can overlap — the harness fans several out
// at once — and a torn session file read by the next invocation would leave it
// with no target at all.
func (s *Session) SetCwd(dir string) error {
	if dir == "" || dir == s.Cwd {
		return nil
	}
	s.Cwd = dir
	return s.save()
}

func (s *Session) save() error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".seam-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer func() { _ = os.Remove(name) }()
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(name, 0o600); err != nil {
		return err
	}
	return os.Rename(name, s.path)
}

// SessionEnv names the run whose target a shell invocation belongs to.
//
// It is how a shell-prefix process, which is started fresh for every single
// tool call and knows nothing else about the run, finds its target.
const SessionEnv = "ONECATCH_SEAM_SESSION"
