package worker

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
)

const ProviderVolcengineSandbox = "volcengine-sandbox"

// IsVolcengineSandboxURL is the deliberately unadvertised entry point. Pasting
// a gateway URL into the existing pairing dialog reveals the sandbox fields.
func IsVolcengineSandboxURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	return err == nil && u.Scheme == "https" && strings.HasSuffix(u.Hostname(), ".volceapi.com") && u.Query().Get("faasInstanceName") != ""
}

func normalizeSandboxInput(input *Input) error {
	if input.Provider == "" {
		return nil
	}
	if input.Provider != ProviderVolcengineSandbox {
		return errors.New("unsupported worker provider")
	}
	u, err := url.Parse(input.BaseURL)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Fragment != "" || u.Query().Get("faasInstanceName") == "" {
		return errors.New("sandbox requires an HTTPS gateway URL with faasInstanceName")
	}
	q := u.Query()
	if token := q.Get("Authorization"); token != "" {
		input.Token = token
	}
	q.Del("Authorization")
	// Never reuse a browser terminal: it may belong to an interactive user.
	for _, key := range []string{"session_id", "command", "protocol", "durable", "restore", "replay_bytes"} {
		q.Del(key)
	}
	u.RawQuery = q.Encode()
	if u.Path == "/terminal" || u.Path == "/v1/shell/ws" {
		u.Path = "/"
	}
	input.BaseURL = u.String()
	input.RemotePath = strings.TrimSpace(input.RemotePath)
	if input.RemotePath == "" {
		input.RemotePath = "/home/gem"
	}
	if !strings.HasPrefix(input.RemotePath, "/") || strings.ContainsAny(input.RemotePath, "\x00\r\n") {
		return errors.New("sandbox working directory must be an absolute POSIX path")
	}
	input.RemotePath = path.Clean(input.RemotePath)
	if input.CAFile != "" || input.ClientCertFile != "" || input.ClientKeyFile != "" || input.ServerName != "" || input.ServerCertificateSHA256 != "" {
		return errors.New("sandbox gateways use system TLS trust; worker pairing TLS settings are not supported")
	}
	return nil
}

