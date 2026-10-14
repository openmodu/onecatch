package keeldesktop

import (
	"bytes"
	"errors"
	"image"
	"image/png"
	"testing"

	"github.com/dyike/keel/gio/io/key"
	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/el"
)

func TestPastedImagesRestoreComposerFocusAndCaretEveryTime(t *testing.T) {
	a := conversationFixture(t)
	a.prompt = "看这张图😀"
	other := ""
	var edit el.InputEdit
	var focused bool
	root := el.Root(el.ViewFunc(func(cx *el.Context) el.Element {
		box := el.Div().WFull().HFull().P(20).Gap(12).Child(a.composer(cx), el.Input().ID("other").Name("其他输入").Bind(&other))
		focused = cx.FocusWithin("prompt")
		edit, _ = cx.InputSelection("prompt")
		return box
	}))
	router, frame, click := alignmentRouter(root, 800, 600, 1)
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image.NewNRGBA(image.Rect(0, 0, 4, 4))); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"", " 附加说明"} {
		field, ok := alignmentNode(router.AppendSemantics(nil), "其他输入", false)
		if !ok {
			t.Fatal("missing alternate focus target")
		}
		click(field.Bounds)
		frame()
		if focused {
			t.Fatal("test did not move focus away from composer")
		}
		data := core.ClipboardData{Text: text, Images: []core.ClipboardImage{{MIME: "image/png", Data: encoded.Bytes()}}}
		existing := append([]selectedAttachment(nil), a.attachments.Items...)
		items, err := stageClipboardAttachments(a.ctx, a.backend.service, data, existing)
		if err != nil {
			t.Fatal(err)
		}
		a.attachments.Version++
		a.attachments.Loading = true
		before := a.prompt + text
		// Deliver the asynchronous staging result on the UI thread, as the
		// production core.Update callback does, then exercise real editor input.
		a.applyPastedAttachments(a.selection, a.attachments.Version, data, items, existing, nil)
		for range 5 {
			frame()
		}
		end := len([]rune(before))
		if !focused || edit.Text != before || edit.Start != end || edit.End != end || a.attachments.FocusPrompt {
			t.Fatalf("paste did not restore focus/caret: focused=%v edit=%+v pending=%v", focused, edit, a.attachments.FocusPrompt)
		}
		router.Queue(key.EditEvent{Range: key.Range{Start: end, End: end}, Text: "继续"})
		frame()
		frame()
		if a.prompt != before+"继续" || other != "" {
			t.Fatalf("typing after paste went to wrong input: prompt=%q other=%q", a.prompt, other)
		}
	}
	if len(a.attachments.Items) != 2 {
		t.Fatal("repeated paste lost images")
	}
}

func TestRejectedPasteDoesNotStealFocusOrApplyToAnotherTask(t *testing.T) {
	a := conversationFixture(t)
	a.prompt = "保留草稿"
	a.attachments.Version = 3
	a.applyPastedAttachments(a.selection, 3, core.ClipboardData{Text: "不应插入"}, nil, nil, errors.New("invalid image"))
	if a.attachments.FocusPrompt || a.prompt != "保留草稿" {
		t.Fatal("failed paste changed focus or draft")
	}
	a.applyPastedAttachments(a.selection-1, 3, core.ClipboardData{Text: "旧任务"}, nil, nil, nil)
	if a.attachments.FocusPrompt || a.prompt != "保留草稿" {
		t.Fatal("stale task paste changed current focus or draft")
	}
	// First paint must also honor a focus request made before editor creation.
	a.attachments.FocusPrompt = true
	var focused bool
	var edit el.InputEdit
	root := el.Root(el.ViewFunc(func(cx *el.Context) el.Element {
		box := a.composer(cx)
		focused = cx.FocusWithin("prompt")
		edit, _ = cx.InputSelection("prompt")
		return box
	}))
	_, frame, _ := alignmentRouter(root, 800, 400, 1)
	for range 5 {
		frame()
	}
	if !focused || edit.Start != len([]rune(a.prompt)) || a.attachments.FocusPrompt {
		t.Fatal("initial editor paint lost pending paste focus")
	}
}

func TestPastedImageKeepsChineseComposition(t *testing.T) {
	a := conversationFixture(t)
	a.prompt = "说明"
	a.attachments.FocusPrompt = true
	root := el.Root(el.ViewFunc(func(cx *el.Context) el.Element {
		return el.Div().WFull().HFull().P(20).Child(a.composer(cx))
	}))
	router, frame, _ := alignmentRouter(root, 800, 600, 1)
	for range 5 {
		frame()
	}
	a.applyPastedAttachments(a.selection, a.attachments.Version, core.ClipboardData{}, []selectedAttachment{{Name: "image.png", Image: image.NewNRGBA(image.Rect(0, 0, 4, 4))}}, nil, nil)
	for _, word := range []string{"ni", "nih", "nihao"} {
		end := len([]rune(a.prompt))
		router.Queue(key.CompositionEvent{Start: 2, End: 2 + len(word)}, key.EditEvent{Range: key.Range{Start: 2, End: end}, Text: word})
		frame()
		// Inspect the very first frame: later frames can hide a caret reset,
		// but macOS already cancels composition on that transient mismatch.
		state := router.EditorState()
		if state.Selection.Start != 2+len(word) || state.Selection.End != 2+len(word) {
			t.Fatalf("caret moved during composition: %+v", state.Selection)
		}
		if state.Selection.CompositionBounds.Empty() {
			t.Fatalf("composition lost for %q: prompt=%q pending=%v state=%+v", word, a.prompt, a.attachments.FocusPrompt, state)
		}
	}
	router.Queue(key.EditEvent{Range: key.Range{Start: 2, End: 7}, Text: "你好"}, key.CompositionEvent{Start: -1, End: -1})
	for range 3 {
		frame()
	}
	if a.prompt != "说明你好" {
		t.Fatalf("committed text = %q", a.prompt)
	}
}

func TestAttachmentRowsPreserveActiveComposition(t *testing.T) {
	a := conversationFixture(t)
	a.prompt = ""
	a.attachments.FocusPrompt = true
	root := el.Root(el.ViewFunc(func(cx *el.Context) el.Element {
		return el.Div().WFull().HFull().P(20).Child(a.composer(cx))
	}))
	router, frame, _ := alignmentRouter(root, 800, 600, 2)
	for range 5 {
		frame()
	}
	router.Queue(key.CompositionEvent{Start: 0, End: 2}, key.EditEvent{Range: key.Range{}, Text: "ni"})
	frame()
	check := func() {
		t.Helper()
		frame()
		state := router.EditorState()
		if state.Selection.CompositionBounds.Empty() || state.Selection.Start != 2 || state.Selection.End != 2 || a.prompt != "ni" {
			t.Fatalf("attachment update interrupted composition: prompt=%q state=%+v", a.prompt, state)
		}
	}
	check()
	a.attachments.Loading = true
	check()
	a.applyPastedAttachments(a.selection, a.attachments.Version, core.ClipboardData{}, []selectedAttachment{{Name: "image.png", Image: image.NewNRGBA(image.Rect(0, 0, 4, 4))}}, nil, nil)
	check()
	a.attachments.Items = nil
	check()
	router.Queue(key.EditEvent{Range: key.Range{Start: 0, End: 2}, Text: "你"}, key.CompositionEvent{Start: -1, End: -1})
	frame()
	frame()
	if a.prompt != "你" {
		t.Fatalf("commit = %q", a.prompt)
	}
}
