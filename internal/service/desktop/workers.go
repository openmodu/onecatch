package desktop

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	domainworkspaces "github.com/openmodu/onecatch/internal/domain/workspaces"
	"github.com/openmodu/onecatch/internal/service/worker"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
)

type WorkerStatus struct {
	Worker              worker.Info   `json:"worker"`
	Health              worker.Health `json:"health"`
	CheckedAt           time.Time     `json:"checkedAt"`
	LatencyMilliseconds int64         `json:"latencyMilliseconds"`
}

type WorkerWorkspaceSetup struct {
	Mapping worker.WorkspaceMapping      `json:"mapping"`
	Local   domainworkspaces.GitSnapshot `json:"local"`
	Remote  domainworkspaces.GitSnapshot `json:"remote"`
}

func (a *Service) ListWorkers(ctx context.Context) ([]worker.Info, error) { return a.workers.List(ctx) }
func (a *Service) UpdateWorker(ctx context.Context, input worker.UpdateInput) (worker.Info, error) {
	return a.workers.Update(ctx, input)
}
func (a *Service) DeleteWorker(ctx context.Context, id string) error {
	return a.workers.Delete(ctx, id)
}
func (a *Service) PairWorker(ctx context.Context, baseURL, code string) (worker.Info, error) {
	paired, err := a.workerClient.Pair(ctx, baseURL, code)
	if err != nil {
		return worker.Info{}, err
	}
	return a.workers.Save(ctx, worker.Input{
		ID: paired.WorkerID, Name: paired.Name, BaseURL: baseURL, Token: paired.Token,
		ServerCertificateSHA256: paired.ServerCertificateSHA256, Enabled: true,
	})
}
func (a *Service) CheckWorker(ctx context.Context, id string) (WorkerStatus, error) {
	config, err := a.workers.Get(ctx, id)
	if err != nil {
		return WorkerStatus{}, coded("worker_not_found", "worker was not found")
	}
	startedAt := time.Now()
	health, err := a.workerClient.Health(ctx, config)
	if err != nil {
		return WorkerStatus{}, err
	}
	return WorkerStatus{Worker: worker.Info{
		ID: config.ID, Name: config.Name, BaseURL: config.BaseURL,
		CAFile: config.CAFile, ClientCertFile: config.ClientCertFile, ClientKeyFile: config.ClientKeyFile,
		ServerName: config.ServerName, ServerCertificateSHA256: config.ServerCertificateSHA256,
		Enabled: config.Enabled, HasToken: config.Token != "", CreatedAt: config.CreatedAt, UpdatedAt: config.UpdatedAt,
	}, Health: health, CheckedAt: time.Now().UTC(), LatencyMilliseconds: time.Since(startedAt).Milliseconds()}, nil
}

// WorkerGitStatus reads the operational git state of a mapped remote clone.
// Writable run changes normally appear locally after patch synchronization.
func (a *Service) WorkerGitStatus(ctx context.Context, workerID, workspaceID string) (domainworkspaces.GitSnapshot, error) {
	config, err := a.workers.Get(ctx, workerID)
	if err != nil {
		return domainworkspaces.GitSnapshot{}, coded("worker_not_found", "worker was not found")
	}
	return a.workerClient.GitStatus(ctx, config, workspaceID)
}

func (a *Service) PrepareWorkerWorkspace(ctx context.Context, workerID, workspaceID string) (WorkerWorkspaceSetup, error) {
	config, err := a.workers.Get(ctx, workerID)
	if err != nil || !config.Enabled {
		return WorkerWorkspaceSetup{}, coded("worker_not_found", "worker was not found or is disabled")
	}
	workspace, err := a.GetWorkspace(ctx, workspaceID)
	if err != nil {
		return WorkerWorkspaceSetup{}, err
	}
	if workspace.RemoteFS != nil {
		return WorkerWorkspaceSetup{}, coded("remote_fs_local_agent_required", "remote FS workspaces cannot be mapped to a remote worker")
	}
	revision, err := worker.WorkspaceBaseline(ctx, workspace.Path)
	if err != nil {
		return WorkerWorkspaceSetup{}, err
	}
	remoteURL, err := worker.WorkspaceRemoteURL(ctx, workspace.Path)
	if err != nil {
		return WorkerWorkspaceSetup{}, err
	}
	localSnapshot, err := a.git.Inspect(ctx, workspace.Path)
	if err != nil {
		return WorkerWorkspaceSetup{}, err
	}
	prepared, err := a.workerClient.PrepareWorkspace(ctx, config, workspaceID, worker.WorkspacePrepareRequest{
		RemoteURL: remoteURL, Revision: revision,
	})
	if err != nil {
		return WorkerWorkspaceSetup{}, err
	}
	return WorkerWorkspaceSetup{Mapping: prepared.Mapping, Local: localSnapshot, Remote: prepared.Git}, nil
}

type remoteExecutor struct {
	registry     *worker.Registry
	client       *worker.Client
	permissions  *remotePermissionRegistry
	preparations *remotePreparationRegistry
}

type remotePreparationCall struct {
	done chan struct{}
	err  error
}

type remotePreparationRegistry struct {
	mu       sync.Mutex
	inFlight map[string]*remotePreparationCall
	ready    map[string]time.Time
}

func newRemotePreparationRegistry() *remotePreparationRegistry {
	return &remotePreparationRegistry{inFlight: make(map[string]*remotePreparationCall), ready: make(map[string]time.Time)}
}

