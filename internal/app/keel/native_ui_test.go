package keeldesktop

import (
	"context"
	"image"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/kit"
)

// Exercise Gio's actual pointer routing, without a GPU or native OS window.
// This verifies the overflowing seam hit area as well as the persisted width;
// it does not substitute for checking macOS window chrome on an unlocked Mac.
func TestSidebarResizePointerAndKeyboard(t *testing.T) {
	rootDir := t.TempDir()
	b, err := openBackendWithEngine(context.Background(), rootDir, testEngine{})
	if err != nil {
		t.Fatal(err)
	}
	defer b.close()
	a := &app{backend: b, ui: uiState{Preferences: defaultUIPreferences()}}
	root := el.Root(el.ViewFunc(func(cx *el.Context) el.Element {
		return el.Div().Row().WFull().HFull().Child(el.Div().W(el.Dp(a.ui.Preferences.SidebarWidth)).HFull(), a.sidebarSeam(cx), el.Div().Grow().W(el.Dp(0)).HFull())
	}))
	var router input.Router
	var ops op.Ops
	frame := func() {
		ops.Reset()
		root.Layout(layout.Context{Ops: &ops, Now: time.Now(), Source: router.Source(), Constraints: layout.Exact(image.Pt(1120, 760)), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}})
		router.Frame(&ops)
	}
	frame()
	// Start three pixels outside the visible 1px seam. The overlapping hit
	// region must accept the press and keep the drag outside its old bounds.
	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: f32.Pt(241, 400)})
	frame()
	router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: f32.Pt(301, 400)})
	frame()
	router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: f32.Pt(301, 400)})
	frame()
	if a.ui.Preferences.SidebarWidth != 304 {
		t.Fatalf("drag width: %v", a.ui.Preferences.SidebarWidth)
	}
	saved, err := loadUIPreferences(rootDir)
	if err != nil || saved.SidebarWidth != 304 {
		t.Fatalf("persisted width: %+v, %v", saved, err)
	}
	router.Queue(key.Event{Name: key.NameRightArrow, State: key.Press}, key.Event{Name: key.NameRightArrow, State: key.Release})
	frame()
	if a.ui.Preferences.SidebarWidth != 316 {
		t.Fatalf("keyboard width: %v", a.ui.Preferences.SidebarWidth)
	}
}

func TestSettingsAppearanceControlsFitAtMinimumWindowSize(t *testing.T) {
	b, err := openBackendWithEngine(context.Background(), t.TempDir(), testEngine{})
	if err != nil {
		t.Fatal(err)
	}
	a := newApp(b)
	defer a.close()
	a.settings, err = b.service.GetSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, size := range []image.Point{{960, 800}, {860, 600}} {
		p := &settingsPage{app: a, runtime: kit.Select(""), enabled: newFilledToggle("", true, true), initialized: true}
		root := el.Root(p)
		var router input.Router
		var ops op.Ops
		root.Layout(layout.Context{Ops: &ops, Now: time.Now(), Source: router.Source(), Constraints: layout.Exact(size), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}})
		router.Frame(&ops)
		wanted := map[string]bool{"跟随系统": false, "浅色": false, "深色": false}
		var inspect func([]input.SemanticNode)
		inspect = func(nodes []input.SemanticNode) {
			for _, node := range nodes {
				switch node.Desc.Label {
				case "主题配色", "森林", "海洋", "紫罗兰", "琥珀":
					t.Fatalf("removed accent option still visible: %s", node.Desc.Label)
				}
				if _, ok := wanted[node.Desc.Label]; ok && node.Desc.Gestures&input.ClickGesture != 0 {
					box := node.Desc.Bounds
					if box.Empty() || !box.In(image.Rectangle{Max: size}) {
						t.Fatalf("%s clipped at %v: %v", node.Desc.Label, size, box)
					}
					wanted[node.Desc.Label] = true
				}
				inspect(node.Children)
			}
		}
		inspect(router.AppendSemantics(nil))
		for label, found := range wanted {
			if !found {
				t.Fatalf("%s missing at %v", label, size)
			}
		}
	}
}

