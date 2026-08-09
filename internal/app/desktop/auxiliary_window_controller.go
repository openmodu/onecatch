package desktop

import (
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const (
	settingsWindowName  = "settings"
	workflowsWindowName = "workflows"
)

type auxiliaryWindowController struct {
	app *application.App
	mu  sync.Mutex
}

func newAuxiliaryWindowController(app *application.App) *auxiliaryWindowController {
	return &auxiliaryWindowController{app: app}
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

type auxiliaryWindowOptions struct {
	name                string
	title, url          string
	width, height       int
	minWidth, minHeight int
	disableResize       bool
	hideZoomButton      bool
	customChrome        bool
}

func (c *auxiliaryWindowController) open(options auxiliaryWindowOptions) {
	if c == nil || c.app == nil {
		return
	}

	// Window creation and lookup are guarded together so a fast repeated
	// shortcut cannot race two windows with the same role into existence.
	c.mu.Lock()
	if existing, ok := c.app.Window.GetByName(options.name); ok {
		c.mu.Unlock()
		existing.Show()
		existing.Restore()
		existing.Focus()
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
		URL:              options.url,
		Mac:              macOptions,
	})
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
	c.mu.Unlock()

	window.Show()
	window.Focus()
}
