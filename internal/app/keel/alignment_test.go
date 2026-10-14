package keeldesktop

import (
	"image"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dyike/keel/gio/f32"
	"github.com/dyike/keel/gio/io/input"
	"github.com/dyike/keel/gio/io/pointer"
	"github.com/dyike/keel/gio/io/semantic"
	"github.com/dyike/keel/gio/layout"
	"github.com/dyike/keel/gio/op"
	"github.com/dyike/keel/gio/unit"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/locale"
	"github.com/openmodu/onecatch/internal/app/keel/terminalview"
	domainworkflows "github.com/openmodu/onecatch/internal/domain/workflows"
	desktop "github.com/openmodu/onecatch/internal/service/desktop"
)

func alignmentFixture(t *testing.T) *app {
	t.Helper()
	a := conversationFixture(t)
	a.task, a.detail = nil, nil
	a.taskID, a.runID = "", ""
	a.runtime.SetValue("codex")
	a.runtimes = []desktop.RuntimeInfo{
		{ID: "codex", Name: "Codex", Available: true, Enabled: true},
		{ID: "claude", Name: "Claude Code", Available: true, Enabled: true},
		{ID: "modu", Name: "Modu Code", Available: true, Enabled: true},
		{ID: "pi", Name: "Pi", Available: true, Enabled: true},
		{ID: "grok", Name: "Grok Build", Available: true, Enabled: true},
		{ID: "trae", Name: "TRAE CLI"},
		{ID: "dsh", Name: "DeepSeek Harness"},
	}
	a.definitions = []domainworkflows.Definition{
		{ID: "single_agent", Name: "单 Agent"},
		{ID: "loop", Name: "实现与审查 Loop"},
		{ID: "dag", Name: "并行审查 DAG"},
		{ID: "nested", Name: "嵌套的实现审查 Loop"},
	}
	for i, title := range []string{"ityike@Mac-mini", "zsh", "very-long-terminal-session-title"} {
		id := []string{"main", "short", "long"}[i]
		s := terminalview.New(nil)
		s.AttachSession("")
		t.Cleanup(s.Close)
		s.Write([]byte("\x1b[34mityike\x1b[0m@\x1b[32mMac-mini\x1b[0m:~ /Code/onecatch (main)\r\n$ "))
		a.terminal.Panes[id] = &terminalPane{ID: id, Title: title, Running: true, Screen: s}
		a.terminal.Tabs = append(a.terminal.Tabs, &terminalTab{ID: id, Root: terminalLeaf(id)})
	}
	a.terminal.Active, a.terminal.Focused = "main", "main"
	return a
}

func alignmentNode(nodes []input.SemanticNode, label string, clickable bool) (input.SemanticDesc, bool) {
	for _, n := range nodes {
		if n.Desc.Label == label && (n.Desc.Gestures&input.ClickGesture != 0) == clickable {
			return n.Desc, true
		}
		if desc, ok := alignmentNode(n.Children, label, clickable); ok {
			return desc, true
		}
	}
	return input.SemanticDesc{}, false
}

func alignmentTextAtRow(nodes []input.SemanticNode, label string, icon image.Rectangle) (image.Rectangle, bool) {
	for _, n := range nodes {
		if n.Desc.Label == label && n.Desc.Class != semantic.Button && n.Desc.Gestures&input.ClickGesture == 0 && n.Desc.Bounds.Min.Y < icon.Max.Y && n.Desc.Bounds.Max.Y > icon.Min.Y {
			return n.Desc.Bounds, true
		}
		if box, ok := alignmentTextAtRow(n.Children, label, icon); ok {
			return box, true
		}
	}
	return image.Rectangle{}, false
}

func alignmentButton(nodes []input.SemanticNode, label string) (input.SemanticDesc, bool) {
	for _, n := range nodes {
		if n.Desc.Label == label && n.Desc.Class == semantic.Button {
			return n.Desc, true
		}
		if desc, ok := alignmentButton(n.Children, label); ok {
			return desc, true
		}
	}
	return input.SemanticDesc{}, false
}

func alignmentRouter(root *el.RootWidget, width, height, scale int) (*input.Router, func(), func(image.Rectangle)) {
	var router input.Router
	var ops op.Ops
	frame := func() {
		ops.Reset()
		root.Layout(layout.Context{Ops: &ops, Now: time.Now(), Source: router.Source(), Constraints: layout.Exact(image.Pt(width*scale, height*scale)), Metric: unit.Metric{PxPerDp: float32(scale), PxPerSp: float32(scale)}})
		router.Frame(&ops)
	}
	click := func(box image.Rectangle) {
		p := f32.Pt(float32(box.Min.X+box.Dx()/2), float32(box.Min.Y+box.Dy()/2))
		router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: p}, pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: p})
		frame()
		router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: p})
		frame()
		frame()
	}
	frame()
	frame()
	return &router, frame, click
}

