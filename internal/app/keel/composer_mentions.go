package keeldesktop

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"github.com/dyike/keel/gio/io/key"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/theme"
	uiassets "github.com/openmodu/onecatch/internal/app/keel/assets"
	domainworkflows "github.com/openmodu/onecatch/internal/domain/workflows"
)

// The composer's @ picker. Before a conversation starts it suggests
// workflows, and choosing one selects it for the new task. Inside a running
// workflow it suggests that workflow's steps, and a chosen step becomes a
// token that addresses the message to that step.
type composerMentionState struct {
	dismissed, query     string
	matches              []mentionOption
	active, start, caret int
	open                 bool
}

type mentionOption struct {
	ID, Name, Detail string
	Step             bool
}

func mentionTrigger(text string, caret int) (int, string, bool) {
	r := []rune(text)
	if caret < 0 || caret > len(r) {
		return 0, "", false
	}
	start := caret
	for start > 0 && !unicode.IsSpace(r[start-1]) && r[start-1] != '@' {
		start--
	}
	if start == 0 || r[start-1] != '@' || start > 1 && !unicode.IsSpace(r[start-2]) {
		return 0, "", false
	}
	return start - 1, string(r[start:caret]), true
}

// mentionWorkflow is the workflow whose steps can be addressed, if any.
func (a *app) mentionWorkflow() (domainworkflows.Definition, bool) {
	if a.detail == nil || a.task == nil || a.runID == "" || len(a.detail.Workflow.Steps) < 2 {
		return domainworkflows.Definition{}, false
	}
	return a.detail.Workflow, true
}

func (a *app) mentionOptions(query string) []mentionOption {
	q := strings.ToLower(query)
	match := func(values ...string) bool {
		for _, v := range values {
			if strings.Contains(strings.ToLower(v), q) {
				return true
			}
		}
		return false
	}
	var out []mentionOption
	if d, ok := a.mentionWorkflow(); ok {
		for _, s := range d.Steps {
			if match(s.Name, s.ID) {
				label, _ := stepStatusLabel(a.detail.Run.Nodes[s.ID])
				out = append(out, mentionOption{ID: s.ID, Name: s.Name, Detail: stepAgentLabel(s) + " · " + label, Step: true})
			}
		}
		return out
	}
	if a.task != nil || a.remoteConversation() {
		return nil
	}
	for _, d := range a.selectableDefinitions() {
		if match(d.Name, d.ID, d.Description) {
			out = append(out, mentionOption{ID: d.ID, Name: d.Name, Detail: workflowSummary(d) + " · " + workflowPath(d)})
		}
	}
	return out
}

func (a *app) composerMentions(cx *el.Context) {
	s := &a.mentionPicker
	wasOpen := s.open
	s.open = false
	edit, ok := cx.InputSelection("prompt")
	if !ok || (!cx.FocusWithin("prompt") && !wasOpen) || edit.Start != edit.End {
		return
	}
	start, query, ok := mentionTrigger(edit.Text, edit.Start)
	if !ok {
		s.dismissed = ""
		return
	}
	signature := fmt.Sprintf("%s/%d", edit.Text, edit.Start)
	if s.dismissed == signature {
		return
	}
	_, steps := a.mentionWorkflow()
	if !steps && (a.task != nil || a.remoteConversation()) {
		return
	}
	if s.query != query {
		s.active = 0
	}
	s.open, s.start, s.caret, s.query = true, start, edit.Start, query
	s.matches = a.mentionOptions(query)
	s.active = min(s.active, max(0, len(s.matches)-1))
	title := "选择工作流"
	if steps {
		title = "发给某个步骤"
	}
	menu := el.Div().W(el.Frac(0.8)).MaxH(el.Dp(340)).ScrollY().P(6).Rounded(10).Bg(theme.Surface).Border(1, seamColor).Shadow(theme.ElevationMd)
	menu.Child(el.Text(title).TextSize(11).TextColor(theme.Muted).Px(10).Pt(4).Pb(2))
	if len(s.matches) == 0 {
		empty := "没有匹配的工作流"
		if steps {
			empty = "没有匹配的步骤"
		}
		menu.Child(el.Text(empty).P(10).TextSize(12).TextColor(theme.Muted))
	}
	for i, option := range s.matches {
		index := i
		icon := "workflow"
		if option.Step {
			icon = "corner-down-right"
		}
		row := el.Div().ID(fmt.Sprintf("composer-mention/%d", i)).Role("option").Name("@" + option.Name).Selected(i == s.active).CursorPointer().Row().Items(el.Center).Gap(10).Px(10).Py(8).Rounded(6).OnClick(func() { a.chooseComposerMention(cx, index) }).Hover(func(st *el.Style) { st.Bg(theme.SubtleHover) })
		if i == s.active {
			row.Bg(theme.Subtle)
		}
		row.Child(uiassets.Lucide(cx, icon, 14, theme.Muted), el.Div().Grow().W(el.Dp(0)).Gap(3).Child(el.Text("@"+option.Name).TextSize(13).Medium().MaxLines(1), el.Text(option.Detail).TextSize(11).TextColor(theme.Muted).MaxLines(1)))
		menu.Child(row)
	}
	popup := el.Div().Row().Justify(el.Center).Child(menu)
	cx.Overlay("composer-mentions", el.Anchored("prompt", popup).MatchAnchorWidth().Placement(el.Top, el.Start).OnDismiss(func() { s.dismissed = signature; s.open = false }))
}

