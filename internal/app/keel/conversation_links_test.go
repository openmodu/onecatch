package keeldesktop

import (
	"errors"
	"testing"

	"github.com/dyike/keel/gio/f32"
	"github.com/dyike/keel/gio/io/pointer"
	"github.com/dyike/keel/ui/el"
)

func TestConversationLinksOpenBrowser(t *testing.T) {
	const destination = "https://github.com/dyike/keel/pull/18"
	for _, tc := range []struct {
		name, source, label string
		user                bool
	}{
		{"bare-url", destination, destination, false},
		{"markdown-link", "[PR #18](" + destination + ")", "PR #18", false},
		{"user-link", destination, destination, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := conversationFixture(t)
			var opened []string
			var launchErr error
			a.openURL = func(url string) error {
				opened = append(opened, url)
				return launchErr
			}
			e := &entry{key: "browser-link", text: tc.source, user: tc.user}
			// Restored documents must get the same handler as newly created ones.
			a.docs[e.key] = newConversationDocument(tc.source)
			a.conversation.selection.SetDocuments(a.docs[e.key])
			root := el.Root(el.ViewFunc(func(cx *el.Context) el.Element {
				return a.conversation.selection.Decorate(cx, el.Div().WFull().HFull().P(20).Child(a.conversationDocument(cx, e, tc.user)))
			}))
			router, frame, click := alignmentRouter(root, 800, 400, 1)
			link, ok := alignmentNode(router.AppendSemantics(nil), tc.label, true)
			if !ok {
				t.Fatal("link is not clickable")
			}
			click(link.Bounds)
			if len(opened) != 1 || opened[0] != destination {
				t.Fatalf("browser received %q", opened)
			}
			launchErr = errors.New("browser unavailable")
			a.openConversationLink(destination)
			if a.errorText != launchErr.Error() {
				t.Fatalf("browser launch failure was hidden: %q", a.errorText)
			}
			// Selecting/copying a URL must not launch the browser.
			opened = nil
			start := f32.Pt(float32(link.Bounds.Min.X+1), float32(link.Bounds.Min.Y+link.Bounds.Dy()/2))
			end := f32.Pt(float32(link.Bounds.Max.X-1), start.Y)
			router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: start})
			frame()
			router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: end})
			frame()
			router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: end})
			frame()
			selectionKey(router, frame, "A")
			selectionKey(router, frame, "C")
			_, data, copied := router.WriteClipboard()
			if !copied || string(data) != tc.label || len(opened) != 0 {
				t.Fatalf("link selection launched browser or lost text: opened=%q copied=%q", opened, data)
			}
		})
	}
}
