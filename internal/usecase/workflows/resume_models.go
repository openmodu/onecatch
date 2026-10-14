package workflows

import domainworkflows "github.com/openmodu/onecatch/internal/domain/workflows"

// RefreshStepSettings updates every field of existing nodes and execution
// policies. Topology changes are adopted on resume or at the next loop round,
// when no old graph nodes are in flight.
func RefreshStepSettings(snapshot, latest domainworkflows.Definition) domainworkflows.Definition {
	if snapshot.ID != latest.ID || snapshot.ID == directAgentWorkflowID {
		return snapshot
	}
	latest = domainworkflows.Normalize(latest)
	snapshot.Name, snapshot.Description = latest.Name, latest.Description
	snapshot.Policy = latest.Policy
	snapshot.Steps = append([]domainworkflows.Step(nil), snapshot.Steps...)
	for i, step := range snapshot.Steps {
		if current, ok := domainworkflows.FindStep(latest, step.ID); ok {
			snapshot.Steps[i] = current
		}
	}
	snapshot.Edges = append([]domainworkflows.Edge(nil), snapshot.Edges...)
	for i, edge := range snapshot.Edges {
		for _, current := range latest.Edges {
			if edge.From == current.From && edge.To == current.To && edge.Kind == current.Kind {
				snapshot.Edges[i] = current
				break
			}
		}
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
