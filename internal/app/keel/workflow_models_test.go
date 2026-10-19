package keeldesktop

import (
	"encoding/json"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/kit"
	domainworkflows "github.com/openmodu/onecatch/internal/domain/workflows"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
)

func TestWorkflowModelPickerSavesAdvertisedID(t *testing.T) {
	for _, scale := range []int{1, 2} {
		t.Run(fmt.Sprint(scale), func(t *testing.T) {
			a := dagTestApp(t)
			a.ui.WorkflowDraft.Steps = []domainworkflows.Step{
				{ID: "design", Name: "设计", Kind: domainworkflows.StepAgent, Runtime: "codex", WorkerID: "local", Model: "6.1-sol"},
				{ID: "review", Name: "审查", Kind: domainworkflows.StepAgent, Runtime: "codex", WorkerID: "local"},
			}
			catalog := &workflowModelCatalog{configuration: agentrun.HarnessConfiguration{Models: []agentrun.HarnessModel{
				{Model: "runtime-model-id", DisplayName: "Friendly model"},
				{Model: "second-model-id", DisplayName: "Second model"},
			}}}
			a.ui.DAG.Models = map[workflowModelKey]*workflowModelCatalog{a.workflowModelKey(a.ui.WorkflowDraft.Steps[0]): catalog}
			index := 0
			root := el.Root(el.ViewFunc(func(cx *el.Context) el.Element {
				return el.Div().WFull().HFull().P(20).Child(a.workflowModelPicker(cx, index))
			}))
			router, frame, click := alignmentRouter(root, 480, 420, scale)
			if _, ok := alignmentNode(router.AppendSemantics(nil), "当前模型不在可用列表中，请重新选择。", false); !ok {
				t.Fatal("invalid saved model is not explained")
			}
			choose := func(label string) {
				t.Helper()
				node, ok := alignmentNode(router.AppendSemantics(nil), label, false)
				if !ok {
					t.Fatalf("missing %q", label)
				}
				click(node.Bounds)
			}
			choose("6.1-sol · 不可用")
			if !slices.ContainsFunc(a.ui.WorkflowSelects["dag-model/design"].Entries(), func(o kit.SelectOption) bool { return o.Value == "6.1-sol" && o.Disabled }) {
				t.Fatal("unadvertised model can be selected")
			}
			choose("Friendly model · runtime-model-id")
			if a.ui.WorkflowDraft.Steps[0].Model != "runtime-model-id" {
				t.Fatal("model picker saved a display label instead of the advertised ID")
			}
			var saved domainworkflows.Definition
			if err := json.Unmarshal([]byte(a.workflowEditor.Value()), &saved); err != nil {
				t.Fatal(err)
			}
			if saved.Steps[0].Model != "runtime-model-id" || saved.Steps[1].Model != "" {
				t.Fatalf("model selection changed another step: %+v", saved.Steps)
			}
			index = 1
			frame()
			frame()
			choose("使用运行环境默认模型")
			choose("Second model · second-model-id")
			if a.ui.WorkflowDraft.Steps[0].Model != "runtime-model-id" || a.ui.WorkflowDraft.Steps[1].Model != "second-model-id" {
				t.Fatal("node model selections are not independent")
			}
			choose("Second model · second-model-id")
			choose("使用运行环境默认模型")
			if a.ui.WorkflowDraft.Steps[1].Model != "" {
				t.Fatal("default model did not clear override")
			}
		})
	}
}

func TestWorkflowModelsRejectInvalidSavedIDsAndRuntimeChanges(t *testing.T) {
	a := dagTestApp(t)
	a.ui.WorkflowDraft.Steps = []domainworkflows.Step{{ID: "design", Name: "设计", Kind: domainworkflows.StepAgent, Runtime: "codex", WorkerID: "local", Model: "6.1-sol"}}
	key := a.workflowModelKey(a.ui.WorkflowDraft.Steps[0])
	a.ui.DAG.Models = map[workflowModelKey]*workflowModelCatalog{key: {configuration: agentrun.HarnessConfiguration{Models: []agentrun.HarnessModel{{Model: "runtime-model-id", DisplayName: "Friendly model"}}}}}
	if err := a.validateWorkflowModels(a.ui.WorkflowDraft); err == nil || !strings.Contains(err.Error(), "6.1-sol") {
		t.Fatalf("invalid model not rejected: %v", err)
	}
	a.ui.WorkflowDraft.Steps[0].Model = "runtime-model-id"
	if err := a.validateWorkflowModels(a.ui.WorkflowDraft); err != nil {
		t.Fatal(err)
	}
	a.ui.DAG.Models[key].err = "offline"
	if err := a.validateWorkflowModels(a.ui.WorkflowDraft); err == nil || !strings.Contains(err.Error(), "offline") {
		t.Fatalf("failed catalog treated as valid: %v", err)
	}
	a.ui.DAG.Models[key].err = ""
	a.ui.WorkflowDraft.Steps[0].Model = ""
	settings := a.settings.Runtimes["codex"]
	settings.DefaultModel = "6.1-sol"
	a.settings.Runtimes["codex"] = settings
	if err := a.validateWorkflowModels(a.ui.WorkflowDraft); err == nil || !strings.Contains(err.Error(), "6.1-sol") {
		t.Fatalf("invalid inherited model not rejected: %v", err)
	}
	a.setDAGRuntime(0, "claude")
	if a.ui.WorkflowDraft.Steps[0].Model != "" {
		t.Fatal("runtime change kept incompatible model")
	}
	a.ui.WorkflowDraft.Steps[0].Model = "local-only"
	a.setDAGWorker(0, "remote")
	if a.ui.WorkflowDraft.Steps[0].Model != "" {
		t.Fatal("worker change kept local model")
	}
	options := workflowModelOptions(a.ui.WorkflowDraft.Steps[0], nil)
	if len(options) != 1 || options[0].Value != "__default" {
		t.Fatalf("local models leaked into remote choices: %+v", options)
	}
}

