package blackboard

import (
	"strings"
)

// The agent writes to the board by emitting a fenced block. The line shape is
// the same one Render produces, so the format an agent reads is the format it
// writes — there is no second syntax to learn, and a model that echoes the
// board back is still parseable.
const (
	fenceOpen  = "<<<BLACKBOARD>>>"
	fenceClose = "<<<END>>>"
)

// WriteInstruction is appended to the prompt to tell an agent how to contribute.
// It is deliberately explicit about the board being shared and about silence
// being acceptable: an agent that writes an entry per turn floods the board for
// everyone behind it.
const WriteInstruction = `若本轮得出了对后续步骤有用的结论，写入共享白板。仅在确有新增结论时输出，没有就完全省略：

` + fenceOpen + `
- [decision] 标题：正文
- [risk] 标题：正文
` + fenceClose + `

类别限 decision / risk / question / note。一条一行，正文写在冒号后。`

// ParseEntries extracts board entries from an agent's message. Everything
// outside the fence is ignored, so ordinary prose in the same message is safe.
// Origin and actor are supplied by the caller rather than parsed — an agent must
// not be able to attribute an entry to another step.
func ParseEntries(message, origin, actor string) []Entry {
	block, ok := fencedBlock(message)
	if !ok {
		return nil
	}
	var entries []Entry
	for _, line := range strings.Split(block, "\n") {
		entry, ok := parseEntryLine(line)
		if !ok {
			continue
		}
		entry.Origin = origin
		entry.Actor = actor
		entries = append(entries, entry)
	}
	return entries
}

// fencedBlock returns the text between the first fence pair. A block that opens
// and never closes is taken to run to the end of the message: models truncate,
// and dropping everything the agent wrote because the closing marker was cut off
// loses more than it protects.
func fencedBlock(message string) (string, bool) {
	start := strings.Index(message, fenceOpen)
	if start < 0 {
		return "", false
	}
	rest := message[start+len(fenceOpen):]
	if end := strings.Index(rest, fenceClose); end >= 0 {
		return rest[:end], true
	}
	return rest, true
}

// parseEntryLine reads "- [kind] title：body". The separator may be either the
// full-width colon the instruction shows or a plain one, since models mix them.
func parseEntryLine(line string) (Entry, bool) {
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "-")
	line = strings.TrimPrefix(line, "*")
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "[") {
		return Entry{}, false
	}
	close := strings.Index(line, "]")
	if close < 0 {
		return Entry{}, false
	}
	kind := normalizeEnum(line[1:close], entryKinds, "note")
	body := strings.TrimSpace(line[close+1:])
	if body == "" {
		return Entry{}, false
	}
	title, detail := splitTitle(body)
	return Entry{Kind: kind, Title: title, Body: detail}, true
}

func splitTitle(text string) (string, string) {
	for _, sep := range []string{"：", ":"} {
		if i := strings.Index(text, sep); i >= 0 {
			title := strings.TrimSpace(text[:i])
			detail := strings.TrimSpace(text[i+len(sep):])
			if title != "" {
				return title, detail
			}
		}
	}
	return text, ""
}
