package keeldesktop

import (
	"encoding/json"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dyike/keel/ui/el"
	domainworkflows "github.com/openmodu/onecatch/internal/domain/workflows"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
)

func TestWorkflowEffortSelectionFollowsModelAndNode(t *testing.T) {
	for _, scale := range []int{1, 2} {
		t.Run(fmt.Sprint(scale), func(t *testing.T) {
			a := dagTestApp(t)
			a.ui.WorkflowDraft.Steps = []domainworkflows.Step{
				{ID: "design", Name: "设计", Runtime: "codex", Model: "large"},
				{ID: "review", Name: "审查", Runtime: "codex", Model: "large"},
			}
			catalog := &workflowModelCatalog{configuration: agentrun.HarnessConfiguration{Models: []agentrun.HarnessModel{
				{Model: "large", DisplayName: "Large", Efforts: []string{"low", "high", "xhigh"}},
				{Model: "small", DisplayName: "Small", Efforts: []string{"low"}},
			}}}
			a.ui.DAG.Models = map[workflowModelKey]*workflowModelCatalog{a.workflowModelKey(a.ui.WorkflowDraft.Steps[0]): catalog}
			index := 0
			root := el.Root(el.ViewFunc(func(cx *el.Context) el.Element {
				return el.Div().WFull().HFull().P(20).Gap(16).Child(a.workflowModelPicker(cx, index), a.workflowEffortPicker(cx, index))
			}))
			router, frame, click := alignmentRouter(root, 480, 480, scale)
			choose := func(label string) {
				t.Helper()
				node, ok := alignmentNode(router.AppendSemantics(nil), label, false)
				if !ok {
					t.Fatalf("missing %q", label)
				}
				click(node.Bounds)
			}
			choose("默认强度")
			if dir := os.Getenv("ONECATCH_KEEL_PREVIEWS"); dir != "" {
				if err := os.MkdirAll(dir, 0755); err != nil {
					t.Fatal(err)
				}
				pixels := interactionPixelsScaled(t, root, router, 480*scale, 480*scale, float32(scale))
				file, err := os.Create(filepath.Join(dir, fmt.Sprintf("workflow-effort-%dx.png", scale)))
				if err != nil {
					t.Fatal(err)
				}
				err = png.Encode(file, pixels)
				file.Close()
				if err != nil {
					t.Fatal(err)
				}
			}
			choose("高 · high")
			var saved domainworkflows.Definition
			if err := json.Unmarshal([]byte(a.workflowEditor.Value()), &saved); err != nil {
				t.Fatal(err)
			}
			if saved.Steps[0].ReasoningEffort != "high" || saved.Steps[1].ReasoningEffort != "" {
				t.Fatalf("effort selection not persisted per node: %+v", saved.Steps)
			}
			index = 1
			frame()
			frame()
			choose("默认强度")
			choose("轻度 · low")
			if a.ui.WorkflowDraft.Steps[0].ReasoningEffort != "high" || a.ui.WorkflowDraft.Steps[1].ReasoningEffort != "low" {
				t.Fatal("effort leaked between nodes")
			}
			index = 0
			frame()
			frame()
			choose("Large · large")
			choose("Small · small")
			if a.ui.WorkflowDraft.Steps[0].ReasoningEffort != "" {
				t.Fatal("model change kept unsupported effort")
			}
			choose("默认强度")
			if _, ok := alignmentNode(router.AppendSemantics(nil), "高 · high", false); ok {
				t.Fatal("small model advertises unsupported high effort")
			}
			choose("轻度 · low")
			choose("轻度 · low")
			choose("默认强度")
			if a.ui.WorkflowDraft.Steps[0].ReasoningEffort != "" {
				t.Fatal("default did not clear effort override")
			}
			a.ui.WorkflowDraft.Steps[0].ReasoningEffort = "high"
			if err := a.validateWorkflowModels(a.ui.WorkflowDraft); err == nil || !strings.Contains(err.Error(), "推理强度") {
				t.Fatalf("invalid model/effort pair accepted: %v", err)
			}
			a.ui.WorkflowDraft.Steps[0].ReasoningEffort = ""
			settings := a.settings.Runtimes["codex"]
			settings.ReasoningEffort = "high"
			a.settings.Runtimes["codex"] = settings
			if err := a.validateWorkflowModels(a.ui.WorkflowDraft); err == nil {
				t.Fatal("unsupported inherited effort accepted for small model")
			}
			a.ui.WorkflowDraft.Steps[0].ReasoningEffort = "low"
			if err := a.validateWorkflowModels(a.ui.WorkflowDraft); err != nil {
				t.Fatal(err)
			}
			a.setDAGRuntime(0, "claude")
			if a.ui.WorkflowDraft.Steps[0].ReasoningEffort != "" {
				t.Fatal("runtime change kept old effort")
			}
		})
	}
}
