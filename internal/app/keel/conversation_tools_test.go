package keeldesktop

import (
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/locale"
	"github.com/dyike/keel/ui/theme"
	"github.com/dyike/keel/ui/window"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
)

func TestToolCodePresentationAndHighlight(t *testing.T) {
	e := &entry{kind: agentrun.KindToolUse, text: `Bash {"command":"git status --short","description":"检查工作区修改","timeout":1000}`}
	title, body, _ := toolDisplay(e)
	if title != "执行命令 · 检查工作区修改" {
		t.Fatal(title)
	}
	sections := toolCodeSections(e, "命令", body)
	if len(sections) != 2 || sections[0].text != "git status --short" || sections[0].language != "bash" || !strings.Contains(sections[1].text, "1000") {
		t.Fatalf("command or options lost: %+v", sections)
	}
	tokens, ok := core.CurrentHighlighter().Highlight(`echo "hello"`, core.HighlightOptions{Language: "bash", Style: "github"})
	if !ok || len(tokens) < 2 {
		t.Fatal("syntax highlighter not installed")
	}
	read := &entry{kind: agentrun.KindToolUse, text: `Read {"file_path":"/repo/main.go"}`}
	if toolResultLanguage(read, "package main") != "go" || toolResultLanguage(read, `{"ok":true}`) != "json" {
		t.Fatal("result language not recognized")
	}
}

func TestToolCodeDragAndButtonCopy(t *testing.T) {
	a := conversationFixture(t)
	e := &entry{key: "code-copy", kind: agentrun.KindToolUse, text: `Bash {"command":"echo hello_world","description":"输出测试文本"}`, details: []*entry{{text: "hello_world"}}}
	a.toggleConversation(e.key, false)
	root := el.Root(el.ViewFunc(func(cx *el.Context) el.Element {
		return el.Div().WFull().HFull().P(20).Child(a.processItem(cx, e, false, false))
	}))
	router, frame, click := alignmentRouter(root, 700, 600, 1)
	node, ok := alignmentNode(router.AppendSemantics(nil), "echo hello_world", false)
	if !ok {
		t.Fatal("missing highlighted command")
	}
	start := f32.Pt(float32(node.Bounds.Min.X+1), float32(node.Bounds.Min.Y+node.Bounds.Dy()/2))
	end := f32.Pt(float32(node.Bounds.Max.X-1), start.Y)
	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: start})
	frame()
	router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: end})
	frame()
	router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: end})
	frame()
	frame()
	selectionKey(router, frame, "C")
	_, data, copied := router.WriteClipboard()
	if !copied || !strings.Contains(string(data), "hello_world") {
		t.Fatalf("drag selection did not copy command: %q", data)
	}
	var buttons []image.Rectangle
	seen := map[image.Rectangle]bool{}
	var collect func([]input.SemanticNode)
	collect = func(nodes []input.SemanticNode) {
		for _, node := range nodes {
			if node.Desc.Label == locale.Current().Copy && node.Desc.Gestures&input.ClickGesture != 0 && !seen[node.Desc.Bounds] {
				seen[node.Desc.Bounds] = true
				buttons = append(buttons, node.Desc.Bounds)
			}
			collect(node.Children)
		}
	}
	collect(router.AppendSemantics(nil))
	if len(buttons) != 2 {
		t.Fatalf("expected command/result copy buttons, got %d", len(buttons))
	}
	for i, button := range buttons {
		click(button)
		_, data, copied := router.WriteClipboard()
		want := []string{"echo hello_world", "hello_world"}[i]
		if !copied || strings.TrimSuffix(string(data), "\n") != want {
			t.Fatalf("button %d copied %q", i, data)
		}
	}
}

func TestRenderToolCodeCard(t *testing.T) {
	dir := os.Getenv("ONECATCH_KEEL_PREVIEWS")
	if dir == "" {
		t.Skip("set ONECATCH_KEEL_PREVIEWS")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	a := conversationFixture(t)
	e := &entry{key: "code-preview", kind: agentrun.KindToolUse, text: `Bash {"command":"git status --short\ngo test ./internal/app/keel -run TestTool -count=1","description":"检查修改并验证工具面板","timeout":120000}`, details: []*entry{{text: " M internal/app/keel/conversation.go\n?? internal/app/keel/conversation_tools.go\nok  github.com/openmodu/onecatch/internal/app/keel  2.134s"}}}
	a.toggleConversation(e.key, false)
	root := el.Root(el.ViewFunc(func(cx *el.Context) el.Element {
		return el.Div().WFull().HFull().P(24).Bg(theme.Surface).Child(a.processItem(cx, e, false, false))
	}))
	if err := window.Screenshot(screenshotRoot{core.Func(root.Layout)}, 900, 650, filepath.Join(dir, "tool-code-card.png")); err != nil {
		t.Fatal(err)
	}
}
