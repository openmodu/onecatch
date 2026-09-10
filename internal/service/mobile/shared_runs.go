package mobile

import (
	"context"
	"strings"
	"time"

	"github.com/openmodu/onecatch/internal/service/worker"
)

func (s *Service) cacheSharedRun(config worker.Config, view RunView) {
	view.WorkerID = config.ID
	view.Shared = true
	s.mu.Lock()
	previous := s.runs[view.ID]
	s.runs[view.ID] = &runState{config: config, view: copyRunView(view), window: view.EventsOffset, cachedAt: time.Now()}
	// Persisting rewrites the whole history file. That is nothing on a laptop
	// and real work on a phone, so a refresh that changed nothing skips it.
	if previous == nil || !sameCachedRun(previous.view, view) {
		_ = s.persistRunsLocked()
	}
	s.mu.Unlock()
}

// RenameConversation and DeleteConversation act on every run of a session at
// once, because a conversation is what the phone lists. A shared one is the
// host's record, so the host decides; a phone-only one lives here alone.
func (s *Service) RenameConversation(ctx context.Context, conversationID, title string) error {
	title = strings.TrimSpace(title)
	if title == "" || len([]rune(title)) > 160 {
		return worker.RemoteError{Code: "mobile_title_invalid", Message: "title must contain 1 to 160 characters"}
	}
	config, runs, shared := s.conversationRuns(conversationID)
	if len(runs) == 0 {
		return worker.RemoteError{Code: "mobile_run_not_found", Message: "conversation was not found on this device"}
	}
	if shared {
		if err := s.client.RenameSharedConversation(ctx, config, conversationID, title); err != nil {
			return err
		}
	}
	s.mu.Lock()
	for _, id := range runs {
		if state := s.runs[id]; state != nil {
			state.view.Title = title
		}
	}
	_ = s.persistRunsLocked()
	s.mu.Unlock()
	return nil
}

func (s *Service) DeleteConversation(ctx context.Context, conversationID string) error {
	config, runs, shared := s.conversationRuns(conversationID)
	if len(runs) == 0 {
		return worker.RemoteError{Code: "mobile_run_not_found", Message: "conversation was not found on this device"}
	}
	for _, id := range runs {
		s.mu.RLock()
		running := s.runs[id] != nil && s.runs[id].view.Status == "running"
		s.mu.RUnlock()
		if running {
			return worker.RemoteError{Code: "mobile_run_active", Message: "stop the running turn before deleting this conversation"}
		}
	}
	if shared {
		if err := s.client.RemoveSharedConversation(ctx, config, conversationID); err != nil {
			return err
		}
	}
	s.mu.Lock()
	for _, id := range runs {
		delete(s.runs, id)
	}
	_ = s.persistRunsLocked()
	s.mu.Unlock()
	return nil
}

// conversationRuns collects a session's runs, the worker that owns them and
// whether the host is the one holding the record.
func (s *Service) conversationRuns(conversationID string) (worker.Config, []string, bool) {
	conversationID = strings.TrimSpace(conversationID)
	s.mu.RLock()
	defer s.mu.RUnlock()
	var config worker.Config
	ids := []string{}
	shared := false
	for id, state := range s.runs {
		if state.view.ConversationID != conversationID && state.view.ID != conversationID {
			continue
		}
		ids = append(ids, id)
		if state.view.Shared {
			config, shared = state.config, true
		}
	}
	return config, ids, shared
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
		// Anything this device caches from here on — a turn the reader just
		// started — is newer than the listing below and survives the sweep.
		listedAt := time.Now()
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
			state := &runState{config: config, view: view}
			if previous := s.runs[view.ID]; previous != nil {
				// A listing carries no transcript, so the page the reader has
				// open survives the sync that refreshes its metadata.
				view.Events = previous.view.Events
				view.EventsOffset, view.EventsTotal = previous.view.EventsOffset, previous.view.EventsTotal
				view.Result = previous.view.Result
				state.view = view
				state.window, state.cachedAt = previous.window, previous.cachedAt
				if sameRunMetadata(previous.view, view) {
					continue
				}
			}
			s.runs[view.ID] = state
			changed = true
		}
		if s.sweepStaleRunsLocked(config.ID, found, listedAt) {
			changed = true
		}
		if changed {
			_ = s.persistRunsLocked()
		}
		s.mu.Unlock()
	}
}

// sweepStaleRunsLocked adopts the host's deletions. A run this device cached
// after the listing was taken — the turn the reader just sent — was never in
// it and stays, which is what lets a send skip the sync's lock entirely.
func (s *Service) sweepStaleRunsLocked(workerID string, found map[string]bool, listedAt time.Time) bool {
	changed := false
	for id, state := range s.runs {
		if state.cachedAt.After(listedAt) || !state.view.Shared || state.view.WorkerID != workerID || found[id] {
			continue
		}
		delete(s.runs, id)
		changed = true
	}
	return changed
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
