package agentrun

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"time"
)

// RunCodexStream drives an already-started app-server. The transport owns the
// remote process: closing it must also stop that process and unblock reads.
// Threads remain resumable on the target; each turn gets a fresh connection.
func RunCodexStream(ctx context.Context, req Request, stream io.ReadWriteCloser, sink Sink) (Result, error) {
	defer stream.Close()
	if sink == nil {
		sink = func(Event) {}
	}
	scanner := bufio.NewScanner(stream)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	p := &codexAppProcess{
		stdin: stream, encoder: json.NewEncoder(stream), scanner: scanner,
		stderr: &lineCapture{}, waitDone: make(chan struct{}),
		stopRemote: func() { _ = stream.Close() },
	}
	r := &CodexRunner{now: time.Now}
	return r.runCodexTurn(ctx, p, req, sink, false)
}
