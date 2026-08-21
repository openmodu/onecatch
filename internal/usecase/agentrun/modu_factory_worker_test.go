//go:build onecatch_worker

package agentrun

import "testing"

func TestWorkerBuildUsesModuCLIAdapter(t *testing.T) {
	if _, ok := NewEngine(Config{}).Runner(RuntimeModu).(*ModuRunner); !ok {
		t.Fatal("worker build must not link the native Modu SDK")
	}
}
