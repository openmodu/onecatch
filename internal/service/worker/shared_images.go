package worker

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

const MaxSharedImageBytes = 20 << 20

// Optional capability so workers without a desktop history remain compatible.
type SharedImages interface {
	ReadImage(ctx context.Context, runID, path string) ([]byte, string, error)
}

func (s *Server) sharedImage(w http.ResponseWriter, r *http.Request) {
	images, ok := s.sharedRuns.(SharedImages)
	if !ok {
		writeError(w, http.StatusNotImplemented, "images_unavailable", "this worker does not share images")
		return
	}
	data, mimeType, err := images.ReadImage(r.Context(), r.PathValue("runID"), r.URL.Query().Get("path"))
	if err != nil {
		writeError(w, http.StatusNotFound, "image_unavailable", "image is unavailable in this conversation")
		return
	}
	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Cache-Control", "private, no-cache")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(data)
}

func (c *Client) SharedImage(ctx context.Context, config Config, runID, path string) ([]byte, string, error) {
	ctx, cancel := context.WithTimeout(ctx, controlRequestTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint(config, "/v1/shared-runs/"+url.PathEscape(runID)+"/image?path="+url.QueryEscape(path)), nil)
	if err != nil {
		return nil, "", err
	}
	request.Header.Set("Authorization", "Bearer "+config.Token)
	client, err := c.httpClient(config)
	if err != nil {
		return nil, "", err
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, "", decodeRemoteError(response)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, MaxSharedImageBytes+1))
	if err != nil {
		return nil, "", err
	}
	mimeType := http.DetectContentType(data)
	if len(data) == 0 || len(data) > MaxSharedImageBytes || !IsPreviewImage(mimeType) {
		return nil, "", fmt.Errorf("invalid image response")
	}
	return data, mimeType, nil
}

func IsPreviewImage(mimeType string) bool {
	switch mimeType {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
		return true
	}
	return false
}
