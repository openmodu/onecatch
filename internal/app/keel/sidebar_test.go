package keeldesktop

import (
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/kit"
	"github.com/dyike/keel/ui/locale"
	"github.com/dyike/keel/ui/theme"
	"github.com/dyike/keel/ui/window"
	domaintasks "github.com/openmodu/onecatch/internal/domain/tasks"
	workspaces "github.com/openmodu/onecatch/internal/domain/workspaces"
	desktopservice "github.com/openmodu/onecatch/internal/service/desktop"
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
		for _, label := range []string{"新建任务", "项目操作"} {
			if _, ok := findSkillControl(router.AppendSemantics(nil), label); ok {
				t.Fatalf("idle project exposes %q", label)
			}
		}
		move := func(p f32.Point) {
			router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: p})
			frame()
			frame()
		}
		move(f32.Pt(float32(project.Min.X+project.Dx()/2), float32(project.Min.Y+project.Dy()/2)))
		plus, actions := find("新建任务"), find("项目操作")
		move(f32.Pt(float32(700*scale), float32(400*scale)))
		for _, label := range []string{"新建任务", "项目操作"} {
			if _, ok := findSkillControl(router.AppendSemantics(nil), label); ok {
				t.Fatalf("project %q remains interactive after hover leaves", label)
			}
		}
		move(f32.Pt(float32(project.Min.X+project.Dx()/2), float32(project.Min.Y+project.Dy()/2)))
		if find("modu") != project || find("新建任务") != plus || find("项目操作") != actions {
			t.Fatal("project hover changed row geometry")
		}
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

func TestKeelSidebarTaskHoverPreviewActions(t *testing.T) {
	locale.Apply(locale.Chinese())
	applyPalette(false)
	for _, scale := range []int{1, 2} {
		a := conversationFixture(t)
		root := el.Root(a)
		var router input.Router
		var ops op.Ops
		now := time.Now()
		frame := func() {
			ops.Reset()
			root.Layout(layout.Context{Ops: &ops, Now: now, Source: router.Source(), Constraints: layout.Exact(image.Pt(1120*scale, 900*scale)), Metric: unit.Metric{PxPerDp: float32(scale), PxPerSp: float32(scale)}})
			router.Frame(&ops)
		}
		settle := func() {
			for range 3 {
				frame()
			}
		}
		move := func(box image.Rectangle) {
			router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: f32.Pt(float32(box.Min.X+box.Dx()/2), float32(box.Min.Y+box.Dy()/2))})
			settle()
		}
		click := func(box image.Rectangle) {
			move(box)
			p := f32.Pt(float32(box.Min.X+box.Dx()/2), float32(box.Min.Y+box.Dy()/2))
			router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: p})
			frame()
			router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: p})
			settle()
		}
		find := func(label string) image.Rectangle {
			t.Helper()
			box, ok := findSkillControl(router.AppendSemantics(nil), label)
			if !ok {
				t.Fatalf("missing %q at %dx", label, scale)
			}
			return box
		}
		shown := func(label string) bool {
			if label == "任务详情：Say hello" {
				_, ok := alignmentNode(router.AppendSemantics(nil), label, false)
				return ok
			}
			_, ok := findSkillControl(router.AppendSemantics(nil), label)
			return ok
		}
		settle()
		hello := find("Say hello")
		if shown("任务操作") {
			t.Fatal("idle task exposes pointer actions")
		}
		move(hello)
		actions := find("任务操作")
		if find("Say hello") != hello || actions.Dy() != 26*scale {
			t.Fatal("hover changed row geometry")
		}
		if shown("任务详情：Say hello") {
			t.Fatal("preview did not wait for hover delay")
		}
		now = now.Add(kit.TooltipDelay + time.Millisecond)
		settle()
		if !shown("任务详情：Say hello") {
			t.Fatalf("preview missing: state=%+v order=%v hello=%v", a.ui.SidebarPreview, a.ui.SidebarVisibleTasks, hello)
		}
		router.Queue(key.Event{Name: key.NameEscape, State: key.Press})
		settle()
		if shown("任务详情：Say hello") {
			t.Fatal("Escape did not dismiss preview")
		}
		move(actions)
		if find("任务操作") != actions || shown("任务详情：Say hello") {
			t.Fatal("moving to actions lost the row or retained preview")
		}
		click(actions)
		click(find("删除"))
		if !a.confirmation.Value() || a.taskID != "hello" {
			t.Fatal("delete skipped confirmation or selected a task")
		}
		router.Queue(key.Event{Name: key.NameEscape, State: key.Press})
		settle()
		if a.confirmation.Value() || a.taskID != "hello" {
			t.Fatal("cancel changed the task")
		}
		click(hello)
		router.Queue(key.Event{Name: "2", Modifiers: key.ModCtrl, State: key.Press}, key.Event{Name: "2", Modifiers: key.ModCtrl, State: key.Release})
		settle()
		if a.taskID != "hello" {
			t.Fatal("removed numeric shortcut still switched tasks")
		}
	}
}

