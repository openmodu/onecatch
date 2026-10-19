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
	if input.Status != domainworkflows.InstructionReady {
		input.Status = domainworkflows.InstructionPending
	}
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

// UpdateInstructionContent edits only pending text under the same lock as
// claiming, so a message can never be rewritten after it has been delivered.
func (r *workflowsImpl) UpdateInstructionContent(ctx context.Context, runID, instructionID, content string) (domainworkflows.Instruction, error) {
	if err := ctx.Err(); err != nil {
		return domainworkflows.Instruction{}, err
	}
	if !localfile.ValidID(runID) || !localfile.ValidID(instructionID) || strings.TrimSpace(content) == "" {
		return domainworkflows.Instruction{}, errors.New("workflow instruction is invalid")
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
		item.Content = strings.TrimSpace(content)
		if err := localfile.WriteJSONAtomic(r.instructionsPath(runID), queue); err != nil {
			return domainworkflows.Instruction{}, fmt.Errorf("edit workflow instruction: %w", err)
		}
		return *item, nil
	}
	return domainworkflows.Instruction{}, errors.New("消息已发送或已删除，无法保存编辑")
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

// ClaimInstructions claims boundary guidance, or one FIFO follow-up when a
// fresh turn has no other input. Follow-ups never join a turn already in flight.
func (r *workflowsImpl) ClaimInstructions(ctx context.Context, runID string, includeFollowUp bool) ([]domainworkflows.Instruction, error) {
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
	sortInstructions(queue.Items)
	now := r.now().UTC()
	var claimed []domainworkflows.Instruction
	for index := range queue.Items {
		item := &queue.Items[index]
		if item.Status != domainworkflows.InstructionReady && (item.Status != domainworkflows.InstructionPending || item.FollowUp) {
			continue
		}
		item.Status, item.AppliedAt = domainworkflows.InstructionApplied, now
		claimed = append(claimed, *item)
	}
	if len(claimed) == 0 && includeFollowUp {
		for index := range queue.Items {
			item := &queue.Items[index]
			if item.Status == domainworkflows.InstructionPending && item.FollowUp {
				item.Status, item.AppliedAt = domainworkflows.InstructionApplied, now
				claimed = append(claimed, *item)
				break
			}
		}
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
			return false // Equal timestamps retain durable insertion order.
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