func TestWorkflowHistoryButtonsThroughNativeInputRouter(t *testing.T) {
	b, err := openBackendWithEngine(context.Background(), t.TempDir(), testEngine{})
	if err != nil {
		t.Fatal(err)
	}
	a := newApp(b)
	defer a.close()
	a.definitions, err = b.service.ListDefinitions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	var ids []string
	for _, d := range a.definitions {
		if d.ID != "single_agent" {
			names = append(names, d.Name)
			ids = append(ids, d.ID)
		}
	}
	if len(ids) < 2 {
		t.Fatal("need two workflow fixtures")
	}
	a.ui.SelectedWorkflow = ids[0]
	root := el.Root(workflowWindowView{a})
	var router input.Router
	var ops op.Ops
	frame := func() {
		ops.Reset()
		root.Layout(layout.Context{Ops: &ops, Now: time.Now(), Source: router.Source(), Constraints: layout.Exact(image.Pt(1000, 720)), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}})
		router.Frame(&ops)
	}
	frame()
	var find func([]input.SemanticNode, string) (image.Rectangle, bool)
	find = func(nodes []input.SemanticNode, label string) (image.Rectangle, bool) {
		for _, n := range nodes {
			if n.Desc.Label == label && n.Desc.Gestures&input.ClickGesture != 0 && !n.Desc.Disabled {
				return n.Desc.Bounds, true
			}
			if box, ok := find(n.Children, label); ok {
				return box, true
			}
		}
		return image.Rectangle{}, false
	}
	click := func(label string) {
		t.Helper()
		box, ok := find(router.AppendSemantics(nil), label)
		if !ok {
			t.Fatalf("no enabled %q control", label)
		}
		p := f32.Pt(float32(box.Min.X+box.Dx()/2), float32(box.Min.Y+box.Dy()/2))
		router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: p}, pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: p}, pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: p})
		frame()
	}
	click(names[1])
	if a.ui.SelectedWorkflow != ids[1] {
		t.Fatal("workflow row click did not select")
	}
	click("返回")
	if a.ui.SelectedWorkflow != ids[0] {
		t.Fatal("back did not restore first workflow")
	}
	click("前进")
	if a.ui.SelectedWorkflow != ids[1] {
		t.Fatal("forward did not restore second workflow")
	}
	click("编辑")
	if !a.ui.WorkflowSource {
		t.Fatal("edit did not open form")
	}
	// A draft made through the form is serialized by the same handler as inputs.
	a.ui.WorkflowDraft.Name = "保留这份草稿"
	a.syncWorkflowDocument()
	frame()
	click("返回")
	if a.ui.WorkflowSource {
		t.Fatal("back did not return to detail")
	}
	click("前进")
	if !a.ui.WorkflowSource || a.ui.WorkflowDraft.Name != "保留这份草稿" {
		t.Fatal("forward discarded edited draft")
	}
	// Ensure invalid imported IDs cannot panic the native select rendering.
	a.ui.WorkflowRaw = true
	a.workflowEditor.SetValue(`{"id":"draft","steps":[{"id":""},{"id":""}]}`)
	a.toggleWorkflowRaw()
	frame()
}

func TestInspectorControlsFitAndMaximizeRestoresPanel(t *testing.T) {
	b, err := openBackendWithEngine(context.Background(), t.TempDir(), testEngine{})
	if err != nil {
		t.Fatal(err)
	}
	a := newApp(b)
	defer a.close()
	// The rendered fixture uses the production header and sidebar; no native
	// window or OS dialog is opened by these pointer/semantic checks.
	a.logo = kit.Image(image.NewRGBA(image.Rect(0, 0, 22, 22)), "OneCatch").Size(22, 22)
	a.workbench.Visible = true
	a.workbench.Tab = "status"
	root := el.Root(a)
	var router input.Router
	var ops op.Ops
	size := image.Pt(1120, 760)
	frame := func() {
		ops.Reset()
		root.Layout(layout.Context{Ops: &ops, Now: time.Now(), Source: router.Source(), Constraints: layout.Exact(size), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}})
		router.Frame(&ops)
	}
	var find func([]input.SemanticNode, string) (image.Rectangle, bool)
	find = func(nodes []input.SemanticNode, label string) (image.Rectangle, bool) {
		for _, node := range nodes {
			if node.Desc.Label == label && node.Desc.Gestures&input.ClickGesture != 0 {
				return node.Desc.Bounds, true
			}
			if rect, ok := find(node.Children, label); ok {
				return rect, true
			}
		}
		return image.Rectangle{}, false
	}
	for _, viewport := range []image.Point{{1120, 760}, {860, 560}} {
		size = viewport
		frame()
		for _, label := range []string{"状态", "文件", "Git", "审阅", "事件", "放大状态栏", "在独立窗口打开", "收起检查面板", "OneCatch 菜单"} {
			rect, ok := find(router.AppendSemantics(nil), label)
			if !ok || rect.Empty() || !rect.In(image.Rectangle{Max: viewport}) {
				t.Fatalf("%q clipped at %v: %v", label, viewport, rect)
			}
		}
	}
	size = image.Pt(1120, 760)
	frame()
	click := func(label string) {
		t.Helper()
		rect, ok := find(router.AppendSemantics(nil), label)
		if !ok {
			t.Fatalf("no %q", label)
		}
		center := f32.Pt(float32(rect.Min.X+rect.Dx()/2), float32(rect.Min.Y+rect.Dy()/2))
		router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: center}, pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: center}, pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: center})
		frame()
	}
	click("放大状态栏")
	if !a.workbench.Maximized {
		t.Fatal("maximize ignored")
	}
	click("还原状态栏")
	if a.workbench.Maximized {
		t.Fatal("restore ignored")
	}
	click("收起检查面板")
	if a.workbench.Visible {
		t.Fatal("collapse ignored")
	}
}

