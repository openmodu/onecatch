package keeldesktop

import (
	"context"
	"fmt"
	desktopservice "github.com/openmodu/onecatch/internal/service/desktop"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/dyike/keel/gio/io/key"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/kit"
	"github.com/dyike/keel/ui/locale"
	"github.com/dyike/keel/ui/window"
)

func TestTableDividerSettingPersistsThroughPointerAndKeyboard(t *testing.T) {
	locale.Apply(locale.Chinese())
	applyPalette(false)
	for _, scale := range []int{1, 2} {
		a := conversationFixture(t)
		p := workerTestSettings(a)
		p.section = ""
		router, frame, click := alignmentRouter(el.Root(p), 960, 800, scale)
		control := func() {
			t.Helper()
			box, ok := findSkillControl(router.AppendSemantics(nil), "表格分隔线")
			if !ok {
				t.Fatal("missing divider setting")
			}
			click(box)
			frame()
		}
		if a.ui.Preferences.HideTableDividers || !p.switches["table-dividers"].Value() {
			t.Fatal("dividers must be enabled by default")
		}
		control()
		saved, err := loadUIPreferences(a.backend.service.DataRoot())
		if err != nil || !saved.HideTableDividers || p.switches["table-dividers"].Value() {
			t.Fatalf("disabled preference was not persisted: %+v %v", saved, err)
		}
		// Reopening/rebuilding settings reads the persisted preference.
		a.ui.Preferences = saved
		p.fields = nil
		frame()
		frame()
		if p.switches["table-dividers"].Value() {
			t.Fatal("rebuilding settings reset the switch")
		}
		// Focus the rebuilt switch without changing it; Space re-enables it.
		requested := false
		root := el.Root(el.ViewFunc(func(cx *el.Context) el.Element {
			view := p.Render(cx)
			if !requested {
				cx.Focus(fmt.Sprintf("filled-toggle/%p", p.switches["table-dividers"]))
				requested = true
			}
			return view
		}))
		router, frame, _ = alignmentRouter(root, 960, 800, scale)
		router.Queue(key.Event{Name: key.NameSpace, State: key.Press}, key.Event{Name: key.NameSpace, State: key.Release})
		frame()
		frame()
		saved, err = loadUIPreferences(a.backend.service.DataRoot())
		if err != nil || saved.HideTableDividers || !p.switches["table-dividers"].Value() {
			t.Fatalf("keyboard did not restore dividers: %+v %v", saved, err)
		}
	}
}

