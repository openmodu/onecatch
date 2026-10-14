package workflows

import (
	"context"
	"errors"
	domainworkflows "github.com/openmodu/onecatch/internal/domain/workflows"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
	"strings"
	"testing"
)

func TestOuterLoopAdvancesThenPausesAndResumesWithGuidance(t *testing.T) {
	def := reviewLoop()
	def.LoopEnabled = true
	engine := &scriptedEngine{available: map[agentrun.Runtime]bool{agentrun.RuntimeCodex: true, agentrun.RuntimeClaude: true}, scripts: []engineScript{
		{result: success(`{"verdict":"pass","summary":"first implementation"}`, "impl")},
		{result: success(`{"verdict":"pass","summary":"first approved"}`, "review")},
		{result: success(`{"verdict":"pass","summary":"second implementation"}`, "impl")},
		{result: success(`{"verdict":"ask_human","summary":"need next direction"}`, "review")},
	}}
	uc, store, task := setupUsecase(t, def, engine)
	run, err := uc.ExecuteTask(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != domainworkflows.RunPaused || run.LoopRound != 2 || len(engine.calls) != 4 {
		t.Fatalf("loop did not advance/stop: %+v calls=%d", run, len(engine.calls))
	}
	if engine.calls[2].ResumeSessionID != "impl" || !strings.Contains(engine.calls[2].Prompt, "first approved") {
		t.Fatalf("lost prior cycle context: %+v", engine.calls[2])
	}
	steps, err := store.Repos.Workflows.ListStepRuns(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	rounds := map[int]int{}
	for _, step := range steps {
		rounds[step.LoopRound]++
	}
	if rounds[1] != 2 || rounds[2] != 2 {
		t.Fatalf("round history: %v", rounds)
	}
	engine.scripts = []engineScript{{result: success(`{"verdict":"pass","summary":"review done"}`, "review")}, {result: success(`{"verdict":"ask_human","summary":"round three pause"}`, "impl")}}
	resumed, err := uc.ResumeRun(context.Background(), run.ID, "next focus: performance")
	if err != nil {
		t.Fatal(err)
	}
	if resumed.LoopRound != 3 || resumed.Status != domainworkflows.RunPaused || !strings.Contains(engine.calls[4].Prompt, "next focus: performance") {
		t.Fatalf("resume failed: %+v", resumed)
	}
}

func TestOuterLoopDoesNotBypassInnerReworkLimit(t *testing.T) {
	def := reviewLoop()
	def.LoopEnabled = true
	def.Policy.MaxReworkRounds = 1
	engine := &scriptedEngine{available: map[agentrun.Runtime]bool{agentrun.RuntimeCodex: true, agentrun.RuntimeClaude: true}, scripts: []engineScript{
		{result: success(`{"verdict":"pass","summary":"implemented"}`, "impl")},
		{result: success(`{"verdict":"rework","summary":"fix"}`, "review")},
		{result: success(`{"verdict":"pass","summary":"fixed"}`, "impl")},
		{result: success(`{"verdict":"rework","summary":"still wrong"}`, "review")},
	}}
	uc, _, task := setupUsecase(t, def, engine)
	run, err := uc.ExecuteTask(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != domainworkflows.RunPaused || run.LoopRound != 1 || run.PauseReason != domainworkflows.PauseReasonReworkLimit || len(engine.calls) != 4 {
		t.Fatalf("loop bypassed rework: %+v", run)
	}
}

func TestOuterLoopCanBeInterruptedAndResumeSameRound(t *testing.T) {
	def := reviewLoop()
	def.LoopEnabled = true
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	engine := &scriptedEngine{available: map[agentrun.Runtime]bool{agentrun.RuntimeCodex: true, agentrun.RuntimeClaude: true}, scripts: []engineScript{
		{result: success(`{"verdict":"pass","summary":"first impl"}`, "impl")},
		{result: success(`{"verdict":"pass","summary":"first review"}`, "review")},
		{result: success(`{"verdict":"pass","summary":"interrupted"}`, "impl")},
	}, onCall: func(i int) {
		if i == 2 {
			cancel()
		}
	}}
	uc, _, task := setupUsecase(t, def, engine)
	run, err := uc.ExecuteTask(ctx, task.ID)
	if (err != nil && !errors.Is(err, context.Canceled)) || run.Status != domainworkflows.RunPaused || run.LoopRound != 2 {
		t.Fatalf("interrupt: %+v %v", run, err)
	}
	engine.onCall = nil
	engine.scripts = []engineScript{{result: success(`{"verdict":"ask_human","summary":"waiting for confirmation"}`, "impl")}}
	next, err := uc.ResumeRun(context.Background(), run.ID, "use the updated design")
	if err != nil || next.Status != domainworkflows.RunPaused || next.LoopRound != 2 || !strings.Contains(engine.calls[3].Prompt, "use the updated design") || engine.calls[3].ResumeSessionID != "impl" {
		t.Fatalf("resume: %+v %v", next, err)
	}
}

func TestEnableLoopOnCompletedRunStartsAtEntry(t *testing.T) {
	def := reviewLoop()
	engine := &scriptedEngine{available: map[agentrun.Runtime]bool{agentrun.RuntimeCodex: true, agentrun.RuntimeClaude: true}, scripts: []engineScript{
		{result: success(`{"verdict":"pass","summary":"implemented"}`, "impl")},
		{result: success(`{"verdict":"pass","summary":"approved"}`, "review")},
	}}
	uc, store, task := setupUsecase(t, def, engine)
	run, err := uc.ExecuteTask(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	run.LoopEnabled = true
	run.LoopRound = 1
	run, err = store.Repos.Workflows.UpdateRun(context.Background(), run, run.Revision)
	if err != nil {
		t.Fatal(err)
	}
	engine.scripts = []engineScript{{result: success(`{"verdict":"ask_human","summary":"new cycle"}`, "impl")}}
	next, err := uc.ResumeRun(context.Background(), run.ID, "new task for next round")
	if err != nil || next.LoopRound != 2 || engine.calls[2].Runtime != agentrun.RuntimeCodex || !strings.Contains(engine.calls[2].Prompt, "new task for next round") {
		t.Fatalf("did not resume entry: %+v %v", next, err)
	}
}

type cancelAtLoopBoundaryRepo struct {
	WorkflowRepository
	cancel context.CancelFunc
}

func (r cancelAtLoopBoundaryRepo) UpdateRun(ctx context.Context, run domainworkflows.Run, revision int64) (domainworkflows.Run, error) {
	saved, err := r.WorkflowRepository.UpdateRun(ctx, run, revision)
	if err == nil && saved.Status == domainworkflows.RunCompleted {
		r.cancel()
	}
	return saved, err
}

func TestPauseAtLoopBoundaryPersistsNextRound(t *testing.T) {
	def := oneStepPauseWorkflow()
	def.LoopEnabled = true
	engine := &scriptedEngine{available: map[agentrun.Runtime]bool{agentrun.RuntimeCodex: true}, scripts: []engineScript{{result: success(`{"verdict":"pass","summary":"round done"}`, "impl")}}}
	uc, store, task := setupUsecase(t, def, engine)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	uc.workflows = cancelAtLoopBoundaryRepo{WorkflowRepository: uc.workflows, cancel: cancel}
	run, err := uc.ExecuteTask(ctx, task.ID)
	if err != nil && !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if run.Status != domainworkflows.RunPaused || run.LoopRound != 2 {
		t.Fatalf("lost boundary pause: %+v", run)
	}
	persisted, err := store.Repos.Workflows.GetRun(context.Background(), run.ID)
	if err != nil || persisted.Status != domainworkflows.RunPaused || len(engine.calls) != 1 {
		t.Fatalf("boundary did not stop: %+v %v", persisted, err)
	}
}