func (r *remotePreparationRegistry) ensure(ctx context.Context, key string, prepare func() error) error {
	r.mu.Lock()
	if expiresAt := r.ready[key]; time.Now().Before(expiresAt) {
		r.mu.Unlock()
		return nil
	}
	if call := r.inFlight[key]; call != nil {
		r.mu.Unlock()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-call.done:
			return call.err
		}
	}
	call := &remotePreparationCall{done: make(chan struct{})}
	r.inFlight[key] = call
	r.mu.Unlock()

	call.err = prepare()
	r.mu.Lock()
	delete(r.inFlight, key)
	if call.err == nil {
		r.ready[key] = time.Now().Add(5 * time.Second)
	} else {
		delete(r.ready, key)
	}
	close(call.done)
	r.mu.Unlock()
	return call.err
}

func (e *remoteExecutor) RunRemote(ctx context.Context, workerID, workspaceID string, request agentrun.Request, sink agentrun.Sink) (agentrun.Result, error) {
	config, err := e.registry.Get(ctx, workerID)
	if err != nil || !config.Enabled {
		return agentrun.Result{}, worker.RemoteError{Code: "worker_not_found", Message: "worker is missing or disabled"}
	}
	baseRevision, err := worker.WorkspaceBaseline(ctx, request.Workspace)
	if err != nil {
		return agentrun.Result{}, err
	}
	remoteURL, err := worker.WorkspaceRemoteURL(ctx, request.Workspace)
	if err != nil {
		return agentrun.Result{}, err
	}
	prepare := func() error {
		_, prepareErr := e.client.PrepareWorkspace(ctx, config, workspaceID, worker.WorkspacePrepareRequest{
			RemoteURL: remoteURL, Revision: baseRevision,
		})
		return prepareErr
	}
	preparationKey := workerID + "\x00" + workspaceID + "\x00" + baseRevision
	if e.preparations != nil {
		err = e.preparations.ensure(ctx, preparationKey, prepare)
	} else {
		err = prepare()
	}
	if err != nil {
		return agentrun.Result{}, err
	}
	runID, err := newRunID()
	if err != nil {
		return agentrun.Result{}, err
	}
	writable := request.Sandbox != agentrun.SandboxReadOnly

	// The streaming POST is deliberately detached from ctx. A cancel must become
	// a graceful interrupt — SIGINT plus grace on the remote agent, so it can
	// flush its final events down the open stream — not a hard connection reset
	// that discards them. A backstop hard-cancels if the worker never closes.
	streamCtx, cancelStream := context.WithCancel(context.Background())
	defer cancelStream()
	if e.permissions != nil {
		defer e.permissions.clearRemoteRun(runID)
	}
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-done:
			return
		case <-ctx.Done():
		}
		interruptCtx, cancelInterrupt := context.WithTimeout(context.Background(), 8*time.Second)
		_ = e.client.Interrupt(interruptCtx, config, runID)
		cancelInterrupt()
		grace := request.InterruptGrace + 5*time.Second
		timer := time.NewTimer(grace)
		defer timer.Stop()
		select {
		case <-done:
		case <-timer.C:
			cancelStream()
		}
	}()

	remoteSink := sink
	if e.permissions != nil {
		remoteSink = func(event agentrun.Event) {
			if event.Permission != nil {
				switch event.Kind {
				case agentrun.KindPermissionRequest:
					e.permissions.add(request.RunID, event.Permission.ID, remotePermissionTarget{config: config, remoteRunID: runID})
				case agentrun.KindPermissionResolved:
					e.permissions.remove(request.RunID, event.Permission.ID)
				}
			}
			if sink != nil {
				sink(event)
			}
		}
	}
	result, patch, executeErr := e.client.ExecuteWithPatch(streamCtx, config, worker.ExecuteRequest{
		RunID:                 runID,
		WorkspaceID:           workspaceID,
		Runtime:               request.Runtime,
		Model:                 request.Model,
		ReasoningEffort:       request.ReasoningEffort,
		ServiceTier:           request.ServiceTier,
		Provider:              request.Provider,
		Sandbox:               request.Sandbox,
		Prompt:                request.Prompt,
		ResumeSessionID:       request.ResumeSessionID,
		EnvironmentAllowlist:  append([]string{}, request.EnvironmentAllowlist...),
		TimeoutSeconds:        remainingSeconds(ctx),
		InterruptGraceSeconds: int(request.InterruptGrace / time.Second),
		BaseRevision:          baseRevision,
		SyncChanges:           writable,
	}, remoteSink)
	if patch == nil {
		return result, executeErr
	}
	syncCtx, cancelSync := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancelSync()
	if err := worker.ApplyWorkspacePatch(syncCtx, request.Workspace, *patch); err != nil {
		return result, err
	}
	ackCtx, cancelAck := context.WithTimeout(context.Background(), 20*time.Second)
	ackErr := e.client.AckPatch(ackCtx, config, runID, patch.Digest)
	cancelAck()
	if ackErr != nil {
		// The first acknowledgement may have reached the worker even when its
		// response was lost. The endpoint is idempotent, so one bounded retry
		// safely resolves that ambiguous transport state.
		retryCtx, cancelRetry := context.WithTimeout(context.Background(), 20*time.Second)
		ackErr = e.client.AckPatch(retryCtx, config, runID, patch.Digest)
		cancelRetry()
	}
	if ackErr != nil {
		return result, fmt.Errorf("remote changes were applied locally but worker cleanup was not acknowledged: %w", ackErr)
	}
	return result, executeErr
}

func remainingSeconds(ctx context.Context) int {
	deadline, ok := ctx.Deadline()
	if !ok {
		return 24 * 60 * 60
	}
	remaining := time.Until(deadline)
	if remaining <= 0 {
		return 1
	}
	// Round up so transport latency does not shorten the configured workflow
	// timeout by almost a full second.
	return int((remaining + time.Second - 1) / time.Second)
}

func newRunID() (string, error) {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return hex.EncodeToString(buffer), nil
}
