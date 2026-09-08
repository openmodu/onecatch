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
	s.runs[view.ID] = &runState{config: config, view: copyRunView(view)}
	_ = s.persistRunsLocked()
	s.mu.Unlock()
}

func (s *Service) syncSharedRuns(ctx context.Context) {
	s.syncMu.Lock()
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
				view.Events = previous.view.Events
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
		left.Title == right.Title && left.Status == right.Status && left.Shared == right.Shared && left.Error == right.Error &&
		left.StartedAt.Equal(right.StartedAt) && sameOptionalTime(left.FinishedAt, right.FinishedAt)
}

func sameOptionalTime(left, right *time.Time) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.Equal(*right)
}
