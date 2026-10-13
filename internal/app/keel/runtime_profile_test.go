package keeldesktop

import (
	"fmt"
	"github.com/dyike/keel/gio/io/key"
	"github.com/dyike/keel/gio/layout"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/locale"
	"github.com/dyike/keel/ui/theme"
	"github.com/dyike/keel/ui/window"
	workflows "github.com/openmodu/onecatch/internal/domain/workflows"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
)

func TestContextReadingMatchesWailsComposer(t *testing.T) {
	cases := []struct {
		name, harness    string
		steps            []workflows.StepRun
		known, remaining bool
		percent          int
	}{
		{"latest", "pi", []workflows.StepRun{{ContextWindow: 200000, ContextTokens: 60000}, {ContextWindow: 200000, ContextTokens: 120000}, {ContextWindow: 200000, ContextTokens: 150000}}, true, false, 75},
		{"round half", "pi", []workflows.StepRun{{ContextWindow: 200000, ContextTokens: 1000}}, true, false, 1},
		{"no sample", "pi", []workflows.StepRun{{ContextWindow: 200000}}, false, false, 0},
		{"overfull", "pi", []workflows.StepRun{{ContextWindow: 200000, ContextTokens: 214000}}, true, false, 100},
		{"retain", "pi", []workflows.StepRun{{ContextWindow: 200000, ContextTokens: 90000}, {InputTokens: 10}}, true, false, 45},
		{"compaction", "pi", []workflows.StepRun{{ContextWindow: 200000, ContextTokens: 180000}, {ContextWindow: 200000, ContextTokens: 32000}}, true, false, 16},
		{"codex remaining", "codex", []workflows.StepRun{{ContextWindow: 258400, ContextTokens: 180000}}, true, true, 32},
		{"codex baseline", "codex", []workflows.StepRun{{ContextWindow: 258400, ContextTokens: 12000}}, true, true, 100},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := summarizeContextWindow(c.steps, c.harness)
			if got.known != c.known || got.remaining != c.remaining || int(math.Round(float64(got.ratio)*100)) != c.percent {
				t.Fatalf("reading: %+v", got)
			}
		})
	}
}

func TestComposerModelPickerAndContextPlacement(t *testing.T) {
	applyPalette(false)
	for _, scale := range []int{1, 2} {
		a := conversationFixture(t)
		a.model = "gpt-6-sol"
		a.task.Harness = "codex"
		a.runtime.SetValue("codex")
		a.detail.StepRuns = []workflows.StepRun{{ContextWindow: 258400, ContextTokens: 180000}}
		a.profile.key = a.runtimeProfileKey()
		a.profile.configuration = agentrun.HarnessConfiguration{Model: "gpt-6-sol", Models: []agentrun.HarnessModel{{Model: "gpt-6-sol", DisplayName: "GPT-6-sol", DefaultEffort: "high"}, {Model: "gpt-6-luna", DisplayName: "GPT-6-luna"}}}
		router, frame, click := alignmentRouter(el.Root(a), 1120, 800, scale)
		model, ok := findSkillControl(router.AppendSemantics(nil), "模型设置：6 sol 高")
		if !ok {
			t.Fatal("missing compact model pill")
		}
		gauge, ok := alignmentNode(router.AppendSemantics(nil), "上下文窗口", false)
		if !ok {
			t.Fatalf("missing remaining gauge: %+v", gauge)
		}
		if _, ok := alignmentNode(router.AppendSemantics(nil), "32%", false); !ok {
			t.Fatal("missing remaining percentage")
		}
		permission, ok := alignmentNode(router.AppendSemantics(nil), "完全访问", false)
		if !ok || gauge.Bounds.Min.X < permission.Bounds.Max.X || gauge.Bounds.Max.X > model.Min.X || abs(gauge.Bounds.Min.Y+gauge.Bounds.Dy()/2-(model.Min.Y+model.Dy()/2)) > 3*scale {
			t.Fatalf("composer alignment: permission=%v gauge=%v model=%v", permission.Bounds, gauge.Bounds, model)
		}
		click(model)
		next, ok := findSkillControl(router.AppendSemantics(nil), "GPT-6-luna")
		if !ok {
			t.Fatal("model catalog unavailable")
		}
		click(next)
		if a.model != "gpt-6-luna" || a.profile.menu.Value() {
			t.Fatal("model selection did not apply and close")
		}
		a.detail.Active = true
		a.profile.menu.SetValue(true)
		frame()
		frame()
		option, ok := alignmentNode(router.AppendSemantics(nil), "GPT-6-sol", false)
		if !ok || !option.Disabled {
			t.Fatal("active run allows model changes")
		}
		click(option.Bounds)
		if a.model != "gpt-6-luna" {
			t.Fatal("active model changed")
		}
	}
}

