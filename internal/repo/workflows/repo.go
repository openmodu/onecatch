package workflows

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	domainworkflows "github.com/openmodu/oneshot/internal/domain/workflows"
	"github.com/openmodu/oneshot/pkg/localfile"
)

var (
	ErrDefinitionNotFound = errors.New("workflow definition not found")
	ErrRunNotFound        = errors.New("workflow run not found")
	ErrStateConflict      = errors.New("workflow state conflict")
)

type WorkflowsRepo interface {
	SaveDefinition(context.Context, domainworkflows.Definition) (domainworkflows.Definition, error)
	GetDefinition(context.Context, string) (domainworkflows.Definition, error)
	ListDefinitions(context.Context) ([]domainworkflows.Definition, error)
	SaveRun(context.Context, domainworkflows.Run, domainworkflows.Definition) error
	GetRun(context.Context, string) (domainworkflows.Run, error)
	GetRunDefinition(context.Context, string) (domainworkflows.Definition, error)
	ListRunsByTask(context.Context, string) ([]domainworkflows.Run, error)
	UpdateRun(context.Context, domainworkflows.Run, int64) (domainworkflows.Run, error)
	SaveStepRun(context.Context, domainworkflows.StepRun) error
	ListStepRuns(context.Context, string) ([]domainworkflows.StepRun, error)
	AppendEvent(context.Context, domainworkflows.WorkflowEvent) (domainworkflows.WorkflowEvent, error)
	ListEvents(context.Context, string, int64, int) ([]domainworkflows.WorkflowEvent, error)
	AppendRuntimeEvent(context.Context, string, string, json.RawMessage) (domainworkflows.RuntimeEvent, error)
	ListRuntimeEvents(context.Context, string, string, int64, int) ([]domainworkflows.RuntimeEvent, error)
	WriteRunSummary(context.Context, string, string) error
}

type workflowsImpl struct {
	workflowsRoot string
	runsRoot      string
	runtimeEvents *runtimeEventStore
	now           func() time.Time
	mu            sync.RWMutex
}

func NewWorkflowsRepo(workflowsRoot, runsRoot string) WorkflowsRepo {
	return &workflowsImpl{
		workflowsRoot: workflowsRoot,
		runsRoot:      runsRoot,
		runtimeEvents: newRuntimeEventStore(runsRoot),
		now:           time.Now,
	}
}

