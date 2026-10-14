package keeldesktop

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMemoryDiagnosticsOptIn(t *testing.T) {
	t.Setenv("ONECATCH_MEMORY_DIAGNOSTICS", "")
	stop, err := startMemoryDiagnostics()
	if err != nil {
		t.Fatal(err)
	}
	stop()
	dir := t.TempDir()
	t.Setenv("ONECATCH_MEMORY_DIAGNOSTICS", dir)
	stop, err = startMemoryDiagnostics()
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	address, err := os.ReadFile(filepath.Join(dir, "address"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(address), "http://127.0.0.1:") {
		t.Fatalf("not loopback: %s", address)
	}
	for _, endpoint := range []string{"/stats", "/heap"} {
		response, err := http.Get(string(address) + endpoint)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusOK {
			t.Fatalf("%s: %s", endpoint, response.Status)
		}
	}
}
