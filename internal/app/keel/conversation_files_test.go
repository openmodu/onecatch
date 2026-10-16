package keeldesktop

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dyike/keel/gio/op"
	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/locale"
	"github.com/dyike/keel/ui/theme"
	"github.com/dyike/keel/ui/window"
	desktop "github.com/openmodu/onecatch/internal/service/desktop"
	"github.com/openmodu/onecatch/internal/service/desktop/runstream"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
)

func previewFileChange() agentrun.FileChange {
	return agentrun.NewFileChange("/repo/agentkit/akx/go.mod", "update", "", "@@ -12,5 +12,6 @@\n require (\n-\tgithub.com/example/rider v0.3.25\n+\tgithub.com/example/rider v0.3.26\n+\tgolang.org/x/sync v0.16.0\n )\n")
}

func TestFileChangeSeparateFromToolsAndRestored(t *testing.T) {
	at := time.Now()
	change := previewFileChange()
	tx := newTranscript()
	tx.frame(runstream.Frame{StepRunID: "step", Seq: 2, Event: agentrun.Event{Kind: agentrun.KindFileChange, Text: change.Path, FileChanges: []agentrun.FileChange{change}, Revision: 2, At: at}})
	tx.history([]desktop.RuntimeEventView{{StepRunID: "step", Seq: 2, Kind: "file_change", Text: change.Path, Revision: 1}})
	rows := buildConversationEntries(append([]*entry{{key: "before", step: "step", kind: agentrun.KindToolUse}}, append(tx.visible(), &entry{key: "after", step: "step", kind: agentrun.KindToolUse})...))
	if len(rows) != 3 || len(rows[0].process) != 1 || len(rows[1].process) != 0 || rows[1].changes[0] != change || len(rows[2].process) != 1 {
		t.Fatalf("file change hidden in tools or overwritten by older history: %+v", rows)
	}
	restored := newTranscript()
	restored.history([]desktop.RuntimeEventView{{StepRunID: "step", Seq: 2, Kind: "file_change", Text: change.Path, FileChanges: []agentrun.FileChange{change}, Revision: 2}})
	if got := restored.visible(); len(got) != 1 || got[0].changes[0] != change {
		t.Fatalf("history lost diff: %+v", got)
	}
}

func TestFileDiffCountsIgnoreHeaders(t *testing.T) {
	v := newFileChangeView(agentrun.NewFileChange("a.go", "update", "", "--- a/a.go\n+++ b/a.go\n@@ -10,2 +20,2 @@\n---old\n+++new\n context\n@@ @@\n-old\n+new"))
	if v.added != 2 || v.removed != 2 {
		t.Fatalf("file headers counted as edits or hunk content skipped: %+v", v)
	}
	want := []fileDiffLine{
		{mark: '@', text: "@@ -10,2 +20,2 @@"},
		{old: 10, mark: '-', text: "--old"},
		{new: 20, mark: '+', text: "++new"},
		{old: 11, new: 21, text: "context"},
		{mark: '@', text: "@@ @@"},
		{mark: '-', text: "old"},
		{mark: '+', text: "new"},
	}
	if fmt.Sprint(v.lines) != fmt.Sprint(want) {
		t.Fatalf("source line columns do not follow hunk coordinates:\n got: %+v\nwant: %+v", v.lines, want)
	}
}

