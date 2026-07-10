package workflows

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	domainworkflows "github.com/openmodu/oneshot/internal/domain/workflows"
	"github.com/openmodu/oneshot/pkg/localfile"
)

const maxRuntimeEventLine = 8 * 1024 * 1024

type runtimeEventStore struct {
	root    string
	mu      sync.Mutex
	nextSeq map[string]int64
}

func newRuntimeEventStore(root string) *runtimeEventStore {
	return &runtimeEventStore{root: root, nextSeq: make(map[string]int64)}
}

func (s *runtimeEventStore) append(ctx context.Context, runID, stepRunID string, payload json.RawMessage, at time.Time) (domainworkflows.RuntimeEvent, error) {
	path, err := s.path(runID, stepRunID)
	if err != nil {
		return domainworkflows.RuntimeEvent{}, err
	}
	if !json.Valid(payload) {
		return domainworkflows.RuntimeEvent{}, errors.New("runtime event payload is invalid JSON")
	}
	if err := ctx.Err(); err != nil {
		return domainworkflows.RuntimeEvent{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := localfile.TrimIncompleteJSONLTail(path); err != nil {
		return domainworkflows.RuntimeEvent{}, err
	}
	if _, known := s.nextSeq[path]; !known {
		last, err := lastRuntimeEventSeq(path)
		if err != nil {
			return domainworkflows.RuntimeEvent{}, err
		}
		s.nextSeq[path] = last + 1
	}
	event := domainworkflows.RuntimeEvent{
		Seq:     s.nextSeq[path],
		At:      at,
		Payload: append(json.RawMessage(nil), payload...),
	}
	if err := localfile.AppendJSONLine(path, event); err != nil {
		delete(s.nextSeq, path)
		return domainworkflows.RuntimeEvent{}, fmt.Errorf("append runtime event: %w", err)
	}
	s.nextSeq[path]++
	return event, nil
}

func (s *runtimeEventStore) list(ctx context.Context, runID, stepRunID string, afterSeq int64, limit int) ([]domainworkflows.RuntimeEvent, error) {
	path, err := s.path(runID, stepRunID)
	if err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 10_000 {
		limit = 10_000
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := localfile.TrimIncompleteJSONLTail(path); err != nil {
		return nil, err
	}
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return []domainworkflows.RuntimeEvent{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open runtime event stream: %w", err)
	}
	defer file.Close()

	var out []domainworkflows.RuntimeEvent
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), maxRuntimeEventLine)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var event domainworkflows.RuntimeEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return nil, fmt.Errorf("decode runtime event line %d: %w", lineNumber, err)
		}
		if event.Seq <= afterSeq {
			continue
		}
		out = append(out, event)
		if len(out) >= limit {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read runtime event stream: %w", err)
	}
	return out, nil
}

func (s *runtimeEventStore) path(runID, stepRunID string) (string, error) {
	if !validPathSegment(runID) || !validPathSegment(stepRunID) {
		return "", errors.New("run and step run IDs must be safe path segments")
	}
	return filepath.Join(s.root, runID, "steps", stepRunID, "events.jsonl"), nil
}

func validPathSegment(value string) bool {
	return localfile.ValidID(value)
}

func lastRuntimeEventSeq(path string) (int64, error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("open runtime event stream for recovery: %w", err)
	}
	defer file.Close()
	var last int64
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), maxRuntimeEventLine)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		var event domainworkflows.RuntimeEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return 0, fmt.Errorf("decode runtime event line %d during recovery: %w", lineNumber, err)
		}
		if event.Seq <= last {
			return 0, fmt.Errorf("runtime event sequence is not increasing at line %d", lineNumber)
		}
		last = event.Seq
	}
	if err := scanner.Err(); err != nil {
		return 0, fmt.Errorf("scan runtime event stream during recovery: %w", err)
	}
	return last, nil
}
