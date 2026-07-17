package workflows_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"testing"
	"time"

	localdata "github.com/openmodu/oneshot/internal/data/local"
	domaintasks "github.com/openmodu/oneshot/internal/domain/tasks"
	domainworkflows "github.com/openmodu/oneshot/internal/domain/workflows"
	domainworkspaces "github.com/openmodu/oneshot/internal/domain/workspaces"
	repoworkflows "github.com/openmodu/oneshot/internal/repo/workflows"
)

func TestWorkflowRepoPersistsRunSnapshotAndEvents(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), ".oneshot")
	store, err := localdata.OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 10, 13, 0, 0, 0, time.UTC)
	workspace := domainworkspaces.Workspace{ID: "ws_1", Name: "project", Path: filepath.Join(t.TempDir(), "project"), DefaultSandbox: "workspace-write", CreatedAt: now, LastOpenedAt: now}
	if err := store.Repos.Tasks.SaveWorkspace(ctx, workspace); err != nil {
		t.Fatal(err)
	}
	task := domaintasks.Task{ID: "task_1", WorkspaceID: workspace.ID, Title: "loop", Prompt: "implement and review", WorkflowID: "review_loop", Status: domaintasks.StatusReady, CreatedAt: now, UpdatedAt: now}
	if err := store.Repos.Tasks.SaveTask(ctx, task); err != nil {
		t.Fatal(err)
	}

	definition := reviewLoopDefinition()
	savedDefinition, err := store.Repos.Workflows.SaveDefinition(ctx, definition)
	if err != nil {
		t.Fatal(err)
	}
	if savedDefinition.CreatedAt.IsZero() || savedDefinition.Policy.MaxTransitions != domainworkflows.DefaultMaxTransitions {
		t.Fatalf("saved definition = %+v", savedDefinition)
	}
	run, err := domainworkflows.NewRun(savedDefinition, "run_1", now)
	if err != nil {
		t.Fatal(err)
	}
	run.TaskID = task.ID
	run, err = domainworkflows.Start(savedDefinition, run, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	run.Sessions["implement"] = "session_codex_1"
	if err := store.Repos.Workflows.SaveRun(ctx, run, savedDefinition); err != nil {
		t.Fatal(err)
	}
	editedDefinition := domainworkflows.Normalize(savedDefinition)
	editedDefinition.Name = "Edited Review Loop"
	editedDefinition.Steps[0].Instruction = "Use the newly edited instructions."
	if _, err := store.Repos.Workflows.SaveDefinition(ctx, editedDefinition); err != nil {
		t.Fatal(err)
	}
	currentDefinition, err := store.Repos.Workflows.GetDefinition(ctx, definition.ID)
	if err != nil || currentDefinition.Name != editedDefinition.Name {
		t.Fatalf("GetDefinition() = %+v, %v", currentDefinition, err)
	}
	definitions, err := store.Repos.Workflows.ListDefinitions(ctx)
	if err != nil || len(definitions) != 1 || definitions[0].Name != editedDefinition.Name {
		t.Fatalf("ListDefinitions() = %+v, %v", definitions, err)
	}
	runDefinition, err := store.Repos.Workflows.GetRunDefinition(ctx, run.ID)
	if err != nil || runDefinition.Name != savedDefinition.Name || runDefinition.Steps[0].Instruction != savedDefinition.Steps[0].Instruction {
		t.Fatalf("GetRunDefinition() = %+v, %v; template edit leaked into run snapshot", runDefinition, err)
	}
	advanced, err := domainworkflows.Advance(savedDefinition, run, domainworkflows.Outcome{Signal: "ready_for_review", Content: "implemented"}, now.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	updated, err := store.Repos.Workflows.UpdateRun(ctx, advanced, run.Revision)
	if err != nil || updated.Revision != 2 || updated.CurrentStepID != "review" {
		t.Fatalf("UpdateRun() = %+v, %v", updated, err)
	}
	if _, err := store.Repos.Workflows.UpdateRun(ctx, advanced, run.Revision); !errors.Is(err, repoworkflows.ErrStateConflict) {
		t.Fatalf("stale UpdateRun() error = %v", err)
	}

	stepRun := domainworkflows.StepRun{
		ID:              "step_run_1",
		RunID:           run.ID,
		StepID:          "implement",
		Attempt:         1,
		Status:          domainworkflows.StepRunSucceeded,
		Signal:          "ready_for_review",
		Content:         "implemented",
		SessionIDBefore: "",
		SessionIDAfter:  "session_codex_1",
		StartedAt:       now,
		FinishedAt:      now.Add(time.Minute),
	}
	if err := store.Repos.Workflows.SaveStepRun(ctx, stepRun); err != nil {
		t.Fatal(err)
	}
	if err := store.Repos.Workflows.WriteRunSummary(ctx, run.ID, "# Summary\n"); err != nil {
		t.Fatal(err)
	}
	firstEvent, err := store.Repos.Workflows.AppendEvent(ctx, domainworkflows.WorkflowEvent{RunID: run.ID, Type: "step.completed", StepID: stepRun.StepID, Payload: json.RawMessage(`{"signal":"ready_for_review"}`), At: now})
	if err != nil || firstEvent.Seq != 1 {
		t.Fatalf("AppendEvent(first) = %+v, %v", firstEvent, err)
	}
	secondEvent, err := store.Repos.Workflows.AppendEvent(ctx, domainworkflows.WorkflowEvent{RunID: run.ID, Type: "transition.applied", Payload: json.RawMessage(`{"target":"review"}`), At: now.Add(time.Second)})
	if err != nil || secondEvent.Seq != 2 {
		t.Fatalf("AppendEvent(second) = %+v, %v", secondEvent, err)
	}
	for i := 1; i <= 2; i++ {
		event, err := store.Repos.Workflows.AppendRuntimeEvent(ctx, run.ID, stepRun.ID, json.RawMessage(`{"kind":"message","text":"chunk"}`))
		if err != nil || event.Seq != int64(i) {
			t.Fatalf("AppendRuntimeEvent(%d) = %+v, %v", i, event, err)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := localdata.OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	gotRun, err := reopened.Repos.Workflows.GetRun(ctx, run.ID)
	if err != nil || gotRun.Revision != 2 || gotRun.CurrentStepID != "review" || gotRun.Sessions["implement"] != "session_codex_1" || len(gotRun.History) != 1 {
		t.Fatalf("GetRun() after reopen = %+v, %v", gotRun, err)
	}
	gotRunDefinition, err := reopened.Repos.Workflows.GetRunDefinition(ctx, run.ID)
	if err != nil || gotRunDefinition.Name != savedDefinition.Name || gotRunDefinition.Steps[0].Instruction != savedDefinition.Steps[0].Instruction {
		t.Fatalf("GetRunDefinition() after reopen = %+v, %v", gotRunDefinition, err)
	}
	stepRuns, err := reopened.Repos.Workflows.ListStepRuns(ctx, run.ID)
	if err != nil || len(stepRuns) != 1 || stepRuns[0].SessionIDAfter != stepRun.SessionIDAfter {
		t.Fatalf("ListStepRuns() = %+v, %v", stepRuns, err)
	}
	events, err := reopened.Repos.Workflows.ListEvents(ctx, run.ID, 1, 10)
	if err != nil || len(events) != 1 || events[0].Seq != 2 {
		t.Fatalf("ListEvents(after=1) = %+v, %v", events, err)
	}
	thirdRuntimeEvent, err := reopened.Repos.Workflows.AppendRuntimeEvent(ctx, run.ID, stepRun.ID, json.RawMessage(`{"kind":"result","text":"done"}`))
	if err != nil || thirdRuntimeEvent.Seq != 3 {
		t.Fatalf("AppendRuntimeEvent after reopen = %+v, %v", thirdRuntimeEvent, err)
	}
	runtimeEvents, err := reopened.Repos.Workflows.ListRuntimeEvents(ctx, run.ID, stepRun.ID, 1, 10)
	if err != nil || len(runtimeEvents) != 2 || runtimeEvents[0].Seq != 2 || runtimeEvents[1].Seq != 3 {
		t.Fatalf("ListRuntimeEvents(after=1) = %+v, %v", runtimeEvents, err)
	}
	streamPath := filepath.Join(root, "runs", run.ID, "steps", stepRun.ID, "events.jsonl")
	if info, err := os.Stat(streamPath); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("runtime stream stat = %+v, %v", info, err)
	}
	for _, path := range []string{
		filepath.Join(root, "workspaces", workspace.ID+".json"),
		filepath.Join(root, "tasks", task.ID+".json"),
		filepath.Join(root, "workflows", definition.ID, "workflow.json"),
		filepath.Join(root, "runs", run.ID, "run.json"),
		filepath.Join(root, "runs", run.ID, "workflow.json"),
		filepath.Join(root, "runs", run.ID, "SUMMARY.md"),
		filepath.Join(root, "runs", run.ID, "events.jsonl"),
		filepath.Join(root, "runs", run.ID, "steps", stepRun.ID, "state.json"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected inspectable persistence file %s: %v", path, err)
		}
	}
}

func TestWorkflowRepoRenamesAndDeletesDefinitions(t *testing.T) {
	ctx := context.Background()
	store, err := localdata.OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	original, err := store.Repos.Workflows.SaveDefinition(ctx, reviewLoopDefinition())
	if err != nil {
		t.Fatal(err)
	}
	conflict := reviewLoopDefinition()
	conflict.ID = "other_loop"
	conflict.Name = "Other Loop"
	if _, err := store.Repos.Workflows.SaveDefinition(ctx, conflict); err != nil {
		t.Fatal(err)
	}

	renamed := original
	renamed.ID = "renamed_loop"
	renamed.Name = "Renamed Loop"
	saved, err := store.Repos.Workflows.UpdateDefinition(ctx, original.ID, renamed)
	if err != nil {
		t.Fatal(err)
	}
	if saved.ID != renamed.ID || saved.Name != renamed.Name || !saved.CreatedAt.Equal(original.CreatedAt) {
		t.Fatalf("renamed definition = %+v", saved)
	}
	if _, err := store.Repos.Workflows.GetDefinition(ctx, original.ID); !errors.Is(err, repoworkflows.ErrDefinitionNotFound) {
		t.Fatalf("old definition lookup error = %v", err)
	}
	if _, err := store.Repos.Workflows.GetDefinition(ctx, renamed.ID); err != nil {
		t.Fatalf("new definition lookup error = %v", err)
	}

	renamed.ID = conflict.ID
	if _, err := store.Repos.Workflows.UpdateDefinition(ctx, saved.ID, renamed); !errors.Is(err, repoworkflows.ErrDefinitionExists) {
		t.Fatalf("conflicting rename error = %v", err)
	}
	if err := store.Repos.Workflows.DeleteDefinition(ctx, saved.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Repos.Workflows.GetDefinition(ctx, saved.ID); !errors.Is(err, repoworkflows.ErrDefinitionNotFound) {
		t.Fatalf("deleted definition lookup error = %v", err)
	}
}

func TestRuntimeEventStoreRejectsUnsafeIDsAndCorruption(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), ".oneshot")
	store, err := localdata.OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.Repos.Workflows.AppendRuntimeEvent(ctx, "../escape", "step", json.RawMessage(`{}`)); err == nil {
		t.Fatal("AppendRuntimeEvent() accepted an unsafe run ID")
	}

	path := filepath.Join(root, "runs", "run_corrupt", "steps", "step_corrupt")
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "events.jsonl"), []byte("not-json\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Repos.Workflows.AppendRuntimeEvent(ctx, "run_corrupt", "step_corrupt", json.RawMessage(`{}`)); err == nil {
		t.Fatal("AppendRuntimeEvent() ignored a corrupt existing stream")
	}
}

