//go:build conformance

package desktop

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	domainsettings "github.com/openmodu/onecatch/internal/domain/settings"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
	"github.com/openmodu/onecatch/internal/usecase/agentrun/seam/seamtest"
)

// Run the installed Codex CLI against a local scripted model. This verifies
// actual writer lease release, including when PATH points to the npm wrapper,
// without using the signed-in account or spending model quota.
func TestCodexSettingsResumeConformance(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires executable symlinks")
	}
	binary, err := exec.LookPath("codex")
	if err != nil {
		t.Skip("Codex CLI is not installed")
	}
	dir := t.TempDir()
	home := filepath.Join(dir, "codex-home")
	if err := os.Mkdir(home, 0o700); err != nil {
		t.Fatal(err)
	}
	mock := seamtest.StartMock(seamtest.DialectResponses, "printf settings-resume-ok")
	defer mock.Close()
	config := fmt.Sprintf(`model = "seam-mock"
model_provider = "seam"
[model_providers.seam]
name = "seam"
base_url = %q
wire_api = "responses"
requires_openai_auth = false
supports_websockets = false
`, mock.BaseURL())
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	alternate := filepath.Join(dir, "codex-new")
	if err := os.Symlink(binary, alternate); err != nil {
		t.Fatal(err)
	}
	registry, err := NewRuntimeRegistry(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer registry.Close()
	settings := map[string]domainsettings.RuntimeSettings{"codex": {Binary: binary}}
	registry.ApplySettings(settings, 2)
	request := agentrun.Request{
		Runtime: agentrun.RuntimeCodex, RunID: "settings-conformance", Workspace: dir,
		Prompt: "Run the requested command.", Sandbox: agentrun.SandboxFull,
		Environment: append(os.Environ(), "CODEX_HOME="+home),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	for turn := 0; turn < 4; turn++ {
		if turn == 1 {
			registry.ApplySettings(settings, 3)
		}
		if turn == 2 {
			settings["codex"] = domainsettings.RuntimeSettings{Binary: alternate}
			registry.ApplySettings(settings, 3)
		}
		changed := false
		result, err := registry.Run(ctx, request, func(event agentrun.Event) {
			if turn == 3 && !changed && event.Kind == agentrun.KindMessage {
				changed = true
				settings["codex"] = domainsettings.RuntimeSettings{Binary: binary}
				registry.ApplySettings(settings, 3)
			}
		})
		if err != nil || !result.Succeeded || result.SessionID == "" {
			t.Fatalf("turn %d: %+v, %v", turn, result, err)
		}
		if request.ResumeSessionID != "" && result.SessionID != request.ResumeSessionID {
			t.Fatal("settings change lost the original thread")
		}
		if turn == 3 && !changed {
			t.Fatal("settings were not changed during the active turn")
		}
		request.ResumeSessionID = result.SessionID
	}
	// The prior turn changed settings before completion; its retired runner
	// must have released the lease by the time the next turn begins.
	if result, err := registry.Run(ctx, request, nil); err != nil || !result.Succeeded {
		t.Fatalf("resume after in-flight replacement: %+v, %v", result, err)
	}
}
