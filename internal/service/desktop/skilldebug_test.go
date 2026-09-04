package desktop

import (
	"context"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/openmodu/onecatch/internal/service/skillmanager"
)

func skillDebugService(t *testing.T) *Service {
	t.Helper()
	manager, err := skillmanager.New(filepath.Join(t.TempDir(), ".onecatch", "skills"))
	if err != nil {
		t.Fatal(err)
	}
	service := &Service{}
	service.skills = manager
	return service
}

func TestSkillDebugHistoryKeepsNewestRunsFirst(t *testing.T) {
	service := skillDebugService(t)

	empty, err := service.ListSkillDebugRuns("release-notes")
	if err != nil {
		t.Fatal(err)
	}
	if len(empty) != 0 {
		t.Fatalf("a skill that was never debugged has no history: %#v", empty)
	}

	for index := range 3 {
		record := SkillDebugRecord{
			RunID:     "run-" + strconv.Itoa(index),
			Skill:     "release-notes",
			Prompt:    "attempt " + strconv.Itoa(index),
			StartedAt: time.Now(),
			Result:    SkillDebugResult{Succeeded: true, Output: "ok", Events: []SkillDebugEvent{{Kind: "message", Text: "hello"}}},
		}
		if err := service.appendSkillDebugRun(record); err != nil {
			t.Fatal(err)
		}
	}

	records, err := service.ListSkillDebugRuns("release-notes")
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 3 || records[0].RunID != "run-2" || records[2].RunID != "run-0" {
		t.Fatalf("history must read newest first: %#v", records)
	}
	if records[0].Result.Events[0].Text != "hello" {
		t.Fatalf("the transcript itself must survive the round trip: %#v", records[0].Result)
	}

	if err := service.ClearSkillDebugRuns("release-notes"); err != nil {
		t.Fatal(err)
	}
	cleared, err := service.ListSkillDebugRuns("release-notes")
	if err != nil {
		t.Fatal(err)
	}
	if len(cleared) != 0 {
		t.Fatalf("clear must forget every run: %#v", cleared)
	}
}

func TestSkillDebugHistoryStaysBounded(t *testing.T) {
	service := skillDebugService(t)
	for index := range skillDebugHistoryLimit + 5 {
		if err := service.appendSkillDebugRun(SkillDebugRecord{RunID: "run-" + strconv.Itoa(index), Skill: "release-notes", StartedAt: time.Now()}); err != nil {
			t.Fatal(err)
		}
	}
	records, err := service.ListSkillDebugRuns("release-notes")
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != skillDebugHistoryLimit {
		t.Fatalf("history must stop at %d runs, got %d", skillDebugHistoryLimit, len(records))
	}

	// One oversized transcript must not push the file past its byte budget,
	// and it must still be readable afterwards.
	if err := service.appendSkillDebugRun(SkillDebugRecord{
		RunID:     "huge",
		Skill:     "release-notes",
		StartedAt: time.Now(),
		Result:    SkillDebugResult{Events: []SkillDebugEvent{{Kind: "tool_result", Text: strings.Repeat("x", skillDebugHistoryBytes)}}},
	}); err != nil {
		t.Fatal(err)
	}
	trimmed, err := service.ListSkillDebugRuns("release-notes")
	if err != nil {
		t.Fatal(err)
	}
	if len(trimmed) != 1 || trimmed[0].RunID != "huge" {
		t.Fatalf("an oversized run keeps only itself: %d records", len(trimmed))
	}
}

func TestSkillDebugHistoryRefusesNamesOutsideTheLibrary(t *testing.T) {
	service := skillDebugService(t)
	for _, name := range []string{"", "../escape", "Release Notes", "nested/skill"} {
		if _, err := service.ListSkillDebugRuns(name); err == nil {
			t.Fatalf("expected %q to be refused", name)
		}
	}
}

func TestStopSkillDebugCancelsTheRegisteredRun(t *testing.T) {
	service := skillDebugService(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	run := &skillDebugRun{cancel: cancel}
	service.registerSkillDebug("run-1", run)

	service.StopSkillDebug("run-1")
	if !run.stopped.Load() {
		t.Fatal("a stopped run must be distinguishable from one that failed")
	}
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("stop must cancel the run context")
	}

	// Releasing makes the id inert, so a late stop cannot cancel whatever run
	// reuses the panel next.
	service.releaseSkillDebug("run-1")
	service.StopSkillDebug("run-1")
}
