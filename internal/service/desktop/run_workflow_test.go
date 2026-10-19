package desktop

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	tasks "github.com/openmodu/onecatch/internal/domain/tasks"
	workflows "github.com/openmodu/onecatch/internal/domain/workflows"
	repoworkflows "github.com/openmodu/onecatch/internal/repo/workflows"
)

func runWorkflowFixture(t *testing.T, project bool) (*Service, workflows.Run, workflows.Definition, string) {
	t.Helper()
	a, _ := newStorageTestApp(t)
	ctx := context.Background()
	if err := a.InitializeSettings(ctx); err != nil {
		t.Fatal(err)
	}
	workspace, err := a.AddWorkspace(ctx, AddWorkspaceInput{Path: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	definition := workflows.Normalize(workflows.Definition{ID: "editable", Name: "Editable", Steps: []workflows.Step{
		{ID: "work", Name: "Work", Runtime: "codex", Model: "model", Prompt: "Original", Sandbox: "read-only"},
		{ID: "review", Name: "Review", Runtime: "codex", Model: "model", Sandbox: "read-only"},
	}, Edges: []workflows.Edge{{From: "work", To: "review"}}})
	file := ""
	if project {
		definition.ID = "repo-editable"
		file = filepath.Join(workspace.Path, repoworkflows.ProjectWorkflowDir, "editable.yaml")
		if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
			t.Fatal(err)
		}
		writeRunWorkflowFile(t, file, definition)
		definition, err = a.definitionFor(ctx, definition.ID, workspace.ID)
	} else {
		definition, err = a.store.Repos.Workflows.SaveDefinition(ctx, definition)
	}
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	task := tasks.Task{ID: "workflow_task", WorkspaceID: workspace.ID, WorkflowID: definition.ID, Title: "Config update", Prompt: "Work", Status: tasks.StatusPaused, CreatedAt: now, UpdatedAt: now}
	if err := a.store.Repos.Tasks.SaveTask(ctx, task); err != nil {
		t.Fatal(err)
	}
	definition, resolution, err := a.resolveExecutionSettings(ctx, task, definition)
	if err != nil {
		t.Fatal(err)
	}
	run, err := workflows.NewRun(definition, "workflow_run", now)
	if err != nil {
		t.Fatal(err)
	}
	run.TaskID, run.Status, run.CurrentStepID = task.ID, workflows.RunPaused, "review"
	run.MaxConcurrency, run.InterruptGraceSeconds, run.RuntimeSettings = resolution.MaxConcurrency, resolution.InterruptGraceSeconds, resolution.RuntimeSettings
	run.Sessions = map[string]string{"work": "work-session", "review": "review-session"}
	run.Nodes["work"] = workflows.NodeState{StepID: "work", Status: workflows.NodeCompleted, Attempt: 2, Summary: "Done"}
	run.Nodes["review"] = workflows.NodeState{StepID: "review", Status: workflows.NodePaused, Attempt: 1, Inbox: []workflows.Message{{From: "human", Text: "Check carefully"}}}
	run.LoopEnabled, run.LoopRound, run.PauseReason = true, 3, "interrupted"
	if err := a.store.Repos.Workflows.SaveRun(ctx, run, definition); err != nil {
		t.Fatal(err)
	}
	if err := a.store.Repos.Workflows.SaveStepRun(ctx, workflows.StepRun{ID: "attempt", RunID: run.ID, StepID: "work", Attempt: 2, Status: workflows.StepRunSucceeded, Runtime: "codex", SessionIDAfter: "work-session", Summary: "History"}); err != nil {
		t.Fatal(err)
	}
	return a, run, definition, file
}

func writeRunWorkflowFile(t *testing.T, file string, definition workflows.Definition) {
	t.Helper()
	data, err := repoworkflows.MarshalWorkflowFile(definition)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, data, 0644); err != nil {
		t.Fatal(err)
	}
}

