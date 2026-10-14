package keeldesktop

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/dyike/keel/gio/io/key"
	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/theme"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
)

type composerSkillState struct {
	key, dismissed, query string
	loading               bool
	err                   string
	skills, matches       []agentrun.Skill
	active, start, caret  int
	open                  bool
}

func skillTrigger(text string, caret int) (int, string, bool) {
	r := []rune(text)
	if caret < 0 || caret > len(r) {
		return 0, "", false
	}
	start := caret
	for start > 0 {
		c := r[start-1]
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("._:-", c)) {
			break
		}
		start--
	}
	if start == 0 || r[start-1] != '$' || start > 1 && !unicode.IsSpace(r[start-2]) {
		return 0, "", false
	}
	return start - 1, string(r[start:caret]), true
}

func matchingSkills(skills []agentrun.Skill, query string) []agentrun.Skill {
	type ranked struct {
		skill agentrun.Skill
		rank  int
	}
	var found []ranked
	q := strings.ToLower(query)
	for _, s := range skills {
		name := strings.ToLower(s.Name)
		rank := 4
		switch {
		case strings.HasPrefix(name, q):
			rank = 0
		case strings.Contains(name, q):
			rank = 1
		case strings.Contains(strings.ToLower(s.DisplayName), q):
			rank = 2
		case strings.Contains(strings.ToLower(s.ShortDescription+" "+s.Description), q):
			rank = 3
		}
		if rank < 4 {
			found = append(found, ranked{s, rank})
		}
	}
	sort.SliceStable(found, func(i, j int) bool { return found[i].rank < found[j].rank })
	var result []agentrun.Skill
	for _, s := range found[:min(8, len(found))] {
		result = append(result, s.skill)
	}
	return result
}

func (a *app) composerSkills(cx *el.Context) {
	s := &a.skillPicker
	wasOpen := s.open
	s.open = false
	if a.remoteConversation() || a.task != nil && a.runID == "" {
		return
	}
	edit, ok := cx.InputSelection("prompt")
	if !ok || (!cx.FocusWithin("prompt") && !wasOpen) || edit.Start != edit.End {
		return
	}
	start, query, ok := skillTrigger(edit.Text, edit.Start)
	if !ok {
		s.dismissed = ""
		return
	}
	signature := fmt.Sprintf("%s/%d", edit.Text, edit.Start)
	if s.dismissed == signature {
		return
	}
	cacheKey := a.runtimeProfileKey()
	if s.key != cacheKey {
		*s = composerSkillState{key: cacheKey, loading: true}
		runtime, path := a.runtime.Value(), a.workspaceForID(a.workspaceID).Path
		go func() {
			skills, err := a.backend.service.ListSkills(a.ctx, runtime, path)
			core.Update(func() {
				if a.closed || a.skillPicker.key != cacheKey {
					return
				}
				s := &a.skillPicker
				s.loading = false
				if err != nil {
					s.err = "无法加载 Skill 列表"
					return
				}
				s.skills = skills
			})
		}()
	}
	if s.query != query {
		s.active = 0
	}
	s.open, s.start, s.caret, s.query = true, start, edit.Start, query
	s.matches = matchingSkills(s.skills, query)
	s.active = min(s.active, max(0, len(s.matches)-1))
	menu := el.Div().W(el.Frac(0.8)).MaxH(el.Dp(320)).ScrollY().P(6).Rounded(10).Bg(theme.Surface).Border(1, seamColor).Shadow(theme.ElevationMd)
	switch {
	case s.loading:
		menu.Child(el.Text("正在加载 Skills…").P(10).TextSize(12).TextColor(theme.Muted))
	case s.err != "":
		menu.Child(el.Text(s.err).P(10).TextSize(12).TextColor(theme.DangerText))
	case len(s.matches) == 0:
		menu.Child(el.Text("没有匹配的 Skill").P(10).TextSize(12).TextColor(theme.Muted))
	default:
		for i, skill := range s.matches {
			index := i
			row := el.Div().ID(fmt.Sprintf("composer-skill/%d", i)).Role("option").Name("$" + skill.Name).Selected(i == s.active).CursorPointer().Px(10).Py(8).Rounded(6).Gap(3).OnClick(func() { a.chooseComposerSkill(cx, index) }).Hover(func(st *el.Style) { st.Bg(theme.SubtleHover) })
			if i == s.active {
				row.Bg(theme.Subtle)
			}
			description := skill.ShortDescription
			if description == "" {
				description = skill.Description
			}
			row.Child(el.Text("$" + skill.Name).TextSize(13).Medium().MaxLines(1))
			if description != "" {
				row.Child(el.Text(description).TextSize(11).TextColor(theme.Muted).MaxLines(1))
			}
			menu.Child(row)
		}
	}
	popup := el.Div().Row().Justify(el.Center).Child(menu)
	cx.Overlay("composer-skills", el.Anchored("prompt", popup).MatchAnchorWidth().Placement(el.Top, el.Start).OnDismiss(func() { s.dismissed = signature; s.open = false }))
}

func (a *app) chooseComposerSkill(cx *el.Context, index int) {
	s := &a.skillPicker
	if index < 0 || index >= len(s.matches) {
		return
	}
	r := []rune(a.prompt)
	if s.start < 0 || s.caret > len(r) {
		return
	}
	suffix := string(r[s.caret:])
	separator := " "
	end := s.caret
	if strings.HasPrefix(suffix, " ") || strings.HasPrefix(suffix, "\t") {
		end++
	} else if strings.HasPrefix(suffix, "\n") || strings.HasPrefix(suffix, "\r") {
		separator = ""
	}
	token := "$" + s.matches[index].Name + separator
	_ = a.promptDocument.Select(el.InputRange{Start: len(string(r[:s.start])), End: len(string(r[:end]))})
	a.promptTokenNext++
	if err := a.promptDocument.ReplaceWithToken(el.InputToken{ID: fmt.Sprintf("skill/%d", a.promptTokenNext), Text: token, Label: token}); err != nil {
		return
	}
	a.prompt = a.promptDocument.Content().Text()
	caret := s.start + len([]rune(token))
	s.open = false
	cx.SelectInput("prompt", caret, caret)
	cx.Focus("prompt")
}

func (a *app) composerSkillKey(cx *el.Context, e el.KeyEvent) bool {
	s := &a.skillPicker
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
		a.chooseComposerSkill(cx, s.active)
	default:
		return false
	}
	return true
}
