package desktop

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	desktopassets "github.com/openmodu/oneshot/internal/app/desktop/assets"
	domainsettings "github.com/openmodu/oneshot/internal/domain/settings"
	"github.com/openmodu/oneshot/internal/repo/git"
	settingsrepo "github.com/openmodu/oneshot/internal/repo/settings"
	localdata "github.com/openmodu/oneshot/internal/repo/store/local"
	repoworkflows "github.com/openmodu/oneshot/internal/repo/workflows"
	"github.com/openmodu/oneshot/internal/repo/workspacelock"
	desktopservice "github.com/openmodu/oneshot/internal/service/desktop"
	"github.com/openmodu/oneshot/internal/service/desktop/runstate"
	"github.com/openmodu/oneshot/internal/service/desktop/runstream"
	"github.com/openmodu/oneshot/internal/transport/wails"
	workflowuc "github.com/openmodu/oneshot/internal/usecase/workflows"
	"github.com/openmodu/oneshot/pkg/logger"
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

	log := logger.MustNew(logger.Config{Service: "oneshot-desktop"})
	defer logger.Sync(log)

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
	settingsValue, err := settingsrepo.NewSettingsRepo(store.Data.Paths.Root).Get(context.Background())
	if err != nil {
		log.Fatal("open settings", zap.Error(err))
	}
	logConfig := func(value domainsettings.Settings) logger.Config {
		return logger.Config{Service: "oneshot-desktop", Level: value.Storage.LogLevel, File: filepath.Join(store.Data.Paths.Logs, "oneshot.log"), MaxSizeMB: value.Storage.LogMaxSizeMB, MaxBackups: value.Storage.LogMaxBackups, MaxAgeDays: value.Storage.LogMaxAgeDays, Compress: true}
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
	if err := service.RecoverInterruptedRuns(context.Background()); err != nil {
		log.Fatal("recover interrupted runs", zap.Error(err))
	}
	if err := service.EnsureBuiltinDefinitions(context.Background()); err != nil {
		log.Fatal("create builtin workflows", zap.Error(err))
	}
	var wailsApp *application.App
	workspaceBinding := wailstransport.NewWorkspaceBinding(service, func() *application.App { return wailsApp })

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
		application.NewService(wailstransport.NewWorkerBinding(service)),
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
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(desktopassets.Frontend),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})
	unsubscribeRunStream := streamHub.Subscribe(func(frame runstream.Frame) {
		wailsApp.Event.Emit(runstream.EventName, frame)
	})
	defer unsubscribeRunStream()
	runStateHub.SetEmitter(func(view runstate.View) {
		wailsApp.Event.Emit(runstate.EventName, view)
	})
	menu := wailsApp.NewMenu()
	menu.AddRole(application.AppMenu)
	menu.AddRole(application.EditMenu)
	menu.AddRole(application.WindowMenu)
	wailsApp.Menu.Set(menu)

	mainWindow := wailsApp.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            Name,
		Width:            1280,
		Height:           800,
		MinWidth:         1080,
		MinHeight:        720,
		BackgroundColour: application.NewRGB(245, 245, 240),
		URL:              "/",
		Mac: application.MacWindow{
			TitleBar: application.MacTitleBarHiddenInsetUnified,
		},
	})
	applyWindowCorner := func(radius float64) {
		application.InvokeSync(func() {
			setNativeWindowCornerRadius(mainWindow.NativeWindow(), radius)
		})
	}
	mainWindow.OnWindowEvent(events.Common.WindowRuntimeReady, func(*application.WindowEvent) { applyWindowCorner(26) })
	mainWindow.OnWindowEvent(events.Common.WindowMaximise, func(*application.WindowEvent) { applyWindowCorner(0) })
	mainWindow.OnWindowEvent(events.Common.WindowUnMaximise, func(*application.WindowEvent) { applyWindowCorner(26) })
	mainWindow.OnWindowEvent(events.Common.WindowFullscreen, func(*application.WindowEvent) { applyWindowCorner(0) })
	mainWindow.OnWindowEvent(events.Common.WindowUnFullscreen, func(*application.WindowEvent) { applyWindowCorner(26) })

	if err := wailsApp.Run(); err != nil {
		log.Fatal("wails app failed", zap.Error(err))
	}
}
