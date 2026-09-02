package workflows

import (
	"context"

	domainworkflows "github.com/openmodu/onecatch/internal/domain/workflows"
)

// Notifier receives a run ID whenever that run's bounded state changed.
type Notifier interface {
	MarkDirty(runID string)
}

// notifyingRepo wraps a WorkflowsRepo and reports every mutation that can move
// a run's bounded state. Decorating the repository keeps the notification in
// one place: the orchestrator mutates run and step state from a dozen sites
// inside its drive loop, and the desktop app mutates instructions from its own
// handlers, but every one of them lands here.
//
// Runtime events are deliberately NOT reported: they are the unbounded half of
// a run and already stream to the frontend as runstream frames.
type notifyingRepo struct {
	WorkflowsRepo
	notify Notifier
}

// WithNotifier returns repo decorated so bounded-state mutations mark the run
// dirty. A nil notifier returns the repo untouched.
func WithNotifier(repo WorkflowsRepo, notify Notifier) WorkflowsRepo {
	if repo == nil || notify == nil {
		return repo
	}
	return &notifyingRepo{WorkflowsRepo: repo, notify: notify}
}

func (r *notifyingRepo) SaveRun(ctx context.Context, run domainworkflows.Run, definition domainworkflows.Definition) error {
	if err := r.WorkflowsRepo.SaveRun(ctx, run, definition); err != nil {
		return err
	}
	r.notify.MarkDirty(run.ID)
	return nil
}

func (r *notifyingRepo) UpdateRun(ctx context.Context, run domainworkflows.Run, expectedRevision int64) (domainworkflows.Run, error) {
	updated, err := r.WorkflowsRepo.UpdateRun(ctx, run, expectedRevision)
	if err != nil {
		return updated, err
	}
	r.notify.MarkDirty(updated.ID)
	return updated, nil
}

func (r *notifyingRepo) SaveStepRun(ctx context.Context, stepRun domainworkflows.StepRun) error {
	if err := r.WorkflowsRepo.SaveStepRun(ctx, stepRun); err != nil {
		return err
	}
	r.notify.MarkDirty(stepRun.RunID)
	return nil
}

func (r *notifyingRepo) AppendEvent(ctx context.Context, event domainworkflows.WorkflowEvent) (domainworkflows.WorkflowEvent, error) {
	stored, err := r.WorkflowsRepo.AppendEvent(ctx, event)
	if err != nil {
		return stored, err
	}
	r.notify.MarkDirty(stored.RunID)
	return stored, nil
}

func (r *notifyingRepo) EnqueueInstruction(ctx context.Context, runID string, instruction domainworkflows.Instruction) (domainworkflows.Instruction, error) {
	stored, err := r.WorkflowsRepo.EnqueueInstruction(ctx, runID, instruction)
	if err != nil {
		return stored, err
	}
	r.notify.MarkDirty(runID)
	return stored, nil
}

func (r *notifyingRepo) RemoveInstruction(ctx context.Context, runID, instructionID string) error {
	if err := r.WorkflowsRepo.RemoveInstruction(ctx, runID, instructionID); err != nil {
		return err
	}
	r.notify.MarkDirty(runID)
	return nil
}

func (r *notifyingRepo) UpdateInstructionMode(ctx context.Context, runID, instructionID string, priority, followUp bool) (domainworkflows.Instruction, error) {
	stored, err := r.WorkflowsRepo.UpdateInstructionMode(ctx, runID, instructionID, priority, followUp)
	if err != nil {
		return stored, err
	}
	r.notify.MarkDirty(runID)
	return stored, nil
}

func (r *notifyingRepo) ClaimInstructions(ctx context.Context, runID string) ([]domainworkflows.Instruction, error) {
	claimed, err := r.WorkflowsRepo.ClaimInstructions(ctx, runID)
	if err != nil {
		return claimed, err
	}
	if len(claimed) > 0 {
		r.notify.MarkDirty(runID)
	}
	return claimed, nil
}
