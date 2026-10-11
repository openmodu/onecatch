package workflows

import (
	"fmt"
	"strings"

	domaintasks "github.com/openmodu/onecatch/internal/domain/tasks"
	domainworkflows "github.com/openmodu/onecatch/internal/domain/workflows"
)

// composeStepPrompt builds the turn sent to an agent step. The first turn of a
// step carries the whole brief. Later turns resume the step's own session,
// which already holds the brief and the step's earlier work, so they carry only
// what changed: inbox messages and the latest upstream results.
func composeStepPrompt(task domaintasks.Task, definition domainworkflows.Definition, step domainworkflows.Step, run domainworkflows.Run) string {
	inbox := run.Nodes[step.ID].Inbox
	if directAgentDefinition(definition) {
		return composeDirectAgentPrompt(task, step, run, messageTexts(inbox))
	}
	resumed := run.Sessions[step.ID] != ""
	var parts []string
	if resumed {
		parts = append(parts, fmt.Sprintf("# Continue workflow step `%s` (%s)", step.ID, step.Name))
		if len(inbox) == 0 {
			parts = append(parts, "", "Continue this step where you left off.")
		}
	} else {
		parts = append(parts, fmt.Sprintf("# OneCatch workflow step `%s` (%s)", step.ID, step.Name), "", "## Task", task.Prompt)
		parts = appendTaskAttachments(parts, task)
		parts = append(parts, "", "## Your step")
		if prompt := strings.TrimSpace(step.Prompt); prompt != "" {
			parts = append(parts, prompt)
		} else {
			parts = append(parts, "Complete your part of the task above.")
		}
		parts = append(parts, "", "## Workflow", describeNeighbours(definition, step))
	}
	if len(inbox) > 0 {
		parts = append(parts, "", "## Messages for you")
		for _, message := range inbox {
			parts = append(parts, "", "### "+messageHeading(definition, message), message.Text)
		}
	}
	if upstream := domainworkflows.Upstream(definition, step.ID); len(upstream) > 0 {
		heading := "## Results from the steps before you"
		if resumed {
			heading = "## Latest results from the steps before you"
		}
		parts = append(parts, "", heading)
		for _, id := range upstream {
			parts = append(parts, "", "### "+stepLabel(definition, id), run.Nodes[id].Summary)
		}
	}
	parts = append(parts, "", "## Response contract", responseContract(definition, step))
	return strings.Join(parts, "\n") + "\n"
}

func describeNeighbours(definition domainworkflows.Definition, step domainworkflows.Step) string {
	var lines []string
	if upstream := domainworkflows.Upstream(definition, step.ID); len(upstream) > 0 {
		lines = append(lines, "- Runs after: "+stepLabels(definition, upstream))
	}
	if downstream := domainworkflows.Downstream(definition, step.ID); len(downstream) > 0 {
		lines = append(lines, "- Hands off to: "+stepLabels(definition, downstream))
	}
	var reviewers []string
	for _, edge := range definition.Edges {
		if edge.Kind == domainworkflows.EdgeRework && edge.To == step.ID {
			reviewers = append(reviewers, edge.From)
		}
	}
	if len(reviewers) > 0 {
		lines = append(lines, "- May send work back to you: "+stepLabels(definition, reviewers)+". Their feedback will arrive in this same conversation.")
	}
	if len(lines) == 0 {
		return "- This step works on its own."
	}
	return strings.Join(lines, "\n")
}

func responseContract(definition domainworkflows.Definition, step domainworkflows.Step) string {
	rework := domainworkflows.ReworkEdges(definition, step.ID)
	lines := []string{
		"When the step is finished, end your reply with exactly one JSON object and nothing after it:",
		`{"verdict":"pass","summary":"what you did and what the next step needs to know"}`,
		"",
		"Verdicts:",
		"- `pass`: this step is done.",
	}
	if len(rework) > 0 {
		targets := make([]string, 0, len(rework))
		for _, edge := range rework {
			targets = append(targets, fmt.Sprintf("`%s` (%s)", edge.To, stepName(definition, edge.To)))
		}
		lines = append(lines, "- `rework`: the work is not acceptable and must go back. Allowed targets: "+strings.Join(targets, ", ")+`. Add "target" and "feedback"; the feedback is all the target sees from you, so make it concrete and actionable.`)
		if len(rework) == 1 {
			lines = append(lines, fmt.Sprintf(`  Example: {"verdict":"rework","target":"%s","summary":"one-line reason","feedback":"what to change and why"}`, rework[0].To))
		}
	}
	lines = append(lines, "- `ask_human`: you need a decision only a person can make; put the question in summary.")
	if len(rework) == 0 {
		lines = append(lines, "", "If you finish without the JSON object, your final reply is taken as a pass and used as your summary.")
	}
	return strings.Join(lines, "\n")
}

func messageHeading(definition domainworkflows.Definition, message domainworkflows.Message) string {
	switch message.From {
	case domainworkflows.MessageFromHuman:
		return "From a person"
	case "onecatch":
		return "From OneCatch"
	default:
		return "Rework feedback from " + stepLabel(definition, message.From)
	}
}

func messageTexts(messages []domainworkflows.Message) string {
	texts := make([]string, 0, len(messages))
	for _, message := range messages {
		texts = append(texts, message.Text)
	}
	return strings.Join(texts, "\n\n---\n\n")
}

func stepName(definition domainworkflows.Definition, id string) string {
	if step, ok := domainworkflows.FindStep(definition, id); ok {
		return step.Name
	}
	return id
}

func stepLabel(definition domainworkflows.Definition, id string) string {
	return fmt.Sprintf("`%s` (%s)", id, stepName(definition, id))
}

func stepLabels(definition domainworkflows.Definition, ids []string) string {
	labels := make([]string, 0, len(ids))
	for _, id := range ids {
		labels = append(labels, stepLabel(definition, id))
	}
	return strings.Join(labels, ", ")
}

func directAgentDefinition(definition domainworkflows.Definition) bool {
	return definition.ID == directAgentWorkflowID && len(definition.Steps) == 1
}

func composeDirectAgentPrompt(task domaintasks.Task, step domainworkflows.Step, run domainworkflows.Run, instruction string) string {
	taskPrompt := strings.TrimSpace(task.Prompt)
	humanInstruction := strings.TrimSpace(instruction)
	if run.Sessions[step.ID] != "" {
		if humanInstruction != "" {
			return humanInstruction
		}
		return "Continue the task."
	}

	prompt := taskPrompt
	if humanInstruction != "" && humanInstruction != taskPrompt {
		prompt = strings.TrimSpace(prompt + "\n\n" + humanInstruction)
	}
	if len(task.Attachments) == 0 {
		return prompt
	}
	parts := []string{prompt, "", "Attachments:"}
	for _, attachment := range task.Attachments {
		parts = append(parts, fmt.Sprintf("- %s: %s", attachment.Name, attachment.StoredPath))
	}
	return strings.Join(parts, "\n")
}

func appendTaskAttachments(parts []string, task domaintasks.Task) []string {
	if len(task.Attachments) == 0 {
		return parts
	}
	parts = append(parts, "", "## Task attachments")
	for _, attachment := range task.Attachments {
		parts = append(parts, fmt.Sprintf("- %s: %s", attachment.Name, attachment.StoredPath))
	}
	parts = append(parts, "Read these files when they are relevant to the task.")
	return parts
}
