package keeldesktop

import (
	"strings"
	"testing"

	"github.com/dyike/keel/ui/el"
)

func TestUserMessageMarkdownPreservesLineBreaks(t *testing.T) {
	for _, tc := range []struct{ name, source, want string }{
		{"long", strings.Repeat("第一行\n第二行\n", 40), "第一行\n第二行\n"},
		{"plain", "第一行\n第二行\nthird line\nfourth line", "第一行\n第二行\nthird line\nfourth line"},
		{"crlf", "第一行\r\n第二行", "第一行\n第二行"},
		{"markdown", "# 标题\n\n**加粗**\n下一行\n\n- 列表一\n- 列表二\n\n> 引用第一行\n> 引用第二行\n\n```go\nfirst()\nsecond()\n```", "加粗\n下一行"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := conversationFixture(t)
			e := &entry{key: "user-markdown", user: true, text: tc.source}
			a.entries = []*entry{e}
			root := el.Root(el.ViewFunc(func(cx *el.Context) el.Element { return el.Div().WFull().HFull().Child(a.message(cx, 0)) }))
			_, frame, _ := alignmentRouter(root, 800, 900, 1)
			doc := a.docs[e.key]
			if got := doc.RenderedText(); !strings.Contains(got, tc.want) {
				t.Fatalf("rendered text = %q, want %q", got, tc.want)
			}
			if tc.name == "markdown" {
				got := doc.RenderedText()
				for _, want := range []string{"标题", "列表一", "列表二", "引用第一行\n引用第二行", "first()\nsecond()"} {
					if !strings.Contains(got, want) {
						t.Errorf("rendered text %q missing %q", got, want)
					}
				}
				for _, marker := range []string{"# 标题", "**", "```", "> 引用"} {
					if strings.Contains(got, marker) {
						t.Errorf("Markdown marker %q leaked into %q", marker, got)
					}
				}
			}
			if doc.Source() != tc.source {
				t.Fatal("source changed")
			}
			e.text += "\n\n新增第一行\n新增第二行"
			frame()
			if a.docs[e.key] != doc || !strings.Contains(doc.RenderedText(), "新增第一行\n新增第二行") {
				t.Fatal("append lost document identity or line breaks")
			}
			e.text = "修正第一行\n修正第二行"
			frame()
			if got := a.docs[e.key].RenderedText(); got != e.text {
				t.Fatalf("replacement rendered %q", got)
			}
		})
	}
}

func TestAssistantMessageKeepsMarkdownSoftBreaks(t *testing.T) {
	a := conversationFixture(t)
	e := &entry{key: "assistant", text: "first\nsecond"}
	root := el.Root(el.ViewFunc(func(cx *el.Context) el.Element { return a.conversationDocument(cx, e, false) }))
	alignmentRouter(root, 800, 400, 1)
	doc := a.docs[e.key]
	if got := doc.RenderedText(); got != "first second" {
		t.Fatalf("assistant rendered %q", got)
	}
}
