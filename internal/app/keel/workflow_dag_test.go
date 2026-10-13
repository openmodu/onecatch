package keeldesktop

import (
	"context"
	"encoding/json"
	"image"
	"math"
	"os"
	"path/filepath"
	"reflect"
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
	"github.com/dyike/keel/ui/window"
	domainworkflows "github.com/openmodu/onecatch/internal/domain/workflows"
	desktopservice "github.com/openmodu/onecatch/internal/service/desktop"
)

func hasEdge(d domainworkflows.Definition, from, to, kind string) bool {
	return slices.ContainsFunc(d.Edges, func(e domainworkflows.Edge) bool { return e.From == from && e.To == to && e.Kind == kind })
}

func dagTestApp(t *testing.T) *app {
	t.Helper()
	b, err := openBackendWithEngine(context.Background(), t.TempDir(), testEngine{})
	if err != nil {
		t.Fatal(err)
	}
	a := newApp(b)
	t.Cleanup(a.close)
	a.settings, err = b.service.GetSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	a.definitions, err = b.service.ListDefinitions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	a.runtimes = []desktopservice.RuntimeInfo{{ID: "codex", Name: "Codex", Enabled: true, Available: true}, {ID: "claude", Name: "Claude Code", Enabled: true, Available: true}}
	a.selectWorkflow("parallel_review", true)
	a.ui.DAG.InspectorOpen = true
	return a
}

