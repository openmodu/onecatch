package keeldesktop

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gioui.org/io/input"
	"gioui.org/io/key"
	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/theme"
	"github.com/dyike/keel/ui/window"
	tasks "github.com/openmodu/onecatch/internal/domain/tasks"
	workflows "github.com/openmodu/onecatch/internal/domain/workflows"
	desktop "github.com/openmodu/onecatch/internal/service/desktop"
)

func conversationImageFixture(t *testing.T) (*app, string, *el.RootWidget) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "attachment_test-screenshot.png")
	img := image.NewNRGBA(image.Rect(0, 0, 320, 180))
	for y := range 180 {
		for x := range 320 {
			img.SetNRGBA(x, y, color.NRGBA{R: 220, G: 40, B: 80, A: 255})
		}
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	err = png.Encode(file, img)
	file.Close()
	if err != nil {
		t.Fatal(err)
	}
	a := conversationFixture(t)
	a.transcript = newTranscript()
	a.detail = &desktop.RunDetail{
		Task:         tasks.Task{Prompt: "请看截图", Attachments: []tasks.Attachment{{Name: "screenshot.png", StoredPath: path, MIMEType: "image/png"}}},
		Run:          workflows.Run{ID: a.runID, Status: workflows.RunRunning},
		Instructions: []workflows.Instruction{{ID: "queued", Content: "稍后处理截图", Attachments: []string{path}, Status: workflows.InstructionPending}},
	}
	a.transcript.detail(*a.detail)
	a.syncMessages()
	root := el.Root(el.ViewFunc(func(cx *el.Context) el.Element {
		box := el.Div().WFull().HFull().Bg(theme.Surface).P(20).Gap(16)
		for i, e := range a.entries {
			box.Child(a.messageEntry(cx, e, i))
		}
		return box.Child(a.pendingInstructionList(cx))
	}))
	return a, path, root
}

func completeConversationImageLoads(t *testing.T, a *app, path string, frame func()) {
	t.Helper()
	frame()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	img, _, err := image.Decode(file)
	file.Close()
	if err != nil {
		t.Fatal(err)
	}
	// This layout harness has no application event loop to drain core.Update.
	// Deliver the fixture's decoded pixels through the native public setter;
	// ImageView's async Source loading is covered by the toolkit itself.
	for _, view := range a.conversation.images {
		if view.Loading() {
			view.SetImage(img)
		}
	}
	frame()
}

func TestConversationAttachmentImagesAndPendingMessages(t *testing.T) {
	a, path, root := conversationImageFixture(t)
	router, frame, click := alignmentRouter(root, 800, 700, 1)
	completeConversationImageLoads(t, a, path, frame)
	if len(a.entries) != 1 || len(a.entries[0].attachments) != 1 {
		t.Fatal("pending instruction appeared in the conversation or prompt lost its image")
	}
	if a.conversation.images["prompt/"+path] == nil || a.conversation.images["pending/queued/"+path] == nil {
		t.Fatal("prompt or queued image was not rendered")
	}
	thumb, ok := alignmentNode(router.AppendSemantics(nil), "screenshot.png", true)
	if !ok {
		t.Fatal("image preview is not clickable")
	}
	pixels := interactionPixels(t, root, router, 800, 700)
	c := color.NRGBAModel.Convert(pixels.At(thumb.Bounds.Min.X+thumb.Bounds.Dx()/2, thumb.Bounds.Min.Y+thumb.Bounds.Dy()/2)).(color.NRGBA)
	if c.R < 180 || c.G > 80 || c.B > 120 {
		t.Fatalf("attachment pixels are missing: %v", c)
	}
	click(thumb.Bounds)
	frame()
	var hasPreview func([]input.SemanticNode) bool
	hasPreview = func(nodes []input.SemanticNode) bool {
		for _, node := range nodes {
			if node.Desc.Description == "dialog" && node.Desc.Label == "screenshot.png" || hasPreview(node.Children) {
				return true
			}
		}
		return false
	}
	if !hasPreview(router.AppendSemantics(nil)) {
		t.Fatal("clicking the thumbnail did not open the image preview")
	}
	router.Queue(key.Event{Name: key.NameEscape, State: key.Press}, key.Event{Name: key.NameEscape, State: key.Release})
	frame()
	frame()
	if hasPreview(router.AppendSemantics(nil)) {
		t.Fatal("Escape did not close the image preview")
	}
	// Applying the queued message moves it to the conversation exactly once.
	a.detail.Instructions[0].Status = workflows.InstructionApplied
	a.detail.Instructions[0].AppliedAt = time.Now()
	a.transcript.detail(*a.detail)
	a.syncMessages()
	frame()
	completeConversationImageLoads(t, a, path, frame)
	if len(a.entries) != 2 || a.conversation.images["instruction/queued/"+path] == nil {
		t.Fatal("applied instruction or its image did not enter the conversation")
	}
	if _, ok := alignmentNode(router.AppendSemantics(nil), "删除后续消息", true); ok {
		t.Fatal("applied message remained in the queue")
	}
	a.transcript.detail(*a.detail)
	a.syncMessages()
	if len(a.entries) != 2 {
		t.Fatal("refresh duplicated an applied message")
	}
}

func TestConversationRestoresAttachmentOnlyMessages(t *testing.T) {
	a, path, root := conversationImageFixture(t)
	a.detail.Task.Prompt = ""
	a.detail.Instructions = []workflows.Instruction{{ID: "image-only", Status: workflows.InstructionApplied, AppliedAt: time.Now(), Attachments: []string{path, "/missing/notes.txt"}}}
	a.transcript.detail(*a.detail)
	a.syncMessages()
	router, frame, _ := alignmentRouter(root, 800, 700, 1)
	completeConversationImageLoads(t, a, path, frame)
	if len(a.entries) != 2 || len(a.entries[1].attachments) != 2 {
		t.Fatal("attachment-only history was hidden")
	}
	if _, ok := alignmentNode(router.AppendSemantics(nil), "notes.txt", false); !ok {
		t.Fatal("non-image attachment filename was hidden")
	}
	if a.conversation.images["instruction/image-only/"+path] == nil {
		t.Fatal("attachment-only message did not render its image")
	}
}

func TestRenderConversationAttachments(t *testing.T) {
	dir := os.Getenv("ONECATCH_KEEL_PREVIEWS")
	if dir == "" {
		t.Skip("optional preview")
	}
	a, path, root := conversationImageFixture(t)
	_, frame, _ := alignmentRouter(root, 800, 500, 1)
	completeConversationImageLoads(t, a, path, frame)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := window.Screenshot(screenshotRoot{core.Func(root.Layout)}, 800, 500, filepath.Join(dir, "conversation-attachments.png")); err != nil {
		t.Fatal(err)
	}
}