func TestFileChangeCardPreviewExpandCopy(t *testing.T) {
	defer applyPalette(false)
	applyPalette(false)
	for _, scale := range []int{1, 2} {
		t.Run(fmt.Sprint(scale), func(t *testing.T) {
			a := conversationFixture(t)
			change := previewFileChange()
			change.Diff += "@@ -50,1 +51,35 @@\n-old\n" + strings.Repeat("+更多改动\n", 35)
			e := &entry{key: "edit", kind: agentrun.KindFileChange, changes: []agentrun.FileChange{change}}
			root := el.Root(el.ViewFunc(func(cx *el.Context) el.Element {
				return el.Div().WFull().HFull().P(20).Gap(10).Bg(theme.Surface).Child(a.fileChangeCards(cx, e), el.Text("后续回复"))
			}))
			router, frame, click := alignmentRouter(root, 620, 600, scale)
			preview, ok := alignmentNode(router.AppendSemantics(nil), "文件差异："+change.Path, false)
			if !ok || preview.Bounds.Dy() > fileChangePreviewLines*fileChangeLineHeight*scale {
				t.Fatalf("collapsed edit is not a compact diff preview: %v", preview.Bounds)
			}
			v := a.conversation.fileChanges["edit/file/0"]
			if len(v.lines) <= fileChangePreviewLines || v.change.Diff != change.Diff {
				t.Fatal("preview must retain the full, original diff")
			}
			footer, ok := alignmentNode(router.AppendSemantics(nil), "后续回复", false)
			if !ok || footer.Bounds.Min.Y > 330*scale {
				t.Fatalf("partial preview is not compact: %v", footer.Bounds)
			}
			if _, ok := alignmentNode(router.AppendSemantics(nil), "+37", false); !ok {
				t.Fatal("partial preview is missing added count")
			}
			for _, label := range []string{"go.mod", "编辑文件", "/repo/agentkit/akx"} {
				if _, ok := alignmentNode(router.AppendSemantics(nil), label, false); !ok {
					t.Fatalf("target-style split file heading is missing %q", label)
				}
			}
			copy, ok := alignmentNode(router.AppendSemantics(nil), locale.Current().Copy, true)
			if !ok {
				t.Fatal("missing copy action")
			}
			click(copy.Bounds)
			_, data, copied := router.WriteClipboard()
			if !copied || string(data) != change.Diff {
				t.Fatalf("copy lost diff: %q", data)
			}
			expand, ok := alignmentNode(router.AppendSemantics(nil), "展开完整差异", true)
			if !ok {
				t.Fatal("missing expand action")
			}
			expandText, ok := alignmentNode(router.AppendSemantics(nil), "展开完整差异", false)
			if !ok || expandText.Bounds.Min.X-preview.Bounds.Min.X > 20*scale {
				t.Fatalf("expand label is not aligned to the reference card's left edge: preview=%v label=%v", preview.Bounds, expandText.Bounds)
			}
			click(expand.Bounds)
			frame()
			field, ok := alignmentNode(router.AppendSemantics(nil), "文件差异："+change.Path, false)
			if !ok || field.Bounds.Dy() > fileChangeMaxHeight*scale || field.Bounds.Dy() <= preview.Bounds.Dy() {
				t.Fatalf("expanded diff viewport missing or unbounded: %v", field.Bounds)
			}
			if field.Bounds.Min != preview.Bounds.Min || field.Bounds.Dx() != preview.Bounds.Dx() {
				t.Fatalf("expansion changed code origin/width: preview=%v expanded=%v", preview.Bounds, field.Bounds)
			}
			frame()
			if a.conversation.fileChanges["edit/file/0"] != v {
				t.Fatal("unchanged card reparsed its diff")
			}
			collapse, ok := alignmentNode(router.AppendSemantics(nil), "收起差异", true)
			if !ok {
				t.Fatal("missing collapse action")
			}
			click(collapse.Bounds)
			frame()
			collapsed, ok := alignmentNode(router.AppendSemantics(nil), "文件差异："+change.Path, false)
			if !ok || collapsed.Bounds != preview.Bounds {
				t.Fatal("collapse did not restore the partial preview")
			}
		})
	}
}

func TestFileChangeLegacyPaths(t *testing.T) {
	a := conversationFixture(t)
	e := &entry{key: "legacy", kind: agentrun.KindFileChange, text: "/repo/a.go\n/repo/b.go"}
	root := el.Root(el.ViewFunc(func(cx *el.Context) el.Element { return el.Div().WFull().HFull().P(20).Child(a.fileChangeCards(cx, e)) }))
	router, _, _ := alignmentRouter(root, 620, 400, 1)
	for _, name := range []string{"a.go", "b.go", "编辑文件", "/repo", "未保存代码差异"} {
		if _, ok := alignmentNode(router.AppendSemantics(nil), name, false); !ok {
			t.Fatalf("missing %s", name)
		}
	}
	if _, ok := alignmentNode(router.AppendSemantics(nil), "展开完整差异", true); ok {
		t.Fatal("legacy path fabricated a diff")
	}
}

