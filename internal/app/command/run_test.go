package command

import (
	"os"
	"os/exec"
	"testing"

	"github.com/openmodu/onecatch/internal/processmode"
	"github.com/openmodu/onecatch/internal/sshcredentials"
)

func TestConsumedRoleDoesNotLeakAndAskpassReusesExecutable(t *testing.T) {
	t.Setenv(processmode.Env, "")
	t.Setenv(sshcredentials.AskPassBinaryEnv, "")
	previous := os.Args
	os.Args = []string{"onecatch"}
	defer func() { os.Args = previous }()
	if Run() {
		t.Fatal("normal launch was consumed")
	}
	if _, exists := os.LookupEnv(processmode.Env); exists {
		t.Fatal("role marker was not removed")
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	path, err := sshcredentials.AskPassPath()
	if err != nil || path != self {
		t.Fatalf("askpass=%q, %v; want self=%q", path, err, self)
	}
	id, err := sshcredentials.NewID()
	if err != nil {
		t.Fatal(err)
	}
	child := exec.Command("ssh")
	child.Env = []string{processmode.Env + "=shell"}
	if err := sshcredentials.ConfigureCommand(child, id, ""); err != nil {
		t.Fatal(err)
	}
	roles := 0
	for _, value := range child.Env {
		if value == processmode.Env+"=shell" {
			t.Fatal("shell role leaked into SSH askpass")
		}
		if value == processmode.Env+"=askpass" {
			roles++
		}
	}
	if roles != 1 {
		t.Fatalf("askpass roles=%d, env=%v", roles, child.Env)
	}
}
