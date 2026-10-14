package keeldesktop

import (
	"errors"
	domaintasks "github.com/openmodu/onecatch/internal/domain/tasks"
	domainworkspaces "github.com/openmodu/onecatch/internal/domain/workspaces"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

func TestConversationFileLinkClick(t *testing.T) {
	path := filepath.Join(t.TempDir(), "OneCatch Keel.app")
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	a := conversationFixture(t)
	var opened string
	a.openPath = func(path string) error { opened = path; return nil }
	a.openURL = func(raw string) error { t.Fatalf("file sent to URL handler: %s", raw); return nil }
	e := &entry{key: "file-link", text: "[OneCatch Keel.app](<" + filepath.ToSlash(path) + ">)"}
	root := el.Root(el.ViewFunc(func(cx *el.Context) el.Element {
		return el.Div().WFull().HFull().P(20).Child(a.conversationDocument(cx, e, false))
	}))
	router, _, click := alignmentRouter(root, 800, 400, 1)
	link, ok := alignmentNode(router.AppendSemantics(nil), "OneCatch Keel.app", true)
	if !ok {
		t.Fatal("file link is not clickable")
	}
	click(link.Bounds)
	if opened != path || a.errorText != "" {
		t.Fatalf("opened %q, error %q", opened, a.errorText)
	}
}

func TestConversationFileLinkResolution(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "文件 with spaces.md")
	if err := os.WriteFile(path, []byte("test"), 0600); err != nil {
		t.Fatal(err)
	}
	a := conversationFixture(t)
	a.workspaces = []domainworkspaces.Workspace{{ID: a.workspaceID, Path: dir}}
	a.task = nil
	var opened string
	var launchErr error
	a.openPath = func(path string) error { opened = path; return launchErr }
	a.openURL = func(raw string) error { t.Fatalf("file sent to browser: %s", raw); return nil }
	fileURL := (&url.URL{Scheme: "file", Path: filepath.ToSlash(path)}).String()
	for _, raw := range []string{path, fileURL, fileURL + "#L12", filepath.Base(path), "./" + filepath.Base(path), path + ":12", path + ":12:3", path + "#L12", strings.ReplaceAll(path, " ", "%20")} {
		opened = ""
		a.openConversationLink(raw)
		if opened != path || a.errorText != "" {
			t.Fatalf("%q: opened %q error %q", raw, opened, a.errorText)
		}
	}
	a.task = &domaintasks.Task{Worktree: &domainworkspaces.Worktree{Path: dir}}
	a.workspaces[0].Path = t.TempDir()
	a.openConversationLink(filepath.Base(path))
	if opened != path || a.errorText != "" {
		t.Fatal("did not use task worktree", a.errorText)
	}
	launchErr = errors.New("launcher unavailable")
	a.openConversationLink(path)
	if a.errorText != launchErr.Error() {
		t.Fatal("lost launch error", a.errorText)
	}
	launchErr = nil
	for _, raw := range []string{filepath.Join(dir, "missing.txt"), "file://remote/tmp/file", "javascript:alert(1)", "data:text/plain,test"} {
		opened = ""
		a.openConversationLink(raw)
		if opened != "" || a.errorText == "" {
			t.Fatalf("invalid link launched: %q", raw)
		}
	}
	a.task = nil
	a.workspaces[0].RemoteFS = &domainworkspaces.RemoteFS{Host: "remote", Root: dir}
	opened = ""
	a.openConversationLink("file.md")
	if opened != "" || a.errorText == "" {
		t.Fatal("remote relative path opened locally")
	}
}

func TestConversationExactFileName(t *testing.T) {
	a := conversationFixture(t)
	var opened string
	a.openPath = func(path string) error { opened = path; return nil }
	for _, name := range []string{"100% ready.txt", "literal%20space.txt", "notes#L12.md", "report:12"} {
		if runtime.GOOS == "windows" && strings.Contains(name, ":") {
			continue
		}
		path := filepath.Join(t.TempDir(), name)
		if err := os.WriteFile(path, nil, 0600); err != nil {
			t.Fatal(err)
		}
		a.openConversationLink(path)
		if opened != path || a.errorText != "" {
			t.Fatalf("%q opened %q: %s", path, opened, a.errorText)
		}
	}
}