func (c *Client) ConnectSandbox(ctx context.Context, rawURL, remotePath string) (Input, error) {
	input := Input{Provider: ProviderVolcengineSandbox, BaseURL: strings.TrimSpace(rawURL), RemotePath: remotePath, Enabled: true}
	if err := normalizeSandboxInput(&input); err != nil {
		return Input{}, err
	}
	if input.Token == "" {
		return Input{}, errors.New("sandbox URL is missing Authorization")
	}
	sum := sha256.Sum256([]byte(input.BaseURL + "\x00" + input.RemotePath))
	input.ID = fmt.Sprintf("sandbox-%x", sum[:8])
	input.Name = "codex volc_sandbox"
	config := Config{BaseURL: input.BaseURL, Token: input.Token, RemotePath: input.RemotePath, Provider: input.Provider}
	// Probe through the same WebSocket transport used by tasks. No model turn,
	// workspace files or software installation is involved.
	probeCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	stream, err := c.openSandboxCodex(probeCtx, config, nil)
	if err != nil {
		return Input{}, err
	}
	defer stream.Close()
	if err := json.NewEncoder(stream).Encode(map[string]any{"id": 1, "method": "initialize", "params": map[string]any{"clientInfo": map[string]string{"name": "onecatch", "version": "0.1.0"}}}); err != nil {
		return Input{}, err
	}
	dec := json.NewDecoder(stream)
	for {
		var message struct {
			ID     int             `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  json.RawMessage `json:"error"`
		}
		if err := dec.Decode(&message); err != nil {
			return Input{}, errors.New("sandbox Codex initialization failed")
		}
		if message.ID != 1 {
			continue
		}
		if len(message.Error) > 0 || len(message.Result) == 0 {
			return Input{}, errors.New("sandbox Codex rejected initialization")
		}
		return input, nil
	}
}

func sandboxURL(config Config, route string, websocketURL bool) (string, error) {
	u, err := url.Parse(config.BaseURL)
	if err != nil || u.Host == "" {
		return "", errors.New("invalid sandbox gateway URL")
	}
	u.Path = strings.TrimRight(u.Path, "/") + route
	q := u.Query()
	q.Set("Authorization", config.Token)
	u.RawQuery = q.Encode()
	if websocketURL {
		if u.Scheme == "https" {
			u.Scheme = "wss"
		} else {
			u.Scheme = "ws"
		}
	}
	return u.String(), nil
}

func (c *Client) sandboxHealth(ctx context.Context, config Config) (Health, error) {
	endpoint, err := sandboxURL(config, "/health", false)
	if err != nil {
		return Health{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, controlRequestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Health{}, errors.New("invalid sandbox health request")
	}
	response, err := sandboxHTTPClient().Do(req)
	if err != nil {
		return Health{}, errors.New("sandbox gateway is unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return Health{}, fmt.Errorf("sandbox health returned HTTP %d", response.StatusCode)
	}
	return Health{WorkerID: config.ID, Name: config.Name, ProtocolVersion: 1, Runtimes: map[string]bool{"codex": true}, Capabilities: map[string]bool{"sandbox": true, "workspaceSync": false}}, nil
}

func sandboxHTTPClient() *http.Client {
	return &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

func (c *Client) RunSandbox(ctx context.Context, config Config, req agentrun.Request, sink agentrun.Sink) (agentrun.Result, error) {
	if req.Runtime != agentrun.RuntimeCodex {
		return agentrun.Result{}, errors.New("sandbox workers support Codex only")
	}
	if req.Remote != nil {
		return agentrun.Result{}, errors.New("sandbox workers cannot run an SSH workspace")
	}
	args := []string{}
	if req.ServiceTier == "fast" {
		args = append(args, "-c", "features.fast_mode=true")
	}
	stream, err := c.openSandboxCodex(ctx, config, args)
	if err != nil {
		return agentrun.Result{}, err
	}
	req.Workspace = config.RemotePath
	// Local paths and environment variables must never be sent to this target.
	req.Environment = nil
	req.EnvironmentAllowlist = nil
	return agentrun.RunCodexStream(ctx, req, stream, sink)
}

type sandboxFrame struct {
	Type      string          `json:"type"`
	Data      json.RawMessage `json:"data"`
	Timestamp json.RawMessage `json:"timestamp"`
}

// sandboxStream strips the shell bootstrap and exposes app-server's NDJSON.
// It never reconnects/replays input: an ambiguous disconnect must fail the turn
// rather than execute a user's prompt twice.
type sandboxStream struct {
	conn        *websocket.Conn
	config      Config
	sessionID   string
	pending     []byte
	sessionMu   sync.Mutex
	writeMu     sync.Mutex
	closeOnce   sync.Once
	stopContext func() bool
}

func (c *Client) openSandboxCodex(ctx context.Context, config Config, args []string) (*sandboxStream, error) {
	command := "codex app-server --listen stdio://"
	for _, arg := range args {
		command += " " + sandboxQuote(arg)
	}
	return c.openSandboxProcess(ctx, config, command)
}

// Each process has a dedicated PTY and connection; file transfers never share
// the app-server stream.
func (c *Client) openSandboxProcess(ctx context.Context, config Config, process string) (*sandboxStream, error) {
	endpoint, err := sandboxURL(config, "/v1/shell/ws", true)
	if err != nil {
		return nil, err
	}
	dialer := *websocket.DefaultDialer
	dialer.HandshakeTimeout = 15 * time.Second
	conn, response, err := dialer.DialContext(ctx, endpoint, nil)
	if err != nil {
		if response != nil && response.Body != nil {
			response.Body.Close()
		}
		return nil, errors.New("sandbox WebSocket connection failed")
	}
	s := &sandboxStream{conn: conn, config: config}
	s.stopContext = context.AfterFunc(ctx, func() { _ = conn.Close() })
	ok := false
	defer func() {
		if !ok {
			_ = s.Close()
		}
	}()
	conn.SetReadLimit(16 << 20)
	_ = conn.SetReadDeadline(time.Now().Add(20 * time.Second))
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, err
	}
	marker := "\nONECATCH_" + hex.EncodeToString(nonce[:]) + "\n"
	command := "stty -echo -icanon -opost && cd " + sandboxQuote(config.RemotePath) + " && printf " + sandboxQuote(marker) + " && exec " + process
	// Send the marker as escaped printf input; its literal newline-delimited
	// value must not appear in the shell's echo of the bootstrap command.
	command = strings.ReplaceAll(command, "\n", "\\n") + "\n"
	ready := false
	var bootstrap []byte
	for {
		frame, data, err := s.readFrame()
		if err != nil {
			return nil, errors.New("sandbox Codex did not start; check its working directory and Codex installation")
		}
		if frame.Type == "ready" && !ready {
			ready = true
			if err := s.send("input", command); err != nil {
				return nil, err
			}
		}
		if frame.Type != "output" {
			continue
		}
		bootstrap = append(bootstrap, data...)
		if index := bytes.Index(bootstrap, []byte(marker)); index >= 0 {
			s.pending = append([]byte(nil), bootstrap[index+len(marker):]...)
			_ = conn.SetReadDeadline(time.Time{})
			ok = true
			return s, nil
		}
		if len(bootstrap) > 64<<10 {
			return nil, errors.New("sandbox bootstrap output exceeded limit")
		}
	}
}

func sandboxQuote(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'" }

func (s *sandboxStream) send(kind string, data any) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_ = s.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	if err := s.conn.WriteJSON(map[string]any{"type": kind, "data": data}); err != nil {
		return errors.New("sandbox WebSocket write failed")
	}
	return nil
}

func (s *sandboxStream) readFrame() (sandboxFrame, string, error) {
	for {
		var frame sandboxFrame
		if err := s.conn.ReadJSON(&frame); err != nil {
			return frame, "", errors.New("sandbox WebSocket closed before completion")
		}
		var data string
		_ = json.Unmarshal(frame.Data, &data)
		switch frame.Type {
		case "session_id":
			s.sessionMu.Lock()
			if s.sessionID == "" {
				s.sessionID = data
			}
			s.sessionMu.Unlock()
		case "ping":
			if err := s.send("pong", map[string]any{"timestamp": frame.Timestamp}); err != nil {
				return frame, "", err
			}
			continue
		case "error":
			return frame, "", errors.New("sandbox shell reported an error")
		case "process_exit":
			return frame, "", io.EOF
		}
		return frame, data, nil
	}
}

func (s *sandboxStream) Read(p []byte) (int, error) {
	for len(s.pending) == 0 {
		frame, data, err := s.readFrame()
		if err != nil {
			return 0, err
		}
		if frame.Type == "output" {
			s.pending = []byte(data)
		}
	}
	n := copy(p, s.pending)
	s.pending = s.pending[n:]
	return n, nil
}
func (s *sandboxStream) Write(p []byte) (int, error) {
	if err := s.send("input", string(p)); err != nil {
		return 0, err
	}
	return len(p), nil
}
func (s *sandboxStream) Close() error {
	s.closeOnce.Do(func() {
		if s.stopContext != nil {
			s.stopContext()
		}
		// Closing the socket unblocks the reader even when the gateway is down.
		_ = s.conn.Close()
		s.sessionMu.Lock()
		sessionID := s.sessionID
		s.sessionMu.Unlock()
		if sessionID == "" {
			return
		}
		endpoint, err := sandboxURL(s.config, "/v1/shell/sessions/"+url.PathEscape(sessionID), false)
		if err != nil {
			return
		}
		req, err := http.NewRequest(http.MethodDelete, endpoint, nil)
		if err != nil {
			return
		}
		response, err := sandboxHTTPClient().Do(req)
		if err == nil {
			response.Body.Close()
		}
	})
	return nil
}
