package keeldesktop

import (
	"fmt"
	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"github.com/dyike/keel/ui/el"
	domainworkspaces "github.com/openmodu/onecatch/internal/domain/workspaces"
	"image"
	"runtime"
	"time"

	"strings"
	"testing"
)

func TestReviewFilterAndCommentDraft(t *testing.T) {
	files := parseGitReview("diff --git a/main.go b/main.go\n--- a/main.go\n+++ b/main.go\n@@ -1 +1 @@\n-old\n+new value\n", "worktree")
	if !reviewMatches(files[0], "MAIN.GO") || !reviewMatches(files[0], "new value") || reviewMatches(files[0], "missing") {
		t.Fatal("review filter mismatch")
	}
	a := &app{prompt: "Existing draft"}
	state := &reviewWorkspace{CommentPath: "main.go", CommentLine: 1, CommentScope: "worktree", Comment: "Handle errors", CommentKey: "line"}
	a.addReviewComment(state)
	if !strings.Contains(a.prompt, "Existing draft\n\n") || !strings.Contains(a.prompt, "main.go 第 1 行") || !strings.Contains(a.prompt, "Handle errors") || state.CommentKey != "" {
		t.Fatalf("comment draft: %q", a.prompt)
	}
}

func TestReviewPresentationTransforms(t *testing.T) {
	lines := []reviewLine{{Kind: "context", Text: "keep", Old: 1, New: 1}, {Kind: "delete", Text: "a = 1", Old: 2}, {Kind: "delete", Text: "old", Old: 3}, {Kind: "add", Text: "a=1", New: 2}, {Kind: "add", Text: "new", New: 3}, {Kind: "add", Text: "extra", New: 4}}
	pairs := pairedReviewLines(lines)
	if len(pairs) != 4 || pairs[1][0].Old != 2 || pairs[1][1].New != 2 || pairs[3][0] != nil {
		t.Fatalf("incorrect paired rows: %+v", pairs)
	}
	filtered := withoutWhitespaceChanges(lines)
	if len(filtered) != 5 || filtered[1].Kind != "context" || filtered[1].Old != 2 || filtered[1].New != 2 {
		t.Fatalf("whitespace filter lost line references: %+v", filtered)
	}
	s := &reviewWorkspace{Files: []reviewFile{{Path: "z/main.go"}, {Path: "a/main.go"}, {Path: "z/main_test.go"}}, GroupFolders: true}
	ordered := orderedReviewFiles(s)
	if ordered[0].Path != "a/main.go" || s.Files[0].Path != "z/main.go" {
		t.Fatal("grouping changed source order")
	}
	s.collapseAll(true)
	if !s.Collapsed["a/main.go"] {
		t.Fatal("collapse all")
	}
	s.collapseAll(false)
	if s.Collapsed["a/main.go"] {
		t.Fatal("expand all")
	}
}

