package mobile

import (
	"context"
	"strings"

	"github.com/openmodu/onecatch/internal/service/worker"
)

// Keep credentials and TLS pinning in the native client, out of WebView URLs.
func (s *Service) ReadConversationImage(ctx context.Context, runID, path string) ([]byte, string, error) {
	s.mu.RLock()
	state := s.runs[strings.TrimSpace(runID)]
	workerID := ""
	if state != nil && state.view.Shared {
		workerID = state.view.WorkerID
	}
	s.mu.RUnlock()
	if workerID == "" {
		return nil, "", worker.RemoteError{Code: "mobile_run_not_found", Message: "shared conversation was not found"}
	}
	config, err := s.enabledWorker(ctx, workerID)
	if err != nil {
		return nil, "", err
	}
	return s.client.SharedImage(ctx, config, runID, path)
}
