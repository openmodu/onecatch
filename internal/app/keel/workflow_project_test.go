package keeldesktop

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/window"
	desktopservice "github.com/openmodu/onecatch/internal/service/desktop"
)

// A workflow file in the project shows up in the library and the picker, and
// its chart follows the file as it is edited, without a manual refresh.
func TestProjectWorkflowFileRendersAndFollowsEdits(t *testing.T) {
	a := dagTestApp(t)
	root := t.TempDir()
	workspace, err := a.backend.service.AddWorkspace(context.Background(), desktopservice.AddWorkspaceInput{Path: root})
	if err != nil {
		t.Fatal(err)
	}
	a.workspaceID = workspace.ID
	a.ui.WorkflowSource = false
	dir := filepath.Join(root, ".onecatch", "workflows")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(text string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, "ship.yaml"), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("name: 发布\nsteps:\n  build:\n    agent: codex\n  check:\n    run: make test\n    after: build\n    rework: build\n")
	_, frame, _ := alignmentRouter(el.Root(workflowView{a}), 1000, 760, 1)
	// The watcher re-reads the files every couple of seconds; apply one read.
	settle := func(done func() bool) {
		t.Helper()
		items, err := a.backend.service.ListProjectWorkflows(context.Background(), workspace.ID)
		if err != nil {
			t.Fatal(err)
		}
		a.applyProjectWorkflows(workspace.ID, items)
		frame()
		if !done() {
			t.Fatalf("project workflows did not update: %+v", a.projectWorkflows.items)
		}
	}
	settle(func() bool { return len(a.projectDefinitions()) == 1 })
	d, ok := a.findDefinition("repo-ship")
	if !ok || d.Name != "发布" || len(d.Steps) != 2 {
		t.Fatalf("project workflow = %+v", d)
	}
	a.selectLocalConversation(a.runtime.Value(), "repo-ship")
	if a.workflow.Value() != "repo-ship" {
		t.Fatal("project workflow cannot be chosen for a new task")
	}
	// Edit the file: a third step appears without any action in the app.
	time.Sleep(10 * time.Millisecond)
	write("name: 发布\nsteps:\n  build:\n    agent: codex\n  check:\n    run: make test\n    after: build\n    rework: build\n  pr:\n    agent: modu\n    after: check\n")
	settle(func() bool { d, _ := a.findDefinition("repo-ship"); return len(d.Steps) == 3 })
	// Break it: the file is listed with its line, and the chosen workflow
	// falls back to a single Agent instead of pointing at nothing.
	time.Sleep(10 * time.Millisecond)
	write("name: 发布\nsteps:\n  build:\n    agnet: codex\n")
	settle(func() bool { return len(a.projectWorkflows.items) == 1 && a.projectWorkflows.items[0].ErrorLine == 4 })
	if a.workflow.Value() != "single_agent" {
		t.Fatalf("selection kept a broken workflow: %q", a.workflow.Value())
	}
}

func TestRenderProjectWorkflowLibraryPreview(t *testing.T) {
	dir := os.Getenv("ONECATCH_KEEL_PREVIEWS")
	if dir == "" {
		t.Skip("set ONECATCH_KEEL_PREVIEWS to render the project workflows")
	}
	a := dagTestApp(t)
	root := t.TempDir()
	workspace, err := a.backend.service.AddWorkspace(context.Background(), desktopservice.AddWorkspaceInput{Path: root})
	if err != nil {
		t.Fatal(err)
	}
	a.workspaceID, a.ui.WorkflowSource = workspace.ID, false
	if _, err := a.backend.service.ExportWorkflowToProject(context.Background(), workspace.ID, "design_to_pr"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".onecatch", "workflows", "broken.yaml"), []byte("name: x\nsteps:\n  a:\n    agnet: codex\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	items, err := a.backend.service.ListProjectWorkflows(context.Background(), workspace.ID)
	if err != nil {
		t.Fatal(err)
	}
	a.applyProjectWorkflows(workspace.ID, items)
	if err := window.Screenshot(el.Root(workflowView{a}), 1100, 900, filepath.Join(dir, "workflow-project-library.png")); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(root, ".onecatch", "workflows", "design_to_pr.yaml"))
	_ = os.WriteFile(filepath.Join(dir, "design_to_pr.yaml"), data, 0o644)
}
