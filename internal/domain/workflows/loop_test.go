package workflows

import (
	"testing"
	"time"
)

func TestOuterLoopKeepsSessionsAndResetsOnlyCompletedRound(t *testing.T) {
	def := Definition{ID: "loop", Name: "Loop", LoopEnabled: true, Steps: []Step{{ID: "design", Name: "Design", Runtime: "codex"}, {ID: "review", Name: "Review", Runtime: "codex"}}, Edges: []Edge{{From: "design", To: "review"}}}
	run, err := NewRun(def, "run", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	run.Status = RunCompleted
	run.Sessions["design"] = "session"
	run.Nodes["design"] = NodeState{StepID: "design", Status: NodeCompleted, Attempt: 4, Summary: "implemented first task"}
	run.Nodes["review"] = NodeState{StepID: "review", Status: NodeCompleted, Attempt: 3, Summary: "approved"}
	run.ReworkRounds = map[string]int{"review>design": 2}
	next, err := NextLoopRound(def, run, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if next.LoopRound != 2 || next.Status != RunRunning || next.Sessions["design"] != "session" || next.Nodes["design"].Attempt != 4 || len(next.ReworkRounds) != 0 || len(next.Nodes["design"].Inbox) != 1 {
		t.Fatalf("invalid outer loop: %+v", next)
	}
	if run.Nodes["design"].Status != NodeCompleted || len(run.ReworkRounds) != 1 {
		t.Fatal("mutated previous round")
	}
	run.Status = RunPaused
	if _, err := NextLoopRound(def, run, time.Now()); err == nil {
		t.Fatal("loop bypassed pause")
	}
}