func (a *app) chooseComposerMention(cx *el.Context, index int) {
	s := &a.mentionPicker
	if index < 0 || index >= len(s.matches) {
		return
	}
	r := []rune(a.prompt)
	if s.start < 0 || s.caret > len(r) {
		return
	}
	option := s.matches[index]
	end := s.caret
	if end < len(r) && (r[end] == ' ' || r[end] == '\t') {
		end++
	}
	s.open = false
	if !option.Step {
		// A workflow is a choice for the whole task, not part of the text.
		a.prompt = string(r[:s.start]) + string(r[end:])
		_ = a.promptDocument.SetText(a.prompt)
		a.selectLocalConversation(a.runtime.Value(), option.ID)
		cx.SelectInput("prompt", s.start, s.start)
		cx.Focus("prompt")
		return
	}
	_ = a.promptDocument.Select(el.InputRange{Start: len(string(r[:s.start])), End: len(string(r[:end]))})
	token := "@" + option.ID + " "
	a.promptTokenNext++
	if err := a.promptDocument.ReplaceWithToken(el.InputToken{ID: fmt.Sprintf("step/%d", a.promptTokenNext), Text: token, Label: "@" + option.Name + " "}); err != nil {
		return
	}
	a.prompt = a.promptDocument.Content().Text()
	caret := s.start + len([]rune(token))
	cx.SelectInput("prompt", caret, caret)
	cx.Focus("prompt")
}

func (a *app) composerMentionKey(cx *el.Context, e el.KeyEvent) bool {
	s := &a.mentionPicker
	if !s.open {
		return false
	}
	if e.State != el.KeyPress {
		return true
	}
	switch key.Name(e.Name) {
	case key.NameEscape:
		if edit, ok := cx.InputSelection("prompt"); ok {
			s.dismissed = fmt.Sprintf("%s/%d", edit.Text, edit.Start)
		}
		s.open = false
	case key.NameDownArrow:
		if len(s.matches) > 0 {
			s.active = (s.active + 1) % len(s.matches)
		}
	case key.NameUpArrow:
		if len(s.matches) > 0 {
			s.active = (s.active + len(s.matches) - 1) % len(s.matches)
		}
	case key.NameReturn, key.NameEnter, key.NameTab:
		a.chooseComposerMention(cx, s.active)
	default:
		return false
	}
	return true
}

var stepMentionPattern = regexp.MustCompile(`(^|\s)@([a-z][a-z0-9_-]{0,63})\b`)

// splitStepMentions removes @step mentions that name steps of d and returns
// those steps in the order they appear with the remaining text.
func splitStepMentions(d domainworkflows.Definition, text string) ([]string, string) {
	var steps []string
	seen := map[string]bool{}
	rest := stepMentionPattern.ReplaceAllStringFunc(text, func(match string) string {
		groups := stepMentionPattern.FindStringSubmatch(match)
		if _, ok := domainworkflows.FindStep(d, groups[2]); !ok {
			return match
		}
		if !seen[groups[2]] {
			seen[groups[2]] = true
			steps = append(steps, groups[2])
		}
		return groups[1]
	})
	return steps, strings.TrimSpace(rest)
}
