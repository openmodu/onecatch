package desktop

import (
	"context"
	"sync"
	"time"

	"github.com/openmodu/onecatch/internal/service/worker"
)

type remotePermissionTarget struct {
	config      worker.Config
	remoteRunID string
}

// remotePermissionRegistry joins the workflow-facing approval card to the
// worker-protocol run ID. Request IDs are provider-generated, so the workflow
// run check remains part of the key and prevents cross-run answers.
type remotePermissionRegistry struct {
	client *worker.Client
	mu     sync.Mutex
	items  map[string]remotePermissionTarget
}

func newRemotePermissionRegistry(client *worker.Client) *remotePermissionRegistry {
	return &remotePermissionRegistry{client: client, items: make(map[string]remotePermissionTarget)}
}

func remotePermissionKey(runID, requestID string) string { return runID + "\x00" + requestID }

func (r *remotePermissionRegistry) add(runID, requestID string, target remotePermissionTarget) {
	r.mu.Lock()
	r.items[remotePermissionKey(runID, requestID)] = target
	r.mu.Unlock()
}

func (r *remotePermissionRegistry) remove(runID, requestID string) {
	r.mu.Lock()
	delete(r.items, remotePermissionKey(runID, requestID))
	r.mu.Unlock()
}

func (r *remotePermissionRegistry) clearRemoteRun(remoteRunID string) {
	r.mu.Lock()
	for key, target := range r.items {
		if target.remoteRunID == remoteRunID {
			delete(r.items, key)
		}
	}
	r.mu.Unlock()
}

func (r *remotePermissionRegistry) resolve(runID, requestID, decision string) (bool, error) {
	r.mu.Lock()
	target, ok := r.items[remotePermissionKey(runID, requestID)]
	r.mu.Unlock()
	if !ok {
		return false, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := r.client.RespondPermission(ctx, target.config, target.remoteRunID, requestID, decision); err != nil {
		return true, err
	}
	r.remove(runID, requestID)
	return true, nil
}