func TestRunWorkflowCheckAndPausedUpdate(t *testing.T) {
	for _, project := range []bool{false, true} {
		t.Run(map[bool]string{false: "library", true: "project"}[project], func(t *testing.T) {
			a, run, snapshot, file := runWorkflowFixture(t, project)
			ctx := context.Background()
			check, err := a.CheckRunWorkflow(ctx, run.ID)
			if err != nil || check.Changed {
				t.Fatalf("initial check: %+v, %v", check, err)
			}
			latest := workflows.Normalize(snapshot)
			latest.Steps[0].Prompt, latest.Steps[0].Model = "Updated instructions", "new-model"
			latest.Steps[0].ReasoningEffort = "high"
			latest.Steps[1].Runtime = "claude"
			latest.Steps = append(latest.Steps, workflows.Step{ID: "ship", Name: "Ship", Kind: workflows.StepCommand, Command: "true"})
			latest.Edges = append(latest.Edges, workflows.Edge{From: "review", To: "ship"})
			latest.Policy.StepTimeoutSeconds++
			if project {
				writeRunWorkflowFile(t, file, latest)
			} else if _, err := a.store.Repos.Workflows.UpdateDefinition(ctx, latest.ID, latest); err != nil {
				t.Fatal(err)
			}
			check, err = a.CheckRunWorkflow(ctx, run.ID)
			if err != nil || !check.Changed || !strings.Contains(strings.Join(check.Changes, ","), "提示词") || !strings.Contains(strings.Join(check.Changes, ","), "推理强度") {
				t.Fatalf("changed check: %+v, %v", check, err)
			}
			before, err := a.store.Repos.Workflows.GetRunDefinition(ctx, run.ID)
			if err != nil || !reflect.DeepEqual(before, snapshot) {
				t.Fatal("checking changed the snapshot", err)
			}
			updated, err := a.UpdateRunWorkflow(ctx, run.ID)
			if err != nil {
				t.Fatal(err)
			}
			if updated.Status != workflows.RunPaused || a.isActive(run.ID) || !updated.LoopEnabled || updated.LoopRound != 3 || updated.PauseReason != run.PauseReason {
				t.Fatalf("update changed run lifecycle: %+v", updated)
			}
			if updated.Sessions["work"] != "work-session" || updated.Sessions["review"] != "" || !reflect.DeepEqual(updated.Nodes["work"], run.Nodes["work"]) || !reflect.DeepEqual(updated.Nodes["review"], run.Nodes["review"]) || updated.Nodes["ship"].Status != workflows.NodePending {
				t.Fatalf("incorrect migrated state: %+v", updated)
			}
			detail, err := a.GetRunDetail(ctx, run.ID)
			if err != nil || len(detail.StepRuns) != 1 || detail.StepRuns[0].Summary != "History" || detail.Workflow.Steps[0].Prompt != latest.Steps[0].Prompt || detail.Workflow.Steps[0].ReasoningEffort != "high" || len(detail.Events) != 1 || detail.Events[0].Type != "run.workflow_updated" {
				t.Fatalf("missing persisted data: %+v, %v", detail, err)
			}
			check, err = a.CheckRunWorkflow(ctx, run.ID)
			if err != nil || check.Changed {
				t.Fatalf("check after update: %+v, %v", check, err)
			}
			again, err := a.UpdateRunWorkflow(ctx, run.ID)
			if err != nil || again.Revision != updated.Revision {
				t.Fatalf("unchanged update must be a no-op: %+v, %v", again, err)
			}
		})
	}
}

