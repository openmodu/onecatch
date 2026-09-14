// Package processmode identifies internal invocations of the OneCatch executable.
// It intentionally has no application dependencies.
package processmode

import (
	"os"
	"sync"
)

const Env = "ONECATCH_INTERNAL_MODE"

var executable string
var mu sync.RWMutex

// Register records the unified entry point. Tests and external library users do
// not register, and retain explicit/legacy helper discovery.
func Register() {
	path, err := os.Executable()
	if err == nil {
		mu.Lock()
		executable = path
		mu.Unlock()
	}
}
func Executable() string {
	mu.RLock()
	defer mu.RUnlock()
	return executable
}
