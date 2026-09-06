package desktop

import (
	"path/filepath"
	"strings"
	"testing"
)

// Remote access has to be off for a user who never asked for it, including on
// the very first launch when no state file exists yet.
func TestHostWorkerStateDefaultsToClosed(t *testing.T) {
	root := filepath.Join(t.TempDir(), "host-worker")
	state, err := readHostWorkerState(root)
	if err != nil || state.Enabled {
		t.Fatalf("state = %+v, err = %v", state, err)
	}
	if err := writeHostWorkerState(root, hostWorkerState{Enabled: true, Port: 9232}); err != nil {
		t.Fatal(err)
	}
	state, err = readHostWorkerState(root)
	if err != nil || !state.Enabled || state.Port != 9232 {
		t.Fatalf("state = %+v, err = %v", state, err)
	}
	if err := writeHostWorkerState(root, hostWorkerState{Enabled: false, Port: 9232}); err != nil {
		t.Fatal(err)
	}
	if state, err := readHostWorkerState(root); err != nil || state.Enabled {
		t.Fatalf("state = %+v, err = %v", state, err)
	}
}

// A phone that re-pairs must land on the worker it already knows, so the id
// cannot drift between launches.
func TestHostWorkerIdentityIsStableAndNamed(t *testing.T) {
	id := hostWorkerID()
	if id != hostWorkerID() || !strings.HasPrefix(id, "desktop") || strings.ContainsAny(id, " .") {
		t.Fatalf("worker id = %q", id)
	}
	if strings.TrimSpace(hostWorkerName()) == "" || strings.HasSuffix(hostWorkerName(), ".local") {
		t.Fatalf("worker name = %q", hostWorkerName())
	}
}
