package desktop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/openmodu/onecatch/internal/service/worker"
	"github.com/openmodu/onecatch/internal/service/worker/hosting"
)

// hostWorkerStateFile records only whether the user turned remote access on.
// The identity it serves lives beside it, written by the hosting package.
const hostWorkerStateFile = "hosting.json"

type hostWorkerState struct {
	Enabled bool `json:"enabled"`
	Port    int  `json:"port,omitempty"`
}

type hostWorkerController struct {
	mu   sync.Mutex
	host *hosting.Host
	root string
	port int
}

// HostedWorker reports whether this desktop is currently reachable as a worker,
// where a phone should point at it, and any pairing code still on screen.
func (a *Service) HostedWorker(context.Context) (hosting.Status, error) {
	controller, err := a.hostWorkerController()
	if err != nil {
		return hosting.Status{}, err
	}
	return controller.status(), nil
}

// StartHostedWorker opens this machine to phones on the local network. It is
// off until the user asks for it: the worker executes agents with the desktop's
// own runtimes, so it stays closed by default.
func (a *Service) StartHostedWorker(ctx context.Context) (hosting.Status, error) {
	controller, err := a.hostWorkerController()
	if err != nil {
		return hosting.Status{}, err
	}
	status, err := controller.start(ctx, a.hostWorkerOptions(controller.root))
	if err != nil {
		return hosting.Status{}, coded("hosted_worker_start_failed", err.Error())
	}
	if err := writeHostWorkerState(controller.root, hostWorkerState{Enabled: true, Port: status.Port}); err != nil {
		return status, coded("hosted_worker_state_failed", err.Error())
	}
	return status, nil
}

func (a *Service) StopHostedWorker(ctx context.Context) (hosting.Status, error) {
	controller, err := a.hostWorkerController()
	if err != nil {
		return hosting.Status{}, err
	}
	if err := controller.stop(ctx); err != nil {
		return hosting.Status{}, coded("hosted_worker_stop_failed", err.Error())
	}
	if err := writeHostWorkerState(controller.root, hostWorkerState{Enabled: false, Port: controller.port}); err != nil {
		return controller.status(), coded("hosted_worker_state_failed", err.Error())
	}
	return controller.status(), nil
}

// PairHostedWorker issues the one-time code the phone asks for. Starting the
// worker does not open pairing, so a machine left reachable is not also left
// permanently pairable.
func (a *Service) PairHostedWorker(ctx context.Context) (hosting.Status, error) {
	controller, err := a.hostWorkerController()
	if err != nil {
		return hosting.Status{}, err
	}
	if !controller.status().Running {
		if _, err := a.StartHostedWorker(ctx); err != nil {
			return hosting.Status{}, err
		}
	}
	if _, err := controller.pair(); err != nil {
		return hosting.Status{}, coded("hosted_worker_pairing_failed", err.Error())
	}
	return controller.status(), nil
}

// RestoreHostedWorker brings the worker back up on launch when the user had
// left it on. A failure here is reported, never fatal: the desktop is still
// perfectly usable with no phone attached.
func (a *Service) RestoreHostedWorker(ctx context.Context) error {
	controller, err := a.hostWorkerController()
	if err != nil {
		return err
	}
	state, err := readHostWorkerState(controller.root)
	if err != nil || !state.Enabled {
		return err
	}
	if state.Port > 0 {
		controller.port = state.Port
	}
	if _, err := controller.start(ctx, a.hostWorkerOptions(controller.root)); err != nil {
		return coded("hosted_worker_start_failed", err.Error())
	}
	return nil
}

func (a *Service) hostWorkerController() (*hostWorkerController, error) {
	a.hostWorkerOnce.Do(func() {
		a.hostWorker = &hostWorkerController{
			root: filepath.Join(a.store.Data.Paths.Root, "host-worker"),
			port: hosting.DefaultPort,
		}
	})
	if a.hostWorker == nil {
		return nil, coded("hosted_worker_unavailable", "remote access is unavailable")
	}
	return a.hostWorker, nil
}

func (a *Service) hostWorkerOptions(root string) hosting.Options {
	return hosting.Options{
		Root: root, ID: hostWorkerID(), Name: hostWorkerName(),
		Engine: a.runtimes, Git: a.git,
		SharedWorkspaces: a.hostedWorkspaces,
	}
}

// hostWorkerID stays stable for the machine so a phone that re-pairs updates
// the worker it already knows instead of collecting duplicates.
func hostWorkerID() string {
	host, err := os.Hostname()
	if err != nil || strings.TrimSpace(host) == "" {
		return "desktop"
	}
	return "desktop-" + strings.ToLower(strings.Split(strings.TrimSpace(host), ".")[0])
}

