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

func (r *WorkspaceRegistry) IsManagedPath(id, path string) bool {
	return filepath.Clean(strings.TrimSpace(path)) == filepath.Clean(r.DefaultPath(strings.TrimSpace(id)))
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
	mapping := WorkspaceMapping{ID: id, Name: id, Path: path, Managed: r.IsManagedPath(id, path), CreatedAt: now, UpdatedAt: now}
	found := false
	for index, current := range items {
		if current.ID != id {
			continue
		}
		mapping = current
		mapping.Path = path
		mapping.Managed = r.IsManagedPath(id, path)
		mapping.UpdatedAt = now
		if strings.TrimSpace(mapping.Name) == "" {
			mapping.Name = id
		}
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

func (r *WorkspaceRegistry) SaveMapping(ctx context.Context, input WorkspaceMapping) (WorkspaceMapping, error) {
	input.ID = strings.TrimSpace(input.ID)
	input.Name = strings.TrimSpace(input.Name)
	input.Path = filepath.Clean(strings.TrimSpace(input.Path))
	input.RemoteURL = strings.TrimSpace(input.RemoteURL)
	input.Revision = strings.TrimSpace(input.Revision)
	if !localfile.ValidID(input.ID) || !filepath.IsAbs(input.Path) {
		return WorkspaceMapping{}, errors.New("workspace id and absolute path are required")
	}
	if input.Name == "" {
		input.Name = input.ID
	}
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
	input.Managed = r.IsManagedPath(input.ID, input.Path)
	input.CreatedAt = now
	input.UpdatedAt = now
	found := false
	for index, current := range items {
		if current.ID != input.ID {
			continue
		}
		input.CreatedAt = current.CreatedAt
		items[index] = input
		found = true
		break
	}
	if !found {
		items = append(items, input)
	}
	if err := localfile.WriteJSONAtomic(r.path, items); err != nil {
		return WorkspaceMapping{}, err
	}
	return input, nil
}

func (r *WorkspaceRegistry) Delete(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if !localfile.ValidID(id) {
		return errors.New("workspace id is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	items, err := r.loadLocked()
	if err != nil {
		return err
	}
	next := items[:0]
	for _, item := range items {
		if item.ID != id {
			next = append(next, item)
		}
	}
	return localfile.WriteJSONAtomic(r.path, next)
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
