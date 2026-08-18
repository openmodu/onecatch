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

// ParseOutcome accepts a JSON object, a single JSON-fenced block, or a terminal
// JSON object after provider prose. Content after the object is rejected:
// orchestration must never infer control flow from the middle of a response.
func ParseOutcome(text string) (Outcome, error) {
	raw, err := unwrapOutcomeJSON(strings.TrimSpace(text))
	if err != nil {
		return Outcome{}, err
	}
	dec := json.NewDecoder(bytes.NewBufferString(raw))
	dec.DisallowUnknownFields()
	var outcome Outcome
	if err := dec.Decode(&outcome); err != nil {
		return Outcome{}, fmt.Errorf("%w: decode JSON: %v", ErrProtocol, err)
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return Outcome{}, fmt.Errorf("%w: multiple JSON values", ErrProtocol)
		}
		return Outcome{}, fmt.Errorf("%w: trailing content: %v", ErrProtocol, err)
	}
	if !identifierPattern.MatchString(outcome.Signal) {
		return Outcome{}, fmt.Errorf("%w: signal must be a lowercase identifier", ErrProtocol)
	}
	if strings.TrimSpace(outcome.Content) == "" {
		return Outcome{}, fmt.Errorf("%w: content is required", ErrProtocol)
	}
	return outcome, nil
}

func unwrapOutcomeJSON(text string) (string, error) {
	if text == "" {
		return "", fmt.Errorf("%w: response is empty", ErrProtocol)
	}
	if !strings.HasPrefix(text, "```") {
		if strings.HasPrefix(text, "{") {
			return text, nil
		}
		if terminal := terminalJSONObject(text); terminal != "" {
			return terminal, nil
		}
		return text, nil
	}
	lines := strings.Split(text, "\n")
	if len(lines) < 3 || (lines[0] != "```json" && lines[0] != "```") || lines[len(lines)-1] != "```" {
		return "", fmt.Errorf("%w: expected a single JSON fenced block", ErrProtocol)
	}
	body := strings.TrimSpace(strings.Join(lines[1:len(lines)-1], "\n"))
	if body == "" || strings.Contains(body, "```") {
		return "", fmt.Errorf("%w: malformed JSON fenced block", ErrProtocol)
	}
	return body, nil
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
