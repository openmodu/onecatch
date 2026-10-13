package keeldesktop

import (
	"encoding/json"
	"path/filepath"
	"strings"

	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/markdown"
	"github.com/dyike/keel/ui/theme"
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
