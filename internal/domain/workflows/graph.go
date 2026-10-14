package workflows

// Graph helpers read the definition's edges. They assume a validated
// definition; unknown step IDs simply have no neighbours.

// An empty kind is a dependency edge, matching Normalize.
func isNext(edge Edge) bool { return edge.Kind == EdgeNext || edge.Kind == "" }

// Upstream returns the steps that id directly depends on, in edge order.
func Upstream(def Definition, id string) []string {
	var out []string
	for _, edge := range def.Edges {
		if isNext(edge) && edge.To == id {
			out = append(out, edge.From)
		}
	}
	return out
}

// Downstream returns the steps that directly depend on id, in edge order.
func Downstream(def Definition, id string) []string {
	var out []string
	for _, edge := range def.Edges {
		if isNext(edge) && edge.From == id {
			out = append(out, edge.To)
		}
	}
	return out
}

// ReworkEdges returns the rework edges leaving id.
func ReworkEdges(def Definition, id string) []Edge {
	var out []Edge
	for _, edge := range def.Edges {
		if edge.Kind == EdgeRework && edge.From == id {
			out = append(out, edge)
		}
	}
	return out
}

// EntrySteps returns steps with no dependencies, in definition order.
func EntrySteps(def Definition) []string {
	var out []string
	for _, step := range def.Steps {
		if len(Upstream(def, step.ID)) == 0 {
			out = append(out, step.ID)
		}
	}
	return out
}

// SinkSteps returns steps nothing depends on, in definition order.
func SinkSteps(def Definition) []string {
	var out []string
	for _, step := range def.Steps {
		if len(Downstream(def, step.ID)) == 0 {
			out = append(out, step.ID)
		}
	}
	return out
}

// Descendants returns every step reachable from id through dependency edges,
// excluding id itself.
func Descendants(def Definition, id string) map[string]bool {
	seen := map[string]bool{}
	queue := []string{id}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, child := range Downstream(def, current) {
			if !seen[child] {
				seen[child] = true
				queue = append(queue, child)
			}
		}
	}
	return seen
}

// ReworkPath returns the steps that must run again when from sends work back
// to target: target, from, and every step on a dependency path between them.
// Parallel branches that do not lead to from are left alone.
func ReworkPath(def Definition, from, target string) map[string]bool {
	below := Descendants(def, target)
	path := map[string]bool{target: true, from: true}
	for stepID := range below {
		if stepID == from || Descendants(def, stepID)[from] {
			path[stepID] = true
		}
	}
	return path
}

// EdgeKey identifies an edge in Run.ReworkRounds.
func EdgeKey(from, to string) string { return from + ">" + to }

// FindStep returns the step with id.
func FindStep(def Definition, id string) (Step, bool) {
	for _, step := range def.Steps {
		if step.ID == id {
			return step, true
		}
	}
	return Step{}, false
}