func TestFilledChoicesKeepKeyboardActivation(t *testing.T) {
	for _, checkbox := range []bool{false, true} {
		choice := newFilledToggle("选择", false, checkbox)
		root := el.Root(el.ViewFunc(func(cx *el.Context) el.Element { return el.Div().P(10).Child(choice.Render(cx)) }))
		var router input.Router
		var ops op.Ops
		frame := func() {
			ops.Reset()
			root.Layout(layout.Context{Ops: &ops, Now: time.Now(), Source: router.Source(), Constraints: layout.Exact(image.Pt(300, 80)), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}})
			router.Frame(&ops)
		}
		frame()
		router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: f32.Pt(20, 22)}, pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: f32.Pt(20, 22)})
		frame()
		if !choice.Value() {
			t.Fatal("pointer did not select filled choice")
		}
		router.Queue(key.Event{Name: key.NameSpace, State: key.Press}, key.Event{Name: key.NameSpace, State: key.Release})
		frame()
		if choice.Value() {
			t.Fatal("Space did not toggle filled choice once")
		}
		choice.SetDisabled(true)
		frame()
		router.Queue(key.Event{Name: key.NameReturn, State: key.Press}, key.Event{Name: key.NameReturn, State: key.Release})
		frame()
		if choice.Value() {
			t.Fatal("disabled choice accepted keyboard activation")
		}
	}
}

func TestReviewScrollRegionSurvivesWideAndNarrowLayout(t *testing.T) {
	b, err := openBackendWithEngine(context.Background(), t.TempDir(), testEngine{})
	if err != nil {
		t.Fatal(err)
	}
	a := newApp(b)
	defer a.close()
	a.logo = kit.Image(image.NewRGBA(image.Rect(0, 0, 22, 22)), "OneCatch").Size(22, 22)
	a.workspaceID = "preview"
	a.workbench.Visible = true
	a.workbench.Tab = "review"
	a.workbench.Maximized = true
	a.reviewState("preview").Files = parseGitReview("diff --git a/main.go b/main.go\n--- a/main.go\n+++ b/main.go\n@@ -1 +1 @@\n-old\n+new\n", "worktree")
	for _, width := range []int{1120, 860} {
		root := el.Root(a)
		var router input.Router
		var ops op.Ops
		size := image.Pt(width, 760)
		root.Layout(layout.Context{Ops: &ops, Now: time.Now(), Source: router.Source(), Constraints: layout.Exact(size), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}})
		router.Frame(&ops)
		var bounds image.Rectangle
		var inspect func([]input.SemanticNode)
		inspect = func(nodes []input.SemanticNode) {
			for _, node := range nodes {
				if node.Desc.Label == "代码差异" {
					bounds = node.Desc.Bounds
				}
				inspect(node.Children)
			}
		}
		inspect(router.AppendSemantics(nil))
		if bounds.Dx() < 300 || bounds.Dy() < 500 {
			t.Fatalf("diff scroll region collapsed at %d: %v", width, bounds)
		}
	}
}
