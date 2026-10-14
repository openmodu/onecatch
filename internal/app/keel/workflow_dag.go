package keeldesktop

import (
	"fmt"
	"math"
	"slices"

	domainworkflows "github.com/openmodu/onecatch/internal/domain/workflows"
)

const dagNodeWidth, dagNodeHeight = 204, 94
const dagCanvasWidth, dagCanvasHeight = 1600, 1000

type dagState struct {
	InspectorOpen bool
	SourceError   string
	ConnectFrom   string
	// ConnectKind is the edge the next connection creates: a dependency, or a
	// rework edge from a checking step back to the step it can return work to.
	ConnectKind  string
	DragID       string
	DragX, DragY float32
	IDError      string
	Validation   domainworkflows.ValidationErrors
	Edges        dagEdgeInput
}

func (s dagState) connectKind() string {
	if s.ConnectKind == domainworkflows.EdgeRework {
		return domainworkflows.EdgeRework
	}
	return domainworkflows.EdgeNext
}

func dagPosition(d domainworkflows.Definition, id string) domainworkflows.Point {
	p, ok := d.Layout.Nodes[id]
	if !ok || math.IsNaN(p.X) || math.IsInf(p.X, 0) || math.IsNaN(p.Y) || math.IsInf(p.Y, 0) {
		return domainworkflows.Point{X: 50, Y: 50}
	}
	return domainworkflows.Point{X: max(10, min(1e6, p.X)), Y: max(10, min(1e6, p.Y))}
}

// dagAutoLayout places each step one column to the right of its deepest
// dependency. Rework edges do not affect columns.
func dagAutoLayout(d domainworkflows.Definition) map[string]domainworkflows.Point {
	levels := map[string]int{}
	visiting := map[string]bool{}
	var levelOf func(string) int
	levelOf = func(id string) int {
		if level, ok := levels[id]; ok {
			return level
		}
		if visiting[id] {
			return 0
		}
		visiting[id] = true
		level := 0
		for _, dep := range domainworkflows.Upstream(d, id) {
			level = max(level, levelOf(dep)+1)
		}
		delete(visiting, id)
		levels[id] = level
		return level
	}
	for _, s := range d.Steps {
		levelOf(s.ID)
	}
	rows := map[int]int{}
	nodes := map[string]domainworkflows.Point{}
	for _, s := range d.Steps {
		level := levels[s.ID]
		row := rows[level]
		rows[level]++
		nodes[s.ID] = domainworkflows.Point{X: float64(60 + level*290), Y: float64(80 + row*175)}
	}
	return nodes
}
func (a *app) layoutDAG() {
	a.ui.WorkflowDraft.Layout.Nodes = dagAutoLayout(a.ui.WorkflowDraft)
	a.syncWorkflowDocument()
}
func (a *app) selectDAGNode(id string) {
	for i, s := range a.ui.WorkflowDraft.Steps {
		if s.ID == id {
			a.ui.WorkflowStep = i
			a.ui.WorkflowStepID = id
			a.ui.DAG.IDError = ""
			a.resetWorkflowSelects()
			return
		}
	}
}

func (a *app) setDAGConnectKind(kind string) {
	a.ui.DAG.ConnectKind = kind
	a.ui.DAG.ConnectFrom = ""
}

// connectDAGNode is a two-click gesture: the first click picks the source, the
// second adds an edge of the current kind to the clicked step.
func (a *app) connectDAGNode(id string) {
	d := &a.ui.WorkflowDraft
	source := a.ui.DAG.ConnectFrom
	exists := func(id string) bool {
		return slices.ContainsFunc(d.Steps, func(s domainworkflows.Step) bool { return s.ID == id })
	}
	if !exists(id) {
		a.ui.DAG.ConnectFrom = ""
		return
	}
	if source == "" {
		a.ui.DAG.ConnectFrom = id
		return
	}
	edge := domainworkflows.Edge{From: source, To: id, Kind: a.ui.DAG.connectKind()}
	if source != id && exists(source) && !slices.ContainsFunc(d.Edges, func(e domainworkflows.Edge) bool {
		return e.From == edge.From && e.To == edge.To && e.Kind == edge.Kind
	}) {
		d.Edges = append(d.Edges, edge)
		a.syncWorkflowDocument()
		a.validateDAG()
	}
	a.ui.DAG.ConnectFrom = ""
}
func (a *app) removeDAGEdge(source, target, kind string) {
	d := &a.ui.WorkflowDraft
	before := len(d.Edges)
	d.Edges = slices.DeleteFunc(d.Edges, func(e domainworkflows.Edge) bool {
		return e.From == source && e.To == target && e.Kind == kind
	})
	if len(d.Edges) != before {
		a.syncWorkflowDocument()
		a.validateDAG()
	}
}
func (a *app) addDAGNode() {
	d := &a.ui.WorkflowDraft
	index := len(d.Steps) + 1
	id := fmt.Sprintf("step_%d", index)
	for slices.ContainsFunc(d.Steps, func(s domainworkflows.Step) bool { return s.ID == id }) {
		index++
		id = fmt.Sprintf("step_%d", index)
	}
	runtime := ""
	for _, r := range a.runtimes {
		if r.Enabled {
			runtime = r.ID
			break
		}
	}
	// New steps start read-only so adding one never creates a parallel write
	// conflict; the user widens access when the step needs it.
	d.Steps = append(d.Steps, domainworkflows.Step{ID: id, Name: "新步骤", Kind: domainworkflows.StepAgent, Runtime: runtime, WorkerID: "local", Sandbox: "read-only"})
	if d.Layout.Nodes == nil {
		d.Layout.Nodes = map[string]domainworkflows.Point{}
	}
	d.Layout.Nodes[id] = domainworkflows.Point{X: float64(120 + (len(d.Steps)-1)*34), Y: float64(110 + (len(d.Steps)-1)*34)}
	a.selectDAGNode(id)
	a.syncWorkflowDocument()
}
func (a *app) moveDAGNode(id string, x, y float64) {
	d := &a.ui.WorkflowDraft
	if !dagFinite(x) || !dagFinite(y) || !slices.ContainsFunc(d.Steps, func(s domainworkflows.Step) bool { return s.ID == id }) {
		return
	}
	if d.Layout.Nodes == nil {
		d.Layout.Nodes = map[string]domainworkflows.Point{}
	}
	w, h := dagExtent(*d)
	d.Layout.Nodes[id] = domainworkflows.Point{X: max(10, min(float64(w)-224, x)), Y: max(10, min(float64(h)-106, y))}
	a.syncWorkflowDocument()
}

func (a *app) validateDAG() {
	a.ui.DAG.Validation = nil
	if err := domainworkflows.Validate(a.ui.WorkflowDraft); err != nil {
		if issues, ok := err.(domainworkflows.ValidationErrors); ok {
			a.ui.DAG.Validation = issues
		}
	}
}

func (a *app) setDAGWorker(index int, id string) {
	step := &a.ui.WorkflowDraft.Steps[index]
	step.WorkerID = id
	if id != "" && id != "local" && step.Sandbox == "full" {
		step.Sandbox = "workspace-write"
	}
}

func (a *app) setDAGKind(index int, kind string) {
	step := &a.ui.WorkflowDraft.Steps[index]
	step.Kind = kind
	if kind == domainworkflows.StepCommand {
		step.WorkerID = "local"
	}
	a.resetWorkflowSelects()
}
