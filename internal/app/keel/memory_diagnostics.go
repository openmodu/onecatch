package keeldesktop

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"path/filepath"
	"runtime"
)

// Explicit local diagnostics only. The private directory contains the ephemeral
// loopback address; normal clients start no listener.
func startMemoryDiagnostics() (func(), error) {
	dir := os.Getenv("ONECATCH_MEMORY_DIAGNOSTICS")
	if dir == "" {
		return func() {}, nil
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/heap", pprof.Handler("heap").ServeHTTP)
	mux.HandleFunc("/allocs", pprof.Handler("allocs").ServeHTTP)
	mux.HandleFunc("/stats", func(w http.ResponseWriter, r *http.Request) {
		var stats runtime.MemStats
		runtime.ReadMemStats(&stats)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(stats)
	})
	server := &http.Server{Handler: mux}
	if err := os.WriteFile(filepath.Join(dir, "address"), []byte("http://"+ln.Addr().String()), 0600); err != nil {
		ln.Close()
		return nil, err
	}
	go server.Serve(ln)
	return func() { _ = server.Close() }, nil
}
