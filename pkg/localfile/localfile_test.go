package localfile_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/openmodu/onecatch/pkg/localfile"
)

func TestWriteJSONAtomicReplacesCompleteSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "state.json")
	first := map[string]any{"revision": 1, "status": "running"}
	if err := localfile.WriteJSONAtomic(path, first); err != nil {
		t.Fatal(err)
	}
	second := map[string]any{"revision": 2, "status": "paused"}
	if err := localfile.WriteJSONAtomic(path, second); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := localfile.ReadJSON(path, &got); err != nil {
		t.Fatal(err)
	}
	if got["revision"] != float64(2) || got["status"] != "paused" {
		t.Fatalf("snapshot = %+v", got)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("snapshot mode = %o, want 600", info.Mode().Perm())
	}
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".onecatch-*.tmp"))
	if err != nil || len(matches) != 0 {
		t.Fatalf("temporary files = %v, %v", matches, err)
	}
}

func TestValidIDRejectsTraversal(t *testing.T) {
	for _, value := range []string{"", ".", "..", "../run", "run/step", "~home"} {
		if localfile.ValidID(value) {
			t.Fatalf("ValidID(%q) = true", value)
		}
	}
	for _, value := range []string{"run_1", "step-2", "workflow.v1"} {
		if !localfile.ValidID(value) {
			t.Fatalf("ValidID(%q) = false", value)
		}
	}
}

func TestTrimIncompleteJSONLTailKeepsCompleteLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	content := []byte("{\"seq\":1}\n{\"seq\":2")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := localfile.TrimIncompleteJSONLTail(path); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "{\"seq\":1}\n" {
		t.Fatalf("recovered stream = %q", got)
	}
	if err := localfile.TrimIncompleteJSONLTail(path); err != nil {
		t.Fatalf("second recovery should be idempotent: %v", err)
	}
}