func TestRenderTableDividerSettings(t *testing.T) {
	dir := os.Getenv("ONECATCH_KEEL_PREVIEWS")
	if dir == "" {
		t.Skip("set ONECATCH_KEEL_PREVIEWS to render settings")
	}
	locale.Apply(locale.Chinese())
	defer applyPalette(false)
	for _, dark := range []bool{false, true} {
		applyPalette(dark)
		a := conversationFixture(t)
		a.dark = dark
		p := workerTestSettings(a)
		p.section = ""
		if err := screenshotWithFocus(el.Root(p), 960, 800, filepath.Join(dir, fmt.Sprintf("settings-table-dividers-dark-%v.png", dark))); err != nil {
			t.Fatal(err)
		}
	}
}

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
	p := &settingsPage{draft: initial, section: "execution", fields: map[string]*string{"max-concurrency": &value}, runtime: kit.Select("")}
	draft, err := p.draftForSave()
	if err != nil {
		t.Fatal(err)
	}
	updated, err := b.service.UpdateExecutionSettings(ctx, draft.Execution, draft.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Execution.MaxConcurrency != 6 || initial.Execution.MaxConcurrency == 6 || updated.Terminal.Shell != initial.Terminal.Shell || updated.Terminal.Theme != initial.Terminal.Theme || !slices.Equal(updated.Terminal.Arguments, initial.Terminal.Arguments) {
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
	if current.Execution.MaxConcurrency != 6 {
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
		value := p.fields["max-concurrency"]
		*value = "6"
		find("安全")
		if p.section != "security" {
			t.Fatal("Keel navigation did not select security repository section")
		}
		find("执行")
		if p.fields["max-concurrency"] != value || *value != "6" || p.view != view {
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
		if err != nil || draft.Execution.MaxConcurrency != 6 {
			t.Fatalf("retained field did not reach repository draft: %+v, %v", draft.Execution, err)
		}
		find("Harness")
		find("Claude Code")
		if !p.harnessExpanded["claude"] || p.view != view {
			t.Fatal("Harness expansion lost Settings identity")
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
		if *p.fields["max-concurrency"] == "6" {
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
		for _, id := range []string{"", "harness", "execution", "security", "storage"} {
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

func TestSettingsModalLifecycle(t *testing.T) {
	a := conversationFixture(t)
	a.openSettings()
	p := a.settingsPanel
	if p == nil {
		t.Fatal("settings did not open")
	}
	a.openSettings()
	if a.settingsPanel != p {
		t.Fatal("opening twice replaced settings")
	}
	router, frame, click := alignmentRouter(el.Root(a), 1280, 800, 1)
	for _, label := range []string{"外观", "手机连接", "执行", "安全", "存储与日志", "远端 Worker"} {
		if _, ok := alignmentNode(router.AppendSemantics(nil), label, true); !ok {
			t.Fatalf("missing settings category %s", label)
		}
	}
	if _, ok := alignmentNode(router.AppendSemantics(nil), "收起侧栏", true); ok {
		t.Fatal("settings must not collapse navigation")
	}

	p.activateSettingsSection("workflows")
	frame()
	frame()
	if _, ok := alignmentNode(router.AppendSemantics(nil), "新建工作流", true); !ok {
		t.Fatal("workflow manager not embedded")
	}
	p.activateSettingsSection("")
	a.goPage("workflows")
	if a.settingsPanel != p || p.section != "workflows" || a.page == "workflows" {
		t.Fatal("workflows must open inside Settings, not as a page or window")
	}
	p.activateSettingsSection("")
	frame()
	frame()

	box, ok := alignmentNode(router.AppendSemantics(nil), "关闭设置", true)
	if !ok {
		t.Fatal("missing modal close button")
	}
	click(box.Bounds)
	frame()
	if a.settingsPanel != nil || !p.workers.closed {
		t.Fatal("closing did not release settings")
	}
	a.openSettings()
	if a.settingsPanel == p {
		t.Fatal("reused closed settings")
	}
	router, frame, _ = alignmentRouter(el.Root(a), 1280, 800, 1)
	router.Queue(key.Event{Name: key.NameEscape, State: key.Press}, key.Event{Name: key.NameEscape, State: key.Release})
	frame()
	frame()
	if a.settingsPanel != nil {
		t.Fatal("Escape did not dismiss settings")
	}
}
func TestRenderSettingsModal(t *testing.T) {
	dir := os.Getenv("ONECATCH_KEEL_PREVIEWS")
	if dir == "" {
		t.Skip("preview disabled")
	}
	for _, dark := range []bool{false, true} {
		a := conversationFixture(t)
		applyPalette(dark)
		a.settingsPanel = workerTestSettings(a)
		a.settingsPanel.section = ""
		if err := screenshotWithFocus(el.Root(a), 1920, 997, filepath.Join(dir, fmt.Sprintf("settings-modal-%v.png", dark))); err != nil {
			t.Fatal(err)
		}
		a.closeSettings()
	}
}

func TestSettingsSaveActionsStayAtBottomRight(t *testing.T) {
	locale.Apply(locale.Chinese())
	a := conversationFixture(t)
	p := workerTestSettings(a)
	p.section = "execution"
	router, _, _ := alignmentRouter(el.Root(p), 1120, 760, 1)
	nodes := router.AppendSemantics(nil)
	save, ok := findSkillControl(nodes, "保存")
	if !ok {
		t.Fatal("missing save action")
	}
	if save.Min.X < 900 || save.Min.Y < 680 || save.Max.Y > 760 {
		t.Fatalf("save action outside compact bottom-right footer: %v", save)
	}
	for _, label := range []string{"恢复默认值", "放弃修改", "重新加载"} {
		box, ok := findSkillControl(nodes, label)
		if !ok || box.Min.Y != save.Min.Y {
			t.Fatalf("action %q is not aligned with save: %v vs %v", label, box, save)
		}
	}
	if got := settingsOptionLabel("default-sandbox", "workspace-write"); got != "工作区访问" {
		t.Fatalf("internal permission value leaked: %q", got)
	}
}

func TestHarnessCardsExpandIndependentlyAndPreserveDrafts(t *testing.T) {
	locale.Apply(locale.Chinese())
	a := conversationFixture(t)
	a.runtimes = []desktopservice.RuntimeInfo{{ID: "claude", Name: "Claude Code"}, {ID: "codex", Name: "Codex"}}
	p := workerTestSettings(a)
	p.section = "harness"
	router, frame, click := alignmentRouter(el.Root(p), 1120, 760, 1)
	entries := p.runtime.Entries()
	if len(entries) < 2 {
		t.Fatal("expected multiple harnesses")
	}
	if len(p.harnessExpanded) != 0 {
		t.Fatal("harnesses must start collapsed")
	}
	for _, entry := range entries[:2] {
		box, ok := findSkillControl(router.AppendSemantics(nil), entry.Label)
		if !ok {
			t.Fatalf("missing harness %s", entry.Label)
		}
		click(box)
		frame()
		if !p.harnessExpanded[entry.Value] {
			t.Fatal("card did not expand")
		}
		field := p.fields["runtime-binary/"+entry.Value]
		if field == nil {
			t.Fatal("missing executable field")
		}
		*field = "/custom/" + entry.Value
	}
	for _, entry := range entries[:2] {
		if !p.harnessExpanded[entry.Value] {
			t.Fatal("opening another harness collapsed existing card")
		}
		if got := p.runtimeDraft(entry.Value).Binary; got != "/custom/"+entry.Value {
			t.Fatalf("draft lost: %s", got)
		}
	}
	p.harnessExpanded[entries[0].Value] = false
	frame()
	p.harnessExpanded[entries[0].Value] = true
	frame()
	if got := p.runtimeDraft(entries[0].Value).Binary; got != "/custom/"+entries[0].Value {
		t.Fatal("collapse discarded draft")
	}
}

func TestWorkflowLibraryOpensEditorInsideSettings(t *testing.T) {
	a := conversationFixture(t)
	a.openSettings()
	a.ui.WorkflowSource = false
	a.settingsPanel.activateSettingsSection("workflows")
	router, frame, click := alignmentRouter(el.Root(a), 1280, 800, 1)
	frame()
	node, ok := findSkillControl(router.AppendSemantics(nil), "配置")
	if !ok {
		t.Fatal("workflow configuration action missing")
	}
	click(node.Bounds())
	frame()
	if !a.ui.WorkflowSource {
		t.Fatal("configuration must open inline")
	}
	a.ui.WorkflowDraft.Description = "unsaved library draft"
	back, ok := findSkillControl(router.AppendSemantics(nil), "返回工作流")
	if !ok {
		t.Fatal("missing return to library")
	}
	click(back.Bounds())
	frame()
	if a.ui.WorkflowSource {
		t.Fatal("return did not restore library")
	}
	node, ok = findSkillControl(router.AppendSemantics(nil), "配置")
	if !ok {
		t.Fatal("configuration action missing after return")
	}
	click(node.Bounds())
	frame()
	if a.ui.WorkflowDraft.Description != "unsaved library draft" {
		t.Fatal("returning to the same workflow lost its draft")
	}
}

func TestWorkflowEditorAloneGoesFullScreen(t *testing.T) {
	a := conversationFixture(t)
	a.goPage("workflows")
	a.selectWorkflow("design_to_pr", true)
	router, frame, click := alignmentRouter(el.Root(a), 1280, 800, 1)
	box, ok := findSkillControl(router.AppendSemantics(nil), "全屏")
	if !ok {
		t.Fatal("workflow editor has no full screen control")
	}
	click(box)
	if _, ok := alignmentNode(router.AppendSemantics(nil), "编排工作流", false); !ok || !a.workflowFullscreen() {
		t.Fatal("editor did not take over the window")
	}
	if _, ok := alignmentNode(router.AppendSemantics(nil), "关闭设置", true); ok {
		t.Fatal("full screen still shows the Settings chrome")
	}
	router.Queue(key.Event{Name: key.NameEscape, State: key.Press}, key.Event{Name: key.NameEscape, State: key.Release})
	frame()
	frame()
	if a.settingsPanel == nil || a.ui.WorkflowFullscreen || a.settingsPanel.section != "workflows" {
		t.Fatal("Escape must leave full screen and keep the workflow settings open")
	}
	// Other Settings sections never render full screen, even with the flag set.
	a.ui.WorkflowFullscreen = true
	a.settingsPanel.activateSettingsSection("")
	if a.workflowFullscreen() {
		t.Fatal("full screen leaked outside the workflow editor")
	}
}

func TestRenderWorkflowFullScreenPreview(t *testing.T) {
	dir := os.Getenv("ONECATCH_KEEL_PREVIEWS")
	if dir == "" {
		t.Skip("set ONECATCH_KEEL_PREVIEWS to render the full screen editor")
	}
	a := dagTestApp(t)
	a.openWorkflows()
	a.selectWorkflow("design_to_pr", true)
	a.ui.WorkflowFullscreen = true
	a.layoutDAG()
	// The full screen panel holds only the editor, so render it at window size.
	if err := window.Screenshot(el.Root(workflowView{a}), 1440, 900, filepath.Join(dir, "workflow-fullscreen.png")); err != nil {
		t.Fatal(err)
	}
}
