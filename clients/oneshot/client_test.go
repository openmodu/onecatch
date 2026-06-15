package oneshot

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestHTTPClientListOrdersPreservesStatusQuery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/orders" {
			t.Fatalf("path = %q, want /api/orders", r.URL.Path)
		}
		if got := r.URL.Query().Get("status"); got != "running" {
			t.Fatalf("status query = %q, want running", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer server.Close()

	baseURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse server URL: %v", err)
	}
	client := &HTTPClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: time.Second,
		},
	}

	if _, err := client.ListOrders(context.Background(), "running"); err != nil {
		t.Fatalf("ListOrders() error = %v", err)
	}
}
