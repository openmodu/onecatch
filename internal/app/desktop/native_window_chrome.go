package desktop

import (
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// inheritNativeWindowAppearance runs before an auxiliary window is shown, so
// its AppKit background and WebView backing layers start in the same light or
// dark appearance as the already-visible main window.
func inheritNativeWindowAppearance(window, source *application.WebviewWindow) {
	if window == nil || source == nil || window.NativeWindow() == nil || source.NativeWindow() == nil {
		return
	}
	application.InvokeSync(func() {
		setNativeWindowAppearance(window.NativeWindow(), source.NativeWindow())
	})
}

// applyNativeWindowChrome keeps every full-size-content Oneshot window on the
// same native path: continuous corners, the outer hairline and the inset
// sidebar material are installed together and disabled at edge-to-edge sizes.
func applyNativeWindowChrome(window *application.WebviewWindow) {
	if window == nil {
		return
	}
	applyCorner := func(radius float64) {
		application.InvokeSync(func() {
			setNativeWindowCornerRadius(window.NativeWindow(), radius)
		})
	}
	window.OnWindowEvent(events.Common.WindowRuntimeReady, func(*application.WindowEvent) { applyCorner(26) })
	window.OnWindowEvent(events.Common.WindowMaximise, func(*application.WindowEvent) { applyCorner(0) })
	window.OnWindowEvent(events.Common.WindowUnMaximise, func(*application.WindowEvent) { applyCorner(26) })
	window.OnWindowEvent(events.Common.WindowFullscreen, func(*application.WindowEvent) { applyCorner(0) })
	window.OnWindowEvent(events.Common.WindowUnFullscreen, func(*application.WindowEvent) { applyCorner(26) })

	// Auxiliary windows are created while the application is already running,
	// so their native view may exist before the runtime-ready listener is added.
	// Do not dispatch before App.Run for the startup window: its main queue has
	// not started yet and a synchronous dispatch would wait forever.
	if window.NativeWindow() != nil {
		applyCorner(26)
	}
}

// hideNativeWindowZoomButton gives fixed-size utility windows the conventional
// two-light macOS chrome: close and minimise remain, while zoom/full screen is
// removed instead of merely appearing disabled.
func hideNativeWindowZoomButton(window *application.WebviewWindow) {
	if window == nil {
		return
	}
	apply := func() {
		application.InvokeSync(func() {
			setNativeWindowZoomButtonHidden(window.NativeWindow(), true)
		})
	}
	window.OnWindowEvent(events.Common.WindowRuntimeReady, func(*application.WindowEvent) { apply() })
	if window.NativeWindow() != nil {
		apply()
	}
}
