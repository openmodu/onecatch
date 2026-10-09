package keeldesktop

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/dyike/keel/ui/kit"
	domainworkflows "github.com/openmodu/onecatch/internal/domain/workflows"
)

func graphFixture() domainworkflows.Definition {
	return domainworkflows.Definition{ID: "review", EntryStepID: "implement", Steps: []domainworkflows.Step{{ID: "implement", Name: "实现", Transitions: map[string]string{"ready": "review"}}, {ID: "review", Name: "审查", DependsOn: []string{"implement"}, Transitions: map[string]string{"retry": "implement", "approved": "$done"}}}, Layout: domainworkflows.Layout{Nodes: map[string]domainworkflows.Point{"implement": {X: 10, Y: 20}}}}
}

func TestWorkflowRenamePreservesGraphReferences(t *testing.T) {
	d := graphFixture()
	if err := renameWorkflowStep(&d, 0, "build"); err != nil {
		t.Fatal(err)
	}
	if d.EntryStepID != "build" || d.Steps[1].DependsOn[0] != "build" || d.Steps[1].Transitions["retry"] != "build" || d.Layout.Nodes["build"].X != 10 {
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
	if d.EntryStepID != "review" || len(d.Steps[0].DependsOn) != 0 || d.Steps[0].Transitions["retry"] != "$pause" || len(d.Layout.Nodes) != 0 {
		t.Fatalf("dangling references: %+v", d)
	}
	if removeWorkflowNode(&d, 0) {
		t.Fatal("last node was removed")
	}
}

func TestWorkflowHistoryRestoresUnsavedNewDraft(t *testing.T) {
	a := &app{workflowEditor: kit.CodeEditor("{}")}
	a.newWorkflow()
	// Form edits and raw edits both survive leaving and returning to a draft.
	a.ui.WorkflowDraft.Name = "未保存的流程"
	a.syncWorkflowDocument()
	a.ui.WorkflowHistory.push(a.currentLocation(), location{Workflow: "existing"})
	a.ui.SelectedWorkflow = "existing"
	a.ui.WorkflowSource = false
	a.goWorkflowHistory(-1)
	if !a.ui.WorkflowSource || a.ui.WorkflowRaw || a.editingWorkflow != "" || a.ui.WorkflowDraft.Name != "未保存的流程" {
		t.Fatalf("new draft lost: %+v", a.ui.WorkflowDraft)
	}
	a.toggleWorkflowRaw()
	a.workflowEditor.SetValue("invalid JSON draft")
	a.goWorkflowHistory(1)
	a.goWorkflowHistory(-1)
	if a.workflowEditor.Value() != "invalid JSON draft" || !a.ui.WorkflowRaw {
		t.Fatal("invalid raw draft discarded")
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
