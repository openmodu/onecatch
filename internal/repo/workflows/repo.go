package workflows

import (
	"bufio"
	"context"
	"encoding/base64"
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
	ErrInvalidRunCursor   = errors.New("workflow run cursor is invalid")
)

type WorkflowsRepo interface {
	SaveDefinition(context.Context, domainworkflows.Definition) (domainworkflows.Definition, error)
	GetDefinition(context.Context, string) (domainworkflows.Definition, error)
	ListDefinitions(context.Context) ([]domainworkflows.Definition, error)
	SaveRun(context.Context, domainworkflows.Run, domainworkflows.Definition) error
	GetRun(context.Context, string) (domainworkflows.Run, error)
	GetRunDefinition(context.Context, string) (domainworkflows.Definition, error)
	ListRunsByTask(context.Context, string) ([]domainworkflows.Run, error)
	ListRuns(context.Context, domainworkflows.RunListQuery) (domainworkflows.RunPage, error)
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

type runIndex struct {
	Version int             `json:"version"`
	Items   []runIndexEntry `json:"items"`
}

type runIndexEntry struct {
	ID         string                    `json:"id"`
	TaskID     string                    `json:"taskId"`
	WorkflowID string                    `json:"workflowId"`
	Status     domainworkflows.RunStatus `json:"status"`
	Sessions   map[string]string         `json:"sessions,omitempty"`
	UpdatedAt  time.Time                 `json:"updatedAt"`
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
	r.refreshRunIndexLocked(run)
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
	runs, err := r.listRunsLocked()
	if err != nil || taskID == "" {
		return runs, err
	}
	filtered := make([]domainworkflows.Run, 0, len(runs))
	for _, run := range runs {
		if run.TaskID == taskID {
			filtered = append(filtered, run)
		}
	}
	return filtered, nil
}

func (r *workflowsImpl) ListRuns(ctx context.Context, query domainworkflows.RunListQuery) (domainworkflows.RunPage, error) {
	if err := ctx.Err(); err != nil {
		return domainworkflows.RunPage{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	index, err := r.ensureRunIndexLocked()
	if err != nil {
		return domainworkflows.RunPage{}, err
	}

	taskIDs := stringSet(query.TaskIDs)
	titleTaskIDs := stringSet(query.TitleTaskIDs)
	keyword := strings.ToLower(strings.TrimSpace(query.Keyword))
	filtered := make([]runIndexEntry, 0, len(index.Items))
	for _, run := range index.Items {
		if query.TaskIDs != nil {
			if _, ok := taskIDs[run.TaskID]; !ok {
				continue
			}
		}
		if query.Status != "" && run.Status != query.Status {
			continue
		}
		if keyword != "" && !runMatchesKeyword(run, keyword, titleTaskIDs) {
			continue
		}
		filtered = append(filtered, run)
	}

	total := len(filtered)
	if query.Cursor != "" {
		cursorTime, cursorID, cursorErr := decodeRunCursor(query.Cursor)
		if cursorErr != nil {
			return domainworkflows.RunPage{}, ErrInvalidRunCursor
		}
		start := sort.Search(len(filtered), func(index int) bool {
			run := filtered[index]
			return run.UpdatedAt.Before(cursorTime) || (run.UpdatedAt.Equal(cursorTime) && run.ID > cursorID)
		})
		filtered = filtered[start:]
	}

	limit := query.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	pageEntries := filtered
	if len(pageEntries) > limit {
		pageEntries = pageEntries[:limit]
	}
	nextCursor := ""
	if len(filtered) > len(pageEntries) && len(pageEntries) > 0 {
		nextCursor = encodeRunCursor(pageEntries[len(pageEntries)-1].UpdatedAt, pageEntries[len(pageEntries)-1].ID)
	}
	pageItems := make([]domainworkflows.Run, 0, len(pageEntries))
	for _, entry := range pageEntries {
		run, getErr := r.getRunLocked(entry.ID)
		if errors.Is(getErr, ErrRunNotFound) {
			_ = os.Remove(r.runIndexPath())
			continue
		}
		if getErr != nil {
			return domainworkflows.RunPage{}, getErr
		}
		pageItems = append(pageItems, run)
	}
	return domainworkflows.RunPage{Items: pageItems, NextCursor: nextCursor, Total: total}, nil
}

func (r *workflowsImpl) listRunsLocked() ([]domainworkflows.Run, error) {
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
		out = append(out, run)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	return out, nil
}

func (r *workflowsImpl) ensureRunIndexLocked() (runIndex, error) {
	var index runIndex
	if err := localfile.ReadJSON(r.runIndexPath(), &index); err != nil || index.Version != 1 {
		return r.rebuildRunIndexLocked()
	}
	entries, err := os.ReadDir(r.runsRoot)
	if err != nil {
		return runIndex{}, fmt.Errorf("list workflow run directories for index: %w", err)
	}
	directoryIDs := make(map[string]struct{})
	for _, entry := range entries {
		if entry.IsDir() && localfile.ValidID(entry.Name()) {
			directoryIDs[entry.Name()] = struct{}{}
		}
	}
	if len(directoryIDs) != len(index.Items) {
		return r.rebuildRunIndexLocked()
	}
	for _, item := range index.Items {
		if _, ok := directoryIDs[item.ID]; !ok {
			return r.rebuildRunIndexLocked()
		}
	}
	sortRunIndex(index.Items)
	return index, nil
}

func (r *workflowsImpl) rebuildRunIndexLocked() (runIndex, error) {
	runs, err := r.listRunsLocked()
	if err != nil {
		return runIndex{}, err
	}
	index := runIndex{Version: 1, Items: make([]runIndexEntry, 0, len(runs))}
	for _, run := range runs {
		index.Items = append(index.Items, runIndexEntryFromRun(run))
	}
	if err := localfile.WriteJSONAtomic(r.runIndexPath(), index); err != nil {
		return runIndex{}, fmt.Errorf("write workflow run index: %w", err)
	}
	return index, nil
}

// refreshRunIndexLocked keeps the derived index cheap without making a Run
// state transition depend on an auxiliary file. If the index cannot be
// refreshed it is removed and rebuilt from authoritative run.json files by the
// next query.
func (r *workflowsImpl) refreshRunIndexLocked(run domainworkflows.Run) {
	index, err := r.ensureRunIndexLocked()
	if err != nil {
		_ = os.Remove(r.runIndexPath())
		return
	}
	entry := runIndexEntryFromRun(run)
	found := false
	for i := range index.Items {
		if index.Items[i].ID == run.ID {
			index.Items[i] = entry
			found = true
			break
		}
	}
	if !found {
		index.Items = append(index.Items, entry)
	}
	sortRunIndex(index.Items)
	if err := localfile.WriteJSONAtomic(r.runIndexPath(), index); err != nil {
		_ = os.Remove(r.runIndexPath())
	}
}

func runIndexEntryFromRun(run domainworkflows.Run) runIndexEntry {
	return runIndexEntry{ID: run.ID, TaskID: run.TaskID, WorkflowID: run.WorkflowID, Status: run.Status, Sessions: run.Sessions, UpdatedAt: run.UpdatedAt}
}

func sortRunIndex(items []runIndexEntry) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].UpdatedAt.Equal(items[j].UpdatedAt) {
			return items[i].ID < items[j].ID
		}
		return items[i].UpdatedAt.After(items[j].UpdatedAt)
	})
}

