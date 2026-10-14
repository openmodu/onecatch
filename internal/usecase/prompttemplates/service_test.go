package prompttemplates

import (
	"context"
	"errors"
	"testing"

	domain "github.com/openmodu/onecatch/internal/domain/prompttemplates"
)

type memoryRepository struct {
	library domain.Library
	err     error
	writes  int
}

func (r *memoryRepository) Load(context.Context) (domain.Library, error) { return r.library, r.err }
func (r *memoryRepository) Save(_ context.Context, items []domain.Template, revision string) (string, error) {
	if revision != r.library.Revision {
		return "", domain.ErrConflict
	}
	r.writes++
	r.library = domain.Library{Templates: items, Revision: "saved"}
	return "saved", nil
}

func TestCatalogAndCustomEdit(t *testing.T) {
	repo := &memoryRepository{}
	u := NewUsecase(repo)
	ctx := context.Background()
	before, err := u.Load(ctx)
	if err != nil || len(before.Templates) == 0 {
		t.Fatalf("builtins: %+v %v", before, err)
	}
	for _, item := range before.Templates {
		if !item.Builtin {
			t.Fatalf("catalog item not builtin: %+v", item)
		}
		if _, err := Arguments(item, before.Templates); err != nil {
			t.Fatal(err)
		}
	}
	draft := domain.Template{ID: "custom-one", Name: "引用内置模板", Body: "{{template:builtin-explain}}"}
	after, err := u.Save(ctx, draft, before.Revision)
	if err != nil || len(after.Templates) != len(before.Templates)+1 {
		t.Fatalf("save: %+v %v", after, err)
	}
	if len(repo.library.Templates) != 1 || repo.library.Templates[0].Builtin {
		t.Fatal("persisted builtins")
	}
	draft.Name = "重命名"
	updated, err := u.Save(ctx, draft, after.Revision)
	if err != nil || len(updated.Templates) != len(after.Templates) {
		t.Fatalf("update: %+v %v", updated, err)
	}
	if _, err := u.Save(ctx, draft, before.Revision); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("stale edit: %v", err)
	}
}

func TestRejectedEditsDoNotWrite(t *testing.T) {
	for _, draft := range []domain.Template{
		{ID: "builtin-explain", Name: "覆盖内置", Body: "text", Builtin: true},
		{ID: "custom-one", Name: "引用缺失", Body: "{{template:missing}}"},
		{ID: "custom-one", Name: "循环", Body: "{{template:custom-one}}"},
	} {
		repo := &memoryRepository{}
		if _, err := NewUsecase(repo).Save(context.Background(), draft, ""); err == nil {
			t.Fatalf("accepted %+v", draft)
		}
		if repo.writes != 0 {
			t.Fatal("invalid edit reached persistence")
		}
	}
}

func TestUnreadableLibraryKeepsBuiltinsAndBlocksSave(t *testing.T) {
	repo := &memoryRepository{err: errors.New("unreadable")}
	u := NewUsecase(repo)
	got, err := u.Load(context.Background())
	if err == nil || len(got.Templates) == 0 {
		t.Fatalf("lost builtins or error: %+v %v", got, err)
	}
	if _, err := u.Save(context.Background(), domain.Template{ID: "custom-one", Name: "test", Body: "text"}, ""); err == nil || repo.writes != 0 {
		t.Fatalf("wrote unreadable library: %v", err)
	}
}
