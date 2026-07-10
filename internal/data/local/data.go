// Package localdata owns local embedded resources and their lifecycle. It
// creates ~/.oneshot by default and exposes paths used by atomic snapshots,
// append-only event storage and workspace locks.
package localdata

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	repotasks "github.com/openmodu/oneshot/internal/repo/tasks"
	repoworkflows "github.com/openmodu/oneshot/internal/repo/workflows"
)

type Paths struct {
	Root       string
	Workspaces string
	Tasks      string
	Workflows  string
	Runs       string
	Locks      string
	Logs       string
}

type Data struct {
	Paths Paths
}

type Repositories struct {
	Tasks     repotasks.TasksRepo
	Workflows repoworkflows.WorkflowsRepo
}

type Store struct {
	Data  *Data
	Repos Repositories
}

func DefaultRoot() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".oneshot"), nil
}

func ResolvePaths(root string) (Paths, error) {
	if strings.TrimSpace(root) == "" {
		var err error
		root, err = DefaultRoot()
		if err != nil {
			return Paths{}, err
		}
	} else if root == "~" || strings.HasPrefix(root, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return Paths{}, fmt.Errorf("resolve home directory: %w", err)
		}
		root = filepath.Join(home, strings.TrimPrefix(root, "~/"))
	} else if strings.HasPrefix(root, "~") {
		return Paths{}, errors.New("~user paths are not supported")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return Paths{}, fmt.Errorf("resolve local data root: %w", err)
	}
	abs = filepath.Clean(abs)
	return Paths{
		Root:       abs,
		Workspaces: filepath.Join(abs, "workspaces"),
		Tasks:      filepath.Join(abs, "tasks"),
		Workflows:  filepath.Join(abs, "workflows"),
		Runs:       filepath.Join(abs, "runs"),
		Locks:      filepath.Join(abs, "locks"),
		Logs:       filepath.Join(abs, "logs"),
	}, nil
}

func Open(root string) (*Data, error) {
	paths, err := ResolvePaths(root)
	if err != nil {
		return nil, err
	}
	for _, dir := range []string{paths.Root, paths.Workspaces, paths.Tasks, paths.Workflows, paths.Runs, paths.Locks, paths.Logs} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("create local data directory %s: %w", dir, err)
		}
		if err := os.Chmod(dir, 0o700); err != nil {
			return nil, fmt.Errorf("secure local data directory %s: %w", dir, err)
		}
	}
	return &Data{Paths: paths}, nil
}

func OpenStore(root string) (*Store, error) {
	data, err := Open(root)
	if err != nil {
		return nil, err
	}
	return &Store{
		Data: data,
		Repos: Repositories{
			Tasks:     repotasks.NewTasksRepo(data.Paths.Workspaces, data.Paths.Tasks),
			Workflows: repoworkflows.NewWorkflowsRepo(data.Paths.Workflows, data.Paths.Runs),
		},
	}, nil
}

func (d *Data) Close() error {
	return nil
}

func (s *Store) Close() error {
	if s == nil {
		return nil
	}
	return s.Data.Close()
}