func TestRunWorkflowUpdateRequiresPausedAndInactive(t *testing.T) {
	for _, status := range []workflows.RunStatus{workflows.RunReady, workflows.RunRunning, workflows.RunCompleted, workflows.RunFailed, workflows.RunCancelled, workflows.RunPaused} {
		t.Run(string(status), func(t *testing.T) {
			a, run, snapshot, _ := runWorkflowFixture(t, false)
			ctx := context.Background()
			run.Status = status
			run, err := a.store.Repos.Workflows.UpdateRun(ctx, run, run.Revision)
			if err != nil {
				t.Fatal(err)
			}
			beforeRun, err := a.store.Repos.Workflows.GetRun(ctx, run.ID)
			if err != nil {
				t.Fatal(err)
			}
			if status == workflows.RunPaused {
				a.active[run.ID] = func() {}
			}
			defer delete(a.active, run.ID)
			latest := workflows.Normalize(snapshot)
			latest.Steps[0].Prompt = "Changed"
			if _, err := a.store.Repos.Workflows.UpdateDefinition(ctx, latest.ID, latest); err != nil {
				t.Fatal(err)
			}
			if check, err := a.CheckRunWorkflow(ctx, run.ID); err != nil || !check.Changed {
				t.Fatalf("read-only check unavailable: %+v %v", check, err)
			}
			if _, err := a.UpdateRunWorkflow(ctx, run.ID); errorCode(err) != "run_invalid_state" {
				t.Fatalf("update allowed from %s (active=%v): %v", status, status == workflows.RunPaused, err)
			}
			stored, err := a.store.Repos.Workflows.GetRunDefinition(ctx, run.ID)
			if err != nil || !reflect.DeepEqual(stored, snapshot) {
				t.Fatal("rejected update wrote snapshot")
			}
			current, err := a.store.Repos.Workflows.GetRun(ctx, run.ID)
			if err != nil || !reflect.DeepEqual(current, beforeRun) {
				t.Fatalf("rejected update changed state:\ncurrent: %#v\nwant:    %#v\nerror:   %v", current, beforeRun, err)
			}
		})
	}
}

func TestRunWorkflowIgnoresMetadataAndKeepsInvalidSourceSnapshot(t *testing.T) {
	a, run, snapshot, file := runWorkflowFixture(t, true)
	ctx := context.Background()
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, append([]byte("# comment only\n"), data...), 0644); err != nil {
		t.Fatal(err)
	}
	if check, err := a.CheckRunWorkflow(ctx, run.ID); err != nil || check.Changed {
		t.Fatalf("comment reported as change: %+v, %v", check, err)
	}
	for _, remove := range []bool{false, true} {
		if remove {
			err = os.Remove(file)
		} else {
			err = os.WriteFile(file, []byte("name: Broken\nsteps: ["), 0644)
		}
		if err != nil {
			t.Fatal(err)
		}
		if _, err := a.CheckRunWorkflow(ctx, run.ID); err == nil {
			t.Fatal("bad source check succeeded")
		}
		if _, err := a.UpdateRunWorkflow(ctx, run.ID); err == nil {
			t.Fatal("bad source update succeeded")
		}
		stored, err := a.store.Repos.Workflows.GetRunDefinition(ctx, run.ID)
		if err != nil || !reflect.DeepEqual(stored, snapshot) {
			t.Fatal("bad source replaced snapshot")
		}
	}
}

func TestRunWorkflowIgnoresSaveTimesAndTemplateLoopDefault(t *testing.T) {
	a, run, snapshot, _ := runWorkflowFixture(t, false)
	ctx := context.Background()
	snapshot.LoopEnabled = !snapshot.LoopEnabled
	if _, err := a.store.Repos.Workflows.UpdateDefinition(ctx, snapshot.ID, snapshot); err != nil {
		t.Fatal(err)
	}
	if check, err := a.CheckRunWorkflow(ctx, run.ID); err != nil || check.Changed {
		t.Fatalf("metadata reported as change: %+v, %v", check, err)
	}
}

func TestRunWorkflowRemovesObsoleteNodesAndSessions(t *testing.T) {
	a, run, snapshot, _ := runWorkflowFixture(t, false)
	ctx := context.Background()
	snapshot.Steps = snapshot.Steps[:1]
	snapshot.Edges = nil
	if _, err := a.store.Repos.Workflows.UpdateDefinition(ctx, snapshot.ID, snapshot); err != nil {
		t.Fatal(err)
	}
	updated, err := a.UpdateRunWorkflow(ctx, run.ID)
	if err != nil || len(updated.Nodes) != 1 || len(updated.Sessions) != 1 || updated.CurrentStepID != "work" || updated.Status != workflows.RunPaused {
		t.Fatalf("removed node remained: %+v, %v", updated, err)
	}
}