func TestDAGNativeCanvasInteractions(t *testing.T) {
	a := dagTestApp(t)
	root := el.Root(workflowView{a})
	var router input.Router
	var ops op.Ops
	// Leave room for both the fixed canvas toolbar and the graph's test
	// connections; narrow viewport checks below verify toolbar wrapping.
	size := image.Pt(1040, 900)
	frame := func() {
		ops.Reset()
		root.Layout(layout.Context{Ops: &ops, Now: time.Now(), Source: router.Source(), Constraints: layout.Exact(size), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}})
		router.Frame(&ops)
	}
	var find func([]input.SemanticNode, string) (image.Rectangle, bool)
	find = func(nodes []input.SemanticNode, label string) (image.Rectangle, bool) {
		for _, n := range nodes {
			if n.Desc.Label == label && !n.Desc.Disabled {
				return n.Desc.Bounds, true
			}
			if box, ok := find(n.Children, label); ok {
				return box, true
			}
		}
		return image.Rectangle{}, false
	}
	box := func(label string) image.Rectangle {
		t.Helper()
		b, ok := find(router.AppendSemantics(nil), label)
		if !ok || b.Empty() {
			t.Fatalf("no %q control", label)
		}
		return b
	}
	clickPoint := func(p f32.Point) {
		router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: p}, pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: p})
		frame()
		router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: p})
		frame()
	}
	click := func(label string) {
		t.Helper()
		b := box(label)
		clickPoint(f32.Pt(float32(b.Min.X+b.Dx()/2), float32(b.Min.Y+b.Dy()/2)))
	}
	frame()
	for _, label := range []string{"自动布局", "步骤", "保存", "校验", "连线类型 返工"} {
		b := box(label)
		if !b.In(image.Rectangle{Max: size}) {
			t.Fatalf("%s clipped: %v", label, b)
		}
	}
	b := box("步骤 安全审查")
	if b.Dx() != 204 || b.Dy() != 94 {
		t.Fatalf("node geometry: %v", b)
	}
	p := f32.Pt(float32(b.Min.X+70), float32(b.Min.Y+40))
	before := a.ui.WorkflowDraft.Layout.Nodes["security"]
	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: p})
	frame()
	router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: p.Add(f32.Pt(80, 35))})
	frame()
	router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: p.Add(f32.Pt(115, 50))})
	frame()
	router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: p.Add(f32.Pt(115, 50))})
	frame()
	want := domainworkflows.Point{X: before.X + 115, Y: before.Y + 50}
	if got := a.ui.WorkflowDraft.Layout.Nodes["security"]; got != want {
		t.Fatalf("drag did not follow moving node: got %+v, want %+v", got, want)
	}
	// Child ports must not capture a node drag or select the inspector.
	before = a.ui.WorkflowDraft.Layout.Nodes["security"]
	click("从 安全审查 连接")
	if a.ui.DAG.ConnectFrom != "security" || a.ui.WorkflowDraft.Layout.Nodes["security"] != before {
		t.Fatalf("port did not start connection independently: edges=%+v connection=%q position=%+v before=%+v port=%v", a.ui.WorkflowDraft.Edges, a.ui.DAG.ConnectFrom, a.ui.WorkflowDraft.Layout.Nodes["security"], before, box("从 安全审查 连接"))
	}
	click("连接到 测试审查")
	if !hasEdge(a.ui.WorkflowDraft, "security", "tests", domainworkflows.EdgeNext) {
		t.Fatalf("ports did not connect nodes: connection=%q target=%+v port=%v", a.ui.DAG.ConnectFrom, a.ui.WorkflowDraft.Steps[1], box("连接到 测试审查"))
	}
	click("从 安全审查 连接")
	click("连接到 测试审查")
	if len(domainworkflows.Upstream(a.ui.WorkflowDraft, "tests")) != 1 {
		t.Fatal("duplicate connection")
	}
	click("删除依赖 security → tests")
	if hasEdge(a.ui.WorkflowDraft, "security", "tests", domainworkflows.EdgeNext) {
		t.Fatal("edge delete did not remove dependency")
	}
	// Actual path clicks delete only that edge, not a broad bounding rectangle.
	edge := dagEdges(a.ui.WorkflowDraft)[0]
	origin := box("步骤 安全审查").Min.Sub(image.Pt(int(before.X), int(before.Y)))
	curve := edge.point(.3).Add(f32.Pt(float32(origin.X), float32(origin.Y)))
	clickPoint(curve)
	if hasEdge(a.ui.WorkflowDraft, "security", "synthesis", domainworkflows.EdgeNext) {
		t.Fatal("Bézier path hit did not remove dependency")
	}
	click("自动布局")
	if got := a.ui.WorkflowDraft.Layout.Nodes["security"]; got != (domainworkflows.Point{X: 60, Y: 80}) {
		t.Fatalf("layout: %+v", got)
	}
	// A rework edge runs from the checking step back to an upstream step.
	a.connectDAGNode("security")
	a.connectDAGNode("synthesis")
	frame()
	click("连线类型 返工")
	// Either port of a step starts a connection; the input port is the one
	// inside this narrow canvas.
	click("连接到 汇总落地")
	click("连接到 安全审查")
	if !hasEdge(a.ui.WorkflowDraft, "synthesis", "security", domainworkflows.EdgeRework) || len(a.ui.DAG.Validation) != 0 {
		t.Fatalf("rework edge not created cleanly: %+v %+v", a.ui.WorkflowDraft.Edges, a.ui.DAG.Validation)
	}
	click("删除返工 synthesis → security")
	if hasEdge(a.ui.WorkflowDraft, "synthesis", "security", domainworkflows.EdgeRework) {
		t.Fatal("rework edge delete failed")
	}
	click("连线类型 依赖")
	click("步骤")
	if len(a.ui.WorkflowDraft.Steps) != 4 || a.ui.WorkflowStepID != "step_4" {
		t.Fatal("step button did not create/select")
	}
	click("删除步骤")
	if len(a.ui.WorkflowDraft.Steps) != 3 || a.ui.WorkflowStepID != "security" {
		t.Fatal("inspector delete did not repair selection")
	}
	// Cycle validation stays in the editor and reports every domain issue.
	a.ui.WorkflowDraft.Edges = append(a.ui.WorkflowDraft.Edges, domainworkflows.Edge{From: "synthesis", To: "security", Kind: domainworkflows.EdgeNext})
	frame()
	click("校验")
	if len(a.ui.DAG.Validation) == 0 {
		t.Fatal("cycle silently accepted")
	}
	for _, small := range []image.Point{{860, 600}, {1040, 760}} {
		size = small
		frame()
		b := box("校验")
		if !b.In(image.Rectangle{Max: size}) {
			t.Fatalf("validation clipped at %v: %v", size, b)
		}
		for _, label := range []string{"步骤", "连线类型 依赖", "连线类型 返工", "自动布局", "节点配置"} {
			b := box(label)
			if !b.In(image.Rectangle{Max: size}) || a.ui.DAG.InspectorOpen && b.Max.X > size.X-320 {
				t.Fatalf("canvas toolbar %q is clipped or covered by inspector at %v: %v", label, size, b)
			}
		}
	}
}

