package workflows

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

const (
	maxTransitionsLimit = 10_000
	maxFailuresLimit    = 100
	maxTimeoutSeconds   = 24 * 60 * 60
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

// Normalize returns a deep-enough copy of def with safe policy defaults. It
// does not silently repair IDs, graph edges or user instructions.
func Normalize(def Definition) Definition {
	out := def
	if out.Mode == "" {
		out.Mode = ModeSerial
	}
	if out.Policy.MaxTransitions == 0 {
		out.Policy.MaxTransitions = DefaultMaxTransitions
	}
	if out.Policy.MaxConsecutiveFailures == 0 {
		out.Policy.MaxConsecutiveFailures = DefaultMaxConsecutiveFailures
	}
	if out.Policy.StepTimeoutSeconds == 0 {
		out.Policy.StepTimeoutSeconds = DefaultStepTimeoutSeconds
	}
	out.Steps = append([]Step(nil), def.Steps...)
	for i := range out.Steps {
		if out.Steps[i].WorkerID == "" {
			out.Steps[i].WorkerID = "local"
		}
		out.Steps[i].DependsOn = append([]string(nil), def.Steps[i].DependsOn...)
		if def.Steps[i].Transitions != nil {
			out.Steps[i].Transitions = make(map[string]string, len(def.Steps[i].Transitions))
			for signal, target := range def.Steps[i].Transitions {
				out.Steps[i].Transitions[signal] = target
			}
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
	if def.Mode != ModeSerial && def.Mode != ModeDAG {
		add("mode", "invalid_value", "must be serial or dag")
	}
	if !identifierPattern.MatchString(def.EntryStepID) {
		add("entryStepId", "invalid_identifier", "must be a lowercase step identifier")
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
		if strings.TrimSpace(step.Runtime) == "" {
			add(path+".runtime", "required", "is required")
		}
		if !identifierPattern.MatchString(step.WorkerID) {
			add(path+".workerId", "invalid_identifier", "must be a lowercase identifier")
		}
		if step.WorkerID != "local" && step.Sandbox == "full" {
			add(path+".sandbox", "remote_full_unsupported", "remote workers can synchronize workspace-write changes only")
		}
		if strings.TrimSpace(step.RolePrompt) == "" {
			add(path+".rolePrompt", "required", "is required")
		}
		if strings.TrimSpace(step.Instruction) == "" {
			add(path+".instruction", "required", "is required")
		}
		if step.Sandbox != "" && step.Sandbox != "read-only" && step.Sandbox != "workspace-write" && step.Sandbox != "full" {
			add(path+".sandbox", "invalid_value", "must be read-only, workspace-write or full")
		}
		if len(step.Transitions) == 0 {
			add(path+".transitions", "required", "must contain at least one signal")
		}
		signals := make([]string, 0, len(step.Transitions))
		for signal := range step.Transitions {
			signals = append(signals, signal)
		}
		sort.Strings(signals)
		hasDone := false
		for _, signal := range signals {
			target := step.Transitions[signal]
			transitionPath := path + ".transitions." + signal
			if !identifierPattern.MatchString(signal) {
				add(transitionPath, "invalid_signal", "signal must be a lowercase identifier")
			}
			if !isTerminalTarget(target) && !identifierPattern.MatchString(target) {
				add(transitionPath, "invalid_target", "target must be a step identifier or reserved terminal")
			}
			if target == TargetDone {
				hasDone = true
			}
			if def.Mode == ModeDAG && !isTerminalTarget(target) {
				add(transitionPath, "dag_invalid_target", "DAG step ordering must use dependsOn; signal targets must be terminal")
			}
		}
		if def.Mode == ModeDAG && !hasDone {
			add(path+".transitions", "no_completion_path", "DAG step must declare a $done outcome")
		}
	}

	if _, ok := steps[def.EntryStepID]; !ok {
		add("entryStepId", "unknown_step", "does not reference an existing step")
	}

	for i, step := range def.Steps {
		for signal, target := range step.Transitions {
			if isTerminalTarget(target) {
				continue
			}
			if _, ok := steps[target]; !ok {
				add(fmt.Sprintf("steps[%d].transitions.%s", i, signal), "unknown_step", "target does not exist")
			}
		}
	}

	if def.Mode == ModeDAG {
		validateDAG(def, steps, add)
	} else if _, ok := steps[def.EntryStepID]; ok {
		reachable, reachesDone := reachableSteps(def.EntryStepID, steps)
		for i, step := range def.Steps {
			if _, ok := reachable[step.ID]; !ok {
				add(fmt.Sprintf("steps[%d].id", i), "unreachable", "step is not reachable from entryStepId")
			}
		}
		if !reachesDone {
			add("steps", "no_completion_path", "entry step cannot reach a $done transition")
		}
	}

	if len(issues) > 0 {
		return issues
	}
	return nil
}

func validateDAG(def Definition, steps map[string]Step, add func(string, string, string)) {
	indegree := make(map[string]int, len(steps))
	children := make(map[string][]string, len(steps))
	for _, step := range def.Steps {
		seen := make(map[string]struct{}, len(step.DependsOn))
		for index, dependency := range step.DependsOn {
			path := fmt.Sprintf("steps[%s].dependsOn[%d]", step.ID, index)
			if dependency == step.ID {
				add(path, "dag_self_dependency", "step cannot depend on itself")
				continue
			}
			if _, duplicate := seen[dependency]; duplicate {
				add(path, "duplicate", "dependency must be unique")
				continue
			}
			seen[dependency] = struct{}{}
			if _, ok := steps[dependency]; !ok {
				add(path, "dag_unknown_dependency", "dependency does not exist")
				continue
			}
			indegree[step.ID]++
			children[dependency] = append(children[dependency], step.ID)
		}
	}
	queue := make([]string, 0, len(steps))
	for stepID := range steps {
		if indegree[stepID] == 0 {
			queue = append(queue, stepID)
		}
	}
	visited := 0
	for len(queue) > 0 {
		stepID := queue[0]
		queue = queue[1:]
		visited++
		for _, child := range children[stepID] {
			indegree[child]--
			if indegree[child] == 0 {
				queue = append(queue, child)
			}
		}
	}
	if visited != len(steps) {
		add("steps", "dag_cycle", "DAG dependencies must not contain a cycle")
		return
	}
	for i := 0; i < len(def.Steps); i++ {
		for j := i + 1; j < len(def.Steps); j++ {
			left, right := def.Steps[i], def.Steps[j]
			if sandboxWrites(left.Sandbox) && sandboxWrites(right.Sandbox) && !dagReaches(left.ID, right.ID, children) && !dagReaches(right.ID, left.ID, children) {
				add(fmt.Sprintf("steps[%d].sandbox", j), "parallel_write_conflict", fmt.Sprintf("write steps %s and %s may run in parallel", left.ID, right.ID))
			}
		}
	}
}

func dagReaches(from, target string, children map[string][]string) bool {
	seen := map[string]struct{}{from: {}}
	queue := []string{from}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, child := range children[current] {
			if child == target {
				return true
			}
			if _, ok := seen[child]; !ok {
				seen[child] = struct{}{}
				queue = append(queue, child)
			}
		}
	}
	return false
}

func sandboxWrites(sandbox string) bool { return sandbox != "read-only" }

func validatePolicy(policy Policy, add func(string, string, string)) {
	if policy.MaxTransitions < 1 || policy.MaxTransitions > maxTransitionsLimit {
		add("policy.maxTransitions", "out_of_range", fmt.Sprintf("must be between 1 and %d", maxTransitionsLimit))
	}
	if policy.MaxConsecutiveFailures < 1 || policy.MaxConsecutiveFailures > maxFailuresLimit {
		add("policy.maxConsecutiveFailures", "out_of_range", fmt.Sprintf("must be between 1 and %d", maxFailuresLimit))
	}
	if policy.StepTimeoutSeconds < 1 || policy.StepTimeoutSeconds > maxTimeoutSeconds {
		add("policy.stepTimeoutSeconds", "out_of_range", fmt.Sprintf("must be between 1 and %d", maxTimeoutSeconds))
	}
}

func reachableSteps(entry string, steps map[string]Step) (map[string]struct{}, bool) {
	seen := make(map[string]struct{}, len(steps))
	stack := []string{entry}
	reachesDone := false
	for len(stack) > 0 {
		id := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if _, ok := seen[id]; ok {
			continue
		}
		step, ok := steps[id]
		if !ok {
			continue
		}
		seen[id] = struct{}{}
		for _, target := range step.Transitions {
			switch {
			case target == TargetDone:
				reachesDone = true
			case !isTerminalTarget(target):
				stack = append(stack, target)
			}
		}
	}
	return seen, reachesDone
}
