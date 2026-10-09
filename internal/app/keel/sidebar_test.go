package keeldesktop

import (
	"image"
	"os"
	"path/filepath"
	"testing"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/op"
	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/locale"
	"github.com/dyike/keel/ui/window"
	domaintasks "github.com/openmodu/onecatch/internal/domain/tasks"
)

func TestKeelSidebarNativeProjectActionsAndExpansion(t *testing.T) {
	applyPalette(false)
	for _, scale := range []int{1, 2} {
		a := conversationFixture(t)
		router, frame, click := alignmentRouter(el.Root(a), 1120, 900, scale)
		find := func(label string) image.Rectangle {
			t.Helper()
			box, ok := findSkillControl(router.AppendSemantics(nil), label)
			if !ok {
				t.Fatalf("missing %q at %dx", label, scale)
			}
			return box
		}
		project := find("modu")
		if project.Dy() != 36*scale {
			t.Fatalf("not the standard Keel row: %v", project)
		}
		plus, actions := find("新建任务"), find("项目操作")
		// Full-height project padding toggles the branch. Suffix controls remain
		// reachable when the pointer moves off the label, with stable geometry.
		p := f32.Pt(float32(project.Min.X+2*scale), float32(project.Min.Y+scale))
		router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: p})
		frame()
		router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: p})
		frame()
		frame()
		if a.ui.SidebarNav.Expanded("project/modu") || a.ui.Expanded["modu"] {
			t.Fatal("project did not collapse")
		}
		for _, box := range []image.Rectangle{project, plus, actions} {
			router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: f32.Pt(float32(box.Min.X+box.Dx()/2), float32(box.Min.Y+box.Dy()/2))})
			frame()
			frame()
			if find("项目操作") != actions || find("新建任务") != plus {
				t.Fatal("suffix moved/disappeared on hover")
			}
		}
		click(actions)
		if !a.projectMenu(a.workspaces[0]).Value() || a.ui.SidebarNav.Expanded("project/modu") {
			t.Fatal("project menu toggled the branch")
		}
		find("编辑项目…")
		router.Queue(key.Event{Name: key.NameEscape, State: key.Press}, key.Event{Name: key.NameEscape, State: key.Release})
		frame()
		frame()
		if a.projectMenu(a.workspaces[0]).Value() {
			t.Fatal("Escape did not dismiss project menu")
		}
		// Keel also owns right-click menus and keyboard branch navigation.
		p = f32.Pt(float32(project.Min.X+project.Dx()/2), float32(project.Min.Y+project.Dy()/2))
		router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonSecondary, Position: p})
		frame()
		router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: p})
		frame()
		frame()
		if !a.projectMenu(a.workspaces[0]).Value() {
			t.Fatal("right-click did not open Keel context menu")
		}
		router.Queue(key.Event{Name: key.NameEscape, State: key.Press}, key.Event{Name: key.NameEscape, State: key.Release})
		frame()
		frame()
		click(project)
		router.Queue(key.Event{Name: key.NameLeftArrow, State: key.Press}, key.Event{Name: key.NameLeftArrow, State: key.Release})
		frame()
		frame()
		router.Queue(key.Event{Name: key.NameRightArrow, State: key.Press}, key.Event{Name: key.NameRightArrow, State: key.Release})
		frame()
		frame()
		if !a.ui.SidebarNav.Expanded("project/modu") {
			t.Fatal("Right did not expand focused project")
		}
		router.Queue(key.Event{Name: key.NameLeftArrow, State: key.Press}, key.Event{Name: key.NameLeftArrow, State: key.Release})
		frame()
		frame()
		if a.ui.SidebarNav.Expanded("project/modu") {
			t.Fatal("Left did not collapse project")
		}
		click(plus)
		if a.task != nil || a.workspaceID != "modu" {
			t.Fatal("plus did not create a draft independently")
		}
		click(find("modu"))
		click(find("modu"))
		if !a.ui.SidebarNav.Expanded("project/modu") {
			t.Fatal("project did not expand")
		}
		click(find("查询北京未来7天天气"))
		if a.taskID != "weather" {
			t.Fatal("task selection not wired to application")
		}
	}
}

func TestKeelSidebarSearchShowAllAndModelRefresh(t *testing.T) {
	a := conversationFixture(t)
	a.tasks["modu"] = append(a.tasks["modu"], domaintasks.Task{ID: "extra", Title: "Fourth task", WorkspaceID: "modu", Prompt: "unique search phrase"})
	router, frame, click := alignmentRouter(el.Root(a), 1120, 900, 1)
	box, ok := findSkillControl(router.AppendSemantics(nil), "展开显示")
	if !ok {
		t.Fatal("missing show-all command")
	}
	view := a.ui.SidebarNav
	click(box)
	if !a.ui.ShowAll["modu"] || a.ui.SidebarNav != view {
		t.Fatal("show-all rebuilt component identity")
	}
	if _, ok := findSkillControl(router.AppendSemantics(nil), "Fourth task"); !ok {
		t.Fatal("show-all omitted task")
	}
	a.query = "unique search phrase"
	frame()
	frame()
	if _, ok := findSkillControl(router.AppendSemantics(nil), "Fourth task"); !ok {
		t.Fatal("prompt search omitted match")
	}
	if _, ok := findSkillControl(router.AppendSemantics(nil), "Say hello"); ok {
		t.Fatal("search retained nonmatching task")
	}
	a.query = ""
	a.tasks["modu"][3].Title = "Renamed task"
	frame()
	frame()
	if _, ok := findSkillControl(router.AppendSemantics(nil), "Renamed task"); !ok {
		t.Fatal("refreshed model omitted renamed task")
	}
}

func TestRenderKeelSidebarStandard(t *testing.T) {
	dir := os.Getenv("ONECATCH_KEEL_PREVIEWS")
	if dir == "" {
		t.Skip("set ONECATCH_KEEL_PREVIEWS")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	locale.Apply(locale.Chinese())
	a := conversationFixture(t)
	// Include every supported Agent mark in the production sidebar capture.
	a.tasks["modu"] = a.tasks["modu"][:2]
	for _, agent := range []struct{ id, name string }{
		{"codex", "Codex"}, {"claude", "Claude Code"}, {"modu", "Modu Code"},
		{"trae", "TRAE CLI"}, {"dsh", "DeepSeek Harness"},
	} {
		a.tasks["modu"] = append(a.tasks["modu"], domaintasks.Task{
			ID: "preview-" + agent.id, Title: agent.name, WorkspaceID: "modu",
			WorkflowID: "single_agent", Harness: agent.id,
		})
	}
	a.ui.ShowAll["modu"] = true
	for _, dark := range []bool{false, true} {
		a.dark = dark
		applyPalette(dark)
		root := el.Root(a)
		var router input.Router
		warm := true
		widget := core.Func(func(gtx core.C) core.D {
			gtx.Source = router.Source()
			if warm {
				var ops op.Ops
				pre := gtx
				pre.Ops = &ops
				frame := func() { ops.Reset(); root.Layout(pre); router.Frame(&ops) }
				for range 4 {
					frame()
				}
				if box, ok := findSkillControl(router.AppendSemantics(nil), "modu"); ok {
					router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: f32.Pt(float32(box.Min.X+20), float32(box.Min.Y+20))})
				}
				for range 3 {
					frame()
				}
				warm = false
			}
			return root.Layout(gtx)
		})
		name := "sidebar-standard-light.png"
		if dark {
			name = "sidebar-standard-dark.png"
		}
		if err := window.Screenshot(screenshotRoot{widget}, 1120, 900, filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
	}
	applyPalette(false)
}
