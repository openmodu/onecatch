package workflows

import (
	"context"
	"fmt"

	workflows "github.com/openmodu/onecatch/internal/domain/workflows"
	"github.com/openmodu/onecatch/pkg/localfile"
)

// UpdatePausedRunWorkflow serializes the state and definition writes with all
// run mutations. A failed snapshot write restores the previous run state.
func (r *workflowsImpl) UpdatePausedRunWorkflow(ctx context.Context, run workflows.Run, definition workflows.Definition, expectedRevision int64) (workflows.Run, error) {
	if err := ctx.Err(); err != nil {
		return workflows.Run{}, err
	}
	if expectedRevision < 1 || !localfile.ValidID(run.ID) || run.Status != workflows.RunPaused {
		return workflows.Run{}, ErrStateConflict
	}
	definition = workflows.Normalize(definition)
	if err := workflows.Validate(definition); err != nil {
		return workflows.Run{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	current, err := r.getRunLocked(run.ID)
	if err != nil {
		return workflows.Run{}, err
	}
	if current.Status != workflows.RunPaused || current.Revision != expectedRevision || current.TaskID != run.TaskID || current.WorkflowID != run.WorkflowID || definition.ID != run.WorkflowID {
		return workflows.Run{}, ErrStateConflict
	}
	run.Revision = expectedRevision + 1
	if err := validateRunForStorage(run); err != nil {
		return workflows.Run{}, err
	}
	if err := localfile.WriteJSONAtomic(r.runPath(run.ID), run); err != nil {
		return workflows.Run{}, fmt.Errorf("update workflow run: %w", err)
	}
	if err := localfile.WriteJSONAtomic(r.runDefinitionPath(run.ID), definition); err != nil {
		if rollbackErr := localfile.WriteJSONAtomic(r.runPath(run.ID), current); rollbackErr != nil {
			return workflows.Run{}, fmt.Errorf("update workflow snapshot: %w; restore run: %v", err, rollbackErr)
		}
		return workflows.Run{}, fmt.Errorf("update workflow snapshot: %w", err)
	}
	r.refreshRunIndexLocked(run)
	return run, nil
}
