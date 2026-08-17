package workflows

import (
	"context"
	"encoding/json"
	"testing"

	domainworkflows "github.com/openmodu/onecatch/internal/domain/workflows"
)

type recordingNotifier struct{ dirty []string }

func (r *recordingNotifier) MarkDirty(runID string) { r.dirty = append(r.dirty, runID) }

// stubRepo embeds the interface so only the methods a test exercises need real
// bodies; the rest panic if unexpectedly called.
type stubRepo struct {
	WorkflowsRepo
	updateRun func(domainworkflows.Run) (domainworkflows.Run, error)
	claim     func(string) ([]domainworkflows.Instruction, error)
}

func (s stubRepo) UpdateRun(_ context.Context, run domainworkflows.Run, _ int64) (domainworkflows.Run, error) {
	return s.updateRun(run)
}

func (s stubRepo) SaveStepRun(context.Context, domainworkflows.StepRun) error { return nil }

func (s stubRepo) ClaimInstructions(_ context.Context, runID string) ([]domainworkflows.Instruction, error) {
	return s.claim(runID)
}

func (s stubRepo) AppendRuntimeEvent(context.Context, string, string, json.RawMessage) (domainworkflows.RuntimeEvent, error) {
	return domainworkflows.RuntimeEvent{}, nil
}

func TestNotifyingRepoMarksRunDirtyOnUpdate(t *testing.T) {
	notifier := &recordingNotifier{}
	repo := WithNotifier(stubRepo{
		updateRun: func(run domainworkflows.Run) (domainworkflows.Run, error) { return run, nil },
	}, notifier)

	if _, err := repo.UpdateRun(context.Background(), domainworkflows.Run{ID: "run_1"}, 0); err != nil {
		t.Fatalf("UpdateRun: %v", err)
	}
	if len(notifier.dirty) != 1 || notifier.dirty[0] != "run_1" {
		t.Fatalf("expected run_1 marked dirty, got %v", notifier.dirty)
	}
}

func TestNotifyingRepoSkipsDirtyWhenUpdateFails(t *testing.T) {
	notifier := &recordingNotifier{}
	repo := WithNotifier(stubRepo{
		updateRun: func(domainworkflows.Run) (domainworkflows.Run, error) { return domainworkflows.Run{}, context.Canceled },
	}, notifier)

	if _, err := repo.UpdateRun(context.Background(), domainworkflows.Run{ID: "run_1"}, 0); err == nil {
		t.Fatal("expected error")
	}
	if len(notifier.dirty) != 0 {
		t.Fatalf("a failed write must not mark dirty, got %v", notifier.dirty)
	}
}

func TestNotifyingRepoSkipsDirtyOnEmptyClaim(t *testing.T) {
	notifier := &recordingNotifier{}
	repo := WithNotifier(stubRepo{
		claim: func(string) ([]domainworkflows.Instruction, error) { return nil, nil },
	}, notifier)

	if _, err := repo.ClaimInstructions(context.Background(), "run_1"); err != nil {
		t.Fatalf("ClaimInstructions: %v", err)
	}
	if len(notifier.dirty) != 0 {
		t.Fatalf("claiming nothing must not mark dirty, got %v", notifier.dirty)
	}
}

func TestNotifyingRepoNilNotifierReturnsRepo(t *testing.T) {
	base := stubRepo{}
	if _, decorated := WithNotifier(base, nil).(*notifyingRepo); decorated {
		t.Fatal("nil notifier should return the undecorated repo, not a decorator")
	}
	if _, decorated := WithNotifier(base, &recordingNotifier{}).(*notifyingRepo); !decorated {
		t.Fatal("a real notifier should return the decorator")
	}
}
