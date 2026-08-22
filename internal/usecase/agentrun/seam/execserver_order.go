package seam

import (
	"encoding/json"
	"sync"
)

// requestSequencer preserves wire order for operations on one path, handle,
// or stdin while allowing independent remote file reads to overlap.
type requestSequencer struct {
	mu    sync.Mutex
	tails map[string]chan struct{}
}

func newRequestSequencer() *requestSequencer {
	return &requestSequencer{tails: map[string]chan struct{}{}}
}

type requestSlot struct {
	sequencer *requestSequencer
	key       string
	after     chan struct{}
	current   chan struct{}
}

func (s *requestSequencer) enter(key string) *requestSlot {
	if key == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current := make(chan struct{})
	slot := &requestSlot{sequencer: s, key: key, after: s.tails[key], current: current}
	s.tails[key] = current
	return slot
}

func (s *requestSlot) wait() {
	if s != nil && s.after != nil {
		<-s.after
	}
}

func (s *requestSlot) done() {
	if s == nil {
		return
	}
	s.sequencer.mu.Lock()
	if s.sequencer.tails[s.key] == s.current {
		delete(s.sequencer.tails, s.key)
	}
	s.sequencer.mu.Unlock()
	close(s.current)
}

func execServerOrderKey(method string, params json.RawMessage) string {
	var field, prefix string
	switch method {
	case "fs/readFile", "fs/writeFile", "fs/createDirectory", "fs/getMetadata",
		"fs/canonicalize", "fs/readDirectory", "fs/remove":
		field, prefix = "path", "path:"
	case "fs/copy":
		field, prefix = "destinationPath", "path:"
	case "fs/open", "fs/readBlock", "fs/close":
		field, prefix = "handleId", "handle:"
	case "process/write":
		field, prefix = "processId", "process:"
	default:
		return ""
	}
	var values map[string]json.RawMessage
	if json.Unmarshal(params, &values) != nil {
		return ""
	}
	var value string
	if json.Unmarshal(values[field], &value) != nil || value == "" {
		return ""
	}
	return prefix + value
}
