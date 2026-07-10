package worker

import (
	"context"
	"errors"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/openmodu/oneshot/pkg/localfile"
)

var ErrNotFound = errors.New("worker not found")
var workerIDPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)

type Registry struct {
	path string
	mu   sync.RWMutex
}

func NewRegistry(path string) *Registry { return &Registry{path: path} }

func (r *Registry) List(ctx context.Context) ([]Info, error) {
	configs, err := r.load(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]Info, 0, len(configs))
	for _, config := range configs {
		items = append(items, publicInfo(config))
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return items, nil
}

func (r *Registry) Get(ctx context.Context, id string) (Config, error) {
	configs, err := r.load(ctx)
	if err != nil {
		return Config{}, err
	}
	for _, config := range configs {
		if config.ID == id {
			return config, nil
		}
	}
	return Config{}, ErrNotFound
}

func (r *Registry) Save(ctx context.Context, input Input) (Info, error) {
	input.ID = strings.TrimSpace(input.ID)
	input.Name = strings.TrimSpace(input.Name)
	input.BaseURL = strings.TrimRight(strings.TrimSpace(input.BaseURL), "/")
	if input.ID == "" || input.ID == "local" || input.Name == "" || !localfile.ValidID(input.ID) || !workerIDPattern.MatchString(input.ID) {
		return Info{}, errors.New("worker id and name are invalid")
	}
	parsed, err := url.Parse(input.BaseURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return Info{}, errors.New("worker base URL must be an absolute HTTP URL")
	}
	if err := ctx.Err(); err != nil {
		return Info{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	configs, err := r.loadLocked()
	if err != nil {
		return Info{}, err
	}
	now := time.Now().UTC()
	config := Config{ID: input.ID, Name: input.Name, BaseURL: input.BaseURL, Token: strings.TrimSpace(input.Token), Enabled: input.Enabled, CreatedAt: now, UpdatedAt: now}
	found := false
	for index, current := range configs {
		if current.ID != input.ID {
			continue
		}
		config.CreatedAt = current.CreatedAt
		if config.Token == "" {
			config.Token = current.Token
		}
		configs[index] = config
		found = true
		break
	}
	if !found {
		if config.Token == "" {
			return Info{}, errors.New("worker token is required")
		}
		configs = append(configs, config)
	}
	if err := localfile.WriteJSONAtomic(r.path, configs); err != nil {
		return Info{}, err
	}
	return publicInfo(config), nil
}

func (r *Registry) Delete(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	configs, err := r.loadLocked()
	if err != nil {
		return err
	}
	out := configs[:0]
	found := false
	for _, config := range configs {
		if config.ID == id {
			found = true
			continue
		}
		out = append(out, config)
	}
	if !found {
		return ErrNotFound
	}
	return localfile.WriteJSONAtomic(r.path, out)
}

func (r *Registry) load(ctx context.Context) ([]Config, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.loadLocked()
}

func (r *Registry) loadLocked() ([]Config, error) {
	var configs []Config
	if err := localfile.ReadJSON(r.path, &configs); errors.Is(err, os.ErrNotExist) {
		return []Config{}, nil
	} else if err != nil {
		return nil, err
	}
	return configs, nil
}

func publicInfo(config Config) Info {
	return Info{ID: config.ID, Name: config.Name, BaseURL: config.BaseURL, Enabled: config.Enabled, HasToken: config.Token != "", CreatedAt: config.CreatedAt, UpdatedAt: config.UpdatedAt}
}
