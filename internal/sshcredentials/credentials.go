// Package sshcredentials keeps Remote FS passwords in the operating system's
// credential store and configures OpenSSH to retrieve them through askpass.
package sshcredentials

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	keyring "github.com/zalando/go-keyring"
)

const (
	keyringService = "app.onecatch.remote-fs"

	// CredentialEnv names the opaque keyring entry the askpass helper reads.
	// It is deliberately an identifier, never the password itself.
	CredentialEnv = "ONECATCH_SSH_CREDENTIAL_ID"
	// AskPassBinaryEnv overrides helper discovery for development and tests.
	AskPassBinaryEnv = "ONECATCH_SSH_ASKPASS"
)

var ErrInvalidID = errors.New("invalid SSH credential identifier")

// Store is the narrow credential-store surface needed by the desktop service.
// Keeping it injectable lets tests prove that secrets never enter workspace
// persistence without touching the user's real keychain.
type Store interface {
	Set(id, password string) error
	Delete(id string) error
}

type KeyringStore struct{}

func NewID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("generate SSH credential identifier: %w", err)
	}
	return "sshcred_" + hex.EncodeToString(value[:]), nil
}

func ValidID(id string) bool {
	if !strings.HasPrefix(id, "sshcred_") || len(id) != len("sshcred_")+32 {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(id, "sshcred_"))
	return err == nil
}

func (KeyringStore) Set(id, password string) error {
	if !ValidID(id) {
		return ErrInvalidID
	}
	if password == "" {
		return errors.New("SSH password is empty")
	}
	if err := keyring.Set(keyringService, id, password); err != nil {
		return fmt.Errorf("save SSH password in system credential store: %w", err)
	}
	return nil
}

func (KeyringStore) Delete(id string) error {
	if !ValidID(id) {
		return ErrInvalidID
	}
	if err := keyring.Delete(keyringService, id); err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return fmt.Errorf("delete SSH password from system credential store: %w", err)
	}
	return nil
}

func Lookup(id string) (string, error) {
	if !ValidID(id) {
		return "", ErrInvalidID
	}
	password, err := keyring.Get(keyringService, id)
	if err != nil {
		return "", fmt.Errorf("read SSH password from system credential store: %w", err)
	}
	return password, nil
}

// ConfigureCommand makes OpenSSH invoke OneCatch's askpass helper. The
// environment carries only an opaque keyring ID; the password never appears in
// argv, the process environment, workspace JSON, or logs.
func ConfigureCommand(cmd *exec.Cmd, credentialID, askPassOverride string) error {
	if credentialID == "" {
		return nil
	}
	if !ValidID(credentialID) {
		return ErrInvalidID
	}
	askPass := strings.TrimSpace(askPassOverride)
	if askPass == "" {
		var err error
		askPass, err = AskPassPath()
		if err != nil {
			return err
		}
	}
	if info, err := os.Stat(askPass); err != nil || info.IsDir() {
		if err == nil {
			err = errors.New("path is a directory")
		}
		return fmt.Errorf("locate SSH askpass helper %q: %w", askPass, err)
	}
	cmd.Env = mergeEnvironment(cmd.Env, []string{
		"SSH_ASKPASS=" + askPass,
		"SSH_ASKPASS_REQUIRE=force",
		"DISPLAY=onecatch",
		"LC_ALL=C",
		CredentialEnv + "=" + credentialID,
	})
	return nil
}

func AskPassPath() (string, error) {
	if configured := strings.TrimSpace(os.Getenv(AskPassBinaryEnv)); configured != "" {
		return configured, nil
	}
	self, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locate OneCatch executable: %w", err)
	}
	if resolved, resolveErr := filepath.EvalSymlinks(self); resolveErr == nil {
		self = resolved
	}
	name := "onecatch-askpass"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	candidates := []string{
		filepath.Join(filepath.Dir(self), name),
		filepath.Clean(filepath.Join(filepath.Dir(self), "..", "Resources", "bin", name)),
	}
	for _, candidate := range candidates {
		if info, statErr := os.Stat(candidate); statErr == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("onecatch-askpass was not found beside %s; build it with `task build:askpass`, or set %s", self, AskPassBinaryEnv)
}

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
