package keeldesktop

import (
	"image"
	"slices"
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
	domainworkflows "github.com/openmodu/onecatch/internal/domain/workflows"
)

func TestDAGCanvasScrollReachesOffscreenNodes(t *testing.T) {
	a := dagTestApp(t)
	a.ui.WorkflowDraft.Layout.Nodes["synthesis"] = domainworkflows.Point{X: 620, Y: 340}
	root := el.Root(workflowWindowView{a})
	var router input.Router
	var ops op.Ops
	frame := func() {
		ops.Reset()
		root.Layout(layout.Context{Ops: &ops, Now: time.Now(), Source: router.Source(), Constraints: layout.Exact(image.Pt(1040, 760)), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}})
		router.Frame(&ops)
	}
	frame()
	router.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(600, 650), Scroll: f32.Pt(450, 100)})
	frame()
	frame()
	var find func([]input.SemanticNode) (image.Rectangle, bool)
	find = func(nodes []input.SemanticNode) (image.Rectangle, bool) {
		for _, n := range nodes {
			if n.Desc.Label == "节点 汇总落地" {
				return n.Desc.Bounds, true
			}
			if box, ok := find(n.Children); ok {
				return box, true
			}
		}
		return image.Rectangle{}, false
	}
	b, ok := find(router.AppendSemantics(nil))
	if !ok || b.Min.X < 240 || b.Max.X > 720 || b.Min.Y < 226 || b.Max.Y > 716 {
		t.Fatalf("scroll did not expose node: %v, %v", b, ok)
	}
	p := f32.Pt(float32(b.Min.X+50), float32(b.Min.Y+40))
	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: p}, pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: p})
	frame()
	if a.ui.WorkflowStepID != "synthesis" {
		t.Fatal("scrolled node could not be selected")
	}
}

func TestDAGInspectorIDThroughNativeEditor(t *testing.T) {
	a := dagTestApp(t)
	focus := "dag-node-id"
	root := el.Root(el.ViewFunc(func(cx *el.Context) el.Element {
		if focus != "" {
			cx.Focus(focus)
			focus = ""
		}
		return workflowWindowView{a}.Render(cx)
	}))
	var router input.Router
	var ops op.Ops
	frame := func() {
		ops.Reset()
		root.Layout(layout.Context{Ops: &ops, Now: time.Now(), Source: router.Source(), Constraints: layout.Exact(image.Pt(1040, 760)), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}})
		router.Frame(&ops)
	}
	frame()
	frame()
	router.Queue(key.EditEvent{Range: key.Range{Start: 0, End: 8}, Text: "security_review"})
	frame()
	frame()
	if a.ui.WorkflowDraft.Steps[0].ID != "security_review" || a.ui.WorkflowDraft.EntryStepID != "security_review" || !slices.Contains(a.ui.WorkflowDraft.Steps[2].DependsOn, "security_review") {
		t.Fatalf("editor did not update graph ID: %+v", a.ui.WorkflowDraft)
	}
	before := a.ui.WorkflowDraft.Layout.Nodes["security_review"]
	router.Queue(key.EditEvent{Range: key.Range{Start: 0, End: 15}, Text: "tests"})
	frame()
	frame()
	if a.ui.DAG.IDError == "" || a.ui.WorkflowDraft.Steps[0].ID != "security_review" || a.ui.WorkflowDraft.Layout.Nodes["security_review"] != before {
		t.Fatal("duplicate ID was not rejected without redirecting edges")
	}
	router.Queue(key.EditEvent{Range: key.Range{Start: 0, End: 5}, Text: "safe"})
	frame()
	frame()
	if a.ui.DAG.IDError != "" || a.ui.WorkflowDraft.Steps[0].ID != "safe" {
		t.Fatal("ID input could not recover after duplicate")
	}
	// Empty IDs remain visible for correction and cannot silently save the
	// previous valid identifier while the field shows an empty value.
	router.Queue(key.EditEvent{Range: key.Range{Start: 0, End: 4}, Text: ""})
	frame()
	frame()
	if a.ui.DAG.IDError == "" {
		t.Fatal("empty ID error missing")
	}
	var disabled bool
	var inspect func([]input.SemanticNode)
	inspect = func(nodes []input.SemanticNode) {
		for _, n := range nodes {
			if n.Desc.Label == "保存 DAG" && n.Desc.Disabled {
				disabled = true
			}
			inspect(n.Children)
		}
	}
	inspect(router.AppendSemantics(nil))
	if !disabled {
		t.Fatal("save allowed mismatched invalid ID field")
	}
}

func TestDAGSignalRenameRetainsNativeFocus(t *testing.T) {
	a := dagTestApp(t)
	focus := ""
	root := el.Root(el.ViewFunc(func(cx *el.Context) el.Element {
		if focus != "" {
			cx.Focus(focus)
			focus = ""
		}
		return workflowWindowView{a}.Render(cx)
	}))
	var router input.Router
	var ops op.Ops
	frame := func() {
		ops.Reset()
		root.Layout(layout.Context{Ops: &ops, Now: time.Now(), Source: router.Source(), Constraints: layout.Exact(image.Pt(1040, 760)), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}})
		router.Frame(&ops)
	}
	frame()
	router.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(900, 650), Scroll: f32.Pt(0, 700)})
	frame()
	frame()
	focus = "dag-signal/security/0"
	frame()
	frame()
	router.Queue(key.EditEvent{Range: key.Range{Start: 0, End: 9}, Text: "approved"})
	frame()
	frame()
	if a.ui.WorkflowDraft.Steps[0].Transitions["approved"] != "$done" {
		t.Fatal("signal field was not edited")
	}
	router.Queue(key.EditEvent{Range: key.Range{Start: 8, End: 8}, Text: "_final"})
	frame()
	frame()
	if a.ui.WorkflowDraft.Steps[0].Transitions["approved_final"] != "$done" {
		t.Fatal("signal rename lost keyboard focus after first edit")
	}
	if _, ok := a.ui.WorkflowDraft.Steps[0].Transitions["approved"]; ok {
		t.Fatal("old signal remained")
	}
	router.Queue(key.EditEvent{Range: key.Range{Start: 0, End: 14}, Text: "need_human"})
	frame()
	frame()
	if a.ui.DAG.SignalError == "" || len(a.ui.WorkflowDraft.Steps[0].Transitions) != 2 {
		t.Fatal("duplicate signal destroyed an existing transition")
	}
	// A rejected collision restores the previous valid field value.
	router.Queue(key.EditEvent{Range: key.Range{Start: 0, End: 14}, Text: ""})
	frame()
	frame()
	if a.ui.DAG.SignalError != "" {
		t.Fatal("signal field could not recover")
	}
	if err := domainworkflows.Validate(a.ui.WorkflowDraft); err == nil {
		t.Fatal("empty signal silently saved the previous valid value")
	}
}
