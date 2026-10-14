// Package keeldesktop is OneCatch's Go-rendered desktop client. It calls the
// transport-neutral desktop service directly; no WebView or Wails bindings run.
package keeldesktop

import (
	"context"
	"errors"
	"sync"

	gitrepo "github.com/openmodu/onecatch/internal/repo/git"
	localstore "github.com/openmodu/onecatch/internal/repo/localstore"
	repotasks "github.com/openmodu/onecatch/internal/repo/tasks"
	repoworkflows "github.com/openmodu/onecatch/internal/repo/workflows"
	"github.com/openmodu/onecatch/internal/repo/workspacelock"
	desktopservice "github.com/openmodu/onecatch/internal/service/desktop"
	"github.com/openmodu/onecatch/internal/service/desktop/listchange"
	"github.com/openmodu/onecatch/internal/service/desktop/runstate"
	"github.com/openmodu/onecatch/internal/service/desktop/runstream"
	terminalservice "github.com/openmodu/onecatch/internal/service/terminal"
	"github.com/openmodu/onecatch/internal/service/worker/hosting"
	workflowuc "github.com/openmodu/onecatch/internal/usecase/workflows"
)

type backend struct {
	service        *desktopservice.Service
	streams        *runstream.Hub
	states         *runstate.Hub
	lists          *listchange.Hub
	store          *localstore.Store
	runtimes       *desktopservice.RuntimeRegistry
	terminal       *terminalservice.Service
	terminalMu     sync.Mutex
	terminalClosed bool
	hostMu         sync.Mutex
	hostClosed     bool
	once           sync.Once
}

func openBackend(ctx context.Context, root string) (*backend, error) {
	return openBackendWithEngine(ctx, root, nil)
}

func openBackendWithEngine(ctx context.Context, root string, engine workflowuc.Engine) (*backend, error) {
	store, err := localstore.OpenStore(root)
	if err != nil {
		return nil, err
	}
	b := &backend{store: store, streams: runstream.NewHub(), states: runstate.NewHub(), lists: listchange.NewHub(), terminal: terminalservice.NewService()}
	store.Repos.Tasks = repotasks.WithNotifier(store.Repos.Tasks, b.lists)
	store.Repos.Workflows = repoworkflows.WithNotifier(store.Repos.Workflows, b.states)
	runtimes, err := desktopservice.NewRuntimeRegistry(store.Data.Paths.Root)
	if err != nil {
		b.close()
		return nil, err
	}
	b.runtimes = runtimes
	git := gitrepo.New("")
	if engine == nil {
		engine = runtimes
	}
	workflow := workflowuc.NewUsecase(store.Repos.Tasks, store.Repos.Workflows, engine, workspacelock.New(store.Data.Paths.Locks), git)
	b.service = desktopservice.NewService(store, workflow, runtimes, git)
	b.service.SetRunStreamHub(b.streams)
	b.service.SetRunStateHub(b.states)
	if err := b.service.InitializeSettings(ctx); err != nil {
		b.close()
		return nil, err
	}
	if err := b.service.EnsureBuiltinDefinitions(ctx); err != nil {
		b.close()
		return nil, err
	}
	// Recovery is deliberately explicit: another desktop client may still be
	// executing against the same store. Reading history must not pause its run.
	return b, nil
}

func (b *backend) close() error {
	var err error
	b.once.Do(func() {
		b.terminalMu.Lock()
		b.terminalClosed = true
		b.terminal.SetEmitter(nil)
		b.terminal.Close()
		b.terminalMu.Unlock()
		b.lists.Close()
		b.states.Close()
		if b.service != nil {
			b.hostMu.Lock()
			b.hostClosed = true
			err = b.service.Close()
			b.hostMu.Unlock()
		} else {
			err = b.store.Close()
		}
	})
	return err
}

// Hosting creates a listener and identity files. Serialize it with shutdown,
// including calls already queued by a closing settings window. A late request
// must never reopen access after the desktop's final close.
func (b *backend) hostedWorkerAction(ctx context.Context, action func(context.Context) (hosting.Status, error)) (hosting.Status, error) {
	b.hostMu.Lock()
	defer b.hostMu.Unlock()
	if b.hostClosed {
		return hosting.Status{}, errors.New("桌面客户端已关闭")
	}
	if err := ctx.Err(); err != nil {
		return hosting.Status{}, err
	}
	return action(ctx)
}

func (b *backend) HostedWorker(ctx context.Context) (hosting.Status, error) {
	return b.hostedWorkerAction(ctx, b.service.HostedWorker)
}
func (b *backend) StartHostedWorker(ctx context.Context) (hosting.Status, error) {
	return b.hostedWorkerAction(ctx, b.service.StartHostedWorker)
}
func (b *backend) StopHostedWorker(ctx context.Context) (hosting.Status, error) {
	return b.hostedWorkerAction(ctx, b.service.StopHostedWorker)
}
func (b *backend) PairHostedWorker(ctx context.Context) (hosting.Status, error) {
	return b.hostedWorkerAction(ctx, b.service.PairHostedWorker)
}
func (b *backend) restoreHostedWorker(ctx context.Context) error {
	_, err := b.hostedWorkerAction(ctx, func(ctx context.Context) (hosting.Status, error) {
		return hosting.Status{}, b.service.RestoreHostedWorker(ctx)
	})
	return err
}

// Serialize process creation with shutdown. A pending Create must not spawn
// a new shell after the main window's final frame has already closed.
func (b *backend) createTerminal(input terminalservice.CreateInput) (terminalservice.Session, error) {
	b.terminalMu.Lock()
	defer b.terminalMu.Unlock()
	if b.terminalClosed {
		return terminalservice.Session{}, errors.New("桌面客户端已关闭")
	}
	return b.terminal.Create(input)
}
