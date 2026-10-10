package keeldesktop

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/kit"
	"github.com/dyike/keel/ui/locale"
	"github.com/dyike/keel/ui/window"
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

func TestKeelSettingsNavigationSearchAndDraftRetention(t *testing.T) {
	locale.Apply(locale.Chinese())
	applyPalette(false)
	for _, scale := range []int{1, 2} {
		a := dagTestApp(t)
		p := workerTestSettings(a)
		p.section = "execution"
		router, frame, click := alignmentRouter(el.Root(p), 960, 800, scale)
		find := func(label string) {
			t.Helper()
			box, ok := findSkillControl(router.AppendSemantics(nil), label)
			if !ok {
				t.Fatalf("missing control %q at %dx", label, scale)
			}
			click(box)
			frame()
		}
		view := p.view
		value := p.fields["dag-concurrency"]
		*value = "6"
		find("终端")
		if p.section != "terminal" {
			t.Fatal("Keel navigation did not select terminal repository section")
		}
		find("执行")
		if p.fields["dag-concurrency"] != value || *value != "6" || p.view != view {
			t.Fatal("page navigation lost draft or component identity")
		}
		p.view.SetQuery("完全访问")
		frame()
		frame()
		if p.section != "security" || p.view.Value() != "安全" {
			t.Fatal("search selected the wrong repository section")
		}
		if _, ok := findSkillControl(router.AppendSemantics(nil), "保存"); !ok {
			t.Fatal("filtering groups hid the save action")
		}
		p.view.SetQuery("no matching setting")
		frame()
		frame()
		if p.section != "security" {
			t.Fatal("empty search changed section")
		}
		p.view.SetQuery("")
		p.section = "execution"
		frame()
		frame()
		draft, err := p.draftForSave()
		if err != nil || draft.Execution.MaxLocalDAGConcurrency != 6 {
			t.Fatalf("retained field did not reach repository draft: %+v, %v", draft.Execution, err)
		}
		find("Harness")
		find("Agent")
		find("Claude Code")
		if p.runtime.Value() != "claude" || p.view != view {
			t.Fatal("Agent switching lost Settings identity")
		}
		if _, ok := findSkillControl(router.AppendSemantics(nil), "允许完全访问"); ok {
			t.Fatal("previous section control leaked into Agent page")
		}
		p.fields = nil // Same reset used by repository reload.
		frame()
		frame()
		p.section = "execution"
		frame()
		frame()
		if *p.fields["dag-concurrency"] == "6" {
			t.Fatal("reload retained discarded draft")
		}
	}
}

func TestRenderKeelSettingsComponent(t *testing.T) {
	dir := os.Getenv("ONECATCH_KEEL_PREVIEWS")
	if dir == "" {
		t.Skip("set ONECATCH_KEEL_PREVIEWS")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	locale.Apply(locale.Chinese())
	a := dagTestApp(t)
	p := workerTestSettings(a)
	for _, dark := range []bool{false, true} {
		a.dark = dark
		applyPalette(dark)
		for _, id := range []string{"", "harness", "execution", "terminal", "security", "storage"} {
			p.section = id
			name := id
			if name == "" {
				name = "appearance"
			}
			mode := "light"
			if dark {
				mode = "dark"
			}
			if err := window.Screenshot(el.Root(p), 960, 800, filepath.Join(dir, "settings-component-"+name+"-"+mode+".png")); err != nil {
				t.Fatal(err)
			}
		}
	}
	applyPalette(false)
}
