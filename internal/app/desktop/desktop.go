package desktop

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	desktopassets "github.com/openmodu/onecatch/internal/app/desktop/assets"
	"github.com/openmodu/onecatch/internal/buildinfo"
	domainsettings "github.com/openmodu/onecatch/internal/domain/settings"
	"github.com/openmodu/onecatch/internal/repo/git"
	settingsrepo "github.com/openmodu/onecatch/internal/repo/settings"
	localdata "github.com/openmodu/onecatch/internal/repo/store/local"
	repotasks "github.com/openmodu/onecatch/internal/repo/tasks"
	repoworkflows "github.com/openmodu/onecatch/internal/repo/workflows"
	"github.com/openmodu/onecatch/internal/repo/workspacelock"
	appupdateservice "github.com/openmodu/onecatch/internal/service/appupdate"
	desktopservice "github.com/openmodu/onecatch/internal/service/desktop"
	"github.com/openmodu/onecatch/internal/service/desktop/listchange"
	"github.com/openmodu/onecatch/internal/service/desktop/runstate"
	"github.com/openmodu/onecatch/internal/service/desktop/runstream"
	terminalservice "github.com/openmodu/onecatch/internal/service/terminal"
	"github.com/openmodu/onecatch/internal/transport/wails"
	workflowuc "github.com/openmodu/onecatch/internal/usecase/workflows"
	"github.com/openmodu/onecatch/pkg/logger"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"
	"go.uber.org/zap"
)

// runningAsBundle reports whether the executable lives inside a macOS .app
// bundle. It mirrors what the notification service's Startup requires (a bundle
// identifier, present only in a packaged app), so it gates registering that
// service without importing its private bundle check.
func runningAsBundle() bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	return strings.Contains(exe, ".app/Contents/MacOS/")
}