func TestExecutorAlignmentAndNativeSelection(t *testing.T) {
	applyPalette(false)
	for _, scale := range []int{1, 2} {
		a := alignmentFixture(t)
		a.buildExecutorMenu()
		a.ui.Executor.SetValue(true)
		router, frame, click := alignmentRouter(el.Root(a), 860, 600, scale)
		nodes := router.AppendSemantics(nil)
		left := -1
		for _, label := range []string{"Codex", "Claude Code", "Modu Code", "Pi", "Grok Build", "TRAE CLI", "DeepSeek Harness", "实现与审查 Loop", "并行审查 DAG"} {
			icon, ok := alignmentNode(nodes, label+"图标", false)
			if !ok || icon.Bounds.Empty() {
				t.Fatalf("missing %s icon at %dx", label, scale)
			}
			text, ok := alignmentTextAtRow(nodes, label, icon.Bounds)
			if !ok || text.Empty() {
				t.Fatalf("missing %s text at %dx", label, scale)
			}
			if left < 0 {
				left = icon.Bounds.Min.X
			}
			// Text semantics include glyph ink; Helvetica Neue has a negative
			// side bearing for T. Allow one physical pixel per scale for ink.
			if icon.Bounds.Min.X != left || abs(text.Min.X-(left+24*scale)) > scale {
				t.Fatalf("%s lost shared leading columns at %dx: icon=%v text=%v", label, scale, icon.Bounds, text)
			}
		}
		pi, ok := findSkillControl(nodes, "Pi")
		if !ok {
			t.Fatal("missing Pi selection")
		}
		// The blank right end belongs to the row as well as its label.
		// Leave the scroll gutter available to its own pointer handler.
		click(image.Rect(pi.Max.X-24*scale, pi.Min.Y, pi.Max.X-20*scale, pi.Max.Y))
		if a.runtime.Value() != "pi" || a.ui.Executor.Value() {
			t.Fatalf("full-row selection did not select Pi and close the picker at %dx: runtime=%s open=%v", scale, a.runtime.Value(), a.ui.Executor.Value())
		}
		a.ui.Executor.SetValue(true)
		frame()
		frame()
		disabled, ok := alignmentButton(router.AppendSemantics(nil), "DeepSeek Harness")
		if !ok || !disabled.Disabled {
			t.Fatal("unavailable runtime lost its disabled state")
		}
		click(disabled.Bounds)
		if a.runtime.Value() != "pi" || !a.ui.Executor.Value() {
			t.Fatal("disabled runtime changed selection")
		}
		workflow, ok := findSkillControl(router.AppendSemantics(nil), "实现与审查 Loop")
		if !ok {
			t.Fatal("missing workflow selection")
		}
		click(workflow)
		if a.workflow.Value() != "loop" || a.ui.Executor.Value() {
			t.Fatal("workflow row did not select and dismiss")
		}
	}
}

func TestTerminalTabHeaderAlignmentAndNativeSelection(t *testing.T) {
	applyPalette(false)
	for _, width := range []int{1120, 860} {
		for _, scale := range []int{1, 2} {
			a := alignmentFixture(t)
			a.page = "skills" // The Skills workspace retains the bottom terminal strip.
			labels := []string{"ityike@Mac-mini", "zsh", "very-long-terminal-session-title"}
			if width == 860 {
				// Keep both a short and a long tab visible in the narrow strip.
				a.terminal.Tabs = a.terminal.Tabs[1:]
				a.terminal.Active, a.terminal.Focused = "short", "short"
				labels = labels[1:]
			}
			initialTabs := len(a.terminal.Tabs)
			a.terminal.Open = true
			router, _, click := alignmentRouter(el.Root(a), width, 760, scale)
			nodes := router.AppendSemantics(nil)
			header, ok := findSkillControl(nodes, "聚焦终端")
			if !ok {
				t.Fatal("missing terminal header")
			}
			center := header.Min.Y + header.Max.Y
			for _, label := range labels {
				title, ok := findSkillControl(nodes, label)
				if !ok {
					t.Fatalf("missing terminal title %s", label)
				}
				close, ok := findSkillControl(nodes, "关闭终端标签 "+label)
				if !ok {
					t.Fatalf("missing terminal close %s", label)
				}
				row, ok := alignmentNode(nodes, "终端标签："+label, false)
				if !ok || !close.In(row.Bounds) {
					t.Fatalf("long label displaced close button: title=%v close=%v row=%v", title, close, row.Bounds)
				}
				if title.Min.Y+title.Max.Y != center || close.Min.Y+close.Max.Y != center {
					t.Fatalf("terminal controls have different centerlines: header=%v title=%v close=%v", header, title, close)
				}
			}
			short, _ := findSkillControl(nodes, "zsh")
			click(short)
			if a.terminal.Active != "short" || a.terminal.Focused != "short" {
				t.Fatal("tab selection did not focus the selected terminal")
			}
			close, _ := findSkillControl(router.AppendSemantics(nil), "关闭终端标签 zsh")
			click(close)
			if a.terminal.Panes["short"] != nil || len(a.terminal.Tabs) != initialTabs-1 || a.terminal.Active == "short" {
				t.Fatal("close button did not remove its tab and focus a sibling")
			}
		}
	}
}

func TestRenderAlignmentPreview(t *testing.T) {
	dir := os.Getenv("ONECATCH_KEEL_PREVIEWS")
	if dir == "" {
		t.Skip("set ONECATCH_KEEL_PREVIEWS to capture")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	locale.Apply(locale.Chinese())
	for _, state := range []struct {
		name string
		w, h int
		dark bool
	}{{"alignment", 1120, 760, false}, {"alignment-narrow", 860, 600, false}, {"alignment-dark", 1120, 760, true}} {
		a := alignmentFixture(t)
		applyPalette(state.dark)
		a.buildExecutorMenu()
		a.ui.Executor.SetValue(true)
		if err := screenshotWithFocus(el.Root(a), state.w, state.h, filepath.Join(dir, state.name+"-menu.png")); err != nil {
			t.Fatal(err)
		}
		a.ui.Executor.SetValue(false)
		a.terminal.Open = true
		if err := screenshotWithFocus(el.Root(a), state.w, state.h, filepath.Join(dir, state.name+"-terminal.png")); err != nil {
			t.Fatal(err)
		}
	}
	applyPalette(false)
}
