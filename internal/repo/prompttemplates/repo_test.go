package prompttemplates

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	domain "github.com/openmodu/onecatch/internal/domain/prompttemplates"
)

func TestLegacyLibraryAndStaleWrite(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "keel-prompt-templates.json")
	legacy := []byte(`{"version":1,"templates":[{"id":"custom-old","name":"旧模板","body":"{{selection}}"}]}`)
	if err := os.WriteFile(path, legacy, 0600); err != nil {
		t.Fatal(err)
	}
	repo := New(root)
	ctx := context.Background()
	before, err := repo.Load(ctx)
	if err != nil || len(before.Templates) != 1 || before.Templates[0].Name != "旧模板" {
		t.Fatalf("load legacy: %+v %v", before, err)
	}
	edited := append([]domain.Template(nil), before.Templates...)
	edited[0].Name = "已修改"
	next, err := repo.Save(ctx, edited, before.Revision)
	if err != nil || next == before.Revision {
		t.Fatalf("save: %s %v", next, err)
	}
	if _, err := repo.Save(ctx, before.Templates, before.Revision); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("stale save: %v", err)
	}
	got, err := New(root).Load(ctx)
	if err != nil || got.Templates[0].Name != "已修改" || got.Revision != next {
		t.Fatalf("reload: %+v %v", got, err)
	}
}

func TestConcurrentEditsHaveOneWinner(t *testing.T) {
	repo := New(t.TempDir())
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, name := range []string{"A", "B"} {
		wg.Go(func() {
			_, err := repo.Save(context.Background(), []domain.Template{{ID: "custom-one", Name: name, Body: "text"}}, "")
			results <- err
		})
	}
	wg.Wait()
	close(results)
	saved, conflicts := 0, 0
	for err := range results {
		if err == nil {
			saved++
		} else if errors.Is(err, domain.ErrConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if saved != 1 || conflicts != 1 {
		t.Fatalf("saved=%d conflicts=%d", saved, conflicts)
	}
}

func TestInvalidFileIsNotOverwritten(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "keel-prompt-templates.json")
	original := []byte(`{"version":99,"templates":[]}`)
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := New(root).Save(context.Background(), nil, ""); err == nil {
		t.Fatal("invalid file overwritten")
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != string(original) {
		t.Fatalf("original changed: %q %v", got, err)
	}
}
