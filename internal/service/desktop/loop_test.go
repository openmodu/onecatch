package desktop

import (
	"context"
	workflows "github.com/openmodu/onecatch/internal/domain/workflows"
	localdata "github.com/openmodu/onecatch/internal/repo/store/local"
	"path/filepath"
	"testing"
	"time"
)

func TestSetRunLoopPersistsAndRejectsActiveRun(t *testing.T) {
	ctx := context.Background()
	store, err := localdata.OpenStore(filepath.Join(t.TempDir(), ".onecatch"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	def := workflows.Definition{ID: "loop", Name: "Loop", Steps: []workflows.Step{{ID: "work", Name: "Work", Runtime: "codex"}}}
	def, err = store.Repos.Workflows.SaveDefinition(ctx, def)
	if err != nil {
		t.Fatal(err)
	}
	run, err := workflows.NewRun(def, "run", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	run.TaskID = "task"
	if err := store.Repos.Workflows.SaveRun(ctx, run, def); err != nil {
		t.Fatal(err)
	}
	service := &Service{store: store, active: map[string]context.CancelFunc{}}
	updated, err := service.SetRunLoop(ctx, run.ID, true)
	if err != nil || !updated.LoopEnabled || updated.LoopRound != 1 {
		t.Fatalf("enable: %+v %v", updated, err)
	}
	persisted, err := store.Repos.Workflows.GetRun(ctx, run.ID)
	if err != nil || !persisted.LoopEnabled {
		t.Fatal("lost loop mode")
	}
	service.active[run.ID] = func() {}
	if _, err := service.SetRunLoop(ctx, run.ID, false); err == nil {
		t.Fatal("changed active run mode")
	}
	delete(service.active, run.ID)
	updated, err = service.SetRunLoop(ctx, run.ID, false)
	if err != nil || updated.LoopEnabled {
		t.Fatalf("disable: %+v %v", updated, err)
	}
}