func hostWorkerName() string {
	host, err := os.Hostname()
	if err != nil || strings.TrimSpace(host) == "" {
		return "OneCatch Desktop"
	}
	return strings.TrimSuffix(strings.TrimSpace(host), ".local")
}

func (c *hostWorkerController) start(ctx context.Context, options hosting.Options) (hosting.Status, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.host == nil {
		options.Port = c.port
		c.host = hosting.New(options)
	}
	return c.host.Start(ctx)
}

func (c *hostWorkerController) stop(ctx context.Context) error {
	c.mu.Lock()
	host := c.host
	c.mu.Unlock()
	if host == nil {
		return nil
	}
	return host.Stop(ctx)
}

func (c *hostWorkerController) pair() (hosting.Pairing, error) {
	c.mu.Lock()
	host := c.host
	c.mu.Unlock()
	if host == nil {
		return hosting.Pairing{}, errors.New("the worker is not running")
	}
	return host.Pair()
}

func (c *hostWorkerController) status() hosting.Status {
	c.mu.Lock()
	host := c.host
	port := c.port
	c.mu.Unlock()
	if host == nil {
		return hosting.Status{Port: port, WorkerID: hostWorkerID(), Name: hostWorkerName()}
	}
	return host.Status()
}

func readHostWorkerState(root string) (hostWorkerState, error) {
	raw, err := os.ReadFile(filepath.Join(root, hostWorkerStateFile))
	if errors.Is(err, fs.ErrNotExist) {
		return hostWorkerState{}, nil
	}
	if err != nil {
		return hostWorkerState{}, err
	}
	var state hostWorkerState
	if err := json.Unmarshal(raw, &state); err != nil {
		return hostWorkerState{}, fmt.Errorf("read remote access state: %w", err)
	}
	return state, nil
}

func writeHostWorkerState(root string, state hostWorkerState) error {
	if err := os.MkdirAll(root, 0o700); err != nil {
		return err
	}
	raw, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(root, hostWorkerStateFile), raw, 0o600)
}

// hostedWorkspaces is what a phone sees when it lists this desktop's projects.
// Sharing the list is the whole point of hosting the worker here: the machine
// is already configured, so the phone should not have to clone anything to get
// started.
func (a *Service) hostedWorkspaces(ctx context.Context) ([]worker.SharedWorkspace, error) {
	items, err := a.ListWorkspaces(ctx)
	if err != nil {
		return nil, err
	}
	shared := make([]worker.SharedWorkspace, 0, len(items))
	for _, item := range items {
		if item.Hidden || strings.TrimSpace(item.Path) == "" {
			continue
		}
		mapping := worker.WorkspaceMapping{
			ID: item.ID, Name: item.Name, Path: item.Path, Shared: true,
			CreatedAt: item.CreatedAt, UpdatedAt: item.LastOpenedAt,
		}
		// A Remote FS project's files live on a third machine. The worker can
		// still serve it, because it runs inside the desktop that already holds
		// the SSH credentials: the agent's commands and its git both travel the
		// same seam a desktop run would use.
		if item.RemoteFS == nil {
			if !filepath.IsAbs(item.Path) {
				continue
			}
			shared = append(shared, worker.SharedWorkspace{Mapping: mapping})
			continue
		}
		mapping.RemoteHost = item.RemoteFS.Host
		shared = append(shared, worker.SharedWorkspace{
			Mapping:   mapping,
			Remote:    remoteSeamTarget(item),
			Git:       remoteWorkerGit{runner: a.remoteGitRunner(item)},
			Inspector: a.gitForWorkspace(item),
		})
	}
	return shared, nil
}

// remoteWorkerGit adapts the desktop's remote git runner to the worker's
// GitRunner, so the worker's baseline and identity checks run on the machine
// that actually holds the files.
type remoteWorkerGit struct {
	runner interface {
		Run(ctx context.Context, workspace string, args ...string) (string, error)
	}
}

func (r remoteWorkerGit) Output(ctx context.Context, workspace string, args ...string) ([]byte, error) {
	output, err := r.runner.Run(ctx, workspace, args...)
	if err != nil {
		return nil, err
	}
	return []byte(output), nil
}

// refreshHostedWorkspaces keeps a connected phone in step with the desktop's
// project list. It is deliberately quiet: nothing here should be able to fail a
// workspace edit the user just made.
func (a *Service) refreshHostedWorkspaces(ctx context.Context) {
	if a.hostWorker == nil {
		return
	}
	a.hostWorker.mu.Lock()
	host := a.hostWorker.host
	a.hostWorker.mu.Unlock()
	if host != nil {
		host.RefreshSharedWorkspaces(ctx)
	}
}
