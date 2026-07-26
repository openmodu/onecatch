package worker

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/openmodu/oneshot/internal/agentrun"
	domainworkspaces "github.com/openmodu/oneshot/internal/domain/workspaces"
)

// maxFrameBytes bounds a single NDJSON line. It matches the runtime event
// persistence cap so a large tool output streamed as one frame is not rejected.
const maxFrameBytes = 34 * 1024 * 1024

type Client struct{ http *http.Client }

type legacyExecuteRequest struct {
	RunID                 string           `json:"runId"`
	WorkspaceID           string           `json:"workspaceId"`
	Runtime               agentrun.Runtime `json:"runtime"`
	Model                 string           `json:"model,omitempty"`
	ReasoningEffort       string           `json:"reasoningEffort,omitempty"`
	ServiceTier           string           `json:"serviceTier,omitempty"`
	Sandbox               agentrun.Sandbox `json:"sandbox"`
	Prompt                string           `json:"prompt"`
	ResumeSessionID       string           `json:"resumeSessionId,omitempty"`
	InterruptGraceSeconds int              `json:"interruptGraceSeconds,omitempty"`
}

const (
	controlRequestTimeout = 15 * time.Second
	streamShutdownGrace   = 2 * time.Minute
	legacyRunTimeout      = 35 * time.Minute
)

func NewClient() *Client { return &Client{http: &http.Client{}} }

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
	result, _, err := c.ExecuteWithPatch(ctx, config, input, sink)
	return result, err
}

// ExecuteWithPatch is Execute plus the recoverable worktree delta emitted by a
// writable run. The patch is returned even when the agent's terminal frame is
// an error so the coordinator can preserve changes made before that failure.
func (c *Client) ExecuteWithPatch(ctx context.Context, config Config, input ExecuteRequest, sink agentrun.Sink) (agentrun.Result, *WorkspacePatch, error) {
	payload, err := json.Marshal(input)
	if err != nil {
		return agentrun.Result{}, nil, err
	}
	streamCtx := ctx
	cancel := func() {}
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		timeout := legacyRunTimeout
		if input.TimeoutSeconds > 0 {
			timeout = time.Duration(input.TimeoutSeconds) * time.Second
		}
		streamCtx, cancel = context.WithTimeout(ctx, timeout+streamShutdownGrace)
	}
	defer cancel()
	response, err := c.openExecute(streamCtx, config, payload)
	if err != nil {
		return agentrun.Result{}, nil, RemoteError{Code: "worker_unavailable", Message: err.Error()}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		remoteErr := decodeRemoteError(response)
		_ = response.Body.Close()
		// The previous protocol rejected newly added fields because it decoded
		// requests strictly. Retry the exact pre-extension shape only when the
		// worker reports a decode failure; no execution has started at this point.
		var remote RemoteError
		if !input.SyncChanges && errors.As(remoteErr, &remote) && remote.Code == "worker_invalid_request" && remote.Message == "invalid execute request" {
			legacyPayload, marshalErr := json.Marshal(legacyExecuteRequest{
				RunID: input.RunID, WorkspaceID: input.WorkspaceID, Runtime: input.Runtime,
				Model: input.Model, ReasoningEffort: input.ReasoningEffort, ServiceTier: input.ServiceTier,
				Sandbox: input.Sandbox, Prompt: input.Prompt, ResumeSessionID: input.ResumeSessionID,
				InterruptGraceSeconds: input.InterruptGraceSeconds,
			})
			if marshalErr != nil {
				return agentrun.Result{}, nil, marshalErr
			}
			response, err = c.openExecute(streamCtx, config, legacyPayload)
			if err != nil {
				return agentrun.Result{}, nil, RemoteError{Code: "worker_unavailable", Message: err.Error()}
			}
			if response.StatusCode < 200 || response.StatusCode >= 300 {
				defer response.Body.Close()
				return agentrun.Result{}, nil, decodeRemoteError(response)
			}
		} else {
			return agentrun.Result{}, nil, remoteErr
		}
	}
	defer response.Body.Close()

	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), maxFrameBytes)
	var patch *WorkspacePatch
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
		case frame.Patch != nil:
			patch = frame.Patch
		case frame.Error != nil:
			return agentrun.Result{}, patch, *frame.Error
		case frame.Result != nil:
			return *frame.Result, patch, nil
		case frame.Event != nil:
			if sink != nil {
				sink(*frame.Event)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return agentrun.Result{}, patch, RemoteError{Code: "worker_stream_broken", Message: err.Error()}
	}
	return agentrun.Result{}, patch, RemoteError{Code: "worker_stream_incomplete", Message: "stream ended before a terminal frame"}
}