// Compare actual glyphs and code surfaces, not just semantic bounds: expanding
// a partial preview must not swap its font, gutter or highlighting.
func TestFileChangeCodeStyleSurvivesCollapse(t *testing.T) {
	defer applyPalette(false)
	for _, dark := range []bool{false, true} {
		for _, scale := range []int{1, 2} {
			for _, width := range []int{360, 840} {
				t.Run(fmt.Sprintf("dark=%v/scale=%d/width=%d", dark, scale, width), func(t *testing.T) {
					applyPalette(dark)
					a := conversationFixture(t)
					change := previewFileChange()
					change.Diff += "@@ -50,1 +51,20 @@\n-old\n" + strings.Repeat("+更多代码改动\n", 20)
					e := &entry{key: "style", kind: agentrun.KindFileChange, changes: []agentrun.FileChange{change}}
					root := el.Root(el.ViewFunc(func(cx *el.Context) el.Element {
						return el.Div().WFull().HFull().P(20).Bg(theme.Bg).Child(a.fileChangeCards(cx, e))
					}))
					router, _, _ := alignmentRouter(root, width, 500, scale)
					widget := core.Func(func(gtx core.C) core.D {
						gtx.Source = router.Source()
						var ops op.Ops
						pre := gtx
						pre.Ops = &ops
						for range 4 {
							ops.Reset()
							root.Layout(pre)
							router.Frame(&ops)
						}
						dims := root.Layout(gtx)
						router.Frame(gtx.Ops)
						return dims
					})
					capture := func(state string) (image.Image, image.Rectangle) {
						t.Helper()
						dir := t.TempDir()
						if preview := os.Getenv("ONECATCH_KEEL_PREVIEWS"); preview != "" {
							dir = preview
							if err := os.MkdirAll(dir, 0755); err != nil {
								t.Fatal(err)
							}
						}
						path := filepath.Join(dir, fmt.Sprintf("file-change-code-dark-%v-%dx-%d-%s.png", dark, scale, width, state))
						if err := window.ScreenshotAtScale(screenshotRoot{widget}, width, 500, float32(scale), path); err != nil {
							t.Fatal(err)
						}
						file, err := os.Open(path)
						if err != nil {
							t.Fatal(err)
						}
						defer file.Close()
						pixels, err := png.Decode(file)
						if err != nil {
							t.Fatal(err)
						}
						field, ok := alignmentNode(router.AppendSemantics(nil), "文件差异："+change.Path, false)
						if !ok {
							t.Fatal("missing code viewport")
						}
						// The compact viewer keeps one code surface inside the original
						// quiet row; it does not add a second path/result card.
						if got := color.NRGBAModel.Convert(pixels.At(field.Bounds.Max.X-32*scale, field.Bounds.Min.Y+2*scale)); got != theme.CodeBg {
							t.Fatalf("%s surface = %v, want %v", state, got, theme.CodeBg)
						}
						// The removed and added code lines must already be colored
						// in the first frame, not just the counts in the header.
						added, removed, addedBG, removedBG := false, false, false, false
						wantAddedBG := mixColor(theme.Success, theme.CodeBg, .10)
						wantRemovedBG := mixColor(theme.Danger, theme.CodeBg, .09)
						for y := field.Bounds.Min.Y; y < field.Bounds.Max.Y; y++ {
							for x := field.Bounds.Min.X; x < field.Bounds.Max.X-14*scale; x++ {
								c := color.NRGBAModel.Convert(pixels.At(x, y))
								added = added || c == theme.Success
								removed = removed || c == theme.Danger
								addedBG = addedBG || c == wantAddedBG
								removedBG = removedBG || c == wantRemovedBG
							}
						}
						if !added || !removed || !addedBG || !removedBG {
							t.Fatalf("%s code lacks diff text/background colors: added=%v removed=%v addedBG=%v removedBG=%v", state, added, removed, addedBG, removedBG)
						}
						return pixels, field.Bounds
					}
					preview, before := capture("preview")
					a.toggleConversation("style/file/0", false)
					expanded, after := capture("expanded")
					if before.Min != after.Min || before.Dx() != after.Dx() || before.Dy() >= after.Dy() {
						t.Fatalf("expansion must only increase viewport height: %v -> %v", before, after)
					}
					// The first four lines include hunk metadata, context, removal
					// and addition, so this checks every diff style and the gutter.
					for y := before.Min.Y; y < before.Min.Y+76*scale; y++ {
						for x := before.Min.X; x < before.Max.X-14*scale; x++ {
							if preview.At(x, y) != expanded.At(x, y) {
								t.Fatalf("code style changed after expansion at %d,%d: %v -> %v", x, y, preview.At(x, y), expanded.At(x, y))
							}
						}
					}
				})
			}
		}
	}
}

func TestRenderFileChangeCards(t *testing.T) {
	dir := os.Getenv("ONECATCH_KEEL_PREVIEWS")
	if dir == "" {
		t.Skip("optional preview")
	}
	a := conversationFixture(t)
	changes := []agentrun.FileChange{previewFileChange(), agentrun.NewFileChange("/repo/agentkit/akx/pkg/rider/activity.go", "update", "", "@@ -42,5 +42,8 @@\n func (a *Activity) Run(ctx context.Context) error {\n-\treturn a.worker.Run(ctx)\n+\tif err := ctx.Err(); err != nil {\n+\t\treturn err\n+\t}\n+\treturn a.worker.Run(ctx)\n }")}
	root := el.Root(el.ViewFunc(func(cx *el.Context) el.Element {
		return el.Div().WFull().HFull().P(28).Gap(16).Bg(theme.Bg).Child(
			el.Text("已更新依赖，并补充任务取消检查。").TextSize(14),
			a.fileChangeCards(cx, &entry{key: "preview", kind: agentrun.KindFileChange, changes: changes}),
			el.Text("运行了 1 个工具  ›").TextSize(13).TextColor(theme.Muted))
	}))
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, width := range []int{840, 360} {
		if err := window.Screenshot(screenshotRoot{core.Func(root.Layout)}, width, 560, filepath.Join(dir, fmt.Sprintf("file-change-cards-%d.png", width))); err != nil {
			t.Fatal(err)
		}
	}
}
