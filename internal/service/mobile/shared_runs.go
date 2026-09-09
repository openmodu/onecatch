package mobile

import (
	"context"
	"time"

	"github.com/openmodu/onecatch/internal/service/worker"
)

func (s *Service) cacheSharedRun(config worker.Config, view RunView) {
	view.WorkerID = config.ID
	view.Shared = true
	s.mu.Lock()
	previous := s.runs[view.ID]
	s.runs[view.ID] = &runState{config: config, view: copyRunView(view), window: view.EventsOffset}
	// Persisting rewrites the whole history file. That is nothing on a laptop
	// and real work on a phone, so a refresh that changed nothing skips it.
	if previous == nil || !sameCachedRun(previous.view, view) {
		_ = s.persistRunsLocked()
	}
	s.mu.Unlock()
}

func sameCachedRun(left, right RunView) bool {
	if !sameRunMetadata(left, right) || len(left.Events) != len(right.Events) ||
		left.EventsOffset != right.EventsOffset || left.EventsTotal != right.EventsTotal {
		return false
	}
	if left.Result == nil || right.Result == nil {
		return left.Result == right.Result
	}
	return *left.Result == *right.Result
}

// syncSharedRuns adopts the host's history. The phone polls it every couple of
// seconds, so a sync that outlives its interval must not stack: a queue of
// syncs holding syncMu is what starves GetRun, and a conversation whose body
// never arrives renders as its prompt and final message alone.
func (s *Service) syncSharedRuns(ctx context.Context) {
	if !s.syncMu.TryLock() {
		return
	}
	defer s.syncMu.Unlock()
	workers, err := s.registry.List(ctx)
	if err != nil {
		return
	}
	for _, info := range workers {
		config, err := s.enabledWorker(ctx, info.ID)
		if err != nil {
			continue
		}
		health, err := s.client.Health(ctx, config)
		if err != nil || !health.Capabilities["sharedRuns"] {
			continue
		}
		mappings, err := s.client.ListWorkspaces(ctx, config)
		if err != nil {
			continue
		}
		sharedWorkspaces := map[string]bool{}
		for _, mapping := range mappings {
			sharedWorkspaces[mapping.ID] = mapping.Shared
		}
		// Upload historical phone-only runs once before adopting host history.
		// A failed import leaves the original local record untouched for retry.
		s.mu.RLock()
		legacy := s.runViewsLocked()
		s.mu.RUnlock()
		for i := len(legacy) - 1; i >= 0; i-- {
			view := legacy[i]
			if view.Shared || view.WorkerID != config.ID || view.Status == "running" || !sharedWorkspaces[view.WorkspaceID] {
				continue
			}
			imported, err := s.client.ImportSharedRun(ctx, config, view)
			if err == nil {
				s.cacheSharedRun(config, imported)
			}
		}
		views := []RunView{}
		cursor := ""
		complete := false
		for {
			page, err := s.client.ListSharedRuns(ctx, config, cursor)
			if err != nil {
				break
			}
			views = append(views, page.Items...)
			if page.NextCursor == "" {
				complete = true
				break
			}
			if page.NextCursor == cursor {
				break
			}
			cursor = page.NextCursor
		}
		if !complete {
			continue
		} // An offline host never deletes cached history.
		s.mu.Lock()
		changed := false
		found := make(map[string]bool, len(views))
		for _, view := range views {
			view.WorkerID, view.Shared = config.ID, true
			found[view.ID] = true
			if previous := s.runs[view.ID]; previous != nil {
				// A listing carries no transcript, so the page the reader has
				// open survives the sync that refreshes its metadata.
				view.Events = previous.view.Events
				view.EventsOffset, view.EventsTotal = previous.view.EventsOffset, previous.view.EventsTotal
				view.Result = previous.view.Result
				if sameRunMetadata(previous.view, view) {
					continue
				}
			}
			s.runs[view.ID] = &runState{config: config, view: view}
			changed = true
		}
		for id, state := range s.runs {
			if state.view.Shared && state.view.WorkerID == config.ID && !found[id] {
				delete(s.runs, id)
				changed = true
			}
		}
		if changed {
			_ = s.persistRunsLocked()
		}
		s.mu.Unlock()
	}
}

func sameRunMetadata(left, right RunView) bool {
	return left.ID == right.ID && left.ConversationID == right.ConversationID && left.WorkerID == right.WorkerID &&
		left.WorkspaceID == right.WorkspaceID && left.Runtime == right.Runtime && left.Prompt == right.Prompt &&
		left.Title == right.Title && left.TurnCount == right.TurnCount && left.Status == right.Status && left.Shared == right.Shared && left.Error == right.Error &&
		left.StartedAt.Equal(right.StartedAt) && sameOptionalTime(left.FinishedAt, right.FinishedAt)
}

func sameOptionalTime(left, right *time.Time) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.Equal(*right)
}
