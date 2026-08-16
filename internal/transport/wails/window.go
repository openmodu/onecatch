package wailstransport

// WindowCallbacks names every native window command the web UI can issue. A nil
// entry turns the matching binding method into a no-op, which keeps the binding
// usable in tests and on platforms where a window role does not exist.
type WindowCallbacks struct {
	OpenSettings   func()
	OpenWorkflows  func()
	OpenInspector  func()
	CloseInspector func()
}

// WindowBinding exposes the small piece of native window management that the
// web UI needs. The actual windows are owned by the desktop package; keeping
// callbacks here avoids coupling the transport layer to a particular window
// layout or platform implementation.
type WindowBinding struct {
	callbacks WindowCallbacks
}

func NewWindowBinding(callbacks WindowCallbacks) *WindowBinding {
	return &WindowBinding{callbacks: callbacks}
}

func (b *WindowBinding) OpenSettings() {
	b.invoke(func(callbacks WindowCallbacks) func() { return callbacks.OpenSettings })
}

func (b *WindowBinding) OpenWorkflows() {
	b.invoke(func(callbacks WindowCallbacks) func() { return callbacks.OpenWorkflows })
}

// OpenInspector detaches the run inspector into its own window so it can live
// on a second display. CloseInspector puts it back in the main workbench.
func (b *WindowBinding) OpenInspector() {
	b.invoke(func(callbacks WindowCallbacks) func() { return callbacks.OpenInspector })
}

func (b *WindowBinding) CloseInspector() {
	b.invoke(func(callbacks WindowCallbacks) func() { return callbacks.CloseInspector })
}

func (b *WindowBinding) invoke(pick func(WindowCallbacks) func()) {
	if b == nil {
		return
	}
	if callback := pick(b.callbacks); callback != nil {
		callback()
	}
}
