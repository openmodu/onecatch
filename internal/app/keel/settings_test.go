package keeldesktop

import (
	"context"
	"slices"
	"testing"

	"github.com/dyike/keel/ui/kit"
)

func TestKeelSettingsDraftUsesRepositoryRevisionAndPreservesOtherSections(t *testing.T) {
	ctx := context.Background()
	b, err := openBackendWithEngine(ctx, t.TempDir(), testEngine{})
	if err != nil {
		t.Fatal(err)
	}
	defer b.close()
	initial, err := b.service.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	value := "6"
	p := &settingsPage{draft: initial, section: "execution", fields: map[string]*string{"dag-concurrency": &value}, runtime: kit.Select("")}
	draft, err := p.draftForSave()
	if err != nil {
		t.Fatal(err)
	}
	updated, err := b.service.UpdateExecutionSettings(ctx, draft.Execution, draft.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Execution.MaxLocalDAGConcurrency != 6 || initial.Execution.MaxLocalDAGConcurrency == 6 || updated.Terminal.Shell != initial.Terminal.Shell || updated.Terminal.Theme != initial.Terminal.Theme || !slices.Equal(updated.Terminal.Arguments, initial.Terminal.Arguments) {
		t.Fatalf("incorrect section update: %+v", updated.Execution)
	}
	// A settings window opened before another save must receive a conflict;
	// silently obtaining the newest revision would overwrite the newer values.
	if _, err = b.service.UpdateExecutionSettings(ctx, initial.Execution, initial.Revision); err == nil {
		t.Fatal("stale draft overwrote settings")
	}
	current, err := b.service.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if current.Execution.MaxLocalDAGConcurrency != 6 {
		t.Fatal("conflict changed data")
	}
	value = "not a number"
	if _, err = p.draftForSave(); err == nil {
		t.Fatal("invalid number accepted")
	}
}