func (r *workflowsImpl) SaveDefinition(ctx context.Context, input domainworkflows.Definition) (domainworkflows.Definition, error) {
	if err := ctx.Err(); err != nil {
		return domainworkflows.Definition{}, err
	}
	def := domainworkflows.Normalize(input)
	if err := domainworkflows.Validate(def); err != nil {
		return domainworkflows.Definition{}, err
	}
	if !localfile.ValidID(def.ID) {
		return domainworkflows.Definition{}, errors.New("workflow definition ID is unsafe")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.now().UTC()
	current, err := r.getDefinitionLocked(def.ID)
	if err == nil {
		if def.CreatedAt.IsZero() {
			def.CreatedAt = current.CreatedAt
		}
	} else if errors.Is(err, ErrDefinitionNotFound) {
		if def.CreatedAt.IsZero() {
			def.CreatedAt = now
		}
	} else {
		return domainworkflows.Definition{}, err
	}
	def.UpdatedAt = now
	if err := localfile.WriteJSONAtomic(r.definitionPath(def.ID), def); err != nil {
		return domainworkflows.Definition{}, fmt.Errorf("save workflow definition: %w", err)
	}
	return def, nil
}

func (r *workflowsImpl) GetDefinition(ctx context.Context, id string) (domainworkflows.Definition, error) {
	if err := ctx.Err(); err != nil {
		return domainworkflows.Definition{}, err
	}
	if !localfile.ValidID(id) {
		return domainworkflows.Definition{}, ErrDefinitionNotFound
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.getDefinitionLocked(id)
}

func (r *workflowsImpl) ListDefinitions(ctx context.Context) ([]domainworkflows.Definition, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	workflowDirs, err := os.ReadDir(r.workflowsRoot)
	if err != nil {
		return nil, fmt.Errorf("list workflow directories: %w", err)
	}
	var out []domainworkflows.Definition
	for _, workflowDir := range workflowDirs {
		if !workflowDir.IsDir() || !localfile.ValidID(workflowDir.Name()) {
			continue
		}
		def, err := r.getDefinitionLocked(workflowDir.Name())
		if errors.Is(err, ErrDefinitionNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		out = append(out, def)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name == out[j].Name {
			return out[i].ID < out[j].ID
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

func (r *workflowsImpl) SaveRun(ctx context.Context, run domainworkflows.Run, definition domainworkflows.Definition) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateRunForStorage(run); err != nil {
		return err
	}
	if !localfile.ValidID(run.ID) || !localfile.ValidID(run.TaskID) || !localfile.ValidID(run.WorkflowID) {
		return errors.New("workflow run contains unsafe IDs")
	}
	definition = domainworkflows.Normalize(definition)
	if err := domainworkflows.Validate(definition); err != nil {
		return err
	}
	if definition.ID != run.WorkflowID {
		return ErrStateConflict
	}
	if run.Revision < 1 {
		run.Revision = 1
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, err := os.Stat(r.runPath(run.ID)); err == nil {
		return ErrStateConflict
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("check workflow run: %w", err)
	}
	if _, err := r.getDefinitionLocked(run.WorkflowID); err != nil {
		return err
	}
	if err := localfile.WriteJSONAtomic(r.runDefinitionPath(run.ID), definition); err != nil {
		return fmt.Errorf("save run workflow snapshot: %w", err)
	}
	if err := localfile.WriteJSONAtomic(r.runPath(run.ID), run); err != nil {
		return fmt.Errorf("save workflow run: %w", err)
	}
	return nil
}

func (r *workflowsImpl) GetRun(ctx context.Context, id string) (domainworkflows.Run, error) {
	if err := ctx.Err(); err != nil {
		return domainworkflows.Run{}, err
	}
	if !localfile.ValidID(id) {
		return domainworkflows.Run{}, ErrRunNotFound
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.getRunLocked(id)
}

func (r *workflowsImpl) GetRunDefinition(ctx context.Context, runID string) (domainworkflows.Definition, error) {
	if err := ctx.Err(); err != nil {
		return domainworkflows.Definition{}, err
	}
	if !localfile.ValidID(runID) {
		return domainworkflows.Definition{}, ErrRunNotFound
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if _, err := r.getRunLocked(runID); err != nil {
		return domainworkflows.Definition{}, err
	}
	var definition domainworkflows.Definition
	if err := localfile.ReadJSON(r.runDefinitionPath(runID), &definition); errors.Is(err, os.ErrNotExist) {
		return domainworkflows.Definition{}, ErrDefinitionNotFound
	} else if err != nil {
		return domainworkflows.Definition{}, fmt.Errorf("get run workflow snapshot: %w", err)
	}
	return definition, nil
}

func (r *workflowsImpl) ListRunsByTask(ctx context.Context, taskID string) ([]domainworkflows.Run, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	runDirs, err := os.ReadDir(r.runsRoot)
	if err != nil {
		return nil, fmt.Errorf("list workflow run directories: %w", err)
	}
	var out []domainworkflows.Run
	for _, runDir := range runDirs {
		if !runDir.IsDir() || !localfile.ValidID(runDir.Name()) {
			continue
		}
		run, err := r.getRunLocked(runDir.Name())
		if errors.Is(err, ErrRunNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if taskID == "" || run.TaskID == taskID {
			out = append(out, run)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	return out, nil
}

func (r *workflowsImpl) UpdateRun(ctx context.Context, input domainworkflows.Run, expectedRevision int64) (domainworkflows.Run, error) {
	if err := ctx.Err(); err != nil {
		return domainworkflows.Run{}, err
	}
	if expectedRevision < 1 || !localfile.ValidID(input.ID) {
		return domainworkflows.Run{}, ErrStateConflict
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	current, err := r.getRunLocked(input.ID)
	if err != nil {
		return domainworkflows.Run{}, err
	}
	if current.Revision != expectedRevision {
		return domainworkflows.Run{}, ErrStateConflict
	}
	run := input
	run.Revision = expectedRevision + 1
	if err := validateRunForStorage(run); err != nil {
		return domainworkflows.Run{}, err
	}
	if run.TaskID != current.TaskID || run.WorkflowID != current.WorkflowID {
		return domainworkflows.Run{}, ErrStateConflict
	}
	if err := localfile.WriteJSONAtomic(r.runPath(run.ID), run); err != nil {
		return domainworkflows.Run{}, fmt.Errorf("update workflow run: %w", err)
	}
	return run, nil
}

func (r *workflowsImpl) SaveStepRun(ctx context.Context, step domainworkflows.StepRun) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(step.ID) == "" || strings.TrimSpace(step.RunID) == "" || strings.TrimSpace(step.StepID) == "" || step.Attempt < 1 || step.Status == "" || !localfile.ValidID(step.ID) || !localfile.ValidID(step.RunID) || !localfile.ValidID(step.StepID) {
		return errors.New("workflow step run is invalid")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, err := r.getRunLocked(step.RunID); err != nil {
		return err
	}
	path := r.stepRunPath(step.RunID, step.ID)
	var current domainworkflows.StepRun
	if err := localfile.ReadJSON(path, &current); err == nil {
		if current.RunID != step.RunID || current.StepID != step.StepID || current.Attempt != step.Attempt {
			return ErrStateConflict
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read workflow step run before update: %w", err)
	}
	if err := localfile.WriteJSONAtomic(path, step); err != nil {
		return fmt.Errorf("save workflow step run: %w", err)
	}
	return nil
}

func (r *workflowsImpl) ListStepRuns(ctx context.Context, runID string) ([]domainworkflows.StepRun, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !localfile.ValidID(runID) {
		return nil, ErrRunNotFound
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	stepsRoot := filepath.Join(r.runsRoot, runID, "steps")
	stepDirs, err := os.ReadDir(stepsRoot)
	if errors.Is(err, os.ErrNotExist) {
		return []domainworkflows.StepRun{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list step run directories: %w", err)
	}
	var out []domainworkflows.StepRun
	for _, stepDir := range stepDirs {
		if !stepDir.IsDir() || !localfile.ValidID(stepDir.Name()) {
			continue
		}
		var step domainworkflows.StepRun
		if err := localfile.ReadJSON(r.stepRunPath(runID, stepDir.Name()), &step); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return nil, fmt.Errorf("read step run %s: %w", stepDir.Name(), err)
		}
		out = append(out, step)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].StartedAt.Equal(out[j].StartedAt) {
			return out[i].Attempt < out[j].Attempt
		}
		return out[i].StartedAt.Before(out[j].StartedAt)
	})
	return out, nil
}

func (r *workflowsImpl) AppendEvent(ctx context.Context, input domainworkflows.WorkflowEvent) (domainworkflows.WorkflowEvent, error) {
	if err := ctx.Err(); err != nil {
		return domainworkflows.WorkflowEvent{}, err
	}
	if strings.TrimSpace(input.RunID) == "" || strings.TrimSpace(input.Type) == "" || !localfile.ValidID(input.RunID) {
		return domainworkflows.WorkflowEvent{}, errors.New("workflow event is invalid")
	}
	if len(input.Payload) == 0 {
		input.Payload = json.RawMessage(`{}`)
	}
	if !json.Valid(input.Payload) {
		return domainworkflows.WorkflowEvent{}, errors.New("workflow event payload is invalid JSON")
	}
	if input.At.IsZero() {
		input.At = r.now().UTC()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, err := r.getRunLocked(input.RunID); err != nil {
		return domainworkflows.WorkflowEvent{}, err
	}
	path := r.workflowEventsPath(input.RunID)
	if err := localfile.TrimIncompleteJSONLTail(path); err != nil {
		return domainworkflows.WorkflowEvent{}, err
	}
	last, err := lastWorkflowEventSeq(path)
	if err != nil {
		return domainworkflows.WorkflowEvent{}, err
	}
	if input.Seq == 0 {
		input.Seq = last + 1
	} else if input.Seq <= last {
		return domainworkflows.WorkflowEvent{}, ErrStateConflict
	}
	if err := localfile.AppendJSONLine(path, input); err != nil {
		return domainworkflows.WorkflowEvent{}, fmt.Errorf("append workflow event: %w", err)
	}
	return input, nil
}

func (r *workflowsImpl) ListEvents(ctx context.Context, runID string, afterSeq int64, limit int) ([]domainworkflows.WorkflowEvent, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !localfile.ValidID(runID) {
		return nil, ErrRunNotFound
	}
	if limit <= 0 || limit > 1000 {
		limit = 1000
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := localfile.TrimIncompleteJSONLTail(r.workflowEventsPath(runID)); err != nil {
		return nil, err
	}
	return readWorkflowEvents(r.workflowEventsPath(runID), afterSeq, limit)
}

func (r *workflowsImpl) AppendRuntimeEvent(ctx context.Context, runID, stepRunID string, payload json.RawMessage) (domainworkflows.RuntimeEvent, error) {
	return r.runtimeEvents.append(ctx, runID, stepRunID, payload, r.now().UTC())
}

func (r *workflowsImpl) ListRuntimeEvents(ctx context.Context, runID, stepRunID string, afterSeq int64, limit int) ([]domainworkflows.RuntimeEvent, error) {
	return r.runtimeEvents.list(ctx, runID, stepRunID, afterSeq, limit)
}

func (r *workflowsImpl) WriteRunSummary(ctx context.Context, runID, content string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !localfile.ValidID(runID) {
		return ErrRunNotFound
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, err := r.getRunLocked(runID); err != nil {
		return err
	}
	if err := localfile.WriteTextAtomic(filepath.Join(r.runsRoot, runID, "SUMMARY.md"), content); err != nil {
		return fmt.Errorf("write run summary: %w", err)
	}
	return nil
}

func (r *workflowsImpl) getDefinitionLocked(id string) (domainworkflows.Definition, error) {
	var def domainworkflows.Definition
	if err := localfile.ReadJSON(r.definitionPath(id), &def); errors.Is(err, os.ErrNotExist) {
		return domainworkflows.Definition{}, ErrDefinitionNotFound
	} else if err != nil {
		return domainworkflows.Definition{}, fmt.Errorf("get workflow definition: %w", err)
	}
	return def, nil
}

func (r *workflowsImpl) getRunLocked(id string) (domainworkflows.Run, error) {
	var run domainworkflows.Run
	if err := localfile.ReadJSON(r.runPath(id), &run); errors.Is(err, os.ErrNotExist) {
		return domainworkflows.Run{}, ErrRunNotFound
	} else if err != nil {
		return domainworkflows.Run{}, fmt.Errorf("get workflow run: %w", err)
	}
	return run, nil
}

func (r *workflowsImpl) definitionPath(id string) string {
	return filepath.Join(r.workflowsRoot, id, "workflow.json")
}

func (r *workflowsImpl) runPath(id string) string {
	return filepath.Join(r.runsRoot, id, "run.json")
}

func (r *workflowsImpl) runDefinitionPath(id string) string {
	return filepath.Join(r.runsRoot, id, "workflow.json")
}

func (r *workflowsImpl) stepRunPath(runID, stepRunID string) string {
	return filepath.Join(r.runsRoot, runID, "steps", stepRunID, "state.json")
}

func (r *workflowsImpl) workflowEventsPath(runID string) string {
	return filepath.Join(r.runsRoot, runID, "events.jsonl")
}

func validateRunForStorage(run domainworkflows.Run) error {
	if strings.TrimSpace(run.ID) == "" || strings.TrimSpace(run.TaskID) == "" || strings.TrimSpace(run.WorkflowID) == "" || strings.TrimSpace(run.CurrentStepID) == "" || run.Status == "" {
		return errors.New("workflow run is invalid")
	}
	for stepID, node := range run.Nodes {
		if stepID == "" || node.StepID != stepID {
			return errors.New("workflow DAG node is invalid")
		}
		switch node.Status {
		case domainworkflows.NodePending, domainworkflows.NodeRunning, domainworkflows.NodeCompleted, domainworkflows.NodePaused, domainworkflows.NodeFailed:
		default:
			return errors.New("workflow DAG node status is invalid")
		}
	}
	return nil
}

func lastWorkflowEventSeq(path string) (int64, error) {
	events, err := readWorkflowEvents(path, 0, 0)
	if err != nil {
		return 0, err
	}
	if len(events) == 0 {
		return 0, nil
	}
	return events[len(events)-1].Seq, nil
}

func readWorkflowEvents(path string, afterSeq int64, limit int) ([]domainworkflows.WorkflowEvent, error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return []domainworkflows.WorkflowEvent{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open workflow event stream: %w", err)
	}
	defer file.Close()
	var out []domainworkflows.WorkflowEvent
	var last int64
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), maxRuntimeEventLine)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		var event domainworkflows.WorkflowEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return nil, fmt.Errorf("decode workflow event line %d: %w", lineNumber, err)
		}
		if event.Seq <= last {
			return nil, fmt.Errorf("workflow event sequence is not increasing at line %d", lineNumber)
		}
		last = event.Seq
		if event.Seq <= afterSeq {
			continue
		}
		out = append(out, event)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read workflow event stream: %w", err)
	}
	return out, nil
}
