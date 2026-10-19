package workflows_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	domainworkflows "github.com/openmodu/onecatch/internal/domain/workflows"
	"github.com/openmodu/onecatch/internal/repo/localstore"
)

func TestInstructionFollowUpFIFOAndEditSurviveReload(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store, err := localstore.OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	def := reviewLoopDefinition()
	if _, err := store.Repos.Workflows.SaveDefinition(ctx, def); err != nil {
		t.Fatal(err)
	}
	run, err := domainworkflows.NewRun(def, "run_queue", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	run.TaskID = "task_queue"
	if err := store.Repos.Workflows.SaveRun(ctx, run, def); err != nil {
		t.Fatal(err)
	}
	at := time.Now().UTC()
	// Deliberately reverse ID order and use equal timestamps. IDs are random,
	// so they must never decide which message runs first.
	for _, id := range []string{"z_first", "a_second", "m_third"} {
		_, err := store.Repos.Workflows.EnqueueInstruction(ctx, run.ID, domainworkflows.Instruction{ID: id, Content: id, FollowUp: true, CreatedAt: at, Attachments: []string{"/saved/notes.txt"}})
		if err != nil {
			t.Fatal(err)
		}
	}
	before, _ := store.Repos.Workflows.ListInstructions(ctx, run.ID)
	edited, err := store.Repos.Workflows.UpdateInstructionContent(ctx, run.ID, "a_second", " edited second ")
	if err != nil {
		t.Fatal(err)
	}
	want := before[1]
	want.Content = "edited second"
	if !reflect.DeepEqual(edited, want) {
		t.Fatalf("edit changed identity/order/attachments: %+v, want %+v", edited, want)
	}
	if _, err := store.Repos.Workflows.UpdateInstructionContent(ctx, run.ID, "a_second", "  "); err == nil {
		t.Fatal("empty edit accepted")
	}
	reloaded, err := localstore.OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	repo := reloaded.Repos.Workflows
	if items, err := repo.ClaimInstructions(ctx, run.ID, false); err != nil || len(items) != 0 {
		t.Fatalf("in-flight turn claimed follow-ups: %+v %v", items, err)
	}
	for i, id := range []string{"z_first", "a_second", "m_third"} {
		items, err := repo.ClaimInstructions(ctx, run.ID, true)
		if err != nil || len(items) != 1 || items[0].ID != id {
			t.Fatalf("turn %d claimed %+v: %v", i, items, err)
		}
		if id == "a_second" && items[0].Content != "edited second" {
			t.Fatal("edit lost on reload")
		}
		if _, err := repo.UpdateInstructionContent(ctx, run.ID, id, "too late"); err == nil {
			t.Fatal("claimed instruction remained editable")
		}
	}
}

func TestInstructionSteeringLeavesFollowUpsPending(t *testing.T) {
	ctx := context.Background()
	store, err := localstore.OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	def := reviewLoopDefinition()
	if _, err := store.Repos.Workflows.SaveDefinition(ctx, def); err != nil {
		t.Fatal(err)
	}
	run, err := domainworkflows.NewRun(def, "run_steer", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	run.TaskID = "task_queue"
	if err := store.Repos.Workflows.SaveRun(ctx, run, def); err != nil {
		t.Fatal(err)
	}
	for _, item := range []domainworkflows.Instruction{
		{ID: "later", Content: "later", FollowUp: true},
		{ID: "urgent", Content: "urgent", Priority: true},
	} {
		if _, err := store.Repos.Workflows.EnqueueInstruction(ctx, run.ID, item); err != nil {
			t.Fatal(err)
		}
	}
	items, err := store.Repos.Workflows.ClaimInstructions(ctx, run.ID, true)
	if err != nil || len(items) != 1 || items[0].ID != "urgent" {
		t.Fatalf("steering drained follow-ups: %+v %v", items, err)
	}
}
