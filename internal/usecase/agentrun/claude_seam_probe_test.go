package agentrun

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/openmodu/onecatch/internal/usecase/agentrun/seam"
)

func TestClassifyClaudeSeamProbe(t *testing.T) {
	local := filepath.Join(t.TempDir(), "local")
	target := filepath.Join(t.TempDir(), "target")
	for _, test := range []struct {
		name           string
		output         string
		observed       bool
		runErr         error
		pass, bypassed bool
	}{
		{name: "remote", output: target + "\n", observed: true, pass: true},
		{name: "local bypass", output: local + "\n", observed: true, bypassed: true},
		{name: "unknown", output: "/somewhere/else\n", observed: true},
		{name: "did not start", runErr: errors.New("boom")},
	} {
		t.Run(test.name, func(t *testing.T) {
			pass, bypassed, _ := classifyClaudeSeamProbe(local, target, test.output, test.observed, test.runErr, nil, "")
			if pass != test.pass || bypassed != test.bypassed {
				t.Fatalf("pass=%v bypassed=%v, want %v/%v", pass, bypassed, test.pass, test.bypassed)
			}
		})
	}
}

func TestClaudeSeamVerdictCache(t *testing.T) {
	t.Setenv(seam.DirEnv, t.TempDir())
	path, err := claudeSeamVerdictPath("/opt/claude", "2.1.233")
	if err != nil {
		t.Fatal(err)
	}
	want := claudeSeamVerdict{Version: "2.1.233", Pass: true}
	if err := saveClaudeSeamVerdict(path, want); err != nil {
		t.Fatal(err)
	}
	got, ok := loadClaudeSeamVerdict(path, want.Version)
	if !ok || got != want {
		t.Fatalf("verdict = %+v, %v", got, ok)
	}
	if _, ok := loadClaudeSeamVerdict(path, "2.1.234"); ok {
		t.Fatal("verdict for another version was reused")
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("cache mode = %v, %v", info, err)
	}
}

func TestScrubClaudeProbeEnvironment(t *testing.T) {
	environment := scrubClaudeProbeEnvironment([]string{
		"PATH=/bin", "ANTHROPIC_API_KEY=real", "OPENAI_API_KEY=real", "SERVICE_TOKEN=real", "KEEP=yes",
	})
	if environmentValue(environment, "PATH") != "/bin" || environmentValue(environment, "KEEP") != "yes" {
		t.Fatalf("safe environment was removed: %v", environment)
	}
	for _, secret := range []string{"ANTHROPIC_API_KEY", "OPENAI_API_KEY", "SERVICE_TOKEN"} {
		if environmentValue(environment, secret) != "" {
			t.Fatalf("%s survived probe scrubbing", secret)
		}
	}
}
