package mobile

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/openmodu/onecatch/internal/service/worker"
)

func reconnectFixture(t *testing.T, fingerprint string) (*Service, worker.Config) {
	t.Helper()
	s, err := NewService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	_, err = s.registry.Save(context.Background(), worker.Input{ID: "desktop", Name: "My computer", BaseURL: "https://127.0.0.1:1", Token: "paired-secret", ServerCertificateSHA256: fingerprint, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	config, err := s.registry.Get(context.Background(), "desktop")
	if err != nil {
		t.Fatal(err)
	}
	return s, config
}

func serverPin(server *httptest.Server) string {
	sum := sha256.Sum256(server.Certificate().Raw)
	return hex.EncodeToString(sum[:])
}

func TestReconnectRelocatesExistingPairingAndCoalescesRequests(t *testing.T) {
	var healthCalls, discoveries atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/health" || r.Method != http.MethodGet {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer paired-secret" {
			t.Error("missing paired token")
		}
		healthCalls.Add(1)
		_, _ = w.Write([]byte(`{"workerId":"desktop"}`))
	}))
	defer server.Close()
	s, original := reconnectFixture(t, serverPin(server))
	s.resolveWorker = func(ctx context.Context, pin string) ([]string, error) {
		discoveries.Add(1)
		if pin != original.ServerCertificateSHA256 {
			t.Error("wrong identity query")
		}
		return []string{server.URL}, nil
	}
	var group sync.WaitGroup
	for range 8 {
		group.Go(func() {
			config, err := s.enabledWorker(context.Background(), original.ID)
			if err != nil {
				t.Error(err)
				return
			}
			if config.BaseURL != server.URL || config.Token != original.Token || config.ServerCertificateSHA256 != original.ServerCertificateSHA256 {
				t.Errorf("pairing changed: %s", config.BaseURL)
			}
		})
	}
	group.Wait()
	if discoveries.Load() != 1 || healthCalls.Load() != 1 {
		t.Fatalf("discovery=%d health=%d", discoveries.Load(), healthCalls.Load())
	}
	saved, err := s.registry.Get(context.Background(), original.ID)
	if err != nil || saved.BaseURL != server.URL {
		t.Fatalf("address not saved: %s, %v", saved.BaseURL, err)
	}
}

func TestReconnectRejectsUntrustedDiscovery(t *testing.T) {
	for _, test := range []struct {
		name, pin, id string
		wantRequests  int32
	}{
		{name: "wrong certificate", pin: strings.Repeat("ab", 32), id: "desktop", wantRequests: 0},
		{name: "wrong worker identity", id: "other", wantRequests: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				_, _ = w.Write([]byte(`{"workerId":"` + test.id + `"}`))
			}))
			defer server.Close()
			pin := test.pin
			if pin == "" {
				pin = serverPin(server)
			}
			s, original := reconnectFixture(t, pin)
			s.resolveWorker = func(context.Context, string) ([]string, error) {
				return []string{"http://127.0.0.1:9", server.URL}, nil
			}
			if _, err := s.enabledWorker(context.Background(), original.ID); err == nil {
				t.Fatal("accepted untrusted worker")
			}
			if requests.Load() != test.wantRequests {
				t.Fatalf("HTTP requests=%d; certificate mismatch must never send credentials", requests.Load())
			}
			saved, _ := s.registry.Get(context.Background(), original.ID)
			if saved != original {
				t.Fatal("failed discovery modified pairing")
			}
		})
	}
}

func TestReconnectDoesNotResurrectDeletedWorker(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{"workerId":"desktop"}`)) }))
	defer server.Close()
	s, original := reconnectFixture(t, serverPin(server))
	s.resolveWorker = func(ctx context.Context, _ string) ([]string, error) {
		if err := s.registry.Delete(ctx, original.ID); err != nil {
			t.Fatal(err)
		}
		return []string{server.URL}, nil
	}
	if _, err := s.enabledWorker(context.Background(), original.ID); !errors.Is(err, worker.ErrNotFound) {
		t.Fatalf("got %v", err)
	}
}

func TestReconnectHonorsCancellation(t *testing.T) {
	s, original := reconnectFixture(t, strings.Repeat("ab", 32))
	s.resolveWorker = func(ctx context.Context, _ string) ([]string, error) { <-ctx.Done(); return nil, ctx.Err() }
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	started := time.Now()
	if _, err := s.enabledWorker(ctx, original.ID); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("got %v", err)
	}
	if time.Since(started) > time.Second {
		t.Fatal("cancellation was not prompt")
	}
}

func TestReconnectDoesNotOverwriteEditedWorker(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"workerId":"desktop"}`))
	}))
	defer server.Close()
	s, original := reconnectFixture(t, serverPin(server))
	s.resolveWorker = func(ctx context.Context, _ string) ([]string, error) {
		_, err := s.registry.Update(ctx, worker.UpdateInput{ID: original.ID, Name: "Edited name", BaseURL: original.BaseURL, ServerCertificateSHA256: original.ServerCertificateSHA256, Enabled: false})
		if err != nil {
			t.Fatal(err)
		}
		return []string{server.URL}, nil
	}
	if _, err := s.enabledWorker(context.Background(), original.ID); err == nil {
		t.Fatal("overwrote concurrent edit")
	}
	saved, _ := s.registry.Get(context.Background(), original.ID)
	if saved.Enabled || saved.Name != "Edited name" || saved.BaseURL != original.BaseURL {
		t.Fatal("lost user settings")
	}
}

func TestReconnectNeverReplaysMutation(t *testing.T) {
	var mutations atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/health" {
			_, _ = w.Write([]byte(`{"workerId":"desktop"}`))
			return
		}
		if r.Method != http.MethodPut {
			t.Errorf("expected PUT, got %s", r.Method)
		}
		mutations.Add(1)
		// Simulate a completed operation whose response was lost.
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		_ = conn.Close()
	}))
	defer server.Close()
	s, original := reconnectFixture(t, serverPin(server))
	s.resolveWorker = func(context.Context, string) ([]string, error) { return []string{server.URL}, nil }
	_, err := s.PrepareWorkspace(context.Background(), original.ID, "project", worker.WorkspacePrepareRequest{})
	if err == nil {
		t.Fatal("expected lost response")
	}
	if mutations.Load() != 1 {
		t.Fatalf("mutation sent %d times", mutations.Load())
	}
}

func TestHealthyPairedWorkerDoesNotUseDiscovery(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{"workerId":"desktop"}`)) }))
	defer server.Close()
	s, original := reconnectFixture(t, serverPin(server))
	if _, err := s.registry.Relocate(context.Background(), original, server.URL); err != nil {
		t.Fatal(err)
	}
	s.resolveWorker = func(context.Context, string) ([]string, error) {
		t.Error("healthy endpoint triggered discovery")
		return nil, nil
	}
	if _, err := s.enabledWorker(context.Background(), original.ID); err != nil {
		t.Fatal(err)
	}
}
