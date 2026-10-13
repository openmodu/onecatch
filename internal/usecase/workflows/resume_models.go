package workflows

import domainworkflows "github.com/openmodu/onecatch/internal/domain/workflows"

// RefreshStepModels refreshes execution settings for matching Agent nodes.
// Graphs, prompts and rework policies stay in the durable run snapshot.
func RefreshStepModels(snapshot, latest domainworkflows.Definition) domainworkflows.Definition {
	if snapshot.ID != latest.ID || snapshot.ID == directAgentWorkflowID {
		return snapshot
	}
	snapshot.Steps = append([]domainworkflows.Step(nil), snapshot.Steps...)
	for i := range snapshot.Steps {
		step := &snapshot.Steps[i]
		current, ok := domainworkflows.FindStep(latest, step.ID)
		if !ok || step.Kind == domainworkflows.StepCommand || current.Kind != step.Kind {
			continue
		}
		step.Runtime, step.Model, step.Sandbox, step.WorkerID = current.Runtime, current.Model, current.Sandbox, current.WorkerID
	}
	return snapshot
}

// RefreshRunDefinition applies explicit template edits at a stopped boundary.
// Loop mode itself belongs to Run and is unaffected by template defaults.
func RefreshRunDefinition(snapshot, latest domainworkflows.Definition) domainworkflows.Definition {
	if snapshot.ID != latest.ID || snapshot.ID == directAgentWorkflowID {
		return snapshot
	}
	return domainworkflows.Normalize(latest)
}
