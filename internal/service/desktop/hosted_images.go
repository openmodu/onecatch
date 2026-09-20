package desktop

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/openmodu/onecatch/internal/service/worker"
)

// A phone may read images from this run's local workspace and its managed
// attachments. Never turn a model-provided path into arbitrary host file access.
func (h *hostedRuns) ReadImage(ctx context.Context, runID, source string) ([]byte, string, error) {
	detail, err := h.app.GetRunDetail(ctx, runID)
	if err != nil {
		return nil, "", err
	}
	if err = h.workspace(ctx, detail.Task.WorkspaceID); err != nil {
		return nil, "", err
	}
	invalid := func() ([]byte, string, error) {
		return nil, "", coded("image_invalid", "image is outside this conversation or unavailable")
	}
	source = strings.TrimSpace(source)
	if source == "" || strings.HasPrefix(source, "//") {
		return invalid()
	}
	if strings.HasPrefix(source, "file:") {
		parsed, parseErr := url.Parse(source)
		if parseErr != nil || (parsed.Host != "" && parsed.Host != "localhost") {
			return invalid()
		}
		source = parsed.Path
	} else if parsed, parseErr := url.Parse(source); parseErr == nil && parsed.Scheme != "" {
		return invalid()
	}
	path := filepath.Clean(source)
	if !filepath.IsAbs(path) {
		path = filepath.Join(detail.Workspace.Path, path)
	}
	allowed := detail.Workspace.RemoteFS == nil && resolvedPathWithin(path, detail.Workspace.Path)
	// Paths outside the workspace must match attachments belonging to this run.
	for _, attachment := range detail.Task.Attachments {
		if filepath.Clean(attachment.StoredPath) == path && resolvedPathWithin(path, h.app.localAttachmentRoot()) {
			allowed = true
		}
	}
	for _, instruction := range detail.Instructions {
		for _, attachment := range instruction.Attachments {
			if filepath.Clean(attachment) == path && resolvedPathWithin(path, h.app.localAttachmentRoot()) {
				allowed = true
			}
		}
	}
	if !allowed {
		return invalid()
	}
	file, err := os.Open(path)
	if err != nil {
		return invalid()
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > worker.MaxSharedImageBytes {
		return invalid()
	}
	data, err := io.ReadAll(io.LimitReader(file, worker.MaxSharedImageBytes+1))
	if err != nil || len(data) == 0 || len(data) > worker.MaxSharedImageBytes {
		return invalid()
	}
	mimeType := http.DetectContentType(data)
	if !worker.IsPreviewImage(mimeType) {
		return invalid()
	}
	return data, mimeType, nil
}
