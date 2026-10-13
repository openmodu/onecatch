package keeldesktop

import (
	"github.com/dyike/keel/gio/io/key"
	"github.com/dyike/keel/ui/el"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
	"testing"
)

func TestComposerSkillTrigger(t *testing.T) {
	for _, tt := range []struct {
		text         string
		caret, start int
		query        string
		ok           bool
	}{
		{"$", 1, 0, "", true}, {"请用 $git-comm 后检查", 11, 3, "git-com", true},
		{"foo$bar", 7, 0, "", false}, {"$git commit", 11, 0, "", false},
		{"\n$openai-docs", 13, 1, "openai-docs", true},
	} {
		start, q, ok := skillTrigger(tt.text, tt.caret)
		if ok != tt.ok || ok && (start != tt.start || q != tt.query) {
			t.Errorf("%q: %d %q %v", tt.text, start, q, ok)
		}
	}
}
func TestComposerSkillRanking(t *testing.T) {
	skills := []agentrun.Skill{{Name: "other", Description: "git helper"}, {Name: "my-git"}, {Name: "git-commit"}}
	got := matchingSkills(skills, "git")
	if len(got) != 3 || got[0].Name != "git-commit" || got[1].Name != "my-git" {
		t.Fatalf("matches: %+v", got)
	}
}

func TestComposerSkillKeyboardSelectionDoesNotSend(t *testing.T) {
	a := conversationFixture(t)
	a.task = nil
	a.prompt = "$git"
	a.skillPicker = composerSkillState{key: a.runtimeProfileKey(), skills: []agentrun.Skill{{Name: "git-commit"}}}
	focus := false
	root := el.Root(el.ViewFunc(func(cx *el.Context) el.Element {
		if focus {
			cx.Focus("prompt")
			cx.SelectInput("prompt", 4, 4)
			focus = false
		}
		return el.Div().WFull().HFull().Child(el.Div().Grow(), a.composer(cx))
	}))
	router, frame, click := alignmentRouter(root, 900, 600, 1)
	focus = true
	frame()
	frame()
	frame()
	if !a.skillPicker.open {
		t.Fatal("typing $git did not open picker")
	}
	router.Queue(key.Event{Name: key.NameReturn, State: key.Press}, key.Event{Name: key.NameReturn, State: key.Release})
	frame()
	frame()
	if a.prompt != "$git-commit " {
		t.Fatalf("selected prompt=%q", a.prompt)
	}
	a.prompt = "$git"
	focus = true
	frame()
	frame()
	frame()
	node, ok := findSkillControl(router.AppendSemantics(nil), "$git-commit")
	if !ok {
		t.Fatal("skill option missing")
	}
	click(node.Bounds())
	frame()
	if a.prompt != "$git-commit " {
		t.Fatalf("clicked prompt=%q", a.prompt)
	}
	if tokens := a.promptDocument.Content().Tokens(); len(tokens) != 1 || tokens[0].Token.Text != "$git-commit " {
		t.Fatalf("selected skill is not an inline reference: %+v", tokens)
	}
	router.Queue(key.Event{Name: key.NameDeleteBackward, State: key.Press}, key.Event{Name: key.NameDeleteBackward, State: key.Release})
	frame()
	frame()
	if a.prompt != "" || len(a.promptDocument.Content().Tokens()) != 0 {
		t.Fatalf("backspace left a partial skill: %q", a.prompt)
	}

}
