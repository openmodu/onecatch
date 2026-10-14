package sshcredentials

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewIDProducesOpaqueValidatedIdentifier(t *testing.T) {
	first, err := NewID()
	if err != nil || !ValidID(first) {
		t.Fatalf("NewID() = %q, %v", first, err)
	}
	second, err := NewID()
	if err != nil || first == second {
		t.Fatalf("second NewID() = %q, %v", second, err)
	}
	if ValidID("../../password") {
		t.Fatal("path-like credential identifier was accepted")
	}
}

func TestConfigureCommandExposesOnlyCredentialIdentifier(t *testing.T) {
	helper := filepath.Join(t.TempDir(), "onecatch-askpass")
	if err := os.WriteFile(helper, []byte("helper"), 0o700); err != nil {
		t.Fatal(err)
	}
	id, err := NewID()
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command("ssh")
	command.Env = []string{"KEEP=yes", "DISPLAY=old"}
	if err := ConfigureCommand(command, id, helper); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(command.Env, "\n")
	for _, expected := range []string{"KEEP=yes", "SSH_ASKPASS=" + helper, "SSH_ASKPASS_REQUIRE=force", "DISPLAY=onecatch", "LC_ALL=C", CredentialEnv + "=" + id} {
		if !strings.Contains(joined, expected) {
			t.Errorf("environment does not contain %q: %v", expected, command.Env)
		}
	}
	if strings.Count(joined, "DISPLAY=") != 1 {
		t.Fatalf("DISPLAY was not replaced: %v", command.Env)
	}
}

func TestAskPassCandidatesFindDevelopmentBuild(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	executable := filepath.Join(root, "bin", "OneCatch.dev.app", "Contents", "MacOS", "onecatch")
	want := filepath.Join(root, "bin", "onecatch-askpass")
	candidates := askPassCandidates(executable, "onecatch-askpass")
	if candidates[len(candidates)-1] != want {
		t.Fatalf("development helper candidate = %q, want %q", candidates[len(candidates)-1], want)
	}

	production := filepath.Join(root, "Applications", "OneCatch.app", "Contents", "MacOS", "onecatch")
	for _, candidate := range askPassCandidates(production, "onecatch-askpass") {
		if candidate == filepath.Join(root, "Applications", "onecatch-askpass") {
			t.Fatalf("production bundle searched an unrelated parent directory: %v", candidate)
		}
	}
}
