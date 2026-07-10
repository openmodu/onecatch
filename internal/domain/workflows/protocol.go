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

// ParseOutcome accepts either a single JSON object or a single json fenced
// block. Extra prose is rejected: orchestration must never infer control flow
// from an ambiguous model response.
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
