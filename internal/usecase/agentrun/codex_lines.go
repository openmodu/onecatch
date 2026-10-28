package agentrun

import (
	"bufio"
	"io"
	"strings"
)

// Resume responses contain the conversation history in one JSONL frame. Read
// complete frames without imposing Scanner's token limit or retaining a large
// scanner buffer for the lifetime of a warm app-server.
type codexLineReader struct {
	reader *bufio.Reader
	line   string
	err    error
}

func newCodexLineReader(r io.Reader) *codexLineReader {
	return &codexLineReader{reader: bufio.NewReader(r)}
}

func (r *codexLineReader) Scan() bool {
	if r.err != nil {
		r.line = ""
		return false
	}
	r.line, r.err = r.reader.ReadString('\n')
	r.line = strings.TrimSuffix(strings.TrimSuffix(r.line, "\n"), "\r")
	return len(r.line) > 0 || r.err == nil
}
func (r *codexLineReader) Text() string { return r.line }
func (r *codexLineReader) Err() error {
	if r.err == io.EOF {
		return nil
	}
	return r.err
}