func TestDAGLayoutPersistenceAndInvalidPositions(t *testing.T) {
	a := dagTestApp(t)
	a.ui.WorkflowDraft.ID = "native_dag_persistence"
	a.editingWorkflow = ""
	a.moveDAGNode("security", 181, 143)
	a.layoutDAG()
	a.moveDAGNode("security", 220, 144)
	a.connectDAGNode("security")
	a.connectDAGNode("tests")
	if err := renameWorkflowStep(&a.ui.WorkflowDraft, 0, "security_review"); err != nil {
		t.Fatal(err)
	}
	a.syncWorkflowDocument()
	var parsed domainworkflows.Definition
	if err := json.Unmarshal([]byte(a.workflowEditor.Value()), &parsed); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(parsed.Layout, a.ui.WorkflowDraft.Layout) || !hasEdge(parsed, "security_review", "tests", domainworkflows.EdgeNext) {
		t.Fatal("JSON did not retain layout/reference changes")
	}
	_, err := a.backend.service.CreateDefinition(context.Background(), parsed)
	if err != nil {
		t.Fatal(err)
	}
	rootDir := a.backend.store.Data.Paths.Root
	if err := a.backend.close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := openBackendWithEngine(context.Background(), rootDir, testEngine{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.close() })
	defs, err := reopened.service.ListDefinitions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, d := range defs {
		if d.ID == parsed.ID {
			found = true
			if !reflect.DeepEqual(d.Layout, parsed.Layout) || !reflect.DeepEqual(d.Steps, parsed.Steps) {
				t.Fatal("saved DAG lost data")
			}
		}
	}
	if !found || !reflect.DeepEqual(defs[slices.IndexFunc(defs, func(d domainworkflows.Definition) bool { return d.ID == parsed.ID })].Edges, parsed.Edges) {
		t.Fatal("saved DAG was missing after reopening store")
	}
	p := dagPosition(parsed, "unknown")
	if p != (domainworkflows.Point{X: 50, Y: 50}) {
		t.Fatalf("missing position: %+v", p)
	}
	parsed.Layout.Nodes["security_review"] = domainworkflows.Point{X: math.NaN(), Y: math.Inf(1)}
	if p := dagPosition(parsed, "security_review"); p != (domainworkflows.Point{X: 50, Y: 50}) {
		t.Fatal("invalid position reached rasterizer")
	}
	a.ui.WorkflowDraft.Layout.Nodes["tests"] = domainworkflows.Point{X: 1800, Y: 1400}
	a.moveDAGNode("tests", 1820, 1420)
	if p := a.ui.WorkflowDraft.Layout.Nodes["tests"]; p.X < 1800 || p.Y < 1400 {
		t.Fatal("imported large canvas snapped to default bounds")
	}
	before := a.ui.WorkflowDraft.Layout.Nodes["tests"]
	a.moveDAGNode("tests", math.NaN(), 0)
	if a.ui.WorkflowDraft.Layout.Nodes["tests"] != before {
		t.Fatal("invalid drag changed layout")
	}
	a.ui.WorkflowDraft.Steps[0].Sandbox = "full"
	a.setDAGWorker(0, "remote-worker")
	if a.ui.WorkflowDraft.Steps[0].Sandbox != "workspace-write" {
		t.Fatal("remote worker retained full sandbox")
	}
}

func TestRenderDAGPreview(t *testing.T) {
	dir := os.Getenv("ONECATCH_KEEL_PREVIEWS")
	if dir == "" {
		t.Skip("set ONECATCH_KEEL_PREVIEWS to render native DAG")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	a := dagTestApp(t)
	applyPalette(false)
	// The browser preview uses its demo definition. Match that data without
	// changing the real backend's built-in definition or a user's stored layout.
	preview := starterWorkflow("design_to_pr")
	a.ui.WorkflowDraft = preview
	a.resetWorkflowForm()
	a.syncWorkflowDocument()
	for _, state := range []string{"default", "connected", "layout", "narrow", "inspector", "dark"} {
		w, h := 1040, 760
		a.ui.DAG.InspectorOpen = state == "inspector"
		switch state {
		case "connected":
			a.ui.DAG.ConnectFrom = "review"
		case "layout":
			a.ui.DAG.ConnectFrom = ""
			a.layoutDAG()
		case "narrow":
			w, h = 860, 600
		case "inspector":
			w, h = 860, 600
		case "dark":
			applyPalette(true)
		}
		if err := window.Screenshot(el.Root(workflowView{a}), w, h, filepath.Join(dir, "workflow-dag-"+state+".png")); err != nil {
			t.Fatal(err)
		}
	}
	applyPalette(false)
}

func TestDAGKeyboardMovement(t *testing.T) {
	a := dagTestApp(t)
	focus := true
	root := el.Root(el.ViewFunc(func(cx *el.Context) el.Element {
		if focus {
			cx.Focus("dag-node/security")
			focus = false
		}
		return el.Div().WFull().HFull().Child(a.dagCanvas(cx))
	}))
	var router input.Router
	var ops op.Ops
	frame := func() {
		ops.Reset()
		root.Layout(layout.Context{Ops: &ops, Now: time.Now(), Source: router.Source(), Constraints: layout.Exact(image.Pt(1600, 1000)), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}})
		router.Frame(&ops)
	}
	frame()
	frame()
	before := a.ui.WorkflowDraft.Layout.Nodes["security"]
	router.Queue(key.Event{Name: key.NameRightArrow, State: key.Press})
	frame()
	if p := a.ui.WorkflowDraft.Layout.Nodes["security"]; p.X != before.X+8 {
		t.Fatalf("keyboard movement: %+v", p)
	}
}
