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

// A retained window keeps its webview alive across a close, so anything it
// read at load time can be arbitrarily old by the time it is shown again. This
// tells the page to reconcile; a first load never sees it.
const auxiliaryWindowShownEvent = "onecatch:aux-window-shown"

// Background runtime probes announce a corrected status on this channel, since
// the list every window rendered came from the cache.
const runtimesChangedEvent = "onecatch:runtimes-changed"

type auxiliaryWindowController struct {
	app     *application.App
	mu      sync.Mutex
	loading map[string]bool
	// wantShow records whether the in-flight load was requested by the user (so
	// it should present itself when ready) or is a background prewarm.
	wantShow map[string]bool
	// retained windows survive a close as hidden webviews. visible tracks which
	// of them the user can currently see; destroying tells the closing hook that
	// this particular close is ours and must go through.
	retained   map[string]*application.WebviewWindow
	visible    map[string]bool
	destroying map[string]bool
}

func newAuxiliaryWindowController(app *application.App) *auxiliaryWindowController {
	return &auxiliaryWindowController{
		app:        app,
		loading:    make(map[string]bool),
		wantShow:   make(map[string]bool),
		retained:   make(map[string]*application.WebviewWindow),
		visible:    make(map[string]bool),
		destroying: make(map[string]bool),
	}
}

func settingsWindowOptions() auxiliaryWindowOptions {
	return auxiliaryWindowOptions{
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
		retain:         true,
	}
}

func (c *auxiliaryWindowController) OpenSettings() {
	c.open(settingsWindowOptions(), true)
}

// PrewarmSettings builds the settings webview ahead of time without showing it.
// Creating a window means loading and parsing the whole frontend bundle again —
// the reason the panel used to appear blank for a beat — so it is paid once, on
// an idle main window, rather than on the keystroke that asks for it.
func (c *auxiliaryWindowController) PrewarmSettings() {
	c.open(settingsWindowOptions(), false)
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
		retain:       true,
	}, true)
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
	}, true)
}

func (c *auxiliaryWindowController) CloseInspector() {
	c.destroy(inspectorWindowName)
}

// ReleaseHiddenWindows destroys every retained window the user cannot currently
// see, and reports whether any auxiliary window is still on screen.
//
// The main window calls this as it closes: a hidden webview still counts as an
// open window, so without it the application would never reach the
// last-window-closed condition that terminates it. A retained window that is
// still on screen is left alone, matching the behaviour before retention —
// closing the workbench does not yank away a settings panel in active use, and
// the caller uses the return value to decide whether anything is left to keep
// the application alive for.
func (c *auxiliaryWindowController) ReleaseHiddenWindows() (stillVisible bool) {
	if c == nil {
		return false
	}
	names, stillVisible := c.hiddenRetainedWindows()
	for _, name := range names {
		c.destroy(name)
	}
	return stillVisible
}

// hiddenRetainedWindows splits the retained windows into the ones to release and
// whether anything the user can see is left. Kept apart from the destruction so
// the decision that determines whether the application quits can be tested
// without a live window server.
func (c *auxiliaryWindowController) hiddenRetainedWindows() (hidden []string, stillVisible bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for name := range c.retained {
		if c.visible[name] {
			stillVisible = true
			continue
		}
		hidden = append(hidden, name)
	}
	return hidden, stillVisible
}

type auxiliaryWindowOptions struct {
	name                string
	title, url          string
	width, height       int
	minWidth, minHeight int
	disableResize       bool
	hideZoomButton      bool
	customChrome        bool
	// retain keeps the webview alive when the window is closed, hiding it
	// instead of destroying it, so reopening costs a Show() rather than a full
	// bundle load. Only worth it for windows the user opens repeatedly.
	retain bool
	// announce names an application event emitted with {"open": bool} whenever
	// this window becomes visible or closes. Empty means the window's lifecycle
	// is of no interest to the rest of the UI.
	announce string
}

func (c *auxiliaryWindowController) open(options auxiliaryWindowOptions, show bool) {
	if c == nil || c.app == nil {
		return
	}

	// Window creation and lookup are guarded together so a fast repeated
	// shortcut cannot race two windows with the same role into existence.
	c.mu.Lock()
	if existing, ok := c.app.Window.GetByName(options.name); ok {
		if c.loading[options.name] {
			// Still loading. Record the intent so the ready handler decides
			// whether to present it — a prewarm must not steal focus, but a
			// user request arriving mid-prewarm must still open the window.
			if show {
				c.wantShow[options.name] = true
			}
			c.mu.Unlock()
			return
		}
		if !show {
			c.mu.Unlock()
			return
		}
		reshown := c.retained[options.name] != nil
		c.visible[options.name] = true
		c.mu.Unlock()
		existing.Show()
		existing.Restore()
		existing.Focus()
		if reshown {
			c.app.Event.Emit(auxiliaryWindowShownEvent, map[string]any{"name": options.name})
		}
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
	c.wantShow[options.name] = show
	if options.retain {
		c.retained[options.name] = window
	}
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
		present := c.wantShow[options.name]
		delete(c.wantShow, options.name)
		if present {
			c.visible[options.name] = true
		}
		c.mu.Unlock()
		if !present {
			return
		}
		window.Show()
		window.Focus()
		c.announce(options.announce, true)
	})
	// Hooks run before listeners and can cancel the event, which is what keeps
	// a retained window's webview alive: Wails' own listener destroys the
	// window, so a retained close has to stop short of it.
	if options.retain {
		window.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) {
			c.mu.Lock()
			deliberate := c.destroying[options.name]
			if !deliberate {
				c.visible[options.name] = false
			}
			c.mu.Unlock()
			if deliberate {
				return
			}
			window.Hide()
			event.Cancel()
			c.announce(options.announce, false)
		})
	}
	// Fires for both the native close button and a programmatic Close(), so a
	// listener never has to guess which one put the window away.
	window.OnWindowEvent(events.Common.WindowClosing, func(*application.WindowEvent) {
		c.mu.Lock()
		delete(c.loading, options.name)
		delete(c.wantShow, options.name)
		delete(c.retained, options.name)
		delete(c.destroying, options.name)
		delete(c.visible, options.name)
		c.mu.Unlock()
		c.announce(options.announce, false)
	})
	c.mu.Unlock()
}

// destroy tears the window down for real, bypassing any retain hook.
func (c *auxiliaryWindowController) destroy(name string) {
	if c == nil || c.app == nil {
		return
	}
	c.mu.Lock()
	window, ok := c.app.Window.GetByName(name)
	if ok {
		c.destroying[name] = true
	}
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
