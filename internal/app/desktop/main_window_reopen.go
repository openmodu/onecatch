package desktop

import "github.com/wailsapp/wails/v3/pkg/application"

type reopenableMainWindow interface {
	UnMinimise()
	Show() application.Window
	Restore()
	Focus()
}

// reopenMainWindow handles a macOS Dock reopen only when the application has
// no visible window. Returning true tells the application-event hook to cancel
// Wails' default "show every hidden window" behaviour.
func reopenMainWindow(hasVisibleWindows bool, mainWindow reopenableMainWindow) bool {
	if hasVisibleWindows || mainWindow == nil {
		return false
	}
	mainWindow.UnMinimise()
	mainWindow.Show()
	mainWindow.Restore()
	mainWindow.Focus()
	return true
}