func TestReviewMenuAndFileIndexInteraction(t *testing.T) {
	a := alignmentFixture(t)
	a.terminal.Open = false
	a.workbench.Visible = true
	a.workbench.Tab = "review"
	a.workbench.Width = 620
	s := a.reviewState(a.workspaceContext())
	s.Files = parseGitReview("diff --git a/main.go b/main.go\n--- a/main.go\n+++ b/main.go\n@@ -1 +1 @@\n-old\n+new\n", "worktree")
	root := el.Root(a)
	var router input.Router
	var ops op.Ops
	frame := func() {
		ops.Reset()
		root.Layout(layout.Context{Ops: &ops, Now: time.Now(), Source: router.Source(), Constraints: layout.Exact(image.Pt(1280, 800)), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}})
		router.Frame(&ops)
	}
	click := func(label string) {
		t.Helper()
		node, ok := alignmentNode(router.AppendSemantics(nil), label, true)
		if !ok {
			t.Fatalf("missing %q", label)
		}
		p := f32.Pt(float32(node.Bounds.Min.X+node.Bounds.Dx()/2), float32(node.Bounds.Min.Y+node.Bounds.Dy()/2))
		router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: p}, pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: p})
		frame()
		frame()
	}
	frame()
	frame()
	click("差异显示选项")
	click("并排对比")
	if !s.SideBySide {
		t.Fatal("side by side did not toggle")
	}
	click("差异显示选项")
	click("折叠所有文件")
	if !s.Collapsed["main.go"] {
		t.Fatal("collapse menu did not collapse file")
	}
	click("搜索改动文件")
	node, ok := alignmentNode(router.AppendSemantics(nil), "跳转到 main.go", true)
	if !ok || node.Bounds.Dx() < 300 {
		t.Fatalf("file search row collapsed: %v", node.Bounds)
	}
	click("跳转到 main.go")
	if s.Collapsed["main.go"] || s.FilePicker.Value() {
		t.Fatal("file index did not reveal selected file")
	}
	click("显示改动文件")
	if !s.ShowFiles {
		t.Fatal("file sidebar did not open")
	}
	a.workbench.Maximized = true
	a.ui.Preferences.Collapsed = true
	frame()
	frame()
	if runtime.GOOS == "darwin" {
		node, ok := alignmentNode(router.AppendSemantics(nil), "展开侧栏", true)
		if !ok || node.Bounds.Min.X != int(macSidebarToggleLeft) || node.Bounds.Min.Y+node.Bounds.Dy()/2 != int(mainTitleBarHeight/2) {
			t.Fatalf("maximized sidebar toggle is offset: %v", node.Bounds)
		}
	}
	click("展开侧栏")
	if a.ui.Preferences.Collapsed {
		t.Fatal("maximized header did not restore main sidebar")
	}

}

func TestReviewFileJumpReachesUnpaintedFile(t *testing.T) {
	for _, paired := range []bool{false, true} {
		a := alignmentFixture(t)
		a.terminal.Open = false
		a.workbench.Visible = true
		a.workbench.Tab = "review"
		a.workbench.Width = 620
		s := a.reviewState(a.workspaceContext())
		s.ShowFiles = true
		s.SideBySide = paired
		s.Limit = 10
		lines := make([]reviewLine, 550)
		for i := range lines {
			lines[i] = reviewLine{Kind: "add", New: i + 1, Text: "some long content that wraps when the panel is narrow"}
		}
		s.Files = []reviewFile{
			{Path: "first.go", Hunks: []reviewHunk{{Scope: "untracked", Lines: lines}}},
			{Path: "target.svg", Hunks: []reviewHunk{{Scope: "untracked", Lines: []reviewLine{{Kind: "add", New: 1, Text: "<svg/>"}}}}},
			{Path: "last.go", Hunks: []reviewHunk{{Scope: "untracked", Lines: lines[:40]}}},
		}
		router, frame, click := alignmentRouter(el.Root(a), 1280, 800, 1)
		if _, ok := alignmentNode(router.AppendSemantics(nil), "target.svg", true); ok {
			t.Fatal("target should start outside the rendered limit")
		}
		item, ok := alignmentNode(router.AppendSemantics(nil), "跳转到 target.svg", true)
		if !ok {
			t.Fatal("missing file navigation")
		}
		click(item.Bounds)
		if s.Limit != 500 {
			t.Fatalf("jump removed render bound: %d", s.Limit)
		}
		frame()
		frame()
		header, ok := alignmentNode(router.AppendSemantics(nil), "target.svg", true)
		if !ok || header.Bounds.Min.Y < 0 || header.Bounds.Max.Y > 800 || s.Jump != "" {
			t.Fatalf("paired=%v: target was not revealed: %v, found=%v, pending=%q", paired, header.Bounds, ok, s.Jump)
		}
	}
}

func TestReviewBatchBoundaries(t *testing.T) {
	files := make([]domainworkspaces.GitFile, 105)
	for i := range files {
		files[i].Path = fmt.Sprintf("file-%d", i)
	}
	for _, tc := range []struct {
		path         string
		start, count int
	}{{"", 0, 20}, {"file-19", 0, 20}, {"file-20", 20, 20}, {"file-104", 100, 5}, {"missing", 0, 20}} {
		batch := reviewBatch(files, tc.path)
		if len(batch) != tc.count || batch[0].Path != files[tc.start].Path {
			t.Fatalf("batch %s: %+v", tc.path, batch)
		}
	}
	if len(reviewBatch(nil, "")) != 0 {
		t.Fatal("empty status")
	}
}
