package settings

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	domainsettings "github.com/openmodu/onecatch/internal/domain/settings"
	"github.com/openmodu/onecatch/pkg/localfile"
)

func TestDefaultsSaveAndRevisionConflict(t *testing.T) {
	root := t.TempDir()
	repo := NewSettingsRepo(root)
	ctx := context.Background()
	initial, err := repo.Get(ctx)
	if err != nil {
		t.Fatal(err)
	}
	initial.Execution.MaxLocalDAGConcurrency = 8
	saved, err := repo.Save(ctx, initial, initial.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Revision != 2 {
		t.Fatalf("revision = %d", saved.Revision)
	}
	if _, err := repo.Save(ctx, initial, initial.Revision); !errors.Is(err, ErrStateConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
	info, err := os.Stat(filepath.Join(root, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o", info.Mode().Perm())
	}
}

func TestMigrateLegacyRuntime(t *testing.T) {
	root := t.TempDir()
	legacy := map[string]string{"codexBinary": "/opt/codex", "claudeBinary": "/opt/claude"}
	if err := localfile.WriteJSONAtomic(filepath.Join(root, "runtime.json"), legacy); err != nil {
		t.Fatal(err)
	}
	got, err := NewSettingsRepo(root).Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.Runtimes["codex"].Binary != "/opt/codex" {
		t.Fatalf("migration failed: %#v", got.Runtimes)
	}
	if _, err := os.Stat(filepath.Join(root, "runtime.v0.backup.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "runtime.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("legacy file still exists: %v", err)
	}
}

func TestMigrationFailurePreservesLegacyRuntime(t *testing.T) {
	root := t.TempDir()
	legacyPath := filepath.Join(root, "runtime.json")
	if err := localfile.WriteJSONAtomic(legacyPath, map[string]string{"codexBinary": "/opt/original"}); err != nil {
		t.Fatal(err)
	}
	if err := localfile.WriteJSONAtomic(filepath.Join(root, "runtime.v0.backup.json"), map[string]string{"codexBinary": "/opt/backup"}); err != nil {
		t.Fatal(err)
	}
	if _, err := NewSettingsRepo(root).Get(context.Background()); err == nil {
		t.Fatal("expected migration failure")
	}
	var legacy map[string]string
	if err := localfile.ReadJSON(legacyPath, &legacy); err != nil {
		t.Fatal(err)
	}
	if legacy["codexBinary"] != "/opt/original" {
		t.Fatalf("legacy changed: %#v", legacy)
	}
	if _, err := os.Stat(filepath.Join(root, "settings.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("partial settings file exists: %v", err)
	}
}

func TestConcurrentCASHasOneWinner(t *testing.T) {
	repo := NewSettingsRepo(t.TempDir())
	initial, err := repo.Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := repo.Save(context.Background(), domainsettings.Defaults(), initial.Revision)
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	successes := 0
	conflicts := 0
	for err := range results {
		if err == nil {
			successes++
		} else if errors.Is(err, ErrStateConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("successes=%d conflicts=%d", successes, conflicts)
	}
}
