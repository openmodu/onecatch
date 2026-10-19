package keeldesktop

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dyike/keel/gio/io/key"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/locale"
	"github.com/dyike/keel/ui/theme"
)

func TestConversationMarkdownImageClickPreviewsLoadedPixels(t *testing.T) {
	locale.Apply(locale.Chinese())
	for _, scale := range []int{1, 2} {
		for _, kind := range []string{"local", "http", "data", "empty-alt"} {
			t.Run(fmt.Sprintf("%s/%dx", kind, scale), func(t *testing.T) {
				a, path, _ := conversationImageFixture(t)
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				source := path
				var requests atomic.Int32
				switch kind {
				case "http":
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						requests.Add(1)
						w.Header().Set("Content-Type", "image/png")
						_, _ = w.Write(data)
					}))
					defer server.Close()
					source = server.URL + "/model-image.png"
				case "data":
					source = "data:image/png;base64," + base64.StdEncoding.EncodeToString(data)
				}
				alt, title, label := "模型截图", "模型截图", "[图片 模型截图]"
				if kind == "empty-alt" {
					alt, title, label = "", "图片", "[图片]"
				}
				e := &entry{key: "model-image", text: "图片如下：\n\n![" + alt + "](<" + source + ">)\n\n图片之后的回复"}
				doc := a.conversationSource(e.key, e.text, false)
				a.conversation.selection.SetDocuments(doc)
				root := el.Root(el.ViewFunc(func(cx *el.Context) el.Element {
					return a.conversation.selection.Decorate(cx, el.Div().WFull().HFull().P(20).Bg(theme.Surface).Child(a.conversationImagePreview(cx), el.Div().W(el.Dp(420)).Child(a.conversationDocument(cx, e, false))))
				}))
				router, frame, click := alignmentRouter(root, 1000, 800, scale)
				state := a.conversation.markdownImages[e.key]
				var pixels image.Image
				for until := time.Now().Add(3 * time.Second); time.Now().Before(until); {
					state.mu.Lock()
					pixels = state.pixels[source]
					state.mu.Unlock()
					if pixels != nil {
						break
					}
					time.Sleep(time.Millisecond)
				}
				if pixels == nil {
					t.Fatal("Markdown did not load image pixels")
				}
				// The router harness does not drain the native window's async updates;
				// the document loader has completed and the preview must reuse its pixels.
				link, ok := alignmentNode(router.AppendSemantics(nil), label, true)
				if !ok {
					t.Fatal("model's Markdown image is not clickable")
				}
				click(link.Bounds)
				frame()
				if state.preview == nil || !state.preview.dialog.Value() || state.preview.natural != pixels.Bounds().Size() {
					t.Fatal("image click did not open the full-resolution preview")
				}
				preview, ok := alignmentNode(router.AppendSemantics(nil), "预览图片："+title, false)
				if !ok || preview.Bounds.Empty() {
					t.Fatal("preview was not painted")
				}
				if kind == "http" && requests.Load() != 1 {
					t.Fatalf("preview reloaded the image: %d requests", requests.Load())
				}
				router.Queue(key.Event{Name: key.NameEscape, State: key.Press}, key.Event{Name: key.NameEscape, State: key.Release})
				frame()
				frame()
				if state.preview != nil && state.preview.dialog.Value() {
					t.Fatal("Escape did not close preview")
				}
				// Stream a later paragraph without reparsing/resetting the existing image.
				if a.conversationSource(e.key, e.text+"\n\n继续回复", false) != doc || a.conversation.markdownImages[e.key] != state {
					t.Fatal("stream append replaced document or image actions")
				}
				e.text += "\n\n继续回复"
				frame()
				frame()
				link, ok = alignmentNode(router.AppendSemantics(nil), label, true)
				if !ok {
					t.Fatal("stream append lost image action")
				}
				click(link.Bounds)
				frame()
				if state.preview == nil || !state.preview.dialog.Value() {
					t.Fatal("image cannot reopen after streaming")
				}
				if kind == "http" && requests.Load() != 1 {
					t.Fatal("stream append fetched image again")
				}
				if dir := os.Getenv("ONECATCH_KEEL_PREVIEWS"); dir != "" && kind == "local" {
					if err := os.MkdirAll(dir, 0755); err != nil {
						t.Fatal(err)
					}
					shot := interactionPixelsScaled(t, root, router, 1000*scale, 800*scale, float32(scale))
					var encoded bytes.Buffer
					if err := png.Encode(&encoded, shot); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("markdown-image-preview-%dx.png", scale)), encoded.Bytes(), 0600); err != nil {
						t.Fatal(err)
					}
				}
			})
		}
	}
}

func TestConversationLinkedImagesKeepTheirDestination(t *testing.T) {
	a, path, _ := conversationImageFixture(t)
	const destination = "https://example.com/image-details"
	var opened string
	a.openURL = func(target string) error { opened = target; return nil }
	e := &entry{key: "linked-image", text: "[![图片链接](<" + path + ">)](" + destination + ")"}
	root := el.Root(el.ViewFunc(func(cx *el.Context) el.Element {
		return el.Div().WFull().HFull().Child(a.conversationDocument(cx, e, false))
	}))
	router, _, click := alignmentRouter(root, 800, 600, 1)
	link, ok := alignmentNode(router.AppendSemantics(nil), "[图片 图片链接]", true)
	if !ok {
		t.Fatal("linked image has no action")
	}
	click(link.Bounds)
	if opened != destination {
		t.Fatalf("linked image destination changed: %q", opened)
	}
}
