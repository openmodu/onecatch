package keeldesktop

import (
	"encoding/json"
	"image/color"
	"path/filepath"
	"strings"

	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/kit"
	"github.com/dyike/keel/ui/locale"
	"github.com/dyike/keel/ui/markdown"
	"github.com/dyike/keel/ui/theme"
	uiassets "github.com/openmodu/onecatch/internal/app/keel/assets"
)

type toolCodeContent struct {
	label, language, text string
}

func toolCodeSections(e *entry, label, body string) []toolCodeContent {
	language := "text"
	var args map[string]any
	if json.Unmarshal([]byte(body), &args) == nil {
		language = "json"
		if label == "命令" {
			for _, key := range []string{"command", "cmd"} {
				if command, ok := args[key].(string); ok && command != "" {
					sections := []toolCodeContent{{"命令", "bash", command}}
					// Keep all execution options available, alongside the readable command.
					delete(args, key)
					delete(args, "description")
					if len(args) > 0 {
						data, _ := json.MarshalIndent(args, "", "  ")
						sections = append(sections, toolCodeContent{"其他参数", "json", string(data)})
					}
					return sections
				}
			}
		}
	} else if label == "命令" {
		language = "bash"
	} else if label == "结果" {
		language = toolResultLanguage(e, body)
	}
	return []toolCodeContent{{label, language, body}}
}

func toolResultLanguage(e *entry, result string) string {
	if json.Valid([]byte(result)) {
		return "json"
	}
	_, body, icon := toolDisplay(e)
	if icon != "book-open" {
		return "text"
	}
	var args map[string]any
	if json.Unmarshal([]byte(body), &args) == nil {
		for _, key := range []string{"file_path", "filePath", "path"} {
			if path, ok := args[key].(string); ok {
				languages := map[string]string{".go": "go", ".py": "python", ".js": "javascript", ".ts": "typescript", ".tsx": "tsx", ".jsx": "jsx", ".sh": "bash", ".json": "json", ".yaml": "yaml", ".yml": "yaml", ".sql": "sql", ".html": "html", ".css": "css", ".toml": "toml", ".md": "markdown"}
				if language := languages[strings.ToLower(filepath.Ext(path))]; language != "" {
					return language
				}
			}
		}
	}
	return "text"
}

func toolCodeSource(language, text string) string {
	// Choose a fence longer than any run in the content, so commands containing
	// Markdown or heredocs remain a single selectable code block.
	fence := "```"
	for strings.Contains(text, fence) {
		fence += "`"
	}
	// A fence can end at EOF. This lets streaming output append without
	// replacing the document and clearing a selection already being dragged.
	return fence + language + "\n" + text
}

func (a *app) toolCodeSection(cx *el.Context, content toolCodeContent, id string) el.Element {
	if content.label == "结果" {
		return a.toolResultSection(cx, content, id)
	}
	if a.conversation.toolDocs == nil {
		a.conversation.toolDocs = map[string]*markdown.Doc{}
	}
	doc := a.conversation.toolDocs[id]
	source := toolCodeSource(content.language, content.text)
	if doc == nil {
		doc = markdown.New(source).CodeBlockWrap(true)
		a.conversation.toolDocs[id] = doc
	} else if doc.Source() != source {
		if strings.HasPrefix(source, doc.Source()) {
			doc.Append(source[len(doc.Source()):])
		} else {
			doc.SetSource(source)
		}
	}
	// Keep the document identity across frames: syntax colors, selection,
	// keyboard copy and code-card copy all share Keel's code renderer.
	return el.Div().ID(id).WFull().Gap(5).Child(
		el.Text(content.label).TextSize(11).Medium().TextColor(theme.Muted),
		doc.Render(cx),
	)
}

const toolResultMaxHeight = 320

type toolResultView struct {
	text     string
	document el.InputDocument
	copy     *kit.CopyButtonView
}

func (a *app) toolResultSection(cx *el.Context, content toolCodeContent, id string) el.Element {
	if a.conversation.toolResults == nil {
		a.conversation.toolResults = map[string]*toolResultView{}
	}
	view := a.conversation.toolResults[id]
	if view == nil {
		view = &toolResultView{}
		view.copy = kit.CopyButton(func() string { return view.text }).Content(el.ViewFunc(func(cx *el.Context) el.Element {
			icon := "copy"
			if view.copy.Copied() {
				icon = "check"
			}
			return uiassets.Lucide(cx, icon, 16, theme.Text)
		}))
		a.conversation.toolResults[id] = view
	}
	// Gio's editor paints tabs as missing glyphs. Match the code renderer's
	// four-space indentation while retaining the original output for full copy.
	display := strings.ReplaceAll(content.text, "\t", "    ")
	view.text = content.text
	if view.document.Content().Text() != display {
		selection := view.document.Selection()
		appended := strings.HasPrefix(display, view.document.Content().Text())
		_ = view.document.SetText(display)
		if appended {
			_ = view.document.Select(selection)
		}
	}
	label := content.language
	if label == "" || label == "text" {
		label = locale.Current().PlainText
	}
	// Tool output is literal text, including Markdown fences and diff markers.
	// The native read-only editor owns selection and scrolls independently;
	// the header and full-output copy action stay visible above its viewport.
	header := el.Div().Row().Items(el.Center).Gap(6).Px(14).Pt(8).Pb(4).Child(
		uiassets.Lucide(cx, "file-code-2", 18, theme.Text),
		el.Text(label).TextSize(13).Grow(), view.copy.Render(cx),
	)
	body := el.TextArea().ID(id+"/text").Name(content.text).Document(&view.document).ReadOnly(true).
		AutoGrow(1, 12).MaxH(el.Dp(toolResultMaxHeight-48)).WFull().Mono().TextSize(13).LineHeight(1.6).
		Border(0, theme.Border).Rounded(0).Bg(color.NRGBA{}).Px(16).Pt(6).Pb(16).
		Scrollbars(el.ScrollbarAlways).FocusStyle(func(s *el.Style) { s.BorderColor(color.NRGBA{}).Bg(color.NRGBA{}) })
	return el.Div().ID(id).WFull().Gap(5).Child(
		el.Text(content.label).TextSize(11).Medium().TextColor(theme.Muted),
		el.Div().WFull().Bg(theme.CodeBg).Rounded(14).Child(header, body),
	)
}
