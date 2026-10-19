package workflows

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	domainworkflows "github.com/openmodu/onecatch/internal/domain/workflows"
)

const designToPR = `name: 设计到 PR
max_rework: 3
steps:
  design:
    agent: claude/opus
    sandbox: read-only
    prompt: 阅读相关代码，给出实现方案，不写代码。
  implement:
    agent: codex
    after: design
  review:
    agent: claude/sonnet
    sandbox: read-only
    after: implement
    rework: [implement, design]     # 不通过时可以退回给谁
  e2e:
    run: npm run e2e
    after: review
    rework: implement
  pr:
    agent: modu
    after: e2e
    prompt: 提交并创建 PR。
`

func TestParseWorkflowFile(t *testing.T) {
	def, err := ParseWorkflowFile("design_to_pr", []byte(designToPR))
	if err != nil {
		t.Fatal(err)
	}
	if def.ID != "repo-design_to_pr" || def.Name != "设计到 PR" || len(def.Steps) != 5 {
		t.Fatalf("definition = %+v", def)
	}
	review := def.Steps[2]
	if review.ID != "review" || review.Runtime != "claude" || review.Model != "sonnet" || review.Sandbox != "read-only" {
		t.Fatalf("review = %+v", review)
	}
	if e2e := def.Steps[3]; e2e.Kind != domainworkflows.StepCommand || e2e.Command != "npm run e2e" {
		t.Fatalf("e2e = %+v", e2e)
	}
	var rework []string
	for _, e := range domainworkflows.ReworkEdges(def, "review") {
		rework = append(rework, e.To)
	}
	if !reflect.DeepEqual(rework, []string{"implement", "design"}) || !reflect.DeepEqual(domainworkflows.Upstream(def, "pr"), []string{"e2e"}) {
		t.Fatalf("edges = %+v", def.Edges)
	}
}

func TestParseWorkflowFileNamesTheLineToFix(t *testing.T) {
	cases := map[string]struct {
		text string
		line int
		want string
	}{
		"unknown step key":  {"name: x\nsteps:\n  a:\n    agnet: codex\n", 4, "unknown step key"},
		"rework downstream": {"name: x\nsteps:\n  a:\n    agent: codex\n    rework: b\n  b:\n    agent: codex\n    sandbox: read-only\n    after: a\n", 3, "upstream"},
		"cycle":             {"name: x\nsteps:\n  a:\n    agent: codex\n    after: b\n  b:\n    agent: codex\n    after: a\n", 0, "cycle"},
		"bad yaml":          {"name: x\nsteps:\n  a: [\n", 3, ""},
		"command and agent": {"name: x\nsteps:\n  a:\n    agent: codex\n    run: make\n", 3, "either a command or an Agent"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := ParseWorkflowFile("x", []byte(c.text))
			if err == nil {
				t.Fatal("invalid file accepted")
			}
			if c.line > 0 && err.Line != c.line || !strings.Contains(err.Message, c.want) {
				t.Fatalf("error = %+v, want line %d containing %q", err, c.line, c.want)
			}
		})
	}
}

func TestWorkflowFileRoundTrip(t *testing.T) {
	def, fileErr := ParseWorkflowFile("design_to_pr", []byte(designToPR))
	if fileErr != nil {
		t.Fatal(fileErr)
	}
	def.Steps[0].ReasoningEffort = "high"
	data, err := MarshalWorkflowFile(def)
	if err != nil {
		t.Fatal(err)
	}
	again, fileErr := ParseWorkflowFile("design_to_pr", data)
	if fileErr != nil {
		t.Fatalf("exported file does not parse: %v\n%s", fileErr, data)
	}
	if !reflect.DeepEqual(def.Steps, again.Steps) || !reflect.DeepEqual(def.Edges, again.Edges) || def.Policy != again.Policy {
		t.Fatalf("round trip changed the workflow:\n%s", data)
	}
}

func TestListProjectWorkflows(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ProjectWorkflowDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, text := range map[string]string{"design_to_pr.yaml": designToPR, "Broken Name.yaml": designToPR, "bad.yml": "steps: 1\n", "notes.txt": "x"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	items, err := ListProjectWorkflows(root)
	if err != nil || len(items) != 3 {
		t.Fatalf("items = %+v, %v", items, err)
	}
	valid := 0
	for _, item := range items {
		if item.Err == nil {
			valid++
		}
	}
	if valid != 1 {
		t.Fatalf("valid = %d: %+v", valid, items)
	}
	got, err := GetProjectWorkflow(root, "repo-design_to_pr")
	if err != nil || got.Err != nil || got.Definition.Name != "设计到 PR" {
		t.Fatalf("GetProjectWorkflow = %+v, %v", got, err)
	}
}

func TestProjectWorkflowSupportsOuterLoop(t *testing.T) {
	def, err := ParseWorkflowFile("design_to_pr", []byte("loop: true\n"+designToPR))
	if err != nil || !def.LoopEnabled {
		t.Fatalf("loop config: %+v %v", def, err)
	}
	if _, err := ParseWorkflowFile("design_to_pr", []byte("loop: forever\n"+designToPR)); err == nil {
		t.Fatal("invalid loop value accepted")
	}
}
