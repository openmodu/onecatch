package execution

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"github.com/openmodu/onecatch/internal/usecase/agentrun"
)

// runFileName is where each order's run log is persisted, inside the order's
// managed metadata directory. It is a dotfile so the artifact collector skips
// it when walking the workspace.
const runFileName = ".onecatch-run.json"

// runStore keeps order run logs in memory and mirrors terminal state to disk so
// the run history (and the session id needed to resume a task) survives a
// server restart. Per-event updates stay in memory; persistence happens at
// turn boundaries to avoid writing on every streamed line.
type runStore struct {
	root string

	mu    sync.Mutex
	cache map[string]*RunLog
}

func newRunStore(root string) *runStore {
	return &runStore{root: root, cache: make(map[string]*RunLog)}
}

func (s *runStore) dir(orderID string) string  { return filepath.Join(s.root, orderID) }
func (s *runStore) path(orderID string) string { return filepath.Join(s.dir(orderID), runFileName) }

// get returns a copy of the order's run log, loading it from disk if it is not
// already cached (e.g. after a restart).
func (s *runStore) get(orderID string) (RunLog, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	log := s.resolve(orderID)
	if log == nil {
		return RunLog{}, false
	}
	return copyLog(log), true
}

// update mutates the order's run log under lock. When persist is true the
// result is flushed to disk (best-effort).
func (s *runStore) update(orderID string, persist bool, fn func(*RunLog)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	log := s.resolve(orderID)
	if log == nil {
		log = &RunLog{OrderID: orderID}
		s.cache[orderID] = log
	}
	fn(log)
	if persist {
		s.saveDisk(orderID, log)
	}
}

// resolve returns the cached log, hydrating from disk if needed. Caller holds mu.
func (s *runStore) resolve(orderID string) *RunLog {
	if log, ok := s.cache[orderID]; ok {
		return log
	}
	if log, ok := s.loadDisk(orderID); ok {
		s.cache[orderID] = &log
		return &log
	}
	return nil
}

func (s *runStore) loadDisk(orderID string) (RunLog, bool) {
	data, err := os.ReadFile(s.path(orderID))
	if err != nil {
		return RunLog{}, false
	}
	var log RunLog
	if err := json.Unmarshal(data, &log); err != nil {
		return RunLog{}, false
	}
	return log, true
}

// saveDisk writes the run log atomically (temp file + rename). Failures are
// swallowed: persistence is a durability bonus, not required for a run to work.
func (s *runStore) saveDisk(orderID string, log *RunLog) {
	if err := os.MkdirAll(s.dir(orderID), 0o755); err != nil {
		return
	}
	data, err := json.MarshalIndent(log, "", "  ")
	if err != nil {
		return
	}
	tmp := s.path(orderID) + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return
	}
	_ = os.Rename(tmp, s.path(orderID))
}

func copyLog(log *RunLog) RunLog {
	cp := *log
	cp.Events = append([]agentrun.Event(nil), log.Events...)
	return cp
}
