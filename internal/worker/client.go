package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct{ http *http.Client }

func NewClient() *Client { return &Client{http: &http.Client{Timeout: 35 * time.Minute}} }

func (c *Client) Health(ctx context.Context, config Config) (Health, error) {
	var health Health
	if err := c.do(ctx, config, http.MethodGet, "/v1/health", nil, &health); err != nil {
		return Health{}, err
	}
	return health, nil
}

func (c *Client) Execute(ctx context.Context, config Config, input ExecuteRequest) (ExecuteResponse, error) {
	var response ExecuteResponse
	if err := c.do(ctx, config, http.MethodPost, "/v1/execute", input, &response); err != nil {
		return ExecuteResponse{}, err
	}
	return response, nil
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
	request, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(config.BaseURL, "/")+path, body)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+config.Token)
	request.Header.Set("Content-Type", "application/json")
	response, err := c.http.Do(request)
	if err != nil {
		return RemoteError{Code: "worker_unavailable", Message: err.Error()}
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var remote RemoteError
		if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&remote); err != nil || remote.Code == "" {
			remote = RemoteError{Code: "worker_unavailable", Message: fmt.Sprintf("worker returned HTTP %d", response.StatusCode)}
		}
		return remote
	}
	return json.NewDecoder(io.LimitReader(response.Body, 16<<20)).Decode(output)
}
