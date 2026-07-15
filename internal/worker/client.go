package worker

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/openmodu/oneshot/internal/agentrun"
	domainworkspaces "github.com/openmodu/oneshot/internal/domain/workspaces"
)

// maxFrameBytes bounds a single NDJSON line. It matches the runtime event
// persistence cap so a large tool output streamed as one frame is not rejected.
const maxFrameBytes = 8 * 1024 * 1024

type Client struct{ http *http.Client }

func NewClient() *Client { return &Client{http: &http.Client{Timeout: 35 * time.Minute}} }

func (c *Client) Health(ctx context.Context, config Config) (Health, error) {
	var health Health
	if err := c.do(ctx, config, http.MethodGet, "/v1/health", nil, &health); err != nil {
		return Health{}, err
	}
	return health, nil
}

// Execute runs a task on the worker and forwards each streamed event to sink as
// it arrives, returning the terminal result. Failures that occur before the
// stream opens surface as an error from the HTTP status; failures during the run
// surface as a terminal error frame. sink may be nil.
func (c *Client) Execute(ctx context.Context, config Config, input ExecuteRequest, sink agentrun.Sink) (agentrun.Result, error) {
	payload, err := json.Marshal(input)
	if err != nil {
		return agentrun.Result{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint(config, "/v1/execute"), bytes.NewReader(payload))
	if err != nil {
		return agentrun.Result{}, err
	}
	request.Header.Set("Authorization", "Bearer "+config.Token)
	request.Header.Set("Content-Type", "application/json")
	response, err := c.http.Do(request)
	if err != nil {
		return agentrun.Result{}, RemoteError{Code: "worker_unavailable", Message: err.Error()}
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return agentrun.Result{}, decodeRemoteError(response)
	}

	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), maxFrameBytes)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var frame Frame
		if err := json.Unmarshal(line, &frame); err != nil {
			continue
		}
		switch {
		case frame.Error != nil:
			return agentrun.Result{}, *frame.Error
		case frame.Result != nil:
			return *frame.Result, nil
		case frame.Event != nil:
			if sink != nil {
				sink(*frame.Event)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return agentrun.Result{}, RemoteError{Code: "worker_stream_broken", Message: err.Error()}
	}
	return agentrun.Result{}, RemoteError{Code: "worker_stream_incomplete", Message: "stream ended before a terminal frame"}
}

// GitStatus reads the git state of a mapped workspace on the worker, so the
// coordinator can show what a remote step changed without syncing files.
func (c *Client) GitStatus(ctx context.Context, config Config, workspaceID string) (domainworkspaces.GitSnapshot, error) {
	var snapshot domainworkspaces.GitSnapshot
	if err := c.do(ctx, config, http.MethodGet, "/v1/workspaces/"+url.PathEscape(workspaceID)+"/git", nil, &snapshot); err != nil {
		return domainworkspaces.GitSnapshot{}, err
	}
	return snapshot, nil
}

// Interrupt asks the worker to gracefully stop an in-flight run. A run that has
// already finished (or never existed) returns worker_run_not_found, which the
// caller can safely ignore.
func (c *Client) Interrupt(ctx context.Context, config Config, runID string) error {
	return c.do(ctx, config, http.MethodPost, "/v1/runs/"+runID+"/interrupt", nil, nil)
}

func (c *Client) do(ctx context.Context, config Config, method, path string, input, output any) error {
	var body io.Reader
	if input != nil {
		payload, err := json.Marshal(input)
		if err != nil {
			return err
		}
		body = bytes.NewReader(payload)
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint(config, path), body)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+config.Token)
	if input != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := c.http.Do(request)
	if err != nil {
		return RemoteError{Code: "worker_unavailable", Message: err.Error()}
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return decodeRemoteError(response)
	}
	if output == nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
		return nil
	}
	return json.NewDecoder(io.LimitReader(response.Body, 16<<20)).Decode(output)
}

func endpoint(config Config, path string) string {
	return strings.TrimRight(config.BaseURL, "/") + path
}

func decodeRemoteError(response *http.Response) error {
	var remote RemoteError
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&remote); err != nil || remote.Code == "" {
		return RemoteError{Code: "worker_unavailable", Message: fmt.Sprintf("worker returned HTTP %d", response.StatusCode)}
	}
	return remote
}
