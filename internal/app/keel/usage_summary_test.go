package keeldesktop

import (
	"testing"
	"time"

	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/kit"
	domainworkflows "github.com/openmodu/onecatch/internal/domain/workflows"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
)

func TestUsageAllCombinesDailyPeaksAndCrossProviderStreaks(t *testing.T) {
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.FixedZone("Asia/Shanghai", 8*3600))
	usages := []agentrun.AccountUsage{
		{Runtime: "codex", DailyUsage: []agentrun.AccountDailyUsage{{StartDate: "2026-10-06", Tokens: 3}, {StartDate: "2026-10-08", Tokens: 10}, {StartDate: "2026-10-09", Tokens: 1}, {StartDate: "invalid", Tokens: 999}}},
		{Runtime: "claude", DailyUsage: []agentrun.AccountDailyUsage{{StartDate: "2026-10-07", Tokens: 4}, {StartDate: "2026-10-08", Tokens: 12}, {StartDate: "2026-10-09", Tokens: -999}}},
	}
	all := summarizeUsage(usages, "", now)
	if all.Peak != 22 || all.Current != 4 || all.Longest != 4 || all.Lifetime != 30 || len(all.Daily) != 4 {
		t.Fatalf("incorrect aggregate: %+v", all)
	}
	if old := summarizeUsage(usages, "", now.AddDate(0, 0, 2)); old.Current != 0 {
		t.Fatalf("stale streak: %+v", old)
	}
	peak := int64(50)
	usages[0].Summary.PeakDailyTokens = &peak
	if single := summarizeUsage(usages, "codex", now); single.Peak != 50 || single.Current != 2 {
		t.Fatalf("single provider: %+v", single)
	}
	if empty := summarizeUsage(nil, "", now); empty.Available != [4]bool{} {
		t.Fatal("missing data rendered as known zero")
	}
}

func TestWorkflowStarterTemplatesRemainValid(t *testing.T) {
	for _, kind := range []string{"single", "loop", "dag"} {
		d := starterWorkflow(kind)
		if err := domainworkflows.Validate(d); err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
	}
	a := &app{workflowEditor: kit.CodeEditor("{}"), definitions: []domainworkflows.Definition{starterWorkflow("loop"), {ID: "my_implement_review_2"}}}
	a.newWorkflow()
	if a.ui.WorkflowDraft.ID != "my_implement_review_3" {
		t.Fatalf("duplicate draft ID: %s", a.ui.WorkflowDraft.ID)
	}
}

func TestUsageChartSurvivesLayoutAndRefresh(t *testing.T) {
	for _, width := range []int{480, 1200} {
		a := conversationFixture(t)
		a.library.Usage = []agentrun.AccountUsage{{Runtime: "codex", DailyUsage: []agentrun.AccountDailyUsage{{StartDate: time.Now().Format("2006-01-02"), Tokens: 123}}, RateLimits: []agentrun.AccountRateLimit{{Primary: &agentrun.AccountRateLimitWindow{UsedPercent: 95, WindowDurationMins: 300}}}}}
		root := el.Root(el.ViewFunc(func(cx *el.Context) el.Element { return el.Div().WFull().HFull().Child(a.usagePage(cx)) }))
		_, frame, _ := alignmentRouter(root, width, 700, 1)
		frame()
		chart := a.library.UsageChart
		if chart == nil {
			t.Fatal("missing activity chart")
		}
		frame()
		if chart != a.library.UsageChart {
			t.Fatal("chart recreated without new data")
		}
		a.library.UsageVersion++
		frame()
		if chart == a.library.UsageChart {
			t.Fatal("chart did not update on refresh")
		}
	}
}
