package workspacelock

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/openmodu/onecatch/pkg/localfile"
)

func TestAcquireRejectsLiveOwnerAndReleaseIsIdempotent(t *testing.T) {
	manager := New(t.TempDir())
	release, err := manager.Acquire(context.Background(), "ws_1", "/tmp/project", "run_1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Acquire(context.Background(), "ws_1", "/tmp/project", "run_2"); !errors.Is(err, ErrLocked) {
		t.Fatalf("second Acquire() error = %v", err)
	}
	if err := release(); err != nil {
		t.Fatal(err)
	}
	if err := release(); err != nil {
		t.Fatalf("second release = %v", err)
	}
	releaseAgain, err := manager.Acquire(context.Background(), "ws_1", "/tmp/project", "run_2")
	if err != nil {
		t.Fatal(err)
	}
	if err := releaseAgain(); err != nil {
		t.Fatal(err)
	}
}

func TestAcquireRemovesStaleLock(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "ws_1.lock")
	stale := Metadata{WorkspaceID: "ws_1", WorkspacePath: "/tmp/project", RunID: "old_run", PID: 424242, StartedAt: time.Now()}
	if err := localfile.WriteJSONExclusive(path, stale); err != nil {
		t.Fatal(err)
	}
	manager := &Manager{root: root, now: time.Now, pid: os.Getpid(), alive: func(int) bool { return false }}
	release, err := manager.Acquire(context.Background(), "ws_1", "/tmp/project", "new_run")
	if err != nil {
		t.Fatal(err)
	}
	var current Metadata
	if err := localfile.ReadJSON(path, &current); err != nil {
		t.Fatal(err)
	}
	if current.RunID != "new_run" {
		t.Fatalf("lock = %+v", current)
	}
	if err := release(); err != nil {
		t.Fatal(err)
	}
}
