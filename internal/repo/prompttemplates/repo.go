// Package prompttemplates persists custom templates in the existing desktop
// JSON format. Builtins and template-language rules belong to the use case.
package prompttemplates

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	domain "github.com/openmodu/onecatch/internal/domain/prompttemplates"
	"github.com/openmodu/onecatch/pkg/localfile"
)

type snapshot struct {
	Version   int               `json:"version"`
	Templates []domain.Template `json:"templates"`
}

type Repository struct {
	path string
	mu   sync.Mutex
}

func New(root string) *Repository {
	// Keep the filename so existing user libraries require no migration.
	return &Repository{path: filepath.Join(root, "keel-prompt-templates.json")}
}

func revision(data []byte) string {
	if len(data) == 0 {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func (r *Repository) load() (domain.Library, error) {
	data, err := os.ReadFile(r.path)
	if errors.Is(err, os.ErrNotExist) {
		return domain.Library{}, nil
	}
	if err != nil {
		return domain.Library{}, err
	}
	var saved snapshot
	if err := json.Unmarshal(data, &saved); err != nil || saved.Version != 1 {
		return domain.Library{}, fmt.Errorf("模板库格式无效")
	}
	return domain.Library{Templates: saved.Templates, Revision: revision(data)}, nil
}

func (r *Repository) Load(ctx context.Context) (domain.Library, error) {
	if err := ctx.Err(); err != nil {
		return domain.Library{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.load()
}

// Save serializes the revision check and write for callers sharing this repo.
// Atomic replacement protects file integrity; it is not a cross-process lock.
func (r *Repository) Save(ctx context.Context, items []domain.Template, expected string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return "", err
	}
	current, err := r.load()
	if err != nil {
		return "", err
	}
	if current.Revision != expected {
		return "", domain.ErrConflict
	}
	data, err := json.MarshalIndent(snapshot{Version: 1, Templates: items}, "", "  ")
	if err != nil {
		return "", err
	}
	if err := localfile.WriteTextAtomic(r.path, string(data)); err != nil {
		return "", err
	}
	return revision(data), nil
}
