package agentrun

import (
	"strings"
	"testing"
)

func TestCodexReadLargeResumeFrame(t *testing.T) {
	frame := `{"id":3,"result":{"thread":{"id":"long-thread","history":"` + strings.Repeat("x", 9*1024*1024) + `"}}}`
	p := &codexAppProcess{scanner: newCodexLineReader(strings.NewReader(frame + "\n" + `{"id":4,"result":{}}`))}
	reads := p.readLines()
	first := <-reads
	if first.err != nil || first.line != frame {
		t.Fatalf("large resume frame lost: len=%d err=%v", len(first.line), first.err)
	}
	next := <-reads
	if next.err != nil || next.line != `{"id":4,"result":{}}` {
		t.Fatalf("following frame: %+v", next)
	}
	if _, ok := <-reads; ok {
		t.Fatal("expected end of stream")
	}
}