func TestWorkflowModelCatalogLoadsOnceAndDiscardsStaleReply(t *testing.T) {
	a := dagTestApp(t)
	bin := filepath.Join(t.TempDir(), "codex")
	script := `#!/bin/sh
if [ "$1" = "--version" ]; then printf 'codex-cli 1.0.0\n'; exit 0; fi
while IFS= read -r line; do
 case "$line" in
  *'"method":"initialize"'*) printf '%s\n' '{"id":1,"result":{}}' ;;
  *'"method":"config/read"'*) printf '%s\n' '{"id":2,"result":{"config":{"model":"runtime-model-id"}}}' ;;
  *'"method":"model/list"'*) printf '%s\n' '{"id":3,"result":{"data":[{"model":"runtime-model-id","displayName":"Friendly model","isDefault":true}]}}' ;;
 esac
done
`
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	settings := a.settings.Runtimes["codex"]
	settings.Binary, settings.Enabled = bin, true
	a.settings.Runtimes["codex"] = settings
	updates := make(chan func(), 4)
	a.postUpdate = func(fn func()) { updates <- fn }
	key := a.workflowModelKey(domainworkflows.Step{Runtime: "codex", WorkerID: "local"})
	a.loadWorkflowModels(key)
	first := a.ui.DAG.Models[key]
	a.loadWorkflowModels(key)
	if a.ui.DAG.Models[key] != first {
		t.Fatal("redraw started another catalog request")
	}
	nextUpdate := func() func() {
		t.Helper()
		select {
		case fn := <-updates:
			return fn
		case <-time.After(10 * time.Second):
			t.Fatal("catalog inspection did not complete")
			return nil
		}
	}
	applyStale := nextUpdate()
	a.resetWorkflowForm()
	a.loadWorkflowModels(key)
	second := a.ui.DAG.Models[key]
	applyStale()
	if !second.loading || len(second.configuration.Models) != 0 {
		t.Fatal("stale inspection changed the new editor")
	}
	nextUpdate()()
	if second.loading || second.err != "" || !workflowCatalogContains(second, "runtime-model-id") {
		t.Fatalf("runtime catalog not loaded: %+v", second)
	}
	options := workflowModelOptions(domainworkflows.Step{}, second)
	if !slices.ContainsFunc(options, func(o kit.SelectOption) bool {
		return o.Value == "runtime-model-id" && o.Label == "Friendly model · runtime-model-id"
	}) {
		t.Fatalf("runtime response did not reach choices: %+v", options)
	}
	// Settings/workspace changes cannot reuse the previous account's catalog.
	a.settings.Revision++
	if a.workflowModelKey(domainworkflows.Step{Runtime: "codex"}) == key {
		t.Fatal("settings revision missing from catalog identity")
	}
}

func TestRenderWorkflowModelCatalog(t *testing.T) {
	dir := os.Getenv("ONECATCH_KEEL_PREVIEWS")
	if dir == "" {
		t.Skip("set ONECATCH_KEEL_PREVIEWS to capture")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	a := dagTestApp(t)
	step := &a.ui.WorkflowDraft.Steps[0]
	step.Runtime, step.Model = "codex", "6.1-sol"
	a.ui.DAG.Models = map[workflowModelKey]*workflowModelCatalog{a.workflowModelKey(*step): {configuration: agentrun.HarnessConfiguration{Models: []agentrun.HarnessModel{
		{Model: "runtime-model-id", DisplayName: "Friendly model"},
		{Model: "second-model-id", DisplayName: "Second model"},
	}}}}
	root := el.Root(workflowView{a})
	router, _, click := alignmentRouter(root, 1280, 900, 2)
	field, ok := alignmentNode(router.AppendSemantics(nil), "6.1-sol · 不可用", false)
	if !ok {
		t.Fatal("model picker missing from workflow inspector")
	}
	click(field.Bounds)
	pixels := interactionPixelsScaled(t, root, router, 2560, 1800, 2)
	file, err := os.Create(filepath.Join(dir, "workflow-model-catalog.png"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := png.Encode(file, pixels); err != nil {
		t.Fatal(err)
	}
}
