package desktop

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	domainsettings "github.com/openmodu/onecatch/internal/domain/settings"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
)

// The writer belongs to the process, not the current turn. A second engine
// cannot resume the same thread until the previous process has exited.
func TestRuntimeSettingsPreserveCodexConversation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("stub uses a POSIX shell")
	}
	for _, scenario := range []string{"preferences", "binary", "update", "during-turn"} {
		t.Run(scenario, func(t *testing.T) {
			dir := t.TempDir()
			binary := filepath.Join(dir, "codex")
			script := `#!/bin/sh
if [ "$1" = --version ]; then echo 'codex-cli 0.1.0'; exit; fi
owns_writer=0
trap 'if [ "$owns_writer" = 1 ]; then rmdir "$ONECATCH_TEST_WRITER"; fi' EXIT
trap 'exit 0' INT TERM
while IFS= read -r line; do
  case "$line" in
    *'"method":"initialize"'*) echo '{"id":1,"result":{}}' ;;
    *'"method":"skills/list"'*) echo '{"id":2,"result":{"data":[]}}' ;;
    *'"id":3'*)
      if mkdir "$ONECATCH_TEST_WRITER" 2>/dev/null; then
        owns_writer=1
        echo '{"id":3,"result":{"thread":{"id":"settings-thread"}}}'
      else
        echo '{"id":3,"error":{"code":-32600,"message":"thread settings-thread already has an active writer"}}'
      fi ;;
    *'"method":"turn/start"'*)
      echo '{"id":4,"result":{"turn":{"id":"turn"}}}'
      echo '{"method":"item/completed","params":{"threadId":"settings-thread","turnId":"turn","item":{"id":"reply","type":"agentMessage","text":"ok"}}}'
      echo '{"method":"turn/completed","params":{"threadId":"settings-thread","turn":{"id":"turn","status":"completed"}}}' ;;
  esac
done
`
			for _, name := range []string{binary, binary + "-new"} {
				if err := os.WriteFile(name, []byte(script), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			registry, err := NewRuntimeRegistry(dir)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = registry.Close() })
			settings := map[string]domainsettings.RuntimeSettings{"codex": {Binary: binary}}
			registry.ApplySettings(settings, 1)
			initial := registry.engine
			t.Cleanup(func() { _ = initial.Close() })
			request := agentrun.Request{Runtime: agentrun.RuntimeCodex, RunID: "settings-run", Workspace: dir, Prompt: "first", Sandbox: agentrun.SandboxFull, Environment: append(os.Environ(), "ONECATCH_TEST_WRITER="+filepath.Join(dir, "writer"))}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			changeSettings := func() {
				if scenario == "update" {
					if _, err := registry.Update(RuntimeConfigInput{Runtime: "codex", Binary: binary + "-new"}); err != nil {
						t.Fatal(err)
					}
					return
				}
				if scenario == "preferences" {
					item := settings["codex"]
					item.ReasoningEffort = "high"
					settings["codex"] = item
				} else {
					settings["codex"] = domainsettings.RuntimeSettings{Binary: binary + "-new"}
				}
				registry.ApplySettings(settings, 2)
			}
			first, err := registry.Run(ctx, request, func(event agentrun.Event) {
				if scenario == "during-turn" && event.Kind == agentrun.KindMessage {
					changeSettings()
				}
			})
			if err != nil || !first.Succeeded {
				t.Fatalf("first turn = %+v, %v", first, err)
			}
			if scenario != "during-turn" {
				changeSettings()
			}
			request.ResumeSessionID, request.Prompt = first.SessionID, "continue"
			second, err := registry.Run(ctx, request, nil)
			if err != nil || !second.Succeeded || second.SessionID != first.SessionID {
				t.Fatalf("follow-up after settings save = %+v, %v", second, err)
			}
			if scenario == "preferences" && registry.engine != initial {
				t.Fatal("preference-only save discarded the warm engine")
			}
			if scenario != "preferences" && !strings.HasSuffix(registry.engine.Runner(agentrun.RuntimeCodex).(*agentrun.CodexRunner).Executable(), "-new") {
				t.Fatal("changed binary did not take effect")
			}
		})
	}
}
