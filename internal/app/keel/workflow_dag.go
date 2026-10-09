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
	ConnectFrom  string
	DragID       string
	DragX, DragY float32
	IDError      string
	SignalError  string
	Validation   domainworkflows.ValidationErrors
	Edges        dagEdgeInput
	SignalOrder  map[string][]string
}

func dagPosition(d domainworkflows.Definition, id string) domainworkflows.Point {
	p, ok := d.Layout.Nodes[id]
	if !ok || math.IsNaN(p.X) || math.IsInf(p.X, 0) || math.IsNaN(p.Y) || math.IsInf(p.Y, 0) {
		return domainworkflows.Point{X: 50, Y: 50}
	}
	return domainworkflows.Point{X: max(10, min(1e6, p.X)), Y: max(10, min(1e6, p.Y))}
}

func dagAutoLayout(d domainworkflows.Definition) map[string]domainworkflows.Point {
	steps := map[string]domainworkflows.Step{}
	for _, s := range d.Steps {
		steps[s.ID] = s
	}
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
		for _, dep := range steps[id].DependsOn {
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
		nodes[s.ID] = domainworkflows.Point{X: float64(60 + level*310), Y: float64(55 + row*155)}
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
			a.ui.WorkflowSignal = ""
			a.ui.DAG.IDError = ""
			a.ui.DAG.SignalError = ""
			a.resetWorkflowSelects()
			return
		}
	}
}
func (a *app) connectDAGNode(id string) {
	d := &a.ui.WorkflowDraft
	source := a.ui.DAG.ConnectFrom
	if !slices.ContainsFunc(d.Steps, func(s domainworkflows.Step) bool { return s.ID == id }) {
		a.ui.DAG.ConnectFrom = ""
		return
	}
	if source == "" {
		a.ui.DAG.ConnectFrom = id
		return
	}
	if source != id && slices.ContainsFunc(d.Steps, func(s domainworkflows.Step) bool { return s.ID == source }) {
		for i := range d.Steps {
			if d.Steps[i].ID == id && !slices.Contains(d.Steps[i].DependsOn, source) {
				d.Steps[i].DependsOn = append(d.Steps[i].DependsOn, source)
				a.syncWorkflowDocument()
				break
			}
		}
	}
	a.ui.DAG.ConnectFrom = ""
}
func (a *app) removeDAGEdge(source, target string) {
	for i := range a.ui.WorkflowDraft.Steps {
		s := &a.ui.WorkflowDraft.Steps[i]
		if s.ID == target {
			s.DependsOn = slices.DeleteFunc(s.DependsOn, func(id string) bool { return id == source })
			a.syncWorkflowDocument()
			return
		}
	}
}
func (a *app) addDAGNode() {
	d := &a.ui.WorkflowDraft
	index := len(d.Steps) + 1
	id := fmt.Sprintf("node_%d", index)
	for slices.ContainsFunc(d.Steps, func(s domainworkflows.Step) bool { return s.ID == id }) {
		index++
		id = fmt.Sprintf("node_%d", index)
	}
	runtime := ""
	for _, r := range a.runtimes {
		if r.Enabled {
			runtime = r.ID
			break
		}
	}
	sandbox := a.settings.Execution.DefaultSandbox
	if sandbox == "" {
		sandbox = "workspace-write"
	}
	d.Steps = append(d.Steps, domainworkflows.Step{ID: id, Name: "新节点", Runtime: runtime, WorkerID: "local", Sandbox: sandbox, RolePrompt: "你是负责这个节点的 Agent。", Instruction: "完成当前节点目标。", Transitions: map[string]string{"completed": "$done", "need_human": "$pause"}})
	if d.Layout.Nodes == nil {
		d.Layout.Nodes = map[string]domainworkflows.Point{}
	}
	d.Layout.Nodes[id] = domainworkflows.Point{X: float64(120 + (len(d.Steps)-1)*34), Y: float64(110 + (len(d.Steps)-1)*34)}
	if d.EntryStepID == "" {
		d.EntryStepID = id
	}
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
