package wailstransport

import "testing"

func TestWindowBindingDelegatesToNativeWindowOwner(t *testing.T) {
	settingsCalls := 0
	workflowCalls := 0
	binding := NewWindowBinding(func() { settingsCalls++ }, func() { workflowCalls++ })

	binding.OpenSettings()
	binding.OpenWorkflows()

	if settingsCalls != 1 || workflowCalls != 1 {
		t.Fatalf("unexpected callback counts: settings=%d workflows=%d", settingsCalls, workflowCalls)
	}
}

func TestWindowBindingAllowsMissingCallbacks(t *testing.T) {
	binding := NewWindowBinding(nil, nil)
	binding.OpenSettings()
	binding.OpenWorkflows()
}
