package terminal

import (
	"slices"
	"strings"
	"testing"

	domainworkspaces "github.com/openmodu/onecatch/internal/domain/workspaces"
)

func TestRemoteSSHInvocationUsesConfiguredTargetAndWorkspace(t *testing.T) {
	binary, arguments, environment, label, err := remoteSSHInvocation(CreateInput{RemoteFS: &domainworkspaces.RemoteFS{
		Host:       "devbox:2222",
		Root:       "/srv/project with space",
		Username:   "deploy",
		SSHOptions: []string{"IdentityFile=/tmp/test-key"},
	}}, []string{"PATH=/usr/bin"})
	if err != nil {
		t.Fatal(err)
	}
	if binary != "ssh" || label != "deploy@devbox:2222" {
		t.Fatalf("binary=%q label=%q", binary, label)
	}
	for _, expected := range []string{"BatchMode=yes", "2222", "deploy", "devbox", "IdentityFile=/tmp/test-key"} {
		if !slices.Contains(arguments, expected) {
			t.Fatalf("arguments %q do not contain %q", arguments, expected)
		}
	}
	command := arguments[len(arguments)-1]
	if !strings.Contains(command, `cd '/srv/project with space'`) || !strings.Contains(command, `exec "${SHELL:-/bin/sh}" -l`) {
		t.Fatalf("remote command = %q", command)
	}
	if !slices.Contains(environment, "PATH=/usr/bin") {
		t.Fatalf("environment = %q", environment)
	}
}
