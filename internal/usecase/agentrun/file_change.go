package agentrun

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// FileChange is one file in a file_change event. Diff holds unified diff
// lines: hunks from runtimes that report them, or ones built here from the
// whole-file content or edit arguments other runtimes report instead, so the
// UI reads a single shape. A hunk built without the file has "@@ @@" for a
// header, because the edit's position in the file is unknown.
type FileChange struct {
	Path     string `json:"path"`
	Kind     string `json:"kind,omitempty"`
	MovePath string `json:"movePath,omitempty"`
	Diff     string `json:"diff,omitempty"`
	// Truncated reports that Diff was cut at maxFileChangeDiff.
	Truncated bool `json:"truncated,omitempty"`
}

// maxFileChangeDiff bounds what one change persists. A generated file can be
// megabytes; the transcript only has to show what happened.
const maxFileChangeDiff = 256 << 10

// NewFileChange normalizes a runtime's change record. Codex and TRAE send a
// hunk for an update but the whole file for an add or a delete.
func NewFileChange(path, kind, movePath, diff string) FileChange {
	if diff != "" && (kind == "add" || kind == "delete") && !isUnifiedDiff(diff) {
		diff = wholeFileHunk(diff, kind == "add")
	}
	return boundedFileChange(FileChange{Path: path, Kind: kind, MovePath: movePath, Diff: diff})
}

func boundedFileChange(change FileChange) FileChange {
	if len(change.Diff) > maxFileChangeDiff {
		cut := strings.LastIndexByte(change.Diff[:maxFileChangeDiff], '\n')
		if cut < 0 {
			cut = maxFileChangeDiff
			for cut > 0 && !utf8.RuneStart(change.Diff[cut]) {
				cut--
			}
		}
		change.Diff, change.Truncated = change.Diff[:cut], true
	}
	return change
}

func isUnifiedDiff(diff string) bool {
	return strings.HasPrefix(diff, "@@") || strings.HasPrefix(diff, "--- ") || strings.HasPrefix(diff, "diff --git ")
}

func wholeFileHunk(content string, add bool) string {
	lines := strings.Split(strings.TrimSuffix(content, "\n"), "\n")
	mark, header := "+", fmt.Sprintf("@@ -0,0 +1,%d @@", len(lines))
	if !add {
		mark, header = "-", fmt.Sprintf("@@ -1,%d +0,0 @@", len(lines))
	}
	var b strings.Builder
	b.Grow(len(content) + len(lines) + len(header))
	b.WriteString(header)
	for _, line := range lines {
		b.WriteByte('\n')
		b.WriteString(mark)
		b.WriteString(line)
	}
	return b.String()
}

// replacementHunk shows an edit's old text replaced by its new text. Lines
// both share at either end stay as context, so a one-line change inside a
// larger snippet reads as one line.
func replacementHunk(oldText, newText string) string {
	split := func(s string) []string {
		if s == "" {
			return nil
		}
		return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
	}
	before, after := split(oldText), split(newText)
	head := 0
	for head < len(before) && head < len(after) && before[head] == after[head] {
		head++
	}
	tail := 0
	for tail < len(before)-head && tail < len(after)-head && before[len(before)-1-tail] == after[len(after)-1-tail] {
		tail++
	}
	var b strings.Builder
	b.WriteString("@@ @@")
	write := func(mark byte, lines []string) {
		for _, line := range lines {
			b.WriteByte('\n')
			b.WriteByte(mark)
			b.WriteString(line)
		}
	}
	write(' ', before[:head])
	write('-', before[head:len(before)-tail])
	write('+', after[head:len(after)-tail])
	write(' ', before[len(before)-tail:])
	return b.String()
}

// toolInputFileChange describes a write or edit tool call from its
// arguments. Harnesses spell the edit fields several ways; Pi also batches
// several replacements into one call.
func toolInputFileChange(input map[string]any) (FileChange, bool) {
	path := toolInputPath(input)
	if path == "" {
		return FileChange{}, false
	}
	text := func(m map[string]any, keys ...string) (string, bool) {
		for _, key := range keys {
			if value, ok := m[key].(string); ok {
				return value, true
			}
		}
		return "", false
	}
	oldKeys := []string{"old_string", "old_str", "oldText", "old_text"}
	newKeys := []string{"new_string", "new_str", "newText", "new_text"}
	if edits, ok := input["edits"].([]any); ok && len(edits) > 0 {
		hunks := make([]string, 0, len(edits))
		for _, item := range edits {
			edit, _ := item.(map[string]any)
			oldText, _ := text(edit, oldKeys...)
			newText, hasNew := text(edit, newKeys...)
			if hasNew {
				hunks = append(hunks, replacementHunk(oldText, newText))
			}
		}
		return boundedFileChange(FileChange{Path: path, Kind: "update", Diff: strings.Join(hunks, "\n")}), true
	}
	if newText, ok := text(input, newKeys...); ok {
		oldText, _ := text(input, oldKeys...)
		return boundedFileChange(FileChange{Path: path, Kind: "update", Diff: replacementHunk(oldText, newText)}), true
	}
	if content, ok := text(input, "content", "file_text"); ok {
		// A write replaces the file; what it held before is not in the call.
		return boundedFileChange(FileChange{Path: path, Kind: "write", Diff: wholeFileHunk(content, true)}), true
	}
	return FileChange{Path: path}, true
}

