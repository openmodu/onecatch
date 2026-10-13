package workflows

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

var ErrProtocol = errors.New("workflow outcome protocol error")

// OutcomeMessage retains the provider's prose and original JSON alongside the
// validated outcome, so presentation and orchestration use the same parser.
type OutcomeMessage struct {
	Outcome
	Prose string
	JSON  string
}

// ParseOutcome accepts a terminal JSON object or JSON-fenced block, optionally
// following provider prose. Content after the object is rejected:
// orchestration must never infer control flow from the middle of a response.
func ParseOutcome(text string) (Outcome, error) {
	message, err := ParseOutcomeMessage(text)
	return message.Outcome, err
}

// ParseOutcomeMessage parses the same protocol and retains its display content.
func ParseOutcomeMessage(text string) (OutcomeMessage, error) {
	prose, raw, err := splitOutcomeJSON(strings.TrimSpace(text))
	if err != nil {
		return OutcomeMessage{}, err
	}
	dec := json.NewDecoder(bytes.NewBufferString(raw))
	var outcome Outcome
	if err := dec.Decode(&outcome); err != nil {
		return OutcomeMessage{}, fmt.Errorf("%w: decode JSON: %v", ErrProtocol, err)
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return OutcomeMessage{}, fmt.Errorf("%w: multiple JSON values", ErrProtocol)
		}
		return OutcomeMessage{}, fmt.Errorf("%w: trailing content: %v", ErrProtocol, err)
	}
	outcome.Verdict = strings.TrimSpace(outcome.Verdict)
	outcome.Target = strings.TrimSpace(outcome.Target)
	switch outcome.Verdict {
	case VerdictPass, VerdictRework, VerdictAskHuman:
	default:
		return OutcomeMessage{}, fmt.Errorf("%w: verdict must be pass, rework or ask_human", ErrProtocol)
	}
	if strings.TrimSpace(outcome.Summary) == "" {
		outcome.Summary = outcome.Feedback
	}
	if strings.TrimSpace(outcome.Summary) == "" {
		return OutcomeMessage{}, fmt.Errorf("%w: summary is required", ErrProtocol)
	}
	if outcome.Verdict == VerdictRework && strings.TrimSpace(outcome.Feedback) == "" {
		outcome.Feedback = outcome.Summary
	}
	return OutcomeMessage{Outcome: outcome, Prose: prose, JSON: raw}, nil
}

func splitOutcomeJSON(text string) (prose, raw string, err error) {
	if text == "" {
		return "", "", fmt.Errorf("%w: response is empty", ErrProtocol)
	}
	if strings.HasPrefix(text, "{") {
		return "", text, nil
	}
	// A terminal JSON fence may follow a report containing Markdown/code.
	if strings.HasSuffix(text, "\n```") {
		end := len(text) - len("\n```")
		start := strings.LastIndex(text[:end], "\n```")
		if start >= 0 {
			start++
		} else if strings.HasPrefix(text, "```") {
			start = 0
		}
		if start >= 0 {
			lines := strings.SplitN(text[start:end], "\n", 2)
			if len(lines) == 2 && (strings.TrimSpace(lines[0]) == "```json" || strings.TrimSpace(lines[0]) == "```") {
				body := strings.TrimSpace(lines[1])
				if body != "" && !outcomeFenceOpen(text[:start]) {
					return strings.TrimSpace(text[:start]), body, nil
				}
			}
		}
	}
	if terminal := terminalJSONObject(text); terminal != "" {
		prose := strings.TrimSpace(text[:len(text)-len(terminal)])
		if !outcomeFenceOpen(prose) {
			return prose, terminal, nil
		}
	}
	return "", text, nil
}

func outcomeFenceOpen(text string) bool {
	var fence string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if fence != "" {
			if line == fence {
				fence = ""
			}
			continue
		}
		for _, marker := range []string{"```", "~~~"} {
			if strings.HasPrefix(line, marker) {
				length := len(marker)
				for length < len(line) && line[length] == marker[0] {
					length++
				}
				fence = line[:length]
				break
			}
		}
	}
	return fence != ""
}

// Some runtimes preserve a short assistant handoff before the outcome despite
// being instructed to return only JSON. Accept that provider quirk only when a
// complete JSON object is the terminal value. Trailing prose remains invalid,
// so orchestration never guesses a control signal from the middle of a reply.
func terminalJSONObject(text string) string {
	for index := strings.LastIndex(text, "{"); index >= 0; index = strings.LastIndex(text[:index], "{") {
		candidate := strings.TrimSpace(text[index:])
		dec := json.NewDecoder(strings.NewReader(candidate))
		var value map[string]any
		if err := dec.Decode(&value); err != nil {
			continue
		}
		var extra any
		if err := dec.Decode(&extra); errors.Is(err, io.EOF) {
			return candidate
		}
	}
	return ""
}