func (c *Client) openExecute(ctx context.Context, config Config, payload []byte) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint(config, "/v1/execute"), bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+config.Token)
	request.Header.Set("Content-Type", "application/json")
	httpClient, err := c.httpClient(config)
	if err != nil {
		return nil, err
	}
	return httpClient.Do(request)
}

// GitStatus reads the operational git state of a mapped worker clone.
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

// RespondPermission forwards a desktop approval decision to a Claude process
// blocked on the remote worker.
func (c *Client) RespondPermission(ctx context.Context, config Config, runID, requestID, decision string) error {
	path := "/v1/runs/" + url.PathEscape(runID) + "/permissions/" + url.PathEscape(requestID)
	return c.do(ctx, config, http.MethodPost, path, PermissionResponse{Decision: decision}, nil)
}

func (c *Client) AckPatch(ctx context.Context, config Config, runID, digest string) error {
	path := "/v1/runs/" + url.PathEscape(runID) + "/patch/ack"
	return c.do(ctx, config, http.MethodPost, path, PatchAckRequest{Digest: digest}, nil)
}

func (c *Client) do(ctx context.Context, config Config, method, path string, input, output any) error {
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, controlRequestTimeout)
		defer cancel()
	}
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
	httpClient, err := c.httpClient(config)
	if err != nil {
		return RemoteError{Code: "worker_tls_invalid", Message: err.Error()}
	}
	response, err := httpClient.Do(request)
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

func (c *Client) httpClient(config Config) (*http.Client, error) {
	if config.CAFile == "" && config.ClientCertFile == "" && config.ServerName == "" && config.ServerCertificateSHA256 == "" {
		return c.http, nil
	}
	parsed, err := url.Parse(config.BaseURL)
	if err != nil || parsed.Scheme != "https" {
		return nil, errors.New("worker TLS settings require an HTTPS base URL")
	}
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12, ServerName: config.ServerName}
	if config.CAFile != "" {
		pem, err := os.ReadFile(config.CAFile)
		if err != nil {
			return nil, fmt.Errorf("read worker CA: %w", err)
		}
		roots, err := x509.SystemCertPool()
		if err != nil || roots == nil {
			roots = x509.NewCertPool()
		}
		if !roots.AppendCertsFromPEM(pem) {
			return nil, errors.New("worker CA file contains no certificates")
		}
		tlsConfig.RootCAs = roots
	}
	if config.ClientCertFile != "" || config.ClientKeyFile != "" {
		if config.ClientCertFile == "" || config.ClientKeyFile == "" {
			return nil, errors.New("worker client certificate and key must be configured together")
		}
		certificate, err := tls.LoadX509KeyPair(config.ClientCertFile, config.ClientKeyFile)
		if err != nil {
			return nil, fmt.Errorf("load worker client certificate: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{certificate}
	}
	if config.ServerCertificateSHA256 != "" {
		expected := normalizeFingerprint(config.ServerCertificateSHA256)
		if !validFingerprint(expected) {
			return nil, errors.New("worker server certificate fingerprint is invalid")
		}
		// Exact certificate pinning performs the authentication in this mode.
		// InsecureSkipVerify only disables Go's CA/hostname verifier; the
		// callback below still rejects every certificate except the paired DER.
		tlsConfig.InsecureSkipVerify = true //nolint:gosec
		tlsConfig.VerifyConnection = func(state tls.ConnectionState) error {
			if len(state.PeerCertificates) == 0 {
				return errors.New("worker supplied no TLS certificate")
			}
			certificate := state.PeerCertificates[0]
			now := time.Now()
			if now.Before(certificate.NotBefore) || now.After(certificate.NotAfter) {
				return errors.New("worker TLS certificate is outside its validity period")
			}
			actual := sha256.Sum256(certificate.Raw)
			if fmt.Sprintf("%x", actual[:]) != expected {
				return errors.New("worker TLS certificate does not match the paired fingerprint")
			}
			return nil
		}
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = tlsConfig
	transport.DisableKeepAlives = true
	return &http.Client{Transport: transport}, nil
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
