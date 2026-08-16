package wailstransport

import "testing"

func TestWindowBindingDelegatesToNativeWindowOwner(t *testing.T) {
	calls := map[string]int{}
	binding := NewWindowBinding(WindowCallbacks{
		OpenSettings:   func() { calls["settings"]++ },
		OpenWorkflows:  func() { calls["workflows"]++ },
		OpenInspector:  func() { calls["inspector-open"]++ },
		CloseInspector: func() { calls["inspector-close"]++ },
	})

	binding.OpenSettings()
	binding.OpenWorkflows()
	binding.OpenInspector()
	binding.CloseInspector()

	for _, name := range []string{"settings", "workflows", "inspector-open", "inspector-close"} {
		if calls[name] != 1 {
			t.Fatalf("unexpected %s callback count: %d", name, calls[name])
		}
	}
}

func TestWindowBindingAllowsMissingCallbacks(t *testing.T) {
	binding := NewWindowBinding(WindowCallbacks{})
	binding.OpenSettings()
	binding.OpenWorkflows()
	binding.OpenInspector()
	binding.CloseInspector()
}
