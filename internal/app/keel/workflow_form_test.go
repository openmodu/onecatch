package keeldesktop

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/dyike/keel/ui/kit"
	domainworkflows "github.com/openmodu/onecatch/internal/domain/workflows"
)

func graphFixture() domainworkflows.Definition {
	return domainworkflows.Definition{ID: "review", Steps: []domainworkflows.Step{{ID: "implement", Name: "实现"}, {ID: "review", Name: "审查"}}, Edges: []domainworkflows.Edge{{From: "implement", To: "review", Kind: domainworkflows.EdgeNext}, {From: "review", To: "implement", Kind: domainworkflows.EdgeRework}}, Layout: domainworkflows.Layout{Nodes: map[string]domainworkflows.Point{"implement": {X: 10, Y: 20}}}}
}

func TestWorkflowRenamePreservesGraphReferences(t *testing.T) {
	d := graphFixture()
	if err := renameWorkflowStep(&d, 0, "build"); err != nil {
		t.Fatal(err)
	}
	if d.Edges[0].From != "build" || d.Edges[1].To != "build" || d.Layout.Nodes["build"].X != 10 {
		t.Fatalf("references not updated: %+v", d)
	}
	if _, exists := d.Layout.Nodes["implement"]; exists {
		t.Fatal("stale layout")
	}
	before := domainworkflows.Normalize(d)
	if err := renameWorkflowStep(&d, 0, "review"); err == nil {
		t.Fatal("duplicate accepted")
	}
	if !reflect.DeepEqual(before, domainworkflows.Normalize(d)) {
		t.Fatal("duplicate rename mutated draft")
	}
}

func TestWorkflowRemoveRepairsEdges(t *testing.T) {
	d := graphFixture()
	if !removeWorkflowNode(&d, 0) {
		t.Fatal("not removed")
	}
	if d.Steps[0].ID != "review" || len(d.Edges) != 0 || len(d.Layout.Nodes) != 0 {
		t.Fatalf("dangling references: %+v", d)
	}
	if removeWorkflowNode(&d, 0) {
		t.Fatal("last node was removed")
	}
}

func TestNewWorkflowCanBeSavedWithoutRepair(t *testing.T) {
	a := &app{workflowEditor: kit.CodeEditor("{}")}
	a.newWorkflow()
	var d domainworkflows.Definition
	if err := json.Unmarshal([]byte(a.workflowEditor.Value()), &d); err != nil {
		t.Fatal(err)
	}
	if err := domainworkflows.Validate(d); err != nil {
		t.Fatal(err)
	}
}

func TestDraftSelectorsTolerateMissingAndDuplicateIDs(t *testing.T) {
	options := uniqueSelectOptions([]kit.SelectOption{{Value: "", Label: "Empty"}, {Value: "a", Label: "First"}, {Value: "a", Label: "Duplicate"}, {Value: "b", Label: "Second"}})
	if len(options) != 2 || options[0].Label != "First" {
		t.Fatalf("options: %+v", options)
	}
}

func TestWorkflowSourceLivePreviewKeepsLastValidDraft(t *testing.T) {
	a := &app{workflowEditor: kit.CodeEditor("{}")}
	a.ui.WorkflowDraft = starterWorkflow("implement_review")
	a.syncWorkflowDocument()
	source := a.workflowEditor.Value()
	a.applyWorkflowSource(source)
	if a.ui.DAG.SourceError != "" {
		t.Fatal(a.ui.DAG.SourceError)
	}
	id := a.ui.WorkflowDraft.ID
	a.applyWorkflowSource("{\n invalid")
	if a.ui.DAG.SourceError == "" || a.ui.WorkflowDraft.ID != id {
		t.Fatal("invalid source replaced preview")
	}
	a.applyWorkflowSource(source)
	if a.ui.DAG.SourceError != "" {
		t.Fatal("source error not cleared")
	}
}
