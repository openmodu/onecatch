package blackboard

import (
	"strings"
	"testing"
)

func TestParseEntriesIgnoresProseOutsideTheFence(t *testing.T) {
	message := `我看了一下调度层，结论如下。

<<<BLACKBOARD>>>
- [decision] 用 FIFO 队列：避免并发恢复冲突
- [risk] 多机恢复：可能需要分布式锁
<<<END>>>

以上，还需要你确认第二点。`
	entries := ParseEntries(message, "step-impl", "codex")
	if len(entries) != 2 {
		t.Fatalf("want 2 entries, got %d: %+v", len(entries), entries)
	}
	if entries[0].Kind != "decision" || entries[0].Title != "用 FIFO 队列" || entries[0].Body != "避免并发恢复冲突" {
		t.Errorf("first entry parsed wrong: %+v", entries[0])
	}
	if entries[1].Kind != "risk" {
		t.Errorf("second entry kind = %q", entries[1].Kind)
	}
}

func TestParseEntriesReturnsNothingWithoutAFence(t *testing.T) {
	// Silence is the common case: an agent with no new conclusion should add
	// nothing, and prose that merely looks like a list must not become entries.
	if got := ParseEntries("- [decision] 这只是正文里的一行", "s", "a"); got != nil {
		t.Errorf("no fence means no entries, got %+v", got)
	}
}

func TestParseEntriesAttributesToTheCaller(t *testing.T) {
	// An agent must not be able to attribute an entry to another step, so
	// origin and actor come from the caller and are never read from the text.
	entries := ParseEntries("<<<BLACKBOARD>>>\n- [note] X：Y\n<<<END>>>", "step-review", "claude")
	if len(entries) != 1 {
		t.Fatalf("want 1 entry, got %d", len(entries))
	}
	if entries[0].Origin != "step-review" || entries[0].Actor != "claude" {
		t.Errorf("attribution = %q/%q", entries[0].Origin, entries[0].Actor)
	}
}

func TestParseEntriesToleratesATruncatedBlock(t *testing.T) {
	// Models get cut off. Dropping everything because the closing marker is
	// missing loses more than it protects.
	entries := ParseEntries("<<<BLACKBOARD>>>\n- [note] 结论一：正文", "s", "a")
	if len(entries) != 1 {
		t.Fatalf("an unterminated block should still yield its lines, got %+v", entries)
	}
}

func TestParseEntriesAcceptsBothColonWidths(t *testing.T) {
	entries := ParseEntries("<<<BLACKBOARD>>>\n- [note] A: B\n- [note] C：D\n<<<END>>>", "s", "a")
	if len(entries) != 2 {
		t.Fatalf("want 2, got %d", len(entries))
	}
	if entries[0].Title != "A" || entries[0].Body != "B" {
		t.Errorf("half-width colon not handled: %+v", entries[0])
	}
	if entries[1].Title != "C" || entries[1].Body != "D" {
		t.Errorf("full-width colon not handled: %+v", entries[1])
	}
}

func TestParseEntriesKeepsATitleOnlyLine(t *testing.T) {
	entries := ParseEntries("<<<BLACKBOARD>>>\n- [risk] 并发恢复未验证\n<<<END>>>", "s", "a")
	if len(entries) != 1 || entries[0].Title != "并发恢复未验证" || entries[0].Body != "" {
		t.Fatalf("a line with no body is still an entry: %+v", entries)
	}
}

func TestParseEntriesSkipsMalformedLines(t *testing.T) {
	block := "<<<BLACKBOARD>>>\n随便一句话\n- 没有类别\n- [note]\n- [note] 好的：内容\n<<<END>>>"
	entries := ParseEntries(block, "s", "a")
	if len(entries) != 1 {
		t.Fatalf("only the well-formed line should survive, got %+v", entries)
	}
}

func TestParseEntriesFallsBackToNoteForAnUnknownKind(t *testing.T) {
	entries := ParseEntries("<<<BLACKBOARD>>>\n- [wharrgarbl] X：Y\n<<<END>>>", "s", "a")
	if len(entries) != 1 || entries[0].Kind != "note" {
		t.Fatalf("unknown kind should degrade to note: %+v", entries)
	}
}

func TestWriteInstructionShowsTheFenceItParses(t *testing.T) {
	// The instruction and the parser must not drift apart.
	if !strings.Contains(WriteInstruction, fenceOpen) || !strings.Contains(WriteInstruction, fenceClose) {
		t.Fatal("the instruction must show the markers the parser looks for")
	}
	for _, kind := range entryKinds {
		if !strings.Contains(WriteInstruction, kind) {
			t.Errorf("instruction does not mention the %q kind", kind)
		}
	}
}

func TestRoundTripRenderThenParse(t *testing.T) {
	// The format an agent reads is the format it writes: a model that echoes
	// the rendered board back must still be parseable.
	board := Board{Entries: []Entry{{Kind: "decision", Actor: "codex", Title: "用 FIFO", Body: "避免冲突"}}}
	rendered := Render(board)
	var lines []string
	for _, line := range strings.Split(rendered, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "- [") {
			// Render appends a "（来源 …）" suffix; the parser keeps it in the
			// body rather than choking, which is what matters here.
			lines = append(lines, line)
		}
	}
	entries := ParseEntries(fenceOpen+"\n"+strings.Join(lines, "\n")+"\n"+fenceClose, "s", "a")
	if len(entries) != 1 {
		t.Fatalf("a rendered board should parse back, got %+v", entries)
	}
	if entries[0].Kind != "decision" || entries[0].Title != "用 FIFO" {
		t.Errorf("round trip lost the entry: %+v", entries[0])
	}
}
