package agentrun

import (
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
	scanner := newCodexLineReader(stream)
	p := &codexAppProcess{
		stdin: stream, encoder: json.NewEncoder(stream), scanner: scanner,
		stderr: &lineCapture{}, waitDone: make(chan struct{}),
		stopRemote: func() { _ = stream.Close() },
	}
	defer p.stop()
	r := &CodexRunner{now: time.Now}
	return r.runCodexTurn(ctx, p, req, sink, false)
}
