package workflows

import domainworkflows "github.com/openmodu/onecatch/internal/domain/workflows"

// RefreshStepModels updates model choices for existing compatible steps only.
// The run retains its graph, prompts, execution boundaries and Agent sessions.
func RefreshStepModels(snapshot, latest domainworkflows.Definition) domainworkflows.Definition {
	if snapshot.ID != latest.ID || snapshot.ID == directAgentWorkflowID {
		return snapshot
	}
	snapshot.Steps = append([]domainworkflows.Step(nil), snapshot.Steps...)
	for i := range snapshot.Steps {
		step := &snapshot.Steps[i]
		current, ok := domainworkflows.FindStep(latest, step.ID)
		if !ok || step.Kind == domainworkflows.StepCommand || current.Kind != step.Kind || current.Runtime != step.Runtime || current.WorkerID != step.WorkerID {
			continue
		}
		step.Model = current.Model
	}
	return snapshot
}
