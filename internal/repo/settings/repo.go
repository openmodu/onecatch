package settings

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	domainsettings "github.com/openmodu/onecatch/internal/domain/settings"
	"github.com/openmodu/onecatch/pkg/localfile"
)

var ErrStateConflict = errors.New("settings state conflict")

type SettingsRepo interface {
	Get(context.Context) (domainsettings.Settings, error)
	Save(context.Context, domainsettings.Settings, int64) (domainsettings.Settings, error)
}

type settingsImpl struct {
	root, path, legacyPath, backupPath string
	now                                func() time.Time
	mu                                 sync.Mutex
}

func NewSettingsRepo(root string) SettingsRepo {
	return &settingsImpl{root: root, path: filepath.Join(root, "settings.json"), legacyPath: filepath.Join(root, "runtime.json"), backupPath: filepath.Join(root, "runtime.v0.backup.json"), now: time.Now}
}

func (r *settingsImpl) Get(ctx context.Context) (domainsettings.Settings, error) {
	if err := ctx.Err(); err != nil {
		return domainsettings.Settings{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.getLocked()
}

func (r *settingsImpl) Save(ctx context.Context, input domainsettings.Settings, expectedRevision int64) (domainsettings.Settings, error) {
	if err := ctx.Err(); err != nil {
		return domainsettings.Settings{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	current, err := r.getLocked()
	if err != nil {
		return domainsettings.Settings{}, err
	}
	if current.Revision != expectedRevision {
		return domainsettings.Settings{}, ErrStateConflict
	}
	input.SchemaVersion = domainsettings.CurrentSchemaVersion
	input.Revision = current.Revision + 1
	input.UpdatedAt = r.now().UTC()
	input, err = domainsettings.Normalize(input)
	if err != nil {
		return domainsettings.Settings{}, err
	}
	if err := domainsettings.Validate(input); err != nil {
		return domainsettings.Settings{}, err
	}
	if err := localfile.WriteJSONAtomic(r.path, input); err != nil {
		return domainsettings.Settings{}, fmt.Errorf("save settings: %w", err)
	}
	return input, nil
}

func (r *settingsImpl) getLocked() (domainsettings.Settings, error) {
	var value domainsettings.Settings
	if err := localfile.ReadJSON(r.path, &value); err == nil {
		value, err = domainsettings.Normalize(value)
		if err != nil {
			return domainsettings.Settings{}, err
		}
		if err := domainsettings.Validate(value); err != nil {
			return domainsettings.Settings{}, fmt.Errorf("validate settings: %w", err)
		}
		return value, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return domainsettings.Settings{}, fmt.Errorf("read settings: %w", err)
	}
	if _, err := os.Stat(r.legacyPath); err == nil {
		return r.migrateLegacyLocked()
	} else if !errors.Is(err, os.ErrNotExist) {
		return domainsettings.Settings{}, fmt.Errorf("inspect legacy runtime settings: %w", err)
	}
	return domainsettings.Defaults(), nil
}

func (r *settingsImpl) migrateLegacyLocked() (domainsettings.Settings, error) {
	if _, err := os.Stat(r.backupPath); err == nil {
		return domainsettings.Settings{}, errors.New("settings migration failed: runtime backup already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return domainsettings.Settings{}, fmt.Errorf("settings migration failed: inspect backup: %w", err)
	}
	var legacy struct {
		CodexBinary  string `json:"codexBinary"`
		ClaudeBinary string `json:"claudeBinary"`
	}
	if err := localfile.ReadJSON(r.legacyPath, &legacy); err != nil {
		return domainsettings.Settings{}, fmt.Errorf("settings migration failed: %w", err)
	}
	value := domainsettings.Defaults()
	codex := value.Runtimes["codex"]
	codex.Binary = legacy.CodexBinary
	value.Runtimes["codex"] = codex
	claude := value.Runtimes["claude"]
	claude.Binary = legacy.ClaudeBinary
	value.Runtimes["claude"] = claude
	value.UpdatedAt = r.now().UTC()
	value, err := domainsettings.Normalize(value)
	if err != nil {
		return domainsettings.Settings{}, fmt.Errorf("settings migration failed: %w", err)
	}
	if err := domainsettings.Validate(value); err != nil {
		return domainsettings.Settings{}, fmt.Errorf("settings migration failed: %w", err)
	}
	if err := os.Rename(r.legacyPath, r.backupPath); err != nil {
		return domainsettings.Settings{}, fmt.Errorf("settings migration failed: %w", err)
	}
	if err := localfile.WriteJSONAtomic(r.path, value); err != nil {
		if rollbackErr := os.Rename(r.backupPath, r.legacyPath); rollbackErr != nil {
			return domainsettings.Settings{}, fmt.Errorf("settings migration failed: %v (rollback failed: %v)", err, rollbackErr)
		}
		return domainsettings.Settings{}, fmt.Errorf("settings migration failed: %w", err)
	}
	return value, nil
}
