// Package prompttemplates owns template validation, expansion and library edits.
package prompttemplates

import (
	"context"
	_ "embed"
	"encoding/json"

	domain "github.com/openmodu/onecatch/internal/domain/prompttemplates"
)

//go:embed templates.json
var catalog []byte

type Repository interface {
	Load(context.Context) (domain.Library, error)
	Save(context.Context, []domain.Template, string) (string, error)
}

type Usecase struct{ repo Repository }

func NewUsecase(repo Repository) *Usecase { return &Usecase{repo: repo} }

func builtins() []domain.Template {
	var items []domain.Template
	// The catalog ships with the binary and is checked by the catalog test.
	if err := json.Unmarshal(catalog, &items); err != nil {
		panic(err)
	}
	return items
}

// Load returns builtins even if a custom library cannot be read. The error
// prevents editing the damaged library while leaving builtins usable.
func (u *Usecase) Load(ctx context.Context) (domain.Library, error) {
	result := domain.Library{Templates: builtins()}
	saved, err := u.repo.Load(ctx)
	if err != nil {
		return result, err
	}
	if err := validateTemplates(saved.Templates); err != nil {
		return result, err
	}
	result.Templates = append(result.Templates, saved.Templates...)
	result.Revision = saved.Revision
	return result, nil
}

func (u *Usecase) Save(ctx context.Context, draft domain.Template, revision string) (domain.Library, error) {
	saved, err := u.repo.Load(ctx)
	if err != nil {
		return domain.Library{}, err
	}
	if saved.Revision != revision {
		return domain.Library{}, domain.ErrConflict
	}
	if err := validateTemplates(saved.Templates); err != nil {
		return domain.Library{}, err
	}
	custom := make([]domain.Template, 0, len(saved.Templates)+1)
	for _, item := range saved.Templates {
		if item.ID != draft.ID {
			custom = append(custom, item)
		}
	}
	custom = append(custom, draft)
	if err := validateTemplates(custom); err != nil {
		return domain.Library{}, err
	}
	library := append(builtins(), custom...)
	if _, err := Arguments(draft, library); err != nil {
		return domain.Library{}, err
	}
	next, err := u.repo.Save(ctx, custom, revision)
	if err != nil {
		return domain.Library{}, err
	}
	return domain.Library{Templates: library, Revision: next}, nil
}
