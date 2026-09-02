package workflows

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	domainworkflows "github.com/openmodu/onecatch/internal/domain/workflows"
	"github.com/openmodu/onecatch/pkg/localfile"
)

type instructionQueue struct {
	Items []domainworkflows.Instruction `json:"items"`
}

func (r *workflowsImpl) EnqueueInstruction(ctx context.Context, runID string, input domainworkflows.Instruction) (domainworkflows.Instruction, error) {
	if err := ctx.Err(); err != nil {
		return domainworkflows.Instruction{}, err
	}
	if !localfile.ValidID(runID) || !localfile.ValidID(input.ID) || strings.TrimSpace(input.Content) == "" {
		return domainworkflows.Instruction{}, errors.New("workflow instruction is invalid")
	}
	if input.CreatedAt.IsZero() {
		input.CreatedAt = r.now().UTC()
	}
	input.Content = strings.TrimSpace(input.Content)
	input.Status = domainworkflows.InstructionPending
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, err := r.getRunLocked(runID); err != nil {
		return domainworkflows.Instruction{}, err
	}
	queue, err := r.readInstructionQueueLocked(runID)
	if err != nil {
		return domainworkflows.Instruction{}, err
	}
	for _, item := range queue.Items {
		if item.ID == input.ID {
			return domainworkflows.Instruction{}, ErrStateConflict
		}
	}
	queue.Items = append(queue.Items, input)
	if err := localfile.WriteJSONAtomic(r.instructionsPath(runID), queue); err != nil {
		return domainworkflows.Instruction{}, fmt.Errorf("enqueue workflow instruction: %w", err)
	}
	return input, nil
}

func (r *workflowsImpl) ListInstructions(ctx context.Context, runID string) ([]domainworkflows.Instruction, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !localfile.ValidID(runID) {
		return nil, ErrRunNotFound
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if _, err := r.getRunLocked(runID); err != nil {
		return nil, err
	}
	queue, err := r.readInstructionQueueLocked(runID)
	if err != nil {
		return nil, err
	}
	out := make([]domainworkflows.Instruction, 0, len(queue.Items))
	for _, item := range queue.Items {
		if item.Status != domainworkflows.InstructionRemoved {
			out = append(out, item)
		}
	}
	sortInstructions(out)
	return out, nil
}

// UpdateInstructionMode atomically changes how a pending instruction will be
// consumed. Keeping the same durable record lets the UI promote a follow-up to
// steer and restore it if interruption fails without replacing the message.
func (r *workflowsImpl) UpdateInstructionMode(ctx context.Context, runID, instructionID string, priority, followUp bool) (domainworkflows.Instruction, error) {
	if err := ctx.Err(); err != nil {
		return domainworkflows.Instruction{}, err
	}
	if !localfile.ValidID(runID) || !localfile.ValidID(instructionID) {
		return domainworkflows.Instruction{}, ErrRunNotFound
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	queue, err := r.readInstructionQueueLocked(runID)
	if err != nil {
		return domainworkflows.Instruction{}, err
	}
	for index := range queue.Items {
		item := &queue.Items[index]
		if item.ID != instructionID || item.Status != domainworkflows.InstructionPending {
			continue
		}
		item.Priority = priority
		item.FollowUp = followUp
		if err := localfile.WriteJSONAtomic(r.instructionsPath(runID), queue); err != nil {
			return domainworkflows.Instruction{}, fmt.Errorf("update workflow instruction mode: %w", err)
		}
		return *item, nil
	}
	return domainworkflows.Instruction{}, errors.New("pending instruction was not found")
}

func (r *workflowsImpl) RemoveInstruction(ctx context.Context, runID, instructionID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !localfile.ValidID(runID) || !localfile.ValidID(instructionID) {
		return ErrRunNotFound
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	queue, err := r.readInstructionQueueLocked(runID)
	if err != nil {
		return err
	}
	found := false
	for index := range queue.Items {
		if queue.Items[index].ID == instructionID && queue.Items[index].Status == domainworkflows.InstructionPending {
			queue.Items[index].Status = domainworkflows.InstructionRemoved
			found = true
			break
		}
	}
	if !found {
		return errors.New("pending instruction was not found")
	}
	return localfile.WriteJSONAtomic(r.instructionsPath(runID), queue)
}

// ClaimInstructions atomically marks every pending instruction as applied.
// Priority items are returned first, preserving FIFO order within each class.
func (r *workflowsImpl) ClaimInstructions(ctx context.Context, runID string) ([]domainworkflows.Instruction, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !localfile.ValidID(runID) {
		return nil, ErrRunNotFound
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	queue, err := r.readInstructionQueueLocked(runID)
	if err != nil {
		return nil, err
	}
	now := r.now().UTC()
	var claimed []domainworkflows.Instruction
	for index := range queue.Items {
		if queue.Items[index].Status != domainworkflows.InstructionPending {
			continue
		}
		queue.Items[index].Status = domainworkflows.InstructionApplied
		queue.Items[index].AppliedAt = now
		claimed = append(claimed, queue.Items[index])
	}
	if len(claimed) == 0 {
		return []domainworkflows.Instruction{}, nil
	}
	sortInstructions(claimed)
	if err := localfile.WriteJSONAtomic(r.instructionsPath(runID), queue); err != nil {
		return nil, fmt.Errorf("claim workflow instructions: %w", err)
	}
	return claimed, nil
}

func sortInstructions(items []domainworkflows.Instruction) {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Priority != items[j].Priority {
			return items[i].Priority
		}
		if items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].ID < items[j].ID
		}
		return items[i].CreatedAt.Before(items[j].CreatedAt)
	})
}

func (r *workflowsImpl) readInstructionQueueLocked(runID string) (instructionQueue, error) {
	var queue instructionQueue
	if err := localfile.ReadJSON(r.instructionsPath(runID), &queue); errors.Is(err, os.ErrNotExist) {
		return instructionQueue{Items: []domainworkflows.Instruction{}}, nil
	} else if err != nil {
		return instructionQueue{}, fmt.Errorf("read workflow instructions: %w", err)
	}
	return queue, nil
}

func (r *workflowsImpl) instructionsPath(runID string) string {
	return filepath.Join(r.runsRoot, runID, "instructions.json")
}
