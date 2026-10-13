package keeldesktop

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/theme"
	"github.com/dyike/keel/ui/window"
	domainworkflows "github.com/openmodu/onecatch/internal/domain/workflows"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
)

func outcomeFixture(t *testing.T) (*app, *entry) {
	a := conversationFixture(t)
	a.detail.Workflow = domainworkflows.Definition{ID: "migration", Steps: []domainworkflows.Step{{ID: "design", Name: "设计迁移方案"}, {ID: "review", Name: "Review 实现", Runtime: "trae", Model: "GPT-6-Astra"}}}
	a.detail.StepRuns = []domainworkflows.StepRun{{ID: "review-run", StepID: "review", Attempt: 7, Status: domainworkflows.StepRunSucceeded}}
	e := &entry{key: "outcome-example", step: "review-run", kind: agentrun.KindMessage, text: `{"verdict":"rework","target":"design","summary":"实现审阅已完成，Go 测试和文档检查通过。需要补全验收方案后再交实现整改。","feedback":"R1，确认当前只验收 CreateTool 本地骨架，已删除的 Provider 和迁移工具不再列为成果。 R2，将完整链测试放入 tests/sandbox_tool，Service 单测使用接口替身。 R3，补全参数边界、上下文取消和错误处理的回归要求。","extra":"retained"}`, at: time.Now()}
	a.docs[e.key] = newConversationDocument(e.text)
	return a, e
}

func TestWorkflowOutcomePresentationPreservesRawAndExpands(t *testing.T) {
	a, e := outcomeFixture(t)
	out, ok := a.workflowMessageOutcome(e)
	if !ok {
		t.Fatal("workflow outcome not recognized")
	}
	a.transcript = newTranscript()
	a.transcript.items[e.key] = e
	a.syncMessages()
	root := el.Root(el.ViewFunc(func(cx *el.Context) el.Element {
		return a.conversation.selection.Decorate(cx, el.Div().WFull().HFull().P(20).ScrollY().Child(a.workflowOutcomeCard(cx, e, out)))
	}))
	router, frame, click := alignmentRouter(root, 800, 900, 1)
	if _, ok := alignmentNode(router.AppendSemantics(nil), "需要返工", false); !ok {
		t.Fatal("missing outcome status")
	}
	for _, label := range []string{"Review 实现", "节点 02", "第 7 次执行 · trae · GPT-6-Astra"} {
		if _, ok := alignmentNode(router.AppendSemantics(nil), label, false); !ok {
			t.Fatalf("result card omitted owning node metadata %q", label)
		}
	}
	paragraph, ok := alignmentNode(router.AppendSemantics(nil), out.Summary, false)
	if !ok {
		t.Fatal("missing readable summary")
	}
	click(paragraph.Bounds)
	selectionKey(router, frame, "A")
	selectionKey(router, frame, "C")
	_, copied, wrote := router.WriteClipboard()
	if !wrote || !strings.Contains(string(copied), out.Summary) || strings.Contains(string(copied), `"verdict"`) {
		t.Fatalf("selection copied hidden JSON instead of displayed prose: %q", copied)
	}
	b, ok := findSkillControl(router.AppendSemantics(nil), "查看原始结果")
	if !ok {
		t.Fatal("missing raw result disclosure")
	}
	click(b)
	doc := a.conversation.toolDocs[e.key+"/raw/code"]
	if doc == nil || !strings.Contains(doc.Source(), `"extra": "retained"`) || a.docs[e.key].Source() != e.text {
		t.Fatal("raw fields or original copy source changed")
	}
	if readable := outcomeReadableText(out.Feedback); !strings.Contains(readable, "\n\n**R2**") || !strings.Contains(readable, "**R3**") {
		t.Fatal("requirements not separated")
	}
	e.user = true
	if _, ok := a.workflowMessageOutcome(e); ok {
		t.Fatal("user JSON reinterpreted as workflow outcome")
	}
}

func TestRenderWorkflowOutcomeCards(t *testing.T) {
	dir := os.Getenv("ONECATCH_KEEL_PREVIEWS")
	if dir == "" {
		t.Skip("set ONECATCH_KEEL_PREVIEWS")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	a, e := outcomeFixture(t)
	out, _ := a.workflowMessageOutcome(e)
	transition := &entry{key: "transition-example", at: e.at, transition: &workflowTransition{verdict: "rework", source: "Review 实现", target: "设计迁移方案", feedback: out.Feedback}}
	root := el.Root(el.ViewFunc(func(cx *el.Context) el.Element {
		return el.Div().WFull().HFull().P(24).Bg(theme.Surface).Gap(24).Child(
			el.Text("Review 实现").TextSize(15).Bold(), a.workflowOutcomeCard(cx, e, out),
			a.workflowTransitionCard(cx, transition),
			el.Text("设计迁移方案").TextSize(15).Bold(),
			a.workflowOutcomeCard(cx, &entry{key: "pass-example", text: `{"verdict":"pass","summary":"验收要求已逐项补全"}`}, domainworkflows.Outcome{Verdict: "pass", Summary: "验收要求已逐项补全。本轮只修改方案，文档检查通过，交给实现步骤继续。"}),
		)
	}))
	if err := window.Screenshot(screenshotRoot{core.Func(root.Layout)}, 860, 1000, filepath.Join(dir, "workflow-outcomes.png")); err != nil {
		t.Fatal(err)
	}
	if err := window.Screenshot(screenshotRoot{core.Func(root.Layout)}, 440, 1300, filepath.Join(dir, "workflow-outcomes-narrow.png")); err != nil {
		t.Fatal(err)
	}
	applyPalette(true)
	defer applyPalette(false)
	if err := window.Screenshot(screenshotRoot{core.Func(root.Layout)}, 860, 1000, filepath.Join(dir, "workflow-outcomes-dark.png")); err != nil {
		t.Fatal(err)
	}
}
