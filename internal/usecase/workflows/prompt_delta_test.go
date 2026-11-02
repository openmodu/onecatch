package workflows

import (
	"context"
	"strings"
	"testing"

	domaintasks "github.com/openmodu/onecatch/internal/domain/tasks"
	dw "github.com/openmodu/onecatch/internal/domain/workflows"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
)

func TestContinuationOnlyIncludesNewWorkflowInputs(t *testing.T) {
	step := dw.Step{ID: "review", Name: "Review", Prompt: "initial step instructions"}
	def := dw.Definition{ID: "pipeline", Steps: []dw.Step{{ID: "build"}, step}, Edges: []dw.Edge{{From: "build", To: "review"}}}
	task := domaintasks.Task{Prompt: "initial task brief"}
	run := dw.Run{Sessions: map[string]string{"review": "session"}, Nodes: map[string]dw.NodeState{
		"build":  {Summary: strings.Repeat("old upstream output ", 1000)},
		"review": {Inbox: []dw.Message{{From: dw.MessageFromHuman, Text: "new instruction"}}},
	}}
	delivered := map[string]string{"build": upstreamDigest(run.Nodes["build"].Summary)}
	prompt := composeStepPrompt(task, def, step, run, delivered)
	for _, forbidden := range []string{"old upstream output", "initial task brief", "initial step instructions", "Response contract"} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("replayed %q", forbidden)
		}
	}
	if !strings.Contains(prompt, "new instruction") {
		t.Fatal("lost user message")
	}
	run.Nodes["build"] = dw.NodeState{Summary: "changed upstream result"}
	if !strings.Contains(composeStepPrompt(task, def, step, run, delivered), "changed upstream result") {
		t.Fatal("lost new dependency result")
	}
	delete(run.Sessions, "review")
	initial := composeStepPrompt(task, def, step, run, delivered)
	for _, required := range []string{"initial task brief", "initial step instructions", "Response contract", "changed upstream result"} {
		if !strings.Contains(initial, required) {
			t.Fatalf("fresh session missing %q", required)
		}
	}
}

func TestWorkflowResumeDoesNotReplayUnchangedUpstream(t *testing.T) {
	engine := &scriptedEngine{
		available: map[agentrun.Runtime]bool{agentrun.RuntimeCodex: true, agentrun.RuntimeClaude: true},
		scripts: []engineScript{
			{runtime: agentrun.RuntimeCodex, result: success(`{"verdict":"pass","summary":"unchanged implementation summary"}`, "impl")},
			{runtime: agentrun.RuntimeClaude, result: success(`{"verdict":"rework","summary":"check again"}`, "review")},
			{runtime: agentrun.RuntimeCodex, result: success(`{"verdict":"pass","summary":"unchanged implementation summary"}`, "impl")},
			{runtime: agentrun.RuntimeClaude, result: success(`{"verdict":"pass","summary":"done"}`, "review")},
		},
	}
	usecase, _, task := setupUsecase(t, reviewLoop(), engine)
	if _, err := usecase.ExecuteTask(context.Background(), task.ID); err != nil {
		t.Fatal(err)
	}
	if len(engine.calls) != 4 {
		t.Fatalf("calls=%d", len(engine.calls))
	}
	if !strings.Contains(engine.calls[1].Prompt, "unchanged implementation summary") {
		t.Fatal("first review lost input")
	}
	if engine.calls[3].ResumeSessionID != "review" || strings.Contains(engine.calls[3].Prompt, "unchanged implementation summary") || strings.Contains(engine.calls[3].Prompt, "Response contract") {
		t.Fatalf("resume replayed context: %s", engine.calls[3].Prompt)
	}
}
