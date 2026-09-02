package agentrun

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openmodu/onecatch/internal/usecase/agentrun/seam"
)

func TestWriteClaudeMirrorSettings(t *testing.T) {
	t.Setenv(seam.DirEnv, t.TempDir())
	shell := filepath.Join(t.TempDir(), "one $catch's sh")
	settings, err := writeClaudeMirrorSettings("mirror-settings", shell)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(settings)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Permissions struct {
			Deny []string `json:"deny"`
		} `json:"permissions"`
		Hooks map[string][]struct {
			Matcher string `json:"matcher"`
			Hooks   []struct {
				Command string `json:"command"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if strings.Join(doc.Permissions.Deny, ",") != "Grep,Glob" {
		t.Fatalf("denied tools = %v", doc.Permissions.Deny)
	}
	if got := doc.Hooks["PreToolUse"][0].Matcher; got != "Read|Write|Edit|NotebookEdit|Grep|Glob" {
		t.Fatalf("pre-tool matcher = %q", got)
	}
	command := doc.Hooks["PostToolUse"][0].Hooks[0].Command
	if command != quotePOSIXCommandWord(shell)+` claude-hook` {
		t.Fatalf("hook command = %q", command)
	}
}
