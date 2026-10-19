package workflows

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	domainworkflows "github.com/openmodu/onecatch/internal/domain/workflows"
	"go.yaml.in/yaml/v3"
)

// Workflow files let a project keep its workflows in the repository as YAML:
//
//	name: 设计到 PR
//	max_rework: 3
//	steps:
//	  design:    { agent: claude/opus, sandbox: read-only, prompt: 给出方案 }
//	  implement: { agent: codex, after: design }
//	  review:    { agent: claude/sonnet, after: implement, rework: [implement, design] }
//	  e2e:       { run: npm run e2e, after: review, rework: implement }
//
// Steps keep their file order. "after" lists dependencies, "rework" the
// upstream steps a step may send work back to, "agent" is runtime[/model],
// "reasoning_effort" optionally overrides that step's reasoning level,
// and "run" makes a command step.

const (
	// ProjectWorkflowDir is where a project keeps its workflow files.
	ProjectWorkflowDir = ".onecatch/workflows"
	// ProjectWorkflowPrefix marks definitions that come from a project file.
	ProjectWorkflowPrefix = "repo-"
)

var workflowFileStem = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,58}$`)

// FileError points at the line of a workflow file that needs fixing.
type FileError struct {
	Line    int
	Message string
}

func (e FileError) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("line %d: %s", e.Line, e.Message)
	}
	return e.Message
}

// ProjectWorkflow is one workflow file of a project. Err is set when the file
// cannot be used; Definition is then empty.
type ProjectWorkflow struct {
	Path       string
	Definition domainworkflows.Definition
	Err        *FileError
	ModTime    time.Time
}

// ProjectWorkflowID is the definition ID of the workflow file stem.yaml.
func ProjectWorkflowID(stem string) string { return ProjectWorkflowPrefix + stem }

// IsProjectWorkflowID reports whether id names a project workflow file.
func IsProjectWorkflowID(id string) bool { return strings.HasPrefix(id, ProjectWorkflowPrefix) }

// ListProjectWorkflows reads every workflow file of the project at root, in
// file name order. A missing directory is simply an empty list.
func ListProjectWorkflows(root string) ([]ProjectWorkflow, error) {
	dir := filepath.Join(root, ProjectWorkflowDir)
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list project workflows: %w", err)
	}
	var out []ProjectWorkflow
	for _, entry := range entries {
		ext := filepath.Ext(entry.Name())
		if entry.IsDir() || ext != ".yaml" && ext != ".yml" {
			continue
		}
		out = append(out, readProjectWorkflow(filepath.Join(dir, entry.Name())))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

// GetProjectWorkflow reads the project workflow with definition ID id.
func GetProjectWorkflow(root, id string) (ProjectWorkflow, error) {
	if !IsProjectWorkflowID(id) {
		return ProjectWorkflow{}, ErrDefinitionNotFound
	}
	stem := strings.TrimPrefix(id, ProjectWorkflowPrefix)
	for _, ext := range []string{".yaml", ".yml"} {
		path := filepath.Join(root, ProjectWorkflowDir, stem+ext)
		if _, err := os.Stat(path); err == nil {
			return readProjectWorkflow(path), nil
		}
	}
	return ProjectWorkflow{}, ErrDefinitionNotFound
}

func readProjectWorkflow(path string) ProjectWorkflow {
	item := ProjectWorkflow{Path: path}
	info, err := os.Stat(path)
	if err != nil {
		item.Err = &FileError{Message: err.Error()}
		return item
	}
	item.ModTime = info.ModTime()
	stem := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if !workflowFileStem.MatchString(stem) {
		item.Err = &FileError{Message: "file name must be a lowercase identifier such as design_to_pr.yaml"}
		return item
	}
	data, err := os.ReadFile(path)
	if err != nil {
		item.Err = &FileError{Message: err.Error()}
		return item
	}
	definition, fileErr := ParseWorkflowFile(stem, data)
	if fileErr != nil {
		item.Err = fileErr
		return item
	}
	item.Definition = definition
	return item
}

// ParseWorkflowFile turns a workflow file into a validated definition whose ID
// is ProjectWorkflowID(stem). Errors name the line to fix.
func ParseWorkflowFile(stem string, data []byte) (domainworkflows.Definition, *FileError) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return domainworkflows.Definition{}, yamlError(err)
	}
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return domainworkflows.Definition{}, &FileError{Line: 1, Message: "the file must be a mapping with name and steps"}
	}
	def := domainworkflows.Definition{ID: ProjectWorkflowID(stem)}
	lines := map[string]int{} // step ID → line, for validation messages
	var stepsNode *yaml.Node
	root := doc.Content[0]
	for i := 0; i+1 < len(root.Content); i += 2 {
		key, value := root.Content[i], root.Content[i+1]
		var err *FileError
		switch key.Value {
		case "name":
			def.Name, err = scalar(value)
		case "description":
			def.Description, err = scalar(value)
		case "loop":
			if value.Kind != yaml.ScalarNode || value.Tag != "!!bool" {
				err = &FileError{Line: value.Line, Message: "loop must be true or false"}
			} else {
				def.LoopEnabled = strings.EqualFold(value.Value, "true")
			}
		case "max_rework":
			def.Policy.MaxReworkRounds, err = integer(value)
		case "max_failures":
			def.Policy.MaxConsecutiveFailures, err = integer(value)
		case "timeout":
			def.Policy.StepTimeoutSeconds, err = seconds(value)
		case "steps":
			stepsNode = value
		default:
			err = &FileError{Line: key.Line, Message: fmt.Sprintf("unknown key %q; use name, description, loop, max_rework, max_failures, timeout or steps", key.Value)}
		}
		if err != nil {
			return domainworkflows.Definition{}, err
		}
	}
	if strings.TrimSpace(def.Name) == "" {
		def.Name = stem
	}
	if stepsNode == nil || stepsNode.Kind != yaml.MappingNode || len(stepsNode.Content) == 0 {
		line := root.Line
		if stepsNode != nil {
			line = stepsNode.Line
		}
		return domainworkflows.Definition{}, &FileError{Line: line, Message: "steps must map each step ID to its settings"}
	}
	for i := 0; i+1 < len(stepsNode.Content); i += 2 {
		key, value := stepsNode.Content[i], stepsNode.Content[i+1]
		lines[key.Value] = key.Line
		step, edges, err := parseStep(key, value)
		if err != nil {
			return domainworkflows.Definition{}, err
		}
		def.Steps = append(def.Steps, step)
		def.Edges = append(def.Edges, edges...)
	}
	def = domainworkflows.Normalize(def)
	if err := domainworkflows.Validate(def); err != nil {
		return domainworkflows.Definition{}, validationLine(def, err, lines)
	}
	return def, nil
}

func parseStep(key, value *yaml.Node) (domainworkflows.Step, []domainworkflows.Edge, *FileError) {
	step := domainworkflows.Step{ID: key.Value, Name: key.Value, Kind: domainworkflows.StepAgent}
	if value.Kind != yaml.MappingNode {
		return step, nil, &FileError{Line: value.Line, Message: fmt.Sprintf("step %q must be a mapping such as { agent: codex }", key.Value)}
	}
	var after, rework []string
	rounds := 0
	for i := 0; i+1 < len(value.Content); i += 2 {
		k, v := value.Content[i], value.Content[i+1]
		var err *FileError
		switch k.Value {
		case "name":
			step.Name, err = scalar(v)
		case "agent":
			var agent string
			if agent, err = scalar(v); err == nil {
				runtime, model, _ := strings.Cut(agent, "/")
				step.Runtime, step.Model = strings.TrimSpace(runtime), strings.TrimSpace(model)
			}
		case "model":
			step.Model, err = scalar(v)
		case "reasoning_effort":
			step.ReasoningEffort, err = scalar(v)
		case "run":
			step.Kind = domainworkflows.StepCommand
			step.Command, err = scalar(v)
		case "prompt":
			step.Prompt, err = scalar(v)
		case "sandbox":
			step.Sandbox, err = scalar(v)
		case "worker":
			step.WorkerID, err = scalar(v)
		case "after":
			after, err = names(v)
		case "rework":
			rework, err = names(v)
		case "rework_rounds":
			rounds, err = integer(v)
		default:
			err = &FileError{Line: k.Line, Message: fmt.Sprintf("unknown step key %q; use name, agent, model, reasoning_effort, run, prompt, sandbox, worker, after, rework or rework_rounds", k.Value)}
		}
		if err != nil {
			return step, nil, err
		}
	}
	if step.Kind == domainworkflows.StepCommand && step.Runtime != "" {
		return step, nil, &FileError{Line: key.Line, Message: fmt.Sprintf("step %q sets both run and agent; a step is either a command or an Agent", key.Value)}
	}
	var edges []domainworkflows.Edge
	for _, from := range after {
		edges = append(edges, domainworkflows.Edge{From: from, To: step.ID, Kind: domainworkflows.EdgeNext})
	}
	for _, to := range rework {
		edges = append(edges, domainworkflows.Edge{From: step.ID, To: to, Kind: domainworkflows.EdgeRework, MaxRounds: rounds})
	}
	return step, edges, nil
}

func scalar(node *yaml.Node) (string, *FileError) {
	if node.Kind != yaml.ScalarNode {
		return "", &FileError{Line: node.Line, Message: "expected a single value"}
	}
	return strings.TrimSpace(node.Value), nil
}

func integer(node *yaml.Node) (int, *FileError) {
	value, err := scalar(node)
	if err != nil {
		return 0, err
	}
	n, convErr := strconv.Atoi(value)
	if convErr != nil {
		return 0, &FileError{Line: node.Line, Message: fmt.Sprintf("%q is not a whole number", value)}
	}
	return n, nil
}

// seconds accepts 1800 or a duration such as 30m.
func seconds(node *yaml.Node) (int, *FileError) {
	value, err := scalar(node)
	if err != nil {
		return 0, err
	}
	if n, convErr := strconv.Atoi(value); convErr == nil {
		return n, nil
	}
	d, parseErr := time.ParseDuration(value)
	if parseErr != nil {
		return 0, &FileError{Line: node.Line, Message: fmt.Sprintf("%q is not a duration such as 30m", value)}
	}
	return int(d / time.Second), nil
}

// names accepts one step ID or a list of them.
func names(node *yaml.Node) ([]string, *FileError) {
	if node.Kind == yaml.ScalarNode {
		if node.Value == "" {
			return nil, nil
		}
		return []string{strings.TrimSpace(node.Value)}, nil
	}
	if node.Kind != yaml.SequenceNode {
		return nil, &FileError{Line: node.Line, Message: "expected a step ID or a list of step IDs"}
	}
	var out []string
	for _, item := range node.Content {
		value, err := scalar(item)
		if err != nil {
			return nil, err
		}
		out = append(out, value)
	}
	return out, nil
}

var yamlLine = regexp.MustCompile(`line (\d+)`)

func yamlError(err error) *FileError {
	message := strings.TrimPrefix(err.Error(), "yaml: ")
	line := 0
	if m := yamlLine.FindStringSubmatch(message); m != nil {
		line, _ = strconv.Atoi(m[1])
		message = strings.TrimSpace(yamlLine.ReplaceAllString(message, ""))
		message = strings.TrimPrefix(message, ": ")
	}
	return &FileError{Line: line, Message: message}
}

var issueStep = regexp.MustCompile(`^steps\[(\d+)\]`)
var issueEdge = regexp.MustCompile(`^edges\[(\d+)\]`)

// validationLine maps the first validation issue back to the step it is about.
func validationLine(def domainworkflows.Definition, err error, lines map[string]int) *FileError {
	var issues domainworkflows.ValidationErrors
	if !errors.As(err, &issues) || len(issues) == 0 {
		return &FileError{Message: err.Error()}
	}
	issue := issues[0]
	line := 0
	if m := issueStep.FindStringSubmatch(issue.Path); m != nil {
		if i, _ := strconv.Atoi(m[1]); i < len(def.Steps) {
			line = lines[def.Steps[i].ID]
		}
	} else if m := issueEdge.FindStringSubmatch(issue.Path); m != nil {
		if i, _ := strconv.Atoi(m[1]); i < len(def.Edges) {
			edge := def.Edges[i]
			owner := edge.To
			if edge.Kind == domainworkflows.EdgeRework {
				owner = edge.From
			}
			line = lines[owner]
		}
	}
	return &FileError{Line: line, Message: issue.Path + ": " + issue.Message}
}

// MarshalWorkflowFile writes def in the workflow file format, so any workflow
// can be moved into a project and edited as text.
func MarshalWorkflowFile(def domainworkflows.Definition) ([]byte, error) {
	def = domainworkflows.Normalize(def)
	defaults := domainworkflows.Normalize(domainworkflows.Definition{})
	root := &yaml.Node{Kind: yaml.MappingNode}
	add := func(parent *yaml.Node, key string, value *yaml.Node) {
		parent.Content = append(parent.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: key}, value)
	}
	str := func(value string) *yaml.Node { return &yaml.Node{Kind: yaml.ScalarNode, Value: value} }
	list := func(values []string) *yaml.Node {
		if len(values) == 1 {
			return str(values[0])
		}
		node := &yaml.Node{Kind: yaml.SequenceNode, Style: yaml.FlowStyle}
		for _, v := range values {
			node.Content = append(node.Content, str(v))
		}
		return node
	}
	add(root, "name", str(def.Name))
	if def.Description != "" {
		add(root, "description", str(def.Description))
	}
	if def.Policy.MaxReworkRounds != defaults.Policy.MaxReworkRounds {
		add(root, "max_rework", str(strconv.Itoa(def.Policy.MaxReworkRounds)))
	}
	if def.Policy.MaxConsecutiveFailures != defaults.Policy.MaxConsecutiveFailures {
		add(root, "max_failures", str(strconv.Itoa(def.Policy.MaxConsecutiveFailures)))
	}
	if def.Policy.StepTimeoutSeconds != defaults.Policy.StepTimeoutSeconds {
		add(root, "timeout", str((time.Duration(def.Policy.StepTimeoutSeconds) * time.Second).String()))
	}
	steps := &yaml.Node{Kind: yaml.MappingNode}
	for _, s := range def.Steps {
		node := &yaml.Node{Kind: yaml.MappingNode}
		if s.Name != s.ID {
			add(node, "name", str(s.Name))
		}
		if s.Kind == domainworkflows.StepCommand {
			add(node, "run", str(s.Command))
		} else {
			agent := s.Runtime
			if s.Model != "" {
				agent += "/" + s.Model
			}
			add(node, "agent", str(agent))
			if s.ReasoningEffort != "" {
				add(node, "reasoning_effort", str(s.ReasoningEffort))
			}
		}
		if s.Sandbox != "" && s.Kind != domainworkflows.StepCommand {
			add(node, "sandbox", str(s.Sandbox))
		}
		if s.WorkerID != "" && s.WorkerID != "local" {
			add(node, "worker", str(s.WorkerID))
		}
		if up := domainworkflows.Upstream(def, s.ID); len(up) > 0 {
			add(node, "after", list(up))
		}
		var rework []string
		rounds := 0
		for _, e := range domainworkflows.ReworkEdges(def, s.ID) {
			rework = append(rework, e.To)
			rounds = max(rounds, e.MaxRounds)
		}
		if len(rework) > 0 {
			add(node, "rework", list(rework))
		}
		if rounds > 0 {
			add(node, "rework_rounds", str(strconv.Itoa(rounds)))
		}
		if s.Prompt != "" {
			add(node, "prompt", str(s.Prompt))
		}
		add(steps, s.ID, node)
	}
	add(root, "steps", steps)
	var out bytes.Buffer
	encoder := yaml.NewEncoder(&out)
	encoder.SetIndent(2)
	if err := encoder.Encode(&yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{root}}); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