func TestDisabledAgentRemainsVisibleAndCannotSelect(t *testing.T) {
	a := alignmentFixture(t)
	a.runtimes[1].Enabled = false // Installed, but switched off by the user.
	a.buildExecutorMenu()
	a.ui.Executor.SetValue(true)
	router, _, click := alignmentRouter(el.Root(a), 860, 600, 1)
	for _, label := range []string{"Claude Code", "DeepSeek Harness"} {
		row, ok := alignmentButton(router.AppendSemantics(nil), label)
		if !ok || !row.Disabled {
			t.Fatalf("%s not disabled", label)
		}
		click(row.Bounds)
		if a.runtime.Value() != "codex" || !a.ui.Executor.Value() {
			t.Fatalf("%s changed target", label)
		}
	}
}

func TestRenderComposerModelAndContext(t *testing.T) {
	dir := os.Getenv("ONECATCH_KEEL_PREVIEWS")
	if dir == "" {
		t.Skip("set ONECATCH_KEEL_PREVIEWS")
	}
	defer applyPalette(false)
	locale.Apply(locale.Chinese())
	for _, dark := range []bool{false, true} {
		applyPalette(dark)
		a := conversationFixture(t)
		a.model = "gpt-6-sol"
		a.runtime.SetValue("codex")
		a.task.Harness = "codex"
		setting := a.settings.Runtimes["codex"]
		setting.ReasoningEffort = "high"
		a.settings.Runtimes["codex"] = setting
		a.detail.StepRuns = []workflows.StepRun{{ContextWindow: 258400, ContextTokens: 180000}}
		path := filepath.Join(dir, fmt.Sprintf("composer-model-context-dark-%v.png", dark))
		if err := screenshotWithFocus(el.Root(el.ViewFunc(func(cx *el.Context) el.Element {
			return el.Div().WFull().HFull().Bg(theme.Bg).P(20).Child(el.Div().Grow(), a.composer(cx))
		})), 960, 400, path); err != nil {
			t.Fatal(err)
		}
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func TestProfileSelectionCompatibilityAndPayload(t *testing.T) {
	a := alignmentFixture(t)
	a.runtime.SetValue("codex")
	a.profile.key = a.runtimeProfileKey()
	a.profile.configuration = agentrun.HarnessConfiguration{Model: "large", Models: []agentrun.HarnessModel{
		{Model: "large", DefaultEffort: "high", Efforts: []string{"low", "medium", "high", "xhigh", "max"}},
		{Model: "small", DefaultEffort: "medium", Efforts: []string{"medium"}},
	}}
	a.profile.codex = agentrun.CodexConfiguration{ReasoningEffort: "high", ServiceTier: "standard", Models: []agentrun.CodexModelInfo{{Model: "large", ServiceTiers: []agentrun.CodexServiceTier{{ID: "priority"}}}}}
	a.reasoningEffort, a.serviceTier = "high", "priority"
	a.setProfileModel("large")
	input := a.createTaskInput("hello")
	if input.Model != "large" || input.ReasoningEffort != "high" || input.ServiceTier != "priority" {
		t.Fatalf("lost profile: %+v", input)
	}
	a.setProfileModel("small")
	if a.reasoningEffort != "medium" || a.serviceTier != "standard" {
		t.Fatalf("unsupported overrides: %s %s", a.reasoningEffort, a.serviceTier)
	}
	a.resetProfileSelection()
	if a.model != "" || a.reasoningEffort != "" || a.serviceTier != "" {
		t.Fatal("reset retained overrides")
	}
	if a.inheritedProfileEffort() != "high" || a.inheritedProfileModel() != "large" {
		t.Fatal("lost inherited configuration")
	}
}

func TestProfileEffortsExcludeUnsupportedSavedPreference(t *testing.T) {
	a := alignmentFixture(t)
	a.runtime.SetValue("codex")
	a.profile.key = a.runtimeProfileKey()
	want := []string{"low", "medium", "high", "xhigh", "max"}
	a.profile.configuration = agentrun.HarnessConfiguration{Model: "test-model", Models: []agentrun.HarnessModel{{Model: "test-model", Efforts: []string{"low", "medium", "high", "xhigh", "max", "ultra"}}}}
	a.profile.codex.ReasoningEffort = "ultra"
	a.reasoningEffort = "minimal"
	for _, includeCurrent := range []bool{false, true} {
		got := a.profileEfforts("", includeCurrent)
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Fatalf("saved preference changed supported stops: %v", got)
		}
	}
}

func TestDirectRuntimeProfileModelList(t *testing.T) {
	applyPalette(false)
	a := alignmentFixture(t)
	a.runtime.SetValue("codex")
	a.profile.key = a.runtimeProfileKey()
	a.profile.configuration = agentrun.HarnessConfiguration{Model: "gpt-6-astra", Models: []agentrun.HarnessModel{
		{Model: "gpt-6-astra", DisplayName: "GPT-6-Astra", Description: "Frontier intelligence for the most demanding tasks.", DefaultEffort: "medium", Efforts: []string{"low", "medium", "high", "xhigh", "max"}},
		{Model: "gpt-6-sol", DisplayName: "GPT-6-Sol", Description: "Previous generation workhorse model."},
	}}
	a.profile.codex = agentrun.CodexConfiguration{ReasoningEffort: "medium", Models: []agentrun.CodexModelInfo{{Model: "gpt-6-astra", ServiceTiers: []agentrun.CodexServiceTier{{ID: "priority"}}}}}
	root := el.Root(a)
	router, frame, click := alignmentRouter(root, 1120, 800, 1)
	choose := func(label string) {
		t.Helper()
		bounds, ok := findSkillControl(router.AppendSemantics(nil), label)
		if !ok {
			t.Fatalf("missing %s", label)
		}
		click(bounds)
	}
	choose("模型设置：6 Astra 中")
	if dir := os.Getenv("ONECATCH_KEEL_PREVIEWS"); dir != "" {
		widget := core.Func(func(gtx layout.Context) layout.Dimensions { gtx.Source = router.Source(); return root.Layout(gtx) })
		if err := window.ScreenshotAtScale(screenshotRoot{widget}, 1120, 800, 2, filepath.Join(dir, "reasoning-slider.png")); err != nil {
			t.Fatal(err)
		}
		a.dark = true
		applyPalette(true)
		frame()
		frame()
		if err := window.ScreenshotAtScale(screenshotRoot{widget}, 1120, 800, 2, filepath.Join(dir, "reasoning-slider-dark.png")); err != nil {
			t.Fatal(err)
		}
		a.dark = false
		applyPalette(false)
	}
	frame()
	frame()
	choose("选择模型")
	if _, ok := findSkillControl(router.AppendSemantics(nil), "默认"); !ok {
		t.Fatal("missing follow config")
	}
	if dir := os.Getenv("ONECATCH_KEEL_PREVIEWS"); dir != "" {
		widget := core.Func(func(gtx layout.Context) layout.Dimensions { gtx.Source = router.Source(); return root.Layout(gtx) })
		if err := window.ScreenshotAtScale(screenshotRoot{widget}, 1120, 800, 2, filepath.Join(dir, "runtime-profile-model-list.png")); err != nil {
			t.Fatal(err)
		}
	}
	frame()
	frame()
	choose("默认")
	frame()
	a.profile.popover.SetValue(true)
	frame()
	frame()
	slider, ok := findSkillControl(router.AppendSemantics(nil), "推理强度")
	if !ok {
		t.Fatal("missing reasoning slider")
	}
	click(slider)
	router.Queue(key.Event{Name: key.NameHome, State: key.Press}, key.Event{Name: key.NameHome, State: key.Release}, key.Event{Name: key.NameRightArrow, State: key.Press}, key.Event{Name: key.NameRightArrow, State: key.Release}, key.Event{Name: key.NameRightArrow, State: key.Press}, key.Event{Name: key.NameRightArrow, State: key.Release})
	frame()
	frame()
	if a.reasoningEffort != "high" {
		t.Fatal("effort not selected")
	}
	frame()
	choose("快速模式")
	if a.serviceTier != "priority" {
		t.Fatal("tier not selected")
	}
	for _, label := range []string{"1.5 倍速度", "用量更多"} {
		if _, ok := alignmentNode(router.AppendSemantics(nil), label, false); !ok {
			t.Fatalf("missing speed tooltip line: %s", label)
		}
	}
	input := a.createTaskInput("hello")
	if input.ReasoningEffort != "high" || input.ServiceTier != "priority" {
		t.Fatal("profile not passed to task")
	}
	frame()
	choose("快速模式")
	if firstProfileValue(a.serviceTier, a.inheritedProfileTier()) != "standard" {
		t.Fatal("fast toggle did not return to standard")
	}
	choose("恢复默认模型设置")
	if a.model != "" || a.reasoningEffort != "" || a.serviceTier != "" {
		t.Fatal("reset did not restore inherited settings")
	}
}
