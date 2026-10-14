package keeldesktop

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestNavigationHistoryBranching(t *testing.T) {
	var h navigationHistory
	start := location{Workspace: "project"}
	workflow := location{Page: "workflows", Workspace: "project", Workflow: "loop"}
	editor := workflow
	editor.Source = true
	h.push(start, workflow)
	h.push(workflow, editor)
	if target, ok := h.move(-1); !ok || target != workflow {
		t.Fatalf("back: %+v %v", target, ok)
	}
	if target, ok := h.move(1); !ok || target != editor {
		t.Fatalf("forward: %+v %v", target, ok)
	}
	h.move(-1)
	next := location{Page: "usage", Workspace: "project"}
	h.push(workflow, next)
	if _, ok := h.move(1); ok {
		t.Fatal("forward history should be discarded after a new navigation")
	}
	h.push(next, next)
	if len(h.Entries) != 3 {
		t.Fatal("duplicate navigation appended")
	}
	h.move(-1)
	h.move(-1)
	if _, ok := h.move(-1); ok {
		t.Fatal("back crossed first entry")
	}
}
func TestUIPreferencesRoundTripAndCorruption(t *testing.T) {
	root := t.TempDir()
	p, err := loadUIPreferences(root)
	if err != nil || p.SidebarWidth != 244 || !reflect.DeepEqual(p.Navigation, []string{"templates", "skills"}) {
		t.Fatalf("defaults: %+v %v", p, err)
	}
	p.Navigation = []string{"usage", "bogus", "usage", "templates"}
	p.SidebarWidth = 900
	p.Theme = "dark"
	p.Collapsed = true
	if err := writeUIPreferences(root, p); err != nil {
		t.Fatal(err)
	}
	got, err := loadUIPreferences(root)
	if err != nil || got.SidebarWidth != 480 || !got.Collapsed || got.Theme != "dark" || !reflect.DeepEqual(got.Navigation, []string{"templates", "usage"}) {
		t.Fatalf("round trip: %+v %v", got, err)
	}
	path := filepath.Join(root, "keel-ui.json")
	if err := os.WriteFile(path, []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadUIPreferences(root); err == nil {
		t.Fatal("corruption must be reported")
	}
}
