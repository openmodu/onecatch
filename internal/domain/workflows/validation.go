package workflows

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/openmodu/onecatch/internal/domain/harnesses"
)

const (
	maxReworkRoundsLimit = 50
	maxFailuresLimit     = 100
	maxTimeoutSeconds    = 24 * 60 * 60
)

var identifierPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)

type ValidationIssue struct {
	Path    string `json:"path"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type ValidationErrors []ValidationIssue

func (e ValidationErrors) Error() string {
	if len(e) == 0 {
		return "workflow definition is invalid"
	}
	return fmt.Sprintf("workflow definition is invalid: %s: %s", e[0].Path, e[0].Message)
}

// Normalize returns a copy of def with safe defaults. It does not silently
// repair IDs, graph edges or user instructions.
func Normalize(def Definition) Definition {
	out := def
	if out.Policy.MaxReworkRounds == 0 {
		out.Policy.MaxReworkRounds = DefaultMaxReworkRounds
	}
	if out.Policy.MaxConsecutiveFailures == 0 {
		out.Policy.MaxConsecutiveFailures = DefaultMaxConsecutiveFailures
	}
	if out.Policy.StepTimeoutSeconds == 0 {
		out.Policy.StepTimeoutSeconds = DefaultStepTimeoutSeconds
	}
	out.Steps = append([]Step(nil), def.Steps...)
	for i := range out.Steps {
		out.Steps[i].ReasoningEffort = strings.ToLower(strings.TrimSpace(out.Steps[i].ReasoningEffort))
		if out.Steps[i].Kind == "" {
			out.Steps[i].Kind = StepAgent
		}
		if out.Steps[i].WorkerID == "" {
			out.Steps[i].WorkerID = "local"
		}
	}
	out.Edges = append([]Edge(nil), def.Edges...)
	for i := range out.Edges {
		if out.Edges[i].Kind == "" {
			out.Edges[i].Kind = EdgeNext
		}
	}
	if def.Layout.Nodes != nil {
		out.Layout.Nodes = make(map[string]Point, len(def.Layout.Nodes))
		for stepID, point := range def.Layout.Nodes {
			out.Layout.Nodes[stepID] = point
		}
	}
	return out
}

// Validate returns all definition problems it can find in one pass so an
// editor can show useful field-level feedback.
func Validate(input Definition) error {
	def := Normalize(input)
	var issues ValidationErrors
	add := func(path, code, message string) {
		issues = append(issues, ValidationIssue{Path: path, Code: code, Message: message})
	}

	if !identifierPattern.MatchString(def.ID) {
		add("id", "invalid_identifier", "must be a lowercase identifier")
	}
	if strings.TrimSpace(def.Name) == "" {
		add("name", "required", "is required")
	}
	if len(def.Steps) == 0 {
		add("steps", "required", "must contain at least one step")
	}
	validatePolicy(def.Policy, add)

	steps := make(map[string]Step, len(def.Steps))
	for i, step := range def.Steps {
		path := fmt.Sprintf("steps[%d]", i)
		if !identifierPattern.MatchString(step.ID) {
			add(path+".id", "invalid_identifier", "must be a lowercase identifier")
		}
		if _, exists := steps[step.ID]; exists {
			add(path+".id", "duplicate", "must be unique")
		} else {
			steps[step.ID] = step
		}
		if strings.TrimSpace(step.Name) == "" {
			add(path+".name", "required", "is required")
		}
		if !identifierPattern.MatchString(step.WorkerID) {
			add(path+".workerId", "invalid_identifier", "must be a lowercase identifier")
		}
		switch step.Kind {
		case StepAgent:
			if step.ReasoningEffort != "" {
				harness, ok := harnesses.Find(step.Runtime)
				if !ok || !harness.SupportsEffort(step.ReasoningEffort) {
					add(path+".reasoningEffort", "invalid_value", "is not supported by this runtime")
				}
			}
			if strings.TrimSpace(step.Runtime) == "" {
				add(path+".runtime", "required", "is required")
			}
			if step.Sandbox != "" && step.Sandbox != "read-only" && step.Sandbox != "workspace-write" && step.Sandbox != "full" {
				add(path+".sandbox", "invalid_value", "must be read-only, workspace-write or full")
			}
			if step.WorkerID != "local" && step.Sandbox == "full" {
				add(path+".sandbox", "remote_full_unsupported", "remote workers can synchronize workspace-write changes only")
			}
		case StepCommand:
			if strings.TrimSpace(step.Command) == "" {
				add(path+".command", "required", "is required")
			}
			if step.WorkerID != "local" {
				add(path+".workerId", "command_local_only", "command steps run on this machine")
			}
		default:
			add(path+".kind", "invalid_value", "must be agent or command")
		}
	}

	type edgeID struct{ from, to, kind string }
	seenEdges := make(map[edgeID]bool, len(def.Edges))
	for i, edge := range def.Edges {
		path := fmt.Sprintf("edges[%d]", i)
		if edge.Kind != EdgeNext && edge.Kind != EdgeRework {
			add(path+".kind", "invalid_value", "must be next or rework")
			continue
		}
		_, fromOK := steps[edge.From]
		_, toOK := steps[edge.To]
		if !fromOK {
			add(path+".from", "unknown_step", "does not reference an existing step")
		}
		if !toOK {
			add(path+".to", "unknown_step", "does not reference an existing step")
		}
		if edge.From == edge.To {
			add(path, "self_edge", "a step cannot connect to itself")
		}
		if edge.MaxRounds < 0 || edge.MaxRounds > maxReworkRoundsLimit {
			add(path+".maxRounds", "out_of_range", fmt.Sprintf("must be between 0 and %d", maxReworkRoundsLimit))
		}
		key := edgeID{edge.From, edge.To, edge.Kind}
		if seenEdges[key] {
			add(path, "duplicate", "edge must be unique")
		}
		seenEdges[key] = true
	}
	if len(issues) > 0 {
		return issues
	}

	if !acyclic(def) {
		add("edges", "cycle", "dependency edges must not form a cycle; use a rework edge to send work back")
		return issues
	}
	for i, edge := range def.Edges {
		if edge.Kind == EdgeRework && !Descendants(def, edge.To)[edge.From] {
			add(fmt.Sprintf("edges[%d]", i), "rework_not_upstream", fmt.Sprintf("rework target %s must be upstream of %s", edge.To, edge.From))
		}
	}
	for i := 0; i < len(def.Steps); i++ {
		for j := i + 1; j < len(def.Steps); j++ {
			left, right := def.Steps[i], def.Steps[j]
			if stepWrites(left) && stepWrites(right) && !Descendants(def, left.ID)[right.ID] && !Descendants(def, right.ID)[left.ID] {
				add(fmt.Sprintf("steps[%d].sandbox", j), "parallel_write_conflict", fmt.Sprintf("write steps %s and %s may run in parallel", left.ID, right.ID))
			}
		}
	}

	if len(issues) > 0 {
		return issues
	}
	return nil
}

func acyclic(def Definition) bool {
	indegree := make(map[string]int, len(def.Steps))
	for _, edge := range def.Edges {
		if isNext(edge) {
			indegree[edge.To]++
		}
	}
	var queue []string
	for _, step := range def.Steps {
		if indegree[step.ID] == 0 {
			queue = append(queue, step.ID)
		}
	}
	visited := 0
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		visited++
		for _, child := range Downstream(def, current) {
			indegree[child]--
			if indegree[child] == 0 {
				queue = append(queue, child)
			}
		}
	}
	return visited == len(def.Steps)
}

// Command steps can do anything a shell can, so they are scheduled as writers.
func stepWrites(step Step) bool {
	return step.Kind == StepCommand || step.Sandbox != "read-only"
}

func validatePolicy(policy Policy, add func(string, string, string)) {
	if policy.MaxReworkRounds < 1 || policy.MaxReworkRounds > maxReworkRoundsLimit {
		add("policy.maxReworkRounds", "out_of_range", fmt.Sprintf("must be between 1 and %d", maxReworkRoundsLimit))
	}
	if policy.MaxConsecutiveFailures < 1 || policy.MaxConsecutiveFailures > maxFailuresLimit {
		add("policy.maxConsecutiveFailures", "out_of_range", fmt.Sprintf("must be between 1 and %d", maxFailuresLimit))
	}
	if policy.StepTimeoutSeconds < 1 || policy.StepTimeoutSeconds > maxTimeoutSeconds {
		add("policy.stepTimeoutSeconds", "out_of_range", fmt.Sprintf("must be between 1 and %d", maxTimeoutSeconds))
	}
}
