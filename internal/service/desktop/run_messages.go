package desktop

import (
	"context"
	"strings"

	domainworkflows "github.com/openmodu/onecatch/internal/domain/workflows"
)

// RunMessageInput expresses intent; the service reads current execution state
// instead of asking the UI to choose enqueue versus resume from a stale frame.
type RunMessageInput struct {
	Resume          ResumeRunInput
	AttachmentPaths []string
	StepIDs         []string
	Steer           bool
}

// SubmitRunMessage delivers a follow-up or a step-directed message. Persisted
// reports whether the draft became durable even if a later resume failed, so
// callers do not submit the same text and attachments twice on retry.
func (a *Service) SubmitRunMessage(ctx context.Context, runID string, input RunMessageInput) (persisted bool, err error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	active := a.isActive(runID)
	if len(input.StepIDs) == 0 {
		return a.submitContinuation(ctx, runID, input.Resume, input.AttachmentPaths, input.Steer)
	}
	definition, err := a.store.Repos.Workflows.GetRunDefinition(ctx, runID)
	if err != nil {
		return false, mapError(err)
	}
	return a.submitStepMessages(ctx, runID, definition, input.StepIDs, input.Resume.Instruction, input.AttachmentPaths, active, input.Steer)
}

func (a *Service) submitContinuation(ctx context.Context, runID string, input ResumeRunInput, paths []string, steer bool) (persisted bool, err error) {
	// Serialize stopped-run submission with automatic follow-up dispatch. The
	// next sender must observe the dispatch started by the previous sender.
	a.followUpMu.Lock()
	if a.isActive(runID) {
		a.followUpMu.Unlock()
		if steer {
			_, err := a.InterruptAndInsert(ctx, runID, InstructionInput{Content: input.Instruction, AttachmentPaths: paths})
			// Failed inserts roll back the instruction. Keep the draft available.
			return err == nil, err
		}
		instruction, err := a.QueueFollowUp(ctx, runID, InstructionInput{Content: input.Instruction, AttachmentPaths: paths})
		return instruction.ID != "", err
	}
	defer a.followUpMu.Unlock()
	if strings.TrimSpace(input.Instruction) != "" || len(paths) > 0 {
		instruction, enqueueErr := a.enqueueInstruction(ctx, runID, InstructionInput{Content: input.Instruction, AttachmentPaths: paths}, false, false, domainworkflows.InstructionReady)
		if enqueueErr != nil {
			return instruction.ID != "", enqueueErr
		}
		persisted = true
		input.Instruction = ""
	}
	_, err = a.ResumeRunConfigured(ctx, runID, input)
	return persisted || err == nil, err
}

// earliestStep picks the addressed step that none of the others precede, so a
// rerun from it reaches every addressed step.
func earliestStep(d domainworkflows.Definition, steps []string) string {
	for _, candidate := range steps {
		upstream := false
		for _, other := range steps {
			if other != candidate && domainworkflows.Descendants(d, other)[candidate] {
				upstream = true
				break
			}
		}
		if !upstream {
			return candidate
		}
	}
	return ""
}

// submitStepMessages delivers text to named steps. During a turn each step
// gets it on its next run (an interrupting send also stops the current turn);
// on a paused or finished run the run resumes from the earliest named step.
func (a *Service) submitStepMessages(ctx context.Context, runID string, d domainworkflows.Definition, steps []string, text string, paths []string, active, steer bool) (bool, error) {
	persisted := false
	for i, step := range steps {
		input := InstructionInput{StepID: step, Content: text}
		if i == 0 {
			input.AttachmentPaths = paths
		}
		var err error
		if active && steer && i == 0 {
			_, err = a.InterruptAndInsert(ctx, runID, input)
		} else {
			_, err = a.EnqueueInstruction(ctx, runID, input)
		}
		if err != nil {
			return persisted, err
		}
		persisted = true
	}
	if active {
		return persisted, nil
	}
	_, err := a.ResumeRunConfigured(ctx, runID, ResumeRunInput{FromStepID: earliestStep(d, steps)})
	return persisted, err
}