// acpFileChanges reads the diff blocks of an ACP tool call's content. A
// missing oldText is the protocol's spelling of a new file.
func acpFileChanges(raw json.RawMessage) []FileChange {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '[' {
		return nil
	}
	var blocks []struct {
		Type    string  `json:"type"`
		Path    string  `json:"path"`
		OldText *string `json:"oldText"`
		NewText string  `json:"newText"`
	}
	if json.Unmarshal(trimmed, &blocks) != nil {
		return nil
	}
	var changes []FileChange
	for _, block := range blocks {
		if block.Type != "diff" || block.Path == "" {
			continue
		}
		if block.OldText == nil {
			changes = append(changes, NewFileChange(block.Path, "add", "", block.NewText))
			continue
		}
		changes = append(changes, boundedFileChange(FileChange{Path: block.Path, Kind: "update", Diff: replacementHunk(*block.OldText, block.NewText)}))
	}
	return changes
}

// ACP may send a diff only in a later tool_call_update and need not include
// locations. Keep one stable record per tool so those updates replace the
// path-only preview, including a later failure.
func (c *acpClient) emitFileChanges(update acpUpdate, raw string, at time.Time, sink Sink) {
	changes := acpFileChanges(update.Content)
	previous := c.toolChanges[update.ToolCallID]
	if len(changes) == 0 && len(previous) > 0 {
		if update.Status != "failed" {
			return
		}
		changes = previous
	}
	if len(changes) == 0 && update.SessionUpdate == "tool_call" && (update.Kind == "edit" || update.Kind == "delete" || update.Kind == "move") {
		for _, location := range update.Locations {
			if location.Path != "" {
				changes = append(changes, FileChange{Path: location.Path})
			}
		}
	}
	if len(changes) == 0 {
		return
	}
	if len(previous) > 0 && update.Status != "failed" {
		// Updates may carry only one file from a multi-file tool call.
		merged := append([]FileChange(nil), previous...)
		for _, change := range changes {
			found := false
			for i := range merged {
				if merged[i].Path == change.Path {
					merged[i], found = change, true
					break
				}
			}
			if !found {
				merged = append(merged, change)
			}
		}
		changes = merged
	}
	if c.toolChanges == nil {
		c.toolChanges = map[string][]FileChange{}
	}
	c.toolChanges[update.ToolCallID] = changes
	id := ""
	if update.ToolCallID != "" {
		id = c.streamID("files-" + update.ToolCallID)
	}
	sink(Event{Kind: KindFileChange, StreamID: id, Text: FileChangePaths(changes), FileChanges: changes, Failed: update.Status == "failed", Raw: raw, At: at})
}

// FileChangePaths lists the changed paths one per line, as Text carries them.
func FileChangePaths(changes []FileChange) string {
	paths := make([]string, 0, len(changes))
	for _, change := range changes {
		if change.Path != "" {
			paths = append(paths, change.Path)
		}
	}
	return strings.Join(paths, "\n")
}

// FileChangesFromEvent also reads the original runtime payload in older
// history. Before FileChanges was stored, Text kept only the paths but Raw
// often still contained the exact edit. Never read today's file to fill it in.
func FileChangesFromEvent(event Event) []FileChange {
	if len(event.FileChanges) > 0 || event.Kind != KindFileChange || event.Raw == "" {
		return event.FileChanges
	}
	var raw struct {
		Params json.RawMessage `json:"params"`
		Item   struct {
			Changes []appServerFileChange `json:"changes"`
		} `json:"item"`
		Update struct {
			Content json.RawMessage `json:"content"`
		} `json:"update"`
		Args map[string]any `json:"args"`
		Data struct {
			Arguments string `json:"arguments"`
		} `json:"data"`
	}
	if json.Unmarshal([]byte(event.Raw), &raw) != nil {
		return nil
	}
	if len(raw.Params) > 0 {
		params := raw.Params
		if json.Unmarshal(params, &raw) != nil {
			return nil
		}
	}
	changes := appServerFileChanges(raw.Item.Changes)
	if len(changes) == 0 {
		changes = acpFileChanges(raw.Update.Content)
	}
	if len(changes) == 0 {
		input := raw.Args
		if input == nil && raw.Data.Arguments != "" {
			_ = json.Unmarshal([]byte(raw.Data.Arguments), &input)
		}
		if change, ok := toolInputFileChange(input); ok {
			changes = []FileChange{change}
		}
	}
	// ACP used to emit one event per location with the same raw payload.
	// Restrict recovered changes to that event's paths to avoid duplicating
	// every file in every old card.
	paths := strings.Split(event.Text, "\n")
	var filtered []FileChange
	for _, change := range changes {
		for _, path := range paths {
			if change.Path == strings.TrimSpace(path) {
				filtered = append(filtered, change)
				break
			}
		}
	}
	return filtered
}
