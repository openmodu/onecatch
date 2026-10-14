package keeldesktop

import (
	"encoding/json"
	"image/color"
	"regexp"
	"strings"

	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/theme"
	uiassets "github.com/openmodu/onecatch/internal/app/keel/assets"
	domainworkflows "github.com/openmodu/onecatch/internal/domain/workflows"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
)

type workflowTransition struct {
	verdict, source, target, feedback, summary string
}

func (a *app) workflowMessageOutcome(e *entry) (domainworkflows.OutcomeMessage, bool) {
	var out domainworkflows.OutcomeMessage
	if e.user || e.kind != agentrun.KindMessage || e.step == "" || a.detail == nil || a.detail.Workflow.ID == "single_agent" {
		return out, false
	}
	out, err := domainworkflows.ParseOutcomeMessage(e.text)
	return out, err == nil
}

func outcomeStyle(verdict string) (string, string, color.NRGBA) {
	switch verdict {
	case domainworkflows.VerdictPass:
		return "步骤通过", "circle-check", theme.Success
	case domainworkflows.VerdictRework:
		return "需要返工", "rotate-ccw", theme.Warning
	default:
		return "等待你的确认", "circle-help", theme.Info
	}
}

var outcomeRequirement = regexp.MustCompile(`(?:^|[\s。；])(?:R)(\d+)[，、：:]\s*`)

func outcomeReadableText(text string) string {
	// Existing R1/R2 references are paragraph boundaries, not new requirements.
	return strings.TrimSpace(outcomeRequirement.ReplaceAllString(text, "\n\n**R$1** · "))
}

func (a *app) workflowStepName(id string) string {
	if a.detail != nil {
		if step, ok := domainworkflows.FindStep(a.detail.Workflow, id); ok {
			return step.Name
		}
	}
	return id
}

func (a *app) workflowOutcomeCard(cx *el.Context, e *entry, out domainworkflows.OutcomeMessage) el.Element {
	label, icon, tone := outcomeStyle(out.Verdict)
	header := el.Div().Row().Wrap().Items(el.Center).Gap(10).Child(
		el.Div().Row().Items(el.Center).Gap(6).Px(9).Py(5).Rounded(7).Bg(mixColor(tone, theme.Surface, .10)).
			Child(uiassets.Lucide(cx, icon, 14, tone), el.Text(label).TextSize(12).Medium().TextColor(tone)),
	)
	if out.Target != "" {
		header.Child(el.Text("交给 " + a.workflowStepName(out.Target)).TextSize(12).Medium().TextColor(theme.Text))
	}
	body := workflowOutcomeText(out.Outcome)
	card := el.Div().WFull().P(16).Gap(14).Rounded(12).Bg(mixColor(theme.Subtle, theme.Surface, .4)).Child(
		a.workflowNodeHeading(cx, e, true), header, a.conversationDocument(cx, &entry{key: e.key + "/readable", text: body}, false),
	)
	return a.workflowOutcomeDetails(cx, e, card, out)
}

func workflowOutcomeText(out domainworkflows.Outcome) string {
	body := "### 摘要\n\n" + outcomeReadableText(out.Summary)
	if out.Feedback != "" {
		section := "补充说明"
		if out.Verdict == domainworkflows.VerdictRework {
			section = "返工要求"
		}
		body += "\n\n### " + section + "\n\n" + outcomeReadableText(out.Feedback)
	}
	return body
}

func (a *app) workflowOutcomeDetails(cx *el.Context, e *entry, card *el.DivEl, out domainworkflows.OutcomeMessage) el.Element {
	key := e.key + "/raw"
	open := a.conversationOpen(key, false)
	caret := "chevron-right"
	if open {
		caret = "chevron-down"
	}
	card.Child(el.Div().H(el.Dp(1)).WFull().Bg(seamColor),
		el.Div().ID(key).Role("button").Name("查看原始结果").Focusable(true).CursorPointer().Row().Items(el.Center).Gap(6).MinH(el.Dp(28)).
			OnClick(func() { a.toggleConversation(key, false); a.invalidateConversationEntry(e.key) }).FocusStyle(neutralFocus).
			Child(uiassets.Lucide(cx, caret, 12, theme.Muted), el.Text("原始结果 · JSON").TextSize(11).TextColor(theme.Muted)),
	)
	if open {
		var value any
		text := out.JSON
		if json.Unmarshal([]byte(text), &value) == nil {
			formatted, _ := json.MarshalIndent(value, "", "  ")
			text = string(formatted)
		}
		card.Child(a.toolCodeSection(cx, toolCodeContent{"原始结果", "json", text}, key+"/code"))
	}
	message := el.Div().ID("message/" + e.key).WFull().Gap(14)
	if out.Prose != "" {
		message.Child(a.conversationDocument(cx, &entry{key: e.key + "/prose", text: out.Prose}, false))
	}
	return message.Child(card, a.messageActions(cx, e, false))
}

func (a *app) workflowTransitionCard(cx *el.Context, e *entry) el.Element {
	t := e.transition
	if t == nil {
		return el.Div().Px(12).Py(10).Rounded(10).Bg(theme.Subtle).Child(el.Text(e.text).TextSize(12).TextColor(theme.Text))
	}
	label, icon, tone := outcomeStyle(t.verdict)
	if t.verdict == domainworkflows.VerdictRework {
		label = "返工流转"
	}
	header := el.Div().Row().Wrap().Items(el.Center).Gap(8).Child(
		uiassets.Lucide(cx, icon, 14, tone), el.Text(label).TextSize(12).Medium().TextColor(tone),
		el.Text(t.source).TextSize(12).Medium(),
	)
	if t.target != "" && t.verdict == domainworkflows.VerdictRework {
		header.Child(uiassets.Lucide(cx, "arrow-right", 12, theme.Muted), el.Text(t.target).TextSize(12).Medium())
	}
	header.Child(el.Div().Grow(), el.Text(e.at.Local().Format("15:04")).TextSize(11).TextColor(theme.Muted))
	text := t.feedback
	if text == "" {
		text = t.summary
	}
	key := e.key + "/details"
	open := a.conversationOpen(key, false)
	card := el.Div().ID("message/" + e.key).WFull().P(12).Gap(8).Rounded(10).Bg(mixColor(tone, theme.Surface, .06)).Child(header)
	if text != "" {
		caret := "chevron-right"
		if open {
			caret = "chevron-down"
		}
		card.Child(el.Div().ID(key).Role("button").Name("查看流转详情").Focusable(true).CursorPointer().Row().Items(el.Center).Gap(6).MinH(el.Dp(26)).OnClick(func() { a.toggleConversation(key, false); a.invalidateConversationEntry(e.key) }).FocusStyle(neutralFocus).
			Child(uiassets.Lucide(cx, caret, 12, theme.Muted), el.Text("查看流转详情").TextSize(11).TextColor(theme.Muted)))
		if open {
			card.Child(a.conversationDocument(cx, &entry{key: key + "/text", text: outcomeReadableText(text)}, false))
		}
	}
	return card
}
