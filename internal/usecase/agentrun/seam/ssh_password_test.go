package seam

import (
	"strings"
	"testing"
)

func TestSSHExecutorUsesAskPassPasswordAuthentication(t *testing.T) {
	target := Target{
		Host: "devbox", Username: "deploy",
		CredentialID: "sshcred_0123456789abcdef0123456789abcdef",
	}
	joined := strings.Join((&sshExecutor{target: target}).sshArgs(0), " ")
	for _, expected := range []string{
		"BatchMode=no", "PubkeyAuthentication=no", "PreferredAuthentications=password",
		"NumberOfPasswordPrompts=1", "-l deploy",
	} {
		if !strings.Contains(joined, expected) {
			t.Errorf("SSH arguments do not contain %q: %s", expected, joined)
		}
	}
	if strings.Contains(joined, target.CredentialID) {
		t.Fatalf("credential identifier leaked into SSH argv: %s", joined)
	}
}

func TestSSHExecutorUsesExplicitPort(t *testing.T) {
	t.Parallel()
	joined := strings.Join((&sshExecutor{target: Target{Host: "devbox:2222"}}).sshArgs(2222), " ")
	if !strings.Contains(joined, "-p 2222") {
		t.Fatalf("SSH arguments do not contain the explicit port: %s", joined)
	}
}
