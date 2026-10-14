package keeldesktop

import (
	"image"
	"image/color"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/theme"
	"github.com/dyike/keel/ui/window"
	domainworkflows "github.com/openmodu/onecatch/internal/domain/workflows"
)

func TestInstructionRecipientSurvivesHistoryReplay(t *testing.T) {
	a := conversationFixture(t)
	a.detail.Workflow.Steps = []domainworkflows.Step{{ID: "implement", Name: "方案实现"}}
	for _, status := range []domainworkflows.InstructionStatus{domainworkflows.InstructionPending, domainworkflows.InstructionApplied} {
		a.detail.Instructions = []domainworkflows.Instruction{{ID: "targeted", StepID: "implement", Content: "补上测试", Status: status, CreatedAt: time.Now()}}
		transcript := newTranscript()
		transcript.detail(*a.detail)
		message := transcript.items["instruction/targeted"]
		if status == domainworkflows.InstructionPending {
			if message != nil {
				t.Fatal("pending targeted message appeared in the conversation")
			}
			continue
		}
		if message == nil || message.recipient != "方案实现" || message.text != "补上测试" {
			t.Fatalf("history lost recipient or altered instruction: %+v", message)
		}
	}
	a.detail.Instructions[0].StepID = "removed_step"
	transcript := newTranscript()
	transcript.detail(*a.detail)
	if transcript.items["instruction/targeted"].recipient != "removed_step" {
		t.Fatal("unknown recipient did not retain stable step ID")
	}
}

func TestRenderComposerTokensAndSentRecipient(t *testing.T) {
	dir := os.Getenv("ONECATCH_KEEL_PREVIEWS")
	if dir == "" {
		t.Skip("set ONECATCH_KEEL_PREVIEWS")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	a := conversationFixture(t)
	tokenText := "@implement"
	prefix := "请 "
	a.prompt = prefix + tokenText + " 检查 Go 接口与数据库映射。\n后面继续输入文本。"
	content, err := el.NewInputContent(a.prompt, el.InputTokenSpan{Range: el.InputRange{Start: len(prefix), End: len(prefix) + len(tokenText)}, Token: el.InputToken{ID: "step/preview", Text: tokenText, Label: "@AKX Monorepo项目学习"}})
	if err != nil {
		t.Fatal(err)
	}
	a.promptDocument.SetContent(content)
	e := &entry{key: "sent-preview", user: true, recipient: "方案实现", text: "检查 Go 接口与数据库映射。", at: time.Now()}
	a.docs[e.key] = newConversationDocument(e.text)
	a.entries = []*entry{e}
	for _, dark := range []bool{false, true} {
		applyPalette(dark)
		root := el.Root(el.ViewFunc(func(cx *el.Context) el.Element {
			return el.Div().WFull().HFull().P(24).Bg(theme.Surface).Gap(20).Child(a.messageEntry(cx, e, 0), el.Div().Grow(), a.composer(cx))
		}))
		name := "mention-light.png"
		if dark {
			name = "mention-dark.png"
		}
		if err := window.Screenshot(screenshotRoot{core.Func(root.Layout)}, 800, 500, filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
	}
	applyPalette(false)
}

func TestComposerTokenBackgroundCentersMixedScriptInk(t *testing.T) {
	defer applyPalette(false)
	for _, dark := range []bool{false, true} {
		applyPalette(dark)
		for _, scale := range []float32{1, 2} {
			var doc el.InputDocument
			content, _ := el.NewInputContent("ref", el.InputTokenSpan{Range: el.InputRange{End: 3}, Token: el.InputToken{ID: "ref", Text: "ref", Label: "@AKX Monorepo项目学习"}})
			doc.SetContent(content)
			p := theme.Current()
			p.Subtle = mixColor(theme.Info, theme.Subtle, .20)
			root := el.Root(el.ViewFunc(func(cx *el.Context) el.Element {
				return el.Div().WFull().HFull().P(20).Bg(theme.Surface).Child(cx.Themed(p, el.ViewFunc(func(cx *el.Context) el.Element {
					return el.TextArea().Document(&doc).TextSize(14).LineHeight(1.6).P(0).Bg(color.NRGBA{}).Border(0, color.NRGBA{}).AutoGrow(1, 1)
				})))
			}))
			router, _, _ := alignmentRouter(root, 800, 150, 1)
			pixels := interactionPixelsScaled(t, root, router, 800, 150, scale)
			bg := mixColor(theme.Info, theme.Subtle, .20)
			var pill, ink image.Rectangle
			for y := 0; y < pixels.Bounds().Dy(); y++ {
				for x := 0; x < pixels.Bounds().Dx(); x++ {
					p := color.NRGBAModel.Convert(pixels.At(x, y)).(color.NRGBA)
					if p == bg {
						pill = pill.Union(image.Rect(x, y, x+1, y+1))
					}
				}
			}
			for y := pill.Min.Y; y < pill.Max.Y; y++ {
				for x := pill.Min.X + int(5*scale); x < pill.Max.X-int(5*scale); x++ {
					p := color.NRGBAModel.Convert(pixels.At(x, y)).(color.NRGBA)
					if abs(int(p.R)-int(theme.Text.R))+abs(int(p.G)-int(theme.Text.G))+abs(int(p.B)-int(theme.Text.B)) < 90 {
						ink = ink.Union(image.Rect(x, y, x+1, y+1))
					}
				}
			}
			top, bottom := ink.Min.Y-pill.Min.Y, pill.Max.Y-ink.Max.Y
			if pill.Empty() || ink.Empty() || top < 1 || bottom < 1 || abs(top-bottom) > int(scale)+1 {
				t.Fatalf("dark=%v scale=%v: asymmetric token ink padding top=%d bottom=%d pill=%v ink=%v", dark, scale, top, bottom, pill, ink)
			}
		}
	}
}
