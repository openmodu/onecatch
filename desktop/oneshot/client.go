package main

import (
	"os"

	oneshot "github.com/openmodu/oneshot/clients/oneshot"
)

// defaultAPIBaseURL points at a locally running oneshot-server. Override with
// ONESHOT_API_BASE_URL to target a remote deployment.
const defaultAPIBaseURL = "http://127.0.0.1:8080"

// newDesktopClient returns an HTTP client bound to oneshot-server. The desktop
// app is a pure API client: all domain logic, persistence and auth live in the
// server, reached over HTTP (local now, a remote domain later).
func newDesktopClient() oneshot.Client {
	baseURL := os.Getenv("ONESHOT_API_BASE_URL")
	if baseURL == "" {
		baseURL = defaultAPIBaseURL
	}
	return oneshot.NewHTTPClient(baseURL)
}
