package keeldesktop

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/kit"
	"github.com/dyike/keel/ui/theme"
	domainworkflows "github.com/openmodu/onecatch/internal/domain/workflows"
)

func (a *app) resetWorkflowForm() {
	a.ui.DAG = dagState{}
	a.ui.WorkflowNumbers = map[string]*kit.NumberInputView{}
	a.ui.WorkflowSelects = map[string]*kit.SelectView{}
	a.ui.WorkflowStep = 0
	a.ui.WorkflowStepID = ""
	if len(a.ui.WorkflowDraft.Steps) > 0 {
		a.ui.WorkflowStepID = a.ui.WorkflowDraft.Steps[0].ID
	}
}

func (a *app) syncWorkflowDocument() {
	data, err := json.MarshalIndent(a.ui.WorkflowDraft, "", "  ")
	if err != nil {
		a.fail(err)
		return
	}
	a.ui.DAG.SourceError = ""
	a.workflowEditor.SetValue(string(data))
}

func (a *app) toggleWorkflowRaw() {
	if a.ui.WorkflowRaw {
		var d domainworkflows.Definition
		if err := json.Unmarshal([]byte(a.workflowEditor.Value()), &d); err != nil {
			a.fail(fmt.Errorf("工作流 JSON 无效：%w", err))
			return
		}
		a.ui.WorkflowDraft = domainworkflows.Normalize(d)
		a.resetWorkflowForm()
	} else {
		a.syncWorkflowDocument()
	}
	a.ui.WorkflowRaw = !a.ui.WorkflowRaw
	// The JSON editor is the common snapshot used for history and persistence.
}

func workflowField(label string, content el.Element) el.Element {
	return el.Div().Gap(6).WFull().Child(el.Text(label).TextSize(12).TextColor(theme.Muted), content)
}

func (a *app) workflowSelect(cx *el.Context, id, label, value string, options []kit.SelectOption, change func(string)) el.Element {
	if a.ui.WorkflowSelects == nil {
		a.ui.WorkflowSelects = map[string]*kit.SelectView{}
	}
	view := a.ui.WorkflowSelects[id]
	if view == nil {
		view = kit.Select("").Hint(label).Appearance(false).MenuWidth(240).MenuAlign(el.End).Size(32)
		if strings.HasPrefix(id, "dag-") {
			view.Size(36)
		}
		if strings.HasPrefix(id, "dag-model/") {
			view.Searchable().MenuWidth(320)
		}
		a.ui.WorkflowSelects[id] = view
	}
	options = uniqueSelectOptions(options)
	if !slices.Equal(view.Entries(), options) {
		view.SetEntries(options...)
	}
	view.SetValue(value)
	view.OnChange(func(value string) { change(value); a.syncWorkflowDocument() })
	return el.Div().WFull().MinH(el.Dp(32)).Px(8).Rounded(8).Bg(theme.Subtle).Child(view.Render(cx))
}

// Invalid drafts must remain editable instead of panicking in kit.Select.
func uniqueSelectOptions(options []kit.SelectOption) []kit.SelectOption {
	seen := map[string]bool{}
	result := make([]kit.SelectOption, 0, len(options))
	for _, option := range options {
		if option.Value != "" && !seen[option.Value] {
			seen[option.Value] = true
			result = append(result, option)
		}
	}
	return result
}

func (a *app) resetWorkflowSelects() { a.ui.WorkflowSelects = map[string]*kit.SelectView{} }

// Renaming a node updates every reference, including canvas positions. Reject
// collisions instead of silently redirecting another node's edges.
func renameWorkflowStep(d *domainworkflows.Definition, index int, id string) error {
	if index < 0 || index >= len(d.Steps) || id == "" || strings.HasPrefix(id, "$") {
		return fmt.Errorf("节点 ID 无效")
	}
	for i, s := range d.Steps {
		if i != index && s.ID == id {
			return fmt.Errorf("节点 ID %s 已存在", id)
		}
	}
	old := d.Steps[index].ID
	d.Steps[index].ID = id
	for i := range d.Edges {
		if d.Edges[i].From == old {
			d.Edges[i].From = id
		}
		if d.Edges[i].To == old {
			d.Edges[i].To = id
		}
	}
	if point, ok := d.Layout.Nodes[old]; ok {
		delete(d.Layout.Nodes, old)
		d.Layout.Nodes[id] = point
	}
	return nil
}

func removeWorkflowNode(d *domainworkflows.Definition, index int) bool {
	if index < 0 || index >= len(d.Steps) || len(d.Steps) <= 1 {
		return false
	}
	id := d.Steps[index].ID
	d.Steps = slices.Delete(d.Steps, index, index+1)
	d.Edges = slices.DeleteFunc(d.Edges, func(e domainworkflows.Edge) bool { return e.From == id || e.To == id })
	delete(d.Layout.Nodes, id)
	return true
}

func (a *app) removeWorkflowStep(index int) {
	if !removeWorkflowNode(&a.ui.WorkflowDraft, index) {
		return
	}
	a.resetWorkflowForm()
	a.syncWorkflowDocument()
}

// applyWorkflowSource keeps incomplete edits out of the interactive preview.
func (a *app) applyWorkflowSource(text string) {
	var d domainworkflows.Definition
	if err := json.Unmarshal([]byte(text), &d); err != nil {
		a.ui.DAG.SourceError = "JSON 无效：" + err.Error()
		if syntax, ok := err.(*json.SyntaxError); ok {
			end := min(len(text), int(syntax.Offset))
			a.ui.DAG.SourceError = fmt.Sprintf("第 %d 行：%s", strings.Count(text[:end], "\n")+1, err)
		}
		return
	}
	d = domainworkflows.Normalize(d)
	if err := domainworkflows.Validate(d); err != nil {
		a.ui.DAG.SourceError = err.Error()
		return
	}
	a.ui.DAG.SourceError = ""
	a.ui.WorkflowDraft = d
	a.ui.WorkflowSelects = map[string]*kit.SelectView{}
	a.ui.DAG.Validation = nil
}
