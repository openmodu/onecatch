package keeldesktop

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/dyike/keel/ui/el"
	desktopservice "github.com/openmodu/onecatch/internal/service/desktop"
)

func filesPanelFixture(t *testing.T) *app {
	a := alignmentFixture(t)
	a.terminal.Open = false
	a.workbench.Visible = true
	a.workbench.Tab = "files"
	a.workbench.Width = 720
	s := a.fileState(a.workspaceContext())
	s.Entries = []desktopservice.WorkspaceFileEntry{{Name: "cmd", Path: "cmd", Directory: true}, {Name: "ui", Path: "ui", Directory: true}, {Name: "README.md", Path: "README.md"}}
	s.Tree[""] = s.Entries
	s.Tree["ui"] = []desktopservice.WorkspaceFileEntry{{Name: "input.go", Path: "ui/input.go"}, {Name: "input_test.go", Path: "ui/input_test.go"}}
	s.Expanded["ui"] = true
	s.Buffers["ui/input.go"] = newFileBuffer(desktopservice.WorkspaceFileDocument{Path: "ui/input.go", Content: "package ui\n\n// Render the input field.\nfunc Render() string {\n\treturn \"Hello\"\n}\n"})
	s.Buffers["README.md"] = newFileBuffer(desktopservice.WorkspaceFileDocument{Path: "README.md", Content: "# Project\n\nExample workspace.\n"})
	s.Tabs = []string{"ui/input.go", "README.md"}
	s.Active = "ui/input.go"
	return a
}
func TestFilesPanelTabsTreeAndSearch(t *testing.T) {
	a := filesPanelFixture(t)
	s := a.fileState(a.workspaceContext())
	router, frame, click := alignmentRouter(el.Root(a), 1280, 800, 1)
	find := func(label string) {
		t.Helper()
		node, ok := alignmentNode(router.AppendSemantics(nil), label, true)
		if !ok {
			t.Fatalf("missing %s", label)
		}
		click(node.Bounds)
		frame()
		frame()
	}
	find("切换文件树")
	if !s.HideTree {
		t.Fatal("tree did not hide")
	}
	find("切换文件树")
	find("README.md")
	if s.Active != "README.md" {
		t.Fatal("file selection did not activate existing buffer")
	}
	find("关闭标签 README.md")
	if s.Active != "ui/input.go" || s.Buffers["README.md"] != nil {
		t.Fatal("closing tab did not choose adjacent tab")
	}
	s.Query = "input"
	s.SearchResult = desktopservice.WorkspaceFileSearch{Matches: []desktopservice.WorkspaceFileMatch{{Path: "ui/input.go", Line: 4}}}
	frame()
	frame()
	find("ui/input.go")
	line, _ := s.Buffers["ui/input.go"].Editor.Cursor()
	if line != 3 {
		t.Fatalf("content match jumped to %d", line)
	}
	find("查找当前文件")
	find("文件更多操作")
	find("折叠所有目录")
	if len(s.Expanded) != 0 {
		t.Fatal("collapse all failed")
	}
}
func TestFileTreeLargeDirectoryRows(t *testing.T) {
	a := filesPanelFixture(t)
	s := a.fileState(a.workspaceContext())
	for i := 0; i < 10000; i++ {
		s.Tree[""] = append(s.Tree[""], desktopservice.WorkspaceFileEntry{Path: fmt.Sprintf("file-%05d.go", i), Name: fmt.Sprintf("file-%05d.go", i)})
	}
	rows := visibleFileRows(s)
	if len(rows) != 10005 {
		t.Fatalf("flattened %d rows", len(rows))
	}
	router, _, _ := alignmentRouter(el.Root(a), 1280, 800, 1)
	if _, ok := alignmentNode(router.AppendSemantics(nil), "file-09999.go", true); ok {
		t.Fatal("offscreen file unexpectedly rendered")
	}
}
func TestRenderFilesPanelReference(t *testing.T) {
	dir := os.Getenv("ONECATCH_KEEL_PREVIEWS")
	if dir == "" {
		t.Skip("preview output not requested")
	}
	defer applyPalette(false)
	for _, dark := range []bool{false, true} {
		for _, width := range []float32{520, 820} {
			a := filesPanelFixture(t)
			a.dark = dark
			applyPalette(dark)
			a.workbench.Width = width
			if err := screenshotWithFocus(el.Root(a), 1280, 800, filepath.Join(dir, fmt.Sprintf("files-reference-%v-%.0f.png", dark, width))); err != nil {
				t.Fatal(err)
			}
			s := a.fileState(a.workspaceContext())
			s.Query = "input"
			s.SearchResult = desktopservice.WorkspaceFileSearch{Matches: []desktopservice.WorkspaceFileMatch{{Path: "ui/input.go"}, {Path: "ui/input_test.go"}}}
			if err := screenshotWithFocus(el.Root(a), 1280, 800, filepath.Join(dir, fmt.Sprintf("files-search-%v-%.0f.png", dark, width))); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestFileClosePreservesUnsavedChanges(t *testing.T) {
	a := filesPanelFixture(t)
	id := a.workspaceContext()
	s := a.fileState(id)
	buffer := s.Buffers[s.Active]
	buffer.Editor.SetValue("unsaved")
	a.closeFile(id, s.Active)
	if s.Buffers[s.Active] != buffer || !buffer.dirty() {
		t.Fatal("closing dirty file discarded content before confirmation")
	}
	version := s.Version
	a.openWorkspaceFile(id, "README.md")
	if s.Version <= version {
		t.Fatal("switching to cached file did not invalidate a pending read")
	}
}
