package agentrun

import (
	"context"
	"strings"
)

// Skill is one user-invocable capability exposed by an agent runtime.
// OneCatch always renders and accepts it as `$name`; each runtime adapter is
// responsible for translating that marker to its native protocol.
type Skill struct {
	Name             string `json:"name"`
	DisplayName      string `json:"displayName,omitempty"`
	Description      string `json:"description,omitempty"`
	ShortDescription string `json:"shortDescription,omitempty"`
	Path             string `json:"path,omitempty"`
	Scope            string `json:"scope,omitempty"`
}

// SkillLister is implemented by runtimes that can discover their effective
// user-invocable Skill catalog without consuming model quota.
type SkillLister interface {
	ListSkills(ctx context.Context, cwd string, environment []string) ([]Skill, error)
}

type skillMention struct {
	start int
	end   int
	name  string
}

func skillMentions(prompt string) []skillMention {
	mentions := make([]skillMention, 0)
	for index := 0; index < len(prompt); {
		marker := strings.IndexByte(prompt[index:], '$')
		if marker < 0 {
			break
		}
		marker += index
		if marker > 0 && !isSkillBoundary(prompt[marker-1]) {
			index = marker + 1
			continue
		}
		end := marker + 1
		for end < len(prompt) && isSkillNameByte(prompt[end]) {
			end++
		}
		if end > marker+1 {
			mentions = append(mentions, skillMention{start: marker, end: end, name: prompt[marker+1 : end]})
		}
		index = end
	}
	return mentions
}

func referencedSkills(prompt string, skills []Skill) []Skill {
	byName := make(map[string]Skill, len(skills))
	for _, skill := range skills {
		byName[skill.Name] = skill
	}
	seen := make(map[string]struct{})
	referenced := make([]Skill, 0)
	for _, mention := range skillMentions(prompt) {
		skill, ok := byName[mention.name]
		if !ok {
			continue
		}
		if _, exists := seen[mention.name]; exists {
			continue
		}
		seen[mention.name] = struct{}{}
		referenced = append(referenced, skill)
	}
	return referenced
}

// adaptClaudeSkillMentions translates OneCatch's runtime-neutral `$name`
// syntax to Claude Code's native `/name` syntax. The whitespace boundary is
// deliberately the same one used by the composer highlighter, so currency and
// identifiers such as price$usd remain ordinary text.
func adaptClaudeSkillMentions(prompt string) string {
	mentions := skillMentions(prompt)
	if len(mentions) == 0 {
		return prompt
	}
	var output strings.Builder
	output.Grow(len(prompt))
	previous := 0
	for _, mention := range mentions {
		output.WriteString(prompt[previous:mention.start])
		output.WriteByte('/')
		output.WriteString(mention.name)
		previous = mention.end
	}
	output.WriteString(prompt[previous:])
	return output.String()
}

func isSkillBoundary(value byte) bool {
	return value == ' ' || value == '\t' || value == '\n' || value == '\r'
}

func isSkillNameByte(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' || value >= '0' && value <= '9' || value == '-' || value == '_' || value == '.' || value == ':'
}