func TestRenderKeelSidebarCodexInteractions(t *testing.T) {
	dir := os.Getenv("ONECATCH_KEEL_PREVIEWS")
	if dir == "" {
		t.Skip("set ONECATCH_KEEL_PREVIEWS")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	locale.Apply(locale.Chinese())
	defer applyPalette(false)
	for _, dark := range []bool{false, true} {
		a := conversationFixture(t)
		a.dark = dark
		applyPalette(dark)
		a.ui.Preferences.SidebarWidth = 320
		a.tasks["modu"][0].Title = "更新 onecatch 桌面菜单"
		a.tasks["modu"][1].Title = "设计 Git Worktree 支持，保留独立工作目录中的修改和执行记录"
		a.tasks["modu"][1].WorkspaceID = "modu"
		a.tasks["modu"][2].Title = "吸收 Void 项目优点"
		a.tasks["modu"] = append(a.tasks["modu"], domaintasks.Task{ID: "ios", Title: "你能自己测 ios 吗", WorkspaceID: "modu", Harness: "codex"})
		a.ui.ShowAll["modu"] = true
		root := el.Root(a)
		var router input.Router
		now := time.Now()
		warm := true
		var hoverSample image.Point
		widget := core.Func(func(gtx core.C) core.D {
			gtx.Source = router.Source()
			gtx.Now = now
			if warm {
				var ops op.Ops
				pre := gtx
				pre.Ops = &ops
				frame := func() { ops.Reset(); pre.Now = now; root.Layout(pre); router.Frame(&ops) }
				for range 4 {
					frame()
				}
				box, ok := findSkillControl(router.AppendSemantics(nil), a.tasks["modu"][1].Title)
				if !ok {
					t.Fatal("missing preview task")
				}
				hoverSample = image.Pt(box.Max.X+2, box.Min.Y+box.Dy()/2)
				router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: f32.Pt(float32(box.Min.X+box.Dx()/2), float32(box.Min.Y+box.Dy()/2))})
				for range 3 {
					frame()
				}
				now = now.Add(kit.TooltipDelay + time.Millisecond)
				for range 4 {
					frame()
				}
				warm = false
			}
			gtx.Now = now
			return root.Layout(gtx)
		})
		name := "sidebar-codex-light.png"
		if dark {
			name = "sidebar-codex-dark.png"
		}
		if err := window.Screenshot(screenshotRoot{widget}, 1120, 900, filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
		capture, err := os.Open(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		pixels, err := png.Decode(capture)
		capture.Close()
		if err != nil {
			t.Fatal(err)
		}
		got := color.NRGBAModel.Convert(pixels.At(hoverSample.X, hoverSample.Y)).(color.NRGBA)
		if got != theme.SubtleHover {
			t.Fatalf("hover background stops before the suffix: got %v want %v at %v", got, theme.SubtleHover, hoverSample)
		}
	}
}

func TestKeelSidebarDirectPinAndConfirmedDeletePersist(t *testing.T) {
	locale.Apply(locale.Chinese())
	a := conversationFixture(t)
	w, err := a.backend.service.AddWorkspace(context.Background(), desktopservice.AddWorkspaceInput{Name: "Actions", Path: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	task, err := a.backend.service.CreateTask(context.Background(), desktopservice.CreateTaskInput{WorkspaceID: w.ID, Title: "Persist sidebar actions", Prompt: "Say hello", WorkflowID: "single_agent", Harness: "codex", Sandbox: "read-only", WorktreeMode: "project"})
	if err != nil {
		t.Fatal(err)
	}
	a.workspaces = []workspaces.Workspace{w}
	a.tasks = map[string][]domaintasks.Task{w.ID: {task}}
	a.ui.Expanded[w.ID] = true
	a.task, a.taskID = nil, ""
	a.workspaceID = w.ID
	router, frame, click := alignmentRouter(el.Root(a), 1120, 900, 1)
	find := func(label string) image.Rectangle {
		t.Helper()
		box, ok := findSkillControl(router.AppendSemantics(nil), label)
		if !ok {
			t.Fatalf("missing %s", label)
		}
		return box
	}
	hover := func() {
		b := find(task.Title)
		router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: f32.Pt(float32(b.Min.X+b.Dx()/2), float32(b.Min.Y+b.Dy()/2))})
		frame()
		frame()
	}
	getTask := func() (domaintasks.Task, error) {
		items, e := a.backend.service.ListTasks(a.ctx, w.ID)
		if e != nil {
			return domaintasks.Task{}, e
		}
		for _, v := range items {
			if v.ID == task.ID {
				return v, nil
			}
		}
		return domaintasks.Task{}, domaintasks.ErrNotFound
	}
	wait := func(done func() bool) {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for !done() {
			if time.Now().After(deadline) {
				t.Fatal("sidebar action did not persist")
			}
			time.Sleep(5 * time.Millisecond)
		}
	}

	hover()
	click(find("任务操作"))
	click(find("置顶"))
	wait(func() bool {
		current, e := getTask()
		return e == nil && current.Pinned
	})
	task, err = getTask()
	if err != nil {
		t.Fatal(err)
	}
	a.tasks[w.ID] = []domaintasks.Task{task}
	frame()
	frame()
	hover()
	click(find("任务操作"))
	click(find("取消置顶"))
	wait(func() bool {
		current, e := getTask()
		return e == nil && !current.Pinned
	})
	hover()
	click(find("任务操作"))
	click(find("删除"))
	click(find("取消"))
	if _, err = getTask(); err != nil {
		t.Fatal("cancel deleted the persisted task", err)
	}
	hover()
	hover()
	click(find("任务操作"))
	click(find("删除"))
	click(find("删除"))
	wait(func() bool { items, e := a.backend.service.ListTasks(a.ctx, w.ID); return e == nil && len(items) == 0 })
}
