// Package hosting runs the worker server inside another process. The desktop
// app uses it so a phone can pair with the machine its user is already sitting
// at, instead of installing and pairing the standalone worker binary.
package hosting

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/openmodu/onecatch/internal/service/worker"
)

// DefaultPort deliberately differs from the standalone worker's 9231 so a
// developer running both on one machine does not have to resolve a port clash
// before either of them starts.
const DefaultPort = 9232

// PairingWindow matches the standalone worker: long enough to walk to the
// phone, short enough that a code left on screen stops working.
const PairingWindow = 10 * time.Minute

type Options struct {
	SharedRuns worker.SharedRuns
	// Root holds the persistent identity: the bearer token and the self-signed
	// certificate whose fingerprint paired phones pin.
	Root           string
	ID             string
	Name           string
	Port           int
	Engine         worker.Engine
	Git            worker.GitInspector
	MaxConcurrency int
	// SharedWorkspaces lists the projects the hosting process already has, so a
	// phone sees the desktop's own workspaces instead of an empty list it has
	// to populate by hand. Optional.
	SharedWorkspaces func(context.Context) ([]worker.SharedWorkspace, error)
}

type Pairing struct {
	Code      string    `json:"code"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type Status struct {
	Running     bool     `json:"running"`
	Port        int      `json:"port"`
	WorkerID    string   `json:"workerId"`
	Name        string   `json:"name"`
	Addresses   []string `json:"addresses"`
	Fingerprint string   `json:"fingerprint"`
	Pairing     *Pairing `json:"pairing,omitempty"`
}

type Host struct {
	options Options

	mu       sync.Mutex
	server   *http.Server
	service  *worker.Server
	listener net.Listener
	identity identity
	token    string
	pairing  *Pairing
}

func New(options Options) *Host {
	if options.Port <= 0 {
		options.Port = DefaultPort
	}
	if strings.TrimSpace(options.ID) == "" {
		options.ID = "desktop"
	}
	if strings.TrimSpace(options.Name) == "" {
		options.Name = "OneCatch Desktop"
	}
	return &Host{options: options}
}

// Start binds the listener before returning, so a port already in use is
// reported to the caller rather than surfacing later as a phone that cannot
// connect.
func (h *Host) Start(ctx context.Context) (Status, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.server != nil {
		return h.statusLocked(), nil
	}
	if h.options.Engine == nil {
		return Status{}, errors.New("hosting: an engine is required")
	}
	identity, err := loadOrCreateCertificate(h.options.Root, h.options.Name)
	if err != nil {
		return Status{}, fmt.Errorf("prepare certificate: %w", err)
	}
	token, _, err := worker.LoadOrCreateToken(h.options.Root)
	if err != nil {
		return Status{}, fmt.Errorf("prepare worker token: %w", err)
	}
	service := worker.NewServer(h.options.ID, h.options.Name, token, nil, h.options.Engine, h.options.MaxConcurrency)
	service.SetSharedRuns(h.options.SharedRuns)
	if err := service.SetWorkspaceRegistry(ctx, worker.NewWorkspaceRegistry(workspaceRegistryPath(h.options.Root))); err != nil {
		return Status{}, fmt.Errorf("load workspace mappings: %w", err)
	}
	if h.options.Git != nil {
		service.SetGitInspector(h.options.Git)
	}
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", h.options.Port))
	if err != nil {
		return Status{}, fmt.Errorf("listen on port %d: %w", h.options.Port, err)
	}
	server := &http.Server{
		Handler:           service.Handler(),
		TLSConfig:         &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{identity.certificate}},
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       time.Minute,
		WriteTimeout:      worker.MaxRunDuration + 2*time.Minute,
		IdleTimeout:       60 * time.Second,
	}
	h.server, h.service, h.listener, h.identity, h.token = server, service, listener, identity, token
	h.publishLocked(ctx)
	go func() {
		// The certificate and key already live in TLSConfig.
		if err := server.ServeTLS(listener, "", ""); err != nil && !errors.Is(err, http.ErrServerClosed) {
			h.recordStopped(server)
		}
	}()
	return h.statusLocked(), nil
}

// RefreshSharedWorkspaces re-publishes the hosting process's projects. The
// desktop calls it whenever its own workspace list changes; a stopped host
// simply has nothing to publish to.
func (h *Host) RefreshSharedWorkspaces(ctx context.Context) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.publishLocked(ctx)
}

func (h *Host) publishLocked(ctx context.Context) {
	if h.service == nil || h.options.SharedWorkspaces == nil {
		return
	}
	items, err := h.options.SharedWorkspaces(ctx)
	if err != nil {
		// A workspace list that cannot be read is not a reason to take the
		// worker down; the phone keeps whatever was published last.
		return
	}
	h.service.SetSharedWorkspaces(items)
}

func (h *Host) Stop(ctx context.Context) error {
	h.mu.Lock()
	server := h.server
	h.server, h.service, h.listener, h.pairing = nil, nil, nil, nil
	h.mu.Unlock()
	if server == nil {
		return nil
	}
	shutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return server.Close()
	}
	return nil
}

// Pair issues a fresh one-time code, replacing any code still on screen. The
// worker only accepts it over TLS, so the phone always pins a certificate.
func (h *Host) Pair() (Pairing, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.service == nil {
		return Pairing{}, errors.New("hosting: the worker is not running")
	}
	code, err := worker.NewPairingCode()
	if err != nil {
		return Pairing{}, err
	}
	pairing := Pairing{Code: code, ExpiresAt: time.Now().Add(PairingWindow).UTC()}
	h.service.EnablePairing(code, pairing.ExpiresAt, false)
	h.pairing = &pairing
	return pairing, nil
}

func (h *Host) Status() Status {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.statusLocked()
}

func (h *Host) statusLocked() Status {
	status := Status{
		Port: h.options.Port, WorkerID: h.options.ID, Name: h.options.Name,
		Running: h.server != nil, Fingerprint: h.identity.fingerprint,
	}
	if !status.Running {
		return status
	}
	status.Addresses = LANAddresses(h.options.Port)
	if h.pairing != nil && time.Now().Before(h.pairing.ExpiresAt) {
		pairing := *h.pairing
		status.Pairing = &pairing
	}
	return status
}

// recordStopped clears the state of a server that died on its own, so Status
// stops advertising a worker nothing is listening for.
func (h *Host) recordStopped(server *http.Server) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.server == server {
		h.server, h.service, h.listener, h.pairing = nil, nil, nil, nil
	}
}

// workspaceRegistryPath keeps the mappings a phone creates beside the identity
// they authenticate against, and out of the desktop's own workspace list.
func workspaceRegistryPath(root string) string {
	return filepath.Join(root, "workspaces.json")
}
