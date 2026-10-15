package keeldesktop

import (
	"encoding/json"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/theme"
	"github.com/dyike/keel/ui/window"
	domainworkflows "github.com/openmodu/onecatch/internal/domain/workflows"
)

const longWorkflowSummary = "已提交并推送：新建提交 7806402 'feat(sandbox): add CreateTool prepare protocol and failure receipts'（35 文件，+3868/-691；新增 R6～R8 的 resolver 白名单与最终规格校验、统一失败回执降级、pkg/intent HMAC 签名、冻结取消决策、4 个 VO 与 5 个测试含 588 行永久回归，修改 pkg/rider/activity、sandbox_rider 适配层与两份文档），快进推送 db1059e..7806402 到 origin/feat/sandbox-tool；本地与远端均为 780640254f428fd9c9d887444d77969fa99d2b02，领先/落后 0/0，工作树干净无未跟踪残留。提交前通过 gofmt、git diff --check、空白与无损扫描，无 DO NOT EDIT 文件，依赖方向正确，HMAC 用法合规无硬编码密钥，pkg/intent 测试通过；因本机无网络凭据且无 jiang/hevi 缓存，usecase/其他包在此环境 setup failed 无法验证，test/vet/race/构建结论以 Review 在可用缓存环境的结果为准。未创建 MR、未合并 master、未部署、未跑全仓测试与生成。下一步：由方案设计继续审阅真实权限/配额、受控事务、未知响应查回及 G1～G4 恢复验收；Workflow 继续禁用、RPC 未实现、CreateTool 未完成、完整 API 仍 0/47。"

func workflowSummaryFixture(t *testing.T, summary string) (*app, *entry) {
	t.Helper()
	a, e := outcomeFixture(t)
	source, err := json.Marshal(domainworkflows.Outcome{Verdict: domainworkflows.VerdictPass, Summary: summary})
	if err != nil {
		t.Fatal(err)
	}
	e.text = string(source)
	a.transcript = newTranscript()
	a.transcript.items[e.key] = e
	a.syncMessages()
	return a, a.entries[0]
}

func TestWorkflowOutcomeSummaryWrapsInsideCard(t *testing.T) {
	for _, summary := range []string{longWorkflowSummary, strings.Repeat("摘要右侧的中文文字必须完整显示，并在卡片内自动换行。", 20)} {
		for _, width := range []int{440, 800, 1200} {
			for _, scale := range []int{1, 2} {
				t.Run(fmt.Sprintf("mixed=%v/width=%d/scale=%d", summary == longWorkflowSummary, width, scale), func(t *testing.T) {
					a, e := workflowSummaryFixture(t, summary)
					root := el.Root(el.ViewFunc(func(cx *el.Context) el.Element {
						return el.Div().WFull().HFull().P(20).Child(a.messageEntry(cx, e, 0))
					}))
					router, _, _ := alignmentRouter(root, width, 4000, scale)
					actual, ok := alignmentNode(router.AppendSemantics(nil), summary, false)
					if !ok {
						t.Fatal("missing summary")
					}
					// The message has 14dp horizontal padding and the result card
					// has another 16dp. Compare to an independently constrained
					// document: a clipped semantic label alone cannot prove wrapping.
					contentWidth := min(width-40, 920)
					bodyWidth := contentWidth - 2*(14+16)
					left := (width-contentWidth)/2 + 14 + 16
					bodyBox := image.Rect(left*scale, 0, (left+bodyWidth)*scale, 4000*scale)
					if actual.Bounds.Empty() || !actual.Bounds.In(bodyBox) {
						t.Fatalf("summary outside card content: summary=%v content=%v", actual.Bounds, bodyBox)
					}
					reference := el.Root(el.ViewFunc(func(cx *el.Context) el.Element {
						return el.Div().WFull().HFull().Child(a.conversationDocument(cx, &entry{key: "summary-reference", text: summary}, false))
					}))
					refRouter, _, _ := alignmentRouter(reference, bodyWidth, 4000, scale)
					want, ok := alignmentNode(refRouter.AppendSemantics(nil), summary, false)
					if !ok || actual.Bounds.Size() != want.Bounds.Size() {
						t.Fatalf("summary uses wider layout than its card: actual=%v correctly wrapped=%v", actual.Bounds, want.Bounds)
					}
				})
			}
		}
	}
}

func TestRenderLongWorkflowOutcomeSummary(t *testing.T) {
	dir := os.Getenv("ONECATCH_KEEL_PREVIEWS")
	if dir == "" {
		t.Skip("set ONECATCH_KEEL_PREVIEWS")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	a, e := workflowSummaryFixture(t, longWorkflowSummary)
	root := el.Root(el.ViewFunc(func(cx *el.Context) el.Element {
		return el.Div().WFull().HFull().P(20).Bg(theme.Surface).Child(a.messageEntry(cx, e, 0))
	}))
	for _, width := range []int{440, 800, 1200} {
		path := filepath.Join(dir, fmt.Sprintf("workflow-long-summary-%d.png", width))
		if err := window.Screenshot(screenshotRoot{core.Func(root.Layout)}, width, 1400, path); err != nil {
			t.Fatal(err)
		}
	}
}
