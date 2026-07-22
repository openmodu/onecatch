package localapp

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	domainworkflows "github.com/openmodu/oneshot/internal/domain/workflows"
)

// seedRunWithRounds builds a run whose transcript is spread over roundCount step
// runs of eventsPerRound events each, without going through the orchestrator.
func seedRunWithRounds(t *testing.T, app *App, roundCount, eventsPerRound int) string {
	t.Helper()
	ctx := context.Background()

	workspace, err := app.AddWorkspace(ctx, AddWorkspaceInput{Path: t.TempDir(), Name: "ws"})
	if err != nil {
		t.Fatal(err)
	}
	definitions, err := app.ListDefinitions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(definitions) == 0 {
		t.Fatal("expected a builtin workflow definition")
	}
	definition := definitions[0]
	task, err := app.CreateTask(ctx, CreateTaskInput{WorkspaceID: workspace.ID, Title: "t", Prompt: "p", WorkflowID: definition.ID})
	if err != nil {
		t.Fatal(err)
	}

	run := domainworkflows.Run{
		ID: "run_pagination", TaskID: task.ID, WorkflowID: definition.ID,
		CurrentStepID: definition.Steps[0].ID, Status: domainworkflows.RunCompleted, StartedAt: time.Now().UTC(),
	}
	if err := app.store.Repos.Workflows.SaveRun(ctx, run, definition); err != nil {
		t.Fatal(err)
	}
	for round := range roundCount {
		stepRunID := fmt.Sprintf("step_%02d", round)
		stepRun := domainworkflows.StepRun{
			ID: stepRunID, RunID: run.ID, StepID: definition.Steps[0].ID, Attempt: 1,
			Status: domainworkflows.StepRunSucceeded, StartedAt: time.Now().UTC().Add(time.Duration(round) * time.Minute),
		}
		if err := app.store.Repos.Workflows.SaveStepRun(ctx, stepRun); err != nil {
			t.Fatal(err)
		}
		for event := range eventsPerRound {
			payload, _ := json.Marshal(map[string]any{"kind": "tool_use", "text": fmt.Sprintf("r%d-e%d", round, event)})
			if _, err := app.store.Repos.Workflows.AppendRuntimeEvent(ctx, run.ID, stepRunID, payload); err != nil {
				t.Fatal(err)
			}
		}
	}
	return run.ID
}

func TestGetRunDetailLoadsOnlyRecentRoundsAndReportsTruncation(t *testing.T) {
	app, _ := newLocalTestApp(t, completingEngine{})
	// 10 rounds x 100 events is far over the 400-event budget.
	runID := seedRunWithRounds(t, app, 10, 100)

	detail, err := app.GetRunDetail(context.Background(), runID)
	if err != nil {
		t.Fatalf("GetRunDetail: %v", err)
	}
	if !detail.TranscriptTruncated {
		t.Fatal("expected the transcript to report truncation")
	}
	if len(detail.StepRuns) != 10 {
		t.Fatalf("every step run must still be listed, got %d", len(detail.StepRuns))
	}
	if len(detail.LoadedStepRunIDs) == 0 || len(detail.LoadedStepRunIDs) >= 10 {
		t.Fatalf("expected a partial set of loaded rounds, got %d", len(detail.LoadedStepRunIDs))
	}
	// The newest round must always be present, and loaded rounds are the tail.
	last := detail.StepRuns[len(detail.StepRuns)-1].ID
	if detail.LoadedStepRunIDs[len(detail.LoadedStepRunIDs)-1] != last {
		t.Fatalf("newest round %q must be loaded, got %v", last, detail.LoadedStepRunIDs)
	}
	for _, event := range detail.RuntimeEvents {
		if event.StepRunID == "step_00" {
			t.Fatal("the oldest round should not have been read")
		}
	}
}

func TestGetRunDetailKeepsShortTranscriptWhole(t *testing.T) {
	app, _ := newLocalTestApp(t, completingEngine{})
	runID := seedRunWithRounds(t, app, 3, 10)

	detail, err := app.GetRunDetail(context.Background(), runID)
	if err != nil {
		t.Fatalf("GetRunDetail: %v", err)
	}
	if detail.TranscriptTruncated {
		t.Fatal("a small transcript must not be truncated")
	}
	if len(detail.LoadedStepRunIDs) != 3 {
		t.Fatalf("expected all 3 rounds loaded, got %v", detail.LoadedStepRunIDs)
	}
	if len(detail.RuntimeEvents) != 30 {
		t.Fatalf("expected 30 runtime events, got %d", len(detail.RuntimeEvents))
	}
}

func TestGetStepRunTranscriptLoadsASkippedRound(t *testing.T) {
	app, _ := newLocalTestApp(t, completingEngine{})
	runID := seedRunWithRounds(t, app, 10, 100)

	events, err := app.GetStepRunTranscript(context.Background(), runID, "step_00")
	if err != nil {
		t.Fatalf("GetStepRunTranscript: %v", err)
	}
	if len(events) == 0 {
		t.Fatal("expected the skipped round to be loadable on demand")
	}
	for _, event := range events {
		if event.StepRunID != "step_00" {
			t.Fatalf("unexpected step run in transcript: %q", event.StepRunID)
		}
	}
}