func stringSet(values []string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		out[value] = struct{}{}
	}
	return out
}

func runMatchesKeyword(run runIndexEntry, keyword string, titleTaskIDs map[string]struct{}) bool {
	if _, ok := titleTaskIDs[run.TaskID]; ok {
		return true
	}
	for _, value := range []string{run.ID, run.TaskID, run.WorkflowID, string(run.Status)} {
		if strings.Contains(strings.ToLower(value), keyword) {
			return true
		}
	}
	for _, sessionID := range run.Sessions {
		if strings.Contains(strings.ToLower(sessionID), keyword) {
			return true
		}
	}
	return false
}

func encodeRunCursor(updatedAt time.Time, id string) string {
	value := updatedAt.UTC().Format(time.RFC3339Nano) + "\n" + id
	return base64.RawURLEncoding.EncodeToString([]byte(value))
}

func decodeRunCursor(cursor string) (time.Time, string, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return time.Time{}, "", err
	}
	parts := strings.SplitN(string(decoded), "\n", 2)
	if len(parts) != 2 || !localfile.ValidID(parts[1]) {
		return time.Time{}, "", ErrInvalidRunCursor
	}
	updatedAt, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return time.Time{}, "", err
	}
	return updatedAt, parts[1], nil
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
	r.refreshRunIndexLocked(run)
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

func (r *workflowsImpl) runIndexPath() string {
	return filepath.Join(r.runsRoot, "index.json")
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
