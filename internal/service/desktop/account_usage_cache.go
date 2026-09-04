package desktop

import (
	"context"
	"path/filepath"
	"sync"
	"time"

	"github.com/openmodu/onecatch/internal/usecase/agentrun"
	"github.com/openmodu/onecatch/pkg/localfile"
)

const (
	accountUsageCacheFile    = "account-usage-cache.json"
	accountUsageCacheVersion = 2
	accountUsageSyncInterval = 30 * time.Minute
)

var accountUsageRuntimes = []agentrun.Runtime{
	agentrun.RuntimeCodex,
	agentrun.RuntimeClaude,
	agentrun.RuntimePi,
	agentrun.RuntimeGrok,
	agentrun.RuntimeModu,
}

type accountUsageCacheEntry struct {
	SyncedAt time.Time             `json:"syncedAt"`
	Usage    agentrun.AccountUsage `json:"usage"`
}

type accountUsageCacheSnapshot struct {
	Version int                               `json:"version"`
	Entries map[string]accountUsageCacheEntry `json:"entries"`
}

// accountUsageCache persists provider snapshots separately from workflow
// history. The provider owns the account-wide series, while the local file
// makes opening the page and restarting the app independent of CLI latency.
type accountUsageCache struct {
	mu      sync.RWMutex
	path    string
	now     func() time.Time
	entries map[string]accountUsageCacheEntry
}

func newAccountUsageCache(root string) *accountUsageCache {
	cache := &accountUsageCache{
		path:    filepath.Join(root, accountUsageCacheFile),
		now:     time.Now,
		entries: make(map[string]accountUsageCacheEntry),
	}
	var snapshot accountUsageCacheSnapshot
	if err := localfile.ReadJSON(cache.path, &snapshot); err == nil && snapshot.Version == accountUsageCacheVersion {
		for runtime, entry := range snapshot.Entries {
			if entry.Usage.Runtime == agentrun.Runtime(runtime) && !entry.SyncedAt.IsZero() {
				cache.entries[runtime] = entry
			}
		}
	}
	return cache
}

func (c *accountUsageCache) get(runtime agentrun.Runtime, maxAge time.Duration) (agentrun.AccountUsage, bool, bool) {
	c.mu.RLock()
	entry, ok := c.entries[string(runtime)]
	now := c.now()
	c.mu.RUnlock()
	if !ok {
		return agentrun.AccountUsage{}, false, false
	}
	age := now.Sub(entry.SyncedAt)
	fresh := age >= 0 && age < maxAge
	return entry.Usage, true, fresh
}

func (c *accountUsageCache) put(runtime agentrun.Runtime, usage agentrun.AccountUsage) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	entries := make(map[string]accountUsageCacheEntry, len(c.entries)+1)
	for id, entry := range c.entries {
		entries[id] = entry
	}
	entries[string(runtime)] = accountUsageCacheEntry{SyncedAt: c.now(), Usage: usage}
	if err := localfile.WriteJSONAtomic(c.path, accountUsageCacheSnapshot{Version: accountUsageCacheVersion, Entries: entries}); err != nil {
		return err
	}
	c.entries = entries
	return nil
}

// accountUsageSyncLoop keeps the persisted snapshot warm even when the usage
// page is not open. A page read also enforces the same TTL, so sleep/wake or a
// delayed ticker can never make a stale cache look current.
func (a *Service) accountUsageSyncLoop() {
	defer a.accountUsageWG.Done()
	ticker := time.NewTicker(accountUsageSyncInterval)
	defer ticker.Stop()
	for {
		select {
		case <-a.rootCtx.Done():
			return
		case <-ticker.C:
			for _, runtime := range accountUsageRuntimes {
				ctx, cancel := context.WithTimeout(a.rootCtx, 25*time.Second)
				_, _ = a.getAccountUsage(ctx, string(runtime), true)
				cancel()
			}
		}
	}
}
