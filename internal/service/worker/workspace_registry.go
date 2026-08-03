package worker

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/openmodu/oneshot/pkg/localfile"
)

type WorkspaceRegistry struct {
	path string
	mu   sync.RWMutex
}

func NewWorkspaceRegistry(path string) *WorkspaceRegistry {
	return &WorkspaceRegistry{path: path}
}

func (r *WorkspaceRegistry) DefaultPath(id string) string {
	return filepath.Join(filepath.Dir(r.path), "projects", id)
}

func (r *WorkspaceRegistry) List(ctx context.Context) ([]WorkspaceMapping, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	items, err := r.loadLocked()
	if err != nil {
		return nil, err
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return items, nil
}

func (r *WorkspaceRegistry) Save(ctx context.Context, id, path string) (WorkspaceMapping, error) {
	id = strings.TrimSpace(id)
	path = strings.TrimSpace(path)
	if !localfile.ValidID(id) || !filepath.IsAbs(path) {
		return WorkspaceMapping{}, errors.New("workspace id and absolute path are required")
	}
	path = filepath.Clean(path)
	if err := ctx.Err(); err != nil {
		return WorkspaceMapping{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	items, err := r.loadLocked()
	if err != nil {
		return WorkspaceMapping{}, err
	}
	now := time.Now().UTC()
	mapping := WorkspaceMapping{ID: id, Path: path, CreatedAt: now, UpdatedAt: now}
	found := false
	for index, current := range items {
		if current.ID != id {
			continue
		}
		mapping.CreatedAt = current.CreatedAt
		items[index] = mapping
		found = true
		break
	}
	if !found {
		items = append(items, mapping)
	}
	if err := localfile.WriteJSONAtomic(r.path, items); err != nil {
		return WorkspaceMapping{}, err
	}
	return mapping, nil
}

func (r *WorkspaceRegistry) loadLocked() ([]WorkspaceMapping, error) {
	var items []WorkspaceMapping
	if err := localfile.ReadJSON(r.path, &items); errors.Is(err, os.ErrNotExist) {
		return []WorkspaceMapping{}, nil
	} else if err != nil {
		return nil, err
	}
	return items, nil
}
