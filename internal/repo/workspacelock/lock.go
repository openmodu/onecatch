package workspacelock

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/openmodu/oneshot/pkg/localfile"
)

var (
	ErrLocked      = errors.New("workspace is locked")
	ErrCorruptLock = errors.New("workspace lock is corrupt")
)

type Metadata struct {
	WorkspaceID   string    `json:"workspaceId"`
	WorkspacePath string    `json:"workspacePath"`
	RunID         string    `json:"runId"`
	PID           int       `json:"pid"`
	StartedAt     time.Time `json:"startedAt"`
}

type LockedError struct {
	Metadata Metadata
}

func (e LockedError) Error() string {
	return fmt.Sprintf("%v: workspace=%s run=%s pid=%d", ErrLocked, e.Metadata.WorkspaceID, e.Metadata.RunID, e.Metadata.PID)
}

func (e LockedError) Unwrap() error { return ErrLocked }

type Manager struct {
	root  string
	now   func() time.Time
	pid   int
	alive func(int) bool
	mu    sync.Mutex
}

func New(root string) *Manager {
	return &Manager{root: root, now: time.Now, pid: os.Getpid(), alive: processAlive}
}

// Acquire returns an idempotent release function. A stale lock whose process
// no longer exists is removed once and acquisition is retried.
func (m *Manager) Acquire(ctx context.Context, workspaceID, workspacePath, runID string) (func() error, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !localfile.ValidID(workspaceID) || !localfile.ValidID(runID) {
		return nil, errors.New("workspace and run IDs must be safe path segments")
	}
	path := filepath.Join(m.root, workspaceID+".lock")
	metadata := Metadata{WorkspaceID: workspaceID, WorkspacePath: workspacePath, RunID: runID, PID: m.pid, StartedAt: m.now().UTC()}

	m.mu.Lock()
	defer m.mu.Unlock()
	for attempt := 0; attempt < 2; attempt++ {
		err := localfile.WriteJSONExclusive(path, metadata)
		if err == nil {
			var once sync.Once
			var releaseErr error
			return func() error {
				once.Do(func() {
					m.mu.Lock()
					defer m.mu.Unlock()
					releaseErr = releaseOwned(path, metadata)
				})
				return releaseErr
			}, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("acquire workspace lock: %w", err)
		}
		var existing Metadata
		if readErr := localfile.ReadJSON(path, &existing); readErr != nil {
			return nil, fmt.Errorf("%w: %v", ErrCorruptLock, readErr)
		}
		if existing.PID > 0 && m.alive(existing.PID) {
			return nil, LockedError{Metadata: existing}
		}
		if removeErr := localfile.RemoveAndSync(path); removeErr != nil {
			return nil, fmt.Errorf("remove stale workspace lock: %w", removeErr)
		}
	}
	return nil, ErrLocked
}

func releaseOwned(path string, owner Metadata) error {
	var current Metadata
	if err := localfile.ReadJSON(path, &current); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	if current.RunID != owner.RunID || current.PID != owner.PID {
		return ErrLocked
	}
	return localfile.RemoveAndSync(path)
}

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = process.Signal(syscall.Signal(0))
	return err == nil || errors.Is(err, os.ErrPermission)
}