func TestRuntimeEventStoreSerializesConcurrentAppends(t *testing.T) {
	ctx := context.Background()
	store, err := localdata.OpenStore(filepath.Join(t.TempDir(), ".oneshot"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	const count = 32
	sequences := make([]int, count)
	errs := make([]error, count)
	var wg sync.WaitGroup
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			event, err := store.Repos.Workflows.AppendRuntimeEvent(ctx, "run_parallel", "step_parallel", json.RawMessage(`{"kind":"message"}`))
			errs[index] = err
			sequences[index] = int(event.Seq)
		}(i)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	sort.Ints(sequences)
	for i, seq := range sequences {
		if seq != i+1 {
			t.Fatalf("sequences = %v", sequences)
		}
	}
}

func TestRunListQueryFiltersSearchesAndPages(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), ".oneshot")
	store, err := localdata.OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	definition, err := store.Repos.Workflows.SaveDefinition(ctx, reviewLoopDefinition())
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)
	runs := []domainworkflows.Run{
		{ID: "run_a", TaskID: "task_a", WorkflowID: definition.ID, Revision: 1, Status: domainworkflows.RunCompleted, CurrentStepID: "review", Sessions: map[string]string{"review": "thread-alpha"}, StartedAt: base, UpdatedAt: base.Add(3 * time.Minute)},
		{ID: "run_b", TaskID: "task_b", WorkflowID: definition.ID, Revision: 1, Status: domainworkflows.RunPaused, CurrentStepID: "implement", Sessions: map[string]string{"implement": "thread-beta"}, StartedAt: base, UpdatedAt: base.Add(3 * time.Minute)},
		{ID: "run_c", TaskID: "task_a", WorkflowID: definition.ID, Revision: 1, Status: domainworkflows.RunFailed, CurrentStepID: "implement", StartedAt: base, UpdatedAt: base.Add(time.Minute)},
	}
	for _, run := range runs {
		if err := store.Repos.Workflows.SaveRun(ctx, run, definition); err != nil {
			t.Fatal(err)
		}
	}

	first, err := store.Repos.Workflows.ListRuns(ctx, domainworkflows.RunListQuery{TaskIDs: []string{"task_a", "task_b"}, Limit: 2})
	if err != nil || len(first.Items) != 2 || first.Items[0].ID != "run_a" || first.Items[1].ID != "run_b" || first.Total != 3 || first.NextCursor == "" {
		t.Fatalf("ListRuns(first) = %+v, %v", first, err)
	}
	second, err := store.Repos.Workflows.ListRuns(ctx, domainworkflows.RunListQuery{TaskIDs: []string{"task_a", "task_b"}, Cursor: first.NextCursor, Limit: 2})
	if err != nil || len(second.Items) != 1 || second.Items[0].ID != "run_c" || second.Total != 3 || second.NextCursor != "" {
		t.Fatalf("ListRuns(second) = %+v, %v", second, err)
	}
	paused, err := store.Repos.Workflows.ListRuns(ctx, domainworkflows.RunListQuery{TaskIDs: []string{"task_a", "task_b"}, Status: domainworkflows.RunPaused})
	if err != nil || len(paused.Items) != 1 || paused.Items[0].ID != "run_b" {
		t.Fatalf("ListRuns(paused) = %+v, %v", paused, err)
	}
	bySession, err := store.Repos.Workflows.ListRuns(ctx, domainworkflows.RunListQuery{TaskIDs: []string{"task_a", "task_b"}, Keyword: "ALPHA"})
	if err != nil || len(bySession.Items) != 1 || bySession.Items[0].ID != "run_a" {
		t.Fatalf("ListRuns(session) = %+v, %v", bySession, err)
	}
	byTitle, err := store.Repos.Workflows.ListRuns(ctx, domainworkflows.RunListQuery{TaskIDs: []string{"task_a", "task_b"}, TitleTaskIDs: []string{"task_b"}, Keyword: "matching title"})
	if err != nil || len(byTitle.Items) != 1 || byTitle.Items[0].ID != "run_b" {
		t.Fatalf("ListRuns(title) = %+v, %v", byTitle, err)
	}
	emptyScope, err := store.Repos.Workflows.ListRuns(ctx, domainworkflows.RunListQuery{TaskIDs: []string{}})
	if err != nil || len(emptyScope.Items) != 0 || emptyScope.Total != 0 {
		t.Fatalf("ListRuns(empty scope) = %+v, %v", emptyScope, err)
	}
	if _, err := store.Repos.Workflows.ListRuns(ctx, domainworkflows.RunListQuery{Cursor: "invalid"}); !errors.Is(err, repoworkflows.ErrInvalidRunCursor) {
		t.Fatalf("ListRuns(invalid cursor) error = %v", err)
	}
	indexPath := filepath.Join(root, "runs", "index.json")
	if err := os.WriteFile(indexPath, []byte("broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	rebuilt, err := store.Repos.Workflows.ListRuns(ctx, domainworkflows.RunListQuery{TaskIDs: []string{"task_a", "task_b"}, Limit: 2})
	if err != nil || len(rebuilt.Items) != 2 || rebuilt.Items[0].ID != "run_a" {
		t.Fatalf("ListRuns(rebuilt index) = %+v, %v", rebuilt, err)
	}
}

func reviewLoopDefinition() domainworkflows.Definition {
	return domainworkflows.Definition{
		ID:          "review_loop",
		Name:        "Review Loop",
		EntryStepID: "implement",
		Steps: []domainworkflows.Step{
			{ID: "implement", Name: "Implement", Runtime: "codex", RolePrompt: "You implement changes.", Instruction: "Implement and test.", Transitions: map[string]string{"ready_for_review": "review", "need_human": domainworkflows.TargetPause}},
			{ID: "review", Name: "Review", Runtime: "claude", RolePrompt: "You review changes.", Instruction: "Review the diff.", Transitions: map[string]string{"approved": domainworkflows.TargetDone, "changes_requested": "implement"}},
		},
	}
}
