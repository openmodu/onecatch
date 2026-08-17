package desktop

import (
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

const (
	settingsWindowName  = "settings"
	workflowsWindowName = "workflows"
	inspectorWindowName = "inspector"
)

// The detached inspector reports its own lifecycle on this application-wide
// channel so the main window can keep its dock/detach toggle in sync with
// reality — including when the user closes the window from its title bar
// rather than from the re-dock button.
const inspectorWindowEvent = "onecatch:inspector-window"

type auxiliaryWindowController struct {
	app     *application.App
	mu      sync.Mutex
	loading map[string]bool
}

func newAuxiliaryWindowController(app *application.App) *auxiliaryWindowController {
	return &auxiliaryWindowController{app: app, loading: make(map[string]bool)}
}

func (c *auxiliaryWindowController) OpenSettings() {
	c.open(auxiliaryWindowOptions{
		name:           settingsWindowName,
		title:          "设置",
		url:            "/?window=settings",
		width:          960,
		height:         800,
		minWidth:       860,
		minHeight:      600,
		disableResize:  false,
		hideZoomButton: true,
		customChrome:   true,
	})
}

func (c *auxiliaryWindowController) OpenWorkflows() {
	c.open(auxiliaryWindowOptions{
		name:         workflowsWindowName,
		title:        "工作流",
		url:          "/?window=workflows",
		width:        1040,
		height:       760,
		minWidth:     860,
		minHeight:    600,
		customChrome: true,
	})
}

// OpenInspector floats the run inspector in its own window. It is deliberately
// tall and narrow: the point is to park it on a second display beside the
// workbench, not to reproduce the workbench itself.
func (c *auxiliaryWindowController) OpenInspector() {
	c.open(auxiliaryWindowOptions{
		name:         inspectorWindowName,
		title:        "状态栏",
		url:          "/?window=inspector",
		width:        420,
		height:       860,
		minWidth:     320,
		minHeight:    380,
		customChrome: true,
		announce:     inspectorWindowEvent,
	})
}

func (c *auxiliaryWindowController) CloseInspector() {
	c.close(inspectorWindowName)
}

type auxiliaryWindowOptions struct {
	name                string
	title, url          string
	width, height       int
	minWidth, minHeight int
	disableResize       bool
	hideZoomButton      bool
	customChrome        bool
	// announce names an application event emitted with {"open": bool} whenever
	// this window becomes visible or closes. Empty means the window's lifecycle
	// is of no interest to the rest of the UI.
	announce string
}

func (c *auxiliaryWindowController) open(options auxiliaryWindowOptions) {
	if c == nil || c.app == nil {
		return
	}

	// Window creation and lookup are guarded together so a fast repeated
	// shortcut cannot race two windows with the same role into existence.
	c.mu.Lock()
	if existing, ok := c.app.Window.GetByName(options.name); ok {
		loading := c.loading[options.name]
		c.mu.Unlock()
		if loading {
			return
		}
		existing.Show()
		existing.Restore()
		existing.Focus()
		c.announce(options.announce, true)
		return
	}

	macOptions := application.MacWindow{
		TitleBar: application.MacTitleBarDefault,
		Backdrop: application.MacBackdropNormal,
	}
	if options.customChrome {
		macOptions.TitleBar = application.MacTitleBarHiddenInsetUnified
		macOptions.Backdrop = application.MacBackdropTransparent
		// Keep a small native titlebar strip above the web content. It provides
		// the drag surface behind the centred title without covering controls.
		macOptions.InvisibleTitleBarHeight = 28
	}

	window := c.app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:             options.name,
		Title:            options.title,
		Width:            options.width,
		Height:           options.height,
		MinWidth:         options.minWidth,
		MinHeight:        options.minHeight,
		DisableResize:    options.disableResize,
		InitialPosition:  application.WindowCentered,
		BackgroundColour: application.NewRGB(245, 245, 240),
		Hidden:           true,
		URL:              options.url,
		Mac:              macOptions,
	})
	c.loading[options.name] = true
	if mainWindow, ok := c.app.Window.GetByName("main"); ok {
		if source, ok := mainWindow.(*application.WebviewWindow); ok {
			inheritNativeWindowAppearance(window, source)
		}
	}
	if options.customChrome {
		applyNativeWindowChrome(window)
	}
	if options.hideZoomButton {
		hideNativeWindowZoomButton(window)
	}
	window.OnWindowEvent(events.Common.WindowRuntimeReady, func(*application.WindowEvent) {
		c.mu.Lock()
		delete(c.loading, options.name)
		c.mu.Unlock()
		window.Show()
		window.Focus()
		c.announce(options.announce, true)
	})
	// Fires for both the native close button and a programmatic Close(), so a
	// listener never has to guess which one put the window away.
	window.OnWindowEvent(events.Common.WindowClosing, func(*application.WindowEvent) {
		c.mu.Lock()
		delete(c.loading, options.name)
		c.mu.Unlock()
		c.announce(options.announce, false)
	})
	c.mu.Unlock()
}

func (c *auxiliaryWindowController) close(name string) {
	if c == nil || c.app == nil {
		return
	}
	c.mu.Lock()
	window, ok := c.app.Window.GetByName(name)
	c.mu.Unlock()
	if !ok {
		return
	}
	window.Close()
}

func (c *auxiliaryWindowController) announce(name string, open bool) {
	if name == "" || c == nil || c.app == nil {
		return
	}
	c.app.Event.Emit(name, map[string]any{"open": open})
}
