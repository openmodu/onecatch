package wailstransport

// WindowBinding exposes the small piece of native window management that the
// web UI needs. The actual windows are owned by the desktop package; keeping
// callbacks here avoids coupling the transport layer to a particular window
// layout or platform implementation.
type WindowBinding struct {
	openSettings  func()
	openWorkflows func()
}

func NewWindowBinding(openSettings, openWorkflows func()) *WindowBinding {
	return &WindowBinding{openSettings: openSettings, openWorkflows: openWorkflows}
}

func (b *WindowBinding) OpenSettings() {
	if b != nil && b.openSettings != nil {
		b.openSettings()
	}
}

func (b *WindowBinding) OpenWorkflows() {
	if b != nil && b.openWorkflows != nil {
		b.openWorkflows()
	}
}