func Run() {
	prepareCommandEnvironment()

	log := logger.MustNew(logger.Config{Service: "onecatch-desktop"})
	defer logger.Sync(log)
	log.Info("starting OneCatch", zap.String("version", buildinfo.Version))

	store, err := localdata.OpenStore("")
	if err != nil {
		log.Fatal("open local store", zap.Error(err))
	}
	// Decorate the workflow repository so every bounded-state mutation (run,
	// step run, instruction) marks the run dirty. Both the orchestrator and the
	// desktop service reads store.Repos.Workflows below, so wrapping it here makes
	// their writes push without threading the hub through either constructor.
	runStateHub := runstate.NewHub()
	store.Repos.Workflows = repoworkflows.WithNotifier(store.Repos.Workflows, runStateHub)
	// The sidebar's task and run lists used to stay fresh by re-reading every
	// task and run file on a 1.4s timer. Decorating the task repository lets the
	// writes announce themselves instead; run writes already reach runStateHub,
	// which forwards to this hub where its emitter is installed below.
	listHub := listchange.NewHub()
	store.Repos.Tasks = repotasks.WithNotifier(store.Repos.Tasks, listHub)
	defer listHub.Close()
	settingsValue, err := settingsrepo.NewSettingsRepo(store.Data.Paths.Root).Get(context.Background())
	if err != nil {
		log.Fatal("open settings", zap.Error(err))
	}
	logConfig := func(value domainsettings.Settings) logger.Config {
		return logger.Config{Service: "onecatch-desktop", Level: value.Storage.LogLevel, File: filepath.Join(store.Data.Paths.Logs, "onecatch.log"), MaxSizeMB: value.Storage.LogMaxSizeMB, MaxBackups: value.Storage.LogMaxBackups, MaxAgeDays: value.Storage.LogMaxAgeDays, Compress: true}
	}
	managedLog, err := logger.NewManaged(logConfig(settingsValue))
	if err != nil {
		log.Fatal("configure logger", zap.Error(err))
	}
	log = managedLog.Logger
	defer logger.Sync(log)
	runtimes, err := desktopservice.NewRuntimeRegistry(store.Data.Paths.Root)
	if err != nil {
		log.Fatal("open runtime registry", zap.Error(err))
	}
	git := gitrepo.New("")
	orchestrator := workflowuc.NewUsecase(store.Repos.Tasks, store.Repos.Workflows, runtimes, workspacelock.New(store.Data.Paths.Locks), git)
	service := desktopservice.NewService(store, orchestrator, runtimes, git)
	streamHub := runstream.NewHub()
	service.SetRunStreamHub(streamHub)
	service.SetRunStateHub(runStateHub)
	defer runStateHub.Close()
	defer service.Close()
	service.SetSettingsReload(func(value domainsettings.Settings) error { return managedLog.Reconfigure(logConfig(value)) })
	if err := service.InitializeSettings(context.Background()); err != nil {
		log.Fatal("initialize settings", zap.Error(err))
	}
	if err := service.EnsureBuiltinDefinitions(context.Background()); err != nil {
		log.Fatal("create builtin workflows", zap.Error(err))
	}
	var wailsApp *application.App
	var mainWindow *application.WebviewWindow
	workspaceBinding := wailstransport.NewWorkspaceBinding(service, func() *application.App { return wailsApp })
	var auxiliaryWindows *auxiliaryWindowController
	windowBinding := wailstransport.NewWindowBinding(wailstransport.WindowCallbacks{
		OpenSettings: func() {
			if auxiliaryWindows != nil {
				auxiliaryWindows.OpenSettings()
			}
		},
		OpenWorkflows: func() {
			if auxiliaryWindows != nil {
				auxiliaryWindows.OpenWorkflows()
			}
		},
		OpenInspector: func() {
			if auxiliaryWindows != nil {
				auxiliaryWindows.OpenInspector()
			}
		},
		CloseInspector: func() {
			if auxiliaryWindows != nil {
				auxiliaryWindows.CloseInspector()
			}
		},
	})
	terminalService := terminalservice.NewService()
	defer terminalService.Close()
	var updateService *appupdateservice.Service
	updateBinding := wailstransport.NewUpdateBinding(func() *appupdateservice.Service { return updateService })

	// The macOS notification service hard-fails its Startup without a bundle
	// identifier, which would abort app launch. It only has one when running
	// from a signed .app bundle, so register it (and hand it to the notifier
	// binding) only then; a raw dev binary keeps a nil notifier that no-ops.
	services := []application.Service{
		application.NewService(wailstransport.NewGitBinding(service)),
		application.NewService(wailstransport.NewRuntimeBinding(service)),
		application.NewService(wailstransport.NewSettingsBinding(service)),
		application.NewService(workspaceBinding),
		application.NewService(wailstransport.NewWorkflowBinding(service)),
		application.NewService(wailstransport.NewTaskRunBinding(service)),
		application.NewService(wailstransport.NewTerminalBinding(terminalService, service)),
		application.NewService(wailstransport.NewWorkerBinding(service)),
		application.NewService(windowBinding),
		application.NewService(updateBinding),
	}
	var notifier *notifications.NotificationService
	if runningAsBundle() {
		notifier = notifications.New()
		services = append(services, application.NewService(notifier))
	}
	services = append(services, application.NewService(wailstransport.NewNotifyBinding(notifier)))

	wailsApp = application.New(application.Options{
		Name:        Name,
		Description: Description,
		Icon:        desktopassets.AppIcon,
		Services:    services,
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: "com.openmodu.onecatch",
			OnSecondInstanceLaunch: func(application.SecondInstanceData) {
				if mainWindow == nil {
					return
				}
				mainWindow.UnMinimise()
				mainWindow.Show()
				mainWindow.Restore()
				mainWindow.Focus()
			},
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(desktopassets.Frontend),
			Middleware: func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
					if request.URL.Path == "/appicon.png" {
						response.Header().Set("Content-Type", "image/png")
						response.Header().Set("Cache-Control", "public, max-age=86400")
						_, _ = response.Write(desktopassets.AppIcon)
						return
					}
					// Bundle filenames carry a content hash, so a given URL can
					// never change meaning. Saying so lets the webview reuse the
					// parsed bundle instead of refetching it for every window.
					if strings.HasPrefix(request.URL.Path, "/assets/") {
						response.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
					}
					next.ServeHTTP(response, request)
				})
			},
		},
		Mac: application.MacOptions{
			// The menu-bar item is a first-class way back into the workbench.
			// Closing the last window therefore leaves the process alive until
			// the user chooses Quit from either native menu.
			ApplicationShouldTerminateAfterLastWindowClosed: false,
		},
	})
	updateService, err = appupdateservice.New(wailsApp)
	if err != nil {
		log.Fatal("configure app updater", zap.Error(err))
	}
	updateService.Start()
	defer updateService.Close()
	var shuttingDown atomic.Bool
	wailsApp.OnShutdown(func() {
		shuttingDown.Store(true)
		// Stop a coalesced list notification before services and native menus
		// begin tearing down.
		listHub.Close()
	})
	terminalService.SetEmitter(func(name string, payload any) {
		wailsApp.Event.Emit(name, payload)
	})
	auxiliaryWindows = newAuxiliaryWindowController(wailsApp)
	unsubscribeRunStream := streamHub.Subscribe(func(frame runstream.Frame) {
		wailsApp.Event.Emit(runstream.EventName, frame)
	})
	defer unsubscribeRunStream()
	runStateHub.SetEmitter(func(view runstate.View) {
		wailsApp.Event.Emit(runstate.EventName, view)
		// Every run mutation already lands here, so the run list rides along
		// rather than needing its own decorator.
		listHub.MarkDirty()
	})
	// ListRuntimes serves its cache immediately and re-probes in the background;
	// this is how a corrected status reaches windows that already rendered.
	runtimes.SetRuntimesChanged(func(items []desktopservice.RuntimeInfo) {
		wailsApp.Event.Emit(runtimesChangedEvent, items)
	})
	menu := wailsApp.NewMenu()
	if runtime.GOOS == "darwin" {
		appMenu := menu.AddSubmenu(Name)
		appMenu.AddRole(application.About)
		appMenu.Add("检查更新…").OnClick(func(*application.Context) {
			auxiliaryWindows.OpenSettings()
			go func() { _, _ = updateService.Check(context.Background()) }()
		})
		appMenu.Add("设置…").SetAccelerator("CmdOrCtrl+,").OnClick(func(*application.Context) {
			auxiliaryWindows.OpenSettings()
		})
		appMenu.AddSeparator()
		appMenu.AddRole(application.ServicesMenu)
		appMenu.AddSeparator()
		appMenu.AddRole(application.Hide)
		appMenu.AddRole(application.HideOthers)
		appMenu.AddRole(application.UnHide)
		appMenu.AddSeparator()
		appMenu.AddRole(application.Quit)
	} else if runtime.GOOS == "windows" {
		// Keep Windows' native caption and menu bar, matching Codex Desktop.
		// Settings is a first-class File-menu command and uses the Windows
		// convention even when the webview does not currently have focus.
		fileMenu := menu.AddSubmenu("文件")
		fileMenu.Add("设置…").SetAccelerator("Ctrl+,").OnClick(func(*application.Context) {
			auxiliaryWindows.OpenSettings()
		})
		fileMenu.Add("检查更新…").OnClick(func(*application.Context) {
			auxiliaryWindows.OpenSettings()
			go func() { _, _ = updateService.Check(context.Background()) }()
		})
		fileMenu.AddSeparator()
		fileMenu.AddRole(application.Quit)
	}
	// Linux gets no native menu bar at all: Settings is already reachable from
	// the sidebar dropdown and Ctrl+,, quitting is a window close away, and
	// Edit/Window role items (cut/copy/paste, minimize/zoom) are either
	// redundant with the webview's own handling or, like minimize, meaningless
	// under a tiling WM. Skip straight to Set so darwin/windows keep theirs.
	if runtime.GOOS != "linux" {
		menu.AddRole(application.EditMenu)
		if runtime.GOOS == "windows" {
			menu.AddRole(application.ViewMenu)
			menu.AddRole(application.HelpMenu)
		} else {
			menu.AddRole(application.WindowMenu)
		}
	}
	wailsApp.Menu.Set(menu)

	mainWindow = wailsApp.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:  "main",
		Title: Name,
		// Frameless on Linux too: unlike Windows/macOS, Wails cannot rely on the
		// compositor to draw window chrome (many Wayland WMs, e.g. tiling ones,
		// never do), so Linux gets the same JS-drawn caption bar as Windows.
		Frameless:        runtime.GOOS == "windows" || runtime.GOOS == "linux",
		Width:            1280,
		Height:           800,
		MinWidth:         860,
		MinHeight:        720,
		BackgroundColour: application.NewRGB(245, 245, 240),
		URL:              "/",
		Mac: application.MacWindow{
			TitleBar: application.MacTitleBarHiddenInsetUnified,
			Backdrop: application.MacBackdropTransparent,
		},
	})
	if runtime.GOOS == "darwin" {
		// Wails' default Dock-reopen handler shows every hidden window when the
		// application has no visible windows. Settings is deliberately prewarmed
		// as a hidden retained webview, so after closing the workbench a second
		// Dock click used to surface Settings instead of the workbench. Claim the
		// event first and restore only the main window; retained auxiliaries stay
		// hidden until the user explicitly asks for them.
		cancelReopenHook := wailsApp.Event.RegisterApplicationEventHook(events.Mac.ApplicationShouldHandleReopen, func(event *application.ApplicationEvent) {
			if reopenMainWindow(event.Context().HasVisibleWindows(), mainWindow) {
				event.Cancel()
			}
		})
		defer cancelReopenHook()
	}
	refreshSystemTray := installDesktopSystemTray(wailsApp, mainWindow, service, log, auxiliaryWindows.OpenSettings)
	applyNativeWindowChrome(mainWindow)
	// Re-assert the constraint after AppKit has created the native window. The
	// option is the source of truth for startup; the runtime call also covers
	// restored/dev windows whose frame was applied after option initialisation.
	mainWindow.OnWindowEvent(events.Common.WindowRuntimeReady, func(*application.WindowEvent) {
		mainWindow.SetMinSize(860, 720)
		if err := appupdateservice.SignalReadyFromEnvironment(); err != nil {
			log.Warn("signal updated app readiness", zap.Error(err))
		}
	})
	// On platforms that actually destroy the main window, the detached inspector
	// must not outlive it — otherwise closing the workbench leaves a frozen panel
	// behind that keeps the application alive. Retained windows the user cannot
	// see go the same way, for the same reason. Windows and Linux return early
	// below because closing there only hides the still-live main window to the tray.
	// A hook rather than a listener: listeners are dispatched concurrently with
	// Wails' own teardown, and this has to finish deciding before the window is
	// gone.
	mainWindow.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) {
		if runtime.GOOS == "windows" || runtime.GOOS == "linux" {
			// Keep the desktop process, active runs, and terminal sessions alive.
			// This must happen before touching any auxiliary window: destroying a
			// prewarmed hidden GTK window re-entrantly from a native close callback
			// can leave Wails holding an invalid GdkSurface and crash in
			// gtk_window_destroy. Quit remains an explicit tray action.
			mainWindow.Hide()
			event.Cancel()
			return
		}
		auxiliaryWindows.CloseInspector()
		if runtime.GOOS == "darwin" && !shuttingDown.Load() {
			// Keep the webview warm and its event subscriptions alive. A tray
			// action can then restore the exact workbench state immediately.
			event.Cancel()
			mainWindow.Hide()
			return
		}
		if auxiliaryWindows.ReleaseHiddenWindows() {
			// A settings or workflow window is still on screen, so the
			// application outlives the workbench exactly as it did before.
			return
		}
		// Everything else was hidden bookkeeping. Quit explicitly instead of
		// leaving it to the last-window-closed rule, which would be racing the
		// teardown of the windows just released.
		wailsApp.Quit()
	})
	mainWindow.OnWindowEvent(events.Common.WindowRuntimeReady, func(*application.WindowEvent) {
		// Recovering interrupted runs walks every run on disk. It used to sit in
		// front of window creation, so the whole scan was charged to the launch
		// the user is watching; nothing in it is needed to paint the workbench,
		// and its progress reaches the UI over the run-state channel either way.
		// A failure here must not take down an application that has already
		// started, so it is logged rather than fatal.
		go func() {
			if err := service.RecoverInterruptedRuns(context.Background()); err != nil {
				log.Error("recover interrupted runs", zap.Error(err))
			}
		}()
		// Build the settings webview while the machine is idle, so opening it
		// later is a Show() rather than a second full bundle load. Delayed so it
		// competes with nothing during the launch it is meant to keep fast.
		time.AfterFunc(3*time.Second, auxiliaryWindows.PrewarmSettings)
	})

	var trayController *macTrayController
	if runtime.GOOS == "darwin" {
		trayController = newMacTrayController(wailsApp, service, desktopassets.AppIcon, func(action trayAction) {
			mainWindow.Show()
			mainWindow.Restore()
			mainWindow.Focus()
			if action.Action != "show" {
				wailsApp.Event.Emit(trayActionEvent, action)
			}
		}, func(err error) {
			log.Warn("refresh system tray conversations", zap.Error(err))
		})
		unsubscribeLanguage := wailsApp.Event.On(languageChangedEvent, func(event *application.CustomEvent) {
			language, ok := event.Data.(string)
			if ok {
				trayController.setLanguage(language)
			}
		})
		defer unsubscribeLanguage()
	}
	listHub.SetEmitter(func() {
		wailsApp.Event.Emit(listchange.EventName, nil)
		if refreshSystemTray != nil {
			refreshSystemTray()
		}
		if trayController != nil {
			trayController.refresh()
		}
	})

	if err := wailsApp.Run(); err != nil {
		log.Fatal("wails app failed", zap.Error(err))
	}
}
