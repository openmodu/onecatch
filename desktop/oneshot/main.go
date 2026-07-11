package main

import (
	"context"
	"embed"
	"path/filepath"

	"github.com/openmodu/oneshot/desktop/oneshot/app"
	"github.com/openmodu/oneshot/desktop/oneshot/bindings"
	"github.com/openmodu/oneshot/internal/app/localapp"
	localdata "github.com/openmodu/oneshot/internal/data/local"
	domainsettings "github.com/openmodu/oneshot/internal/domain/settings"
	"github.com/openmodu/oneshot/internal/gitinspect"
	settingsrepo "github.com/openmodu/oneshot/internal/repo/settings"
	workflowuc "github.com/openmodu/oneshot/internal/usecase/workflows"
	"github.com/openmodu/oneshot/internal/workspacelock"
	"github.com/openmodu/oneshot/pkg/logger"
	"github.com/wailsapp/wails/v3/pkg/application"
	"go.uber.org/zap"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	log := logger.MustNew(logger.Config{Service: "oneshot-desktop"})
	defer logger.Sync(log)

	store, err := localdata.OpenStore("")
	if err != nil {
		log.Fatal("open local store", zap.Error(err))
	}
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
	runtimes, err := localapp.NewRuntimeRegistry(store.Data.Paths.Root)
	if err != nil {
		log.Fatal("open runtime registry", zap.Error(err))
	}
	git := gitinspect.New("")
	orchestrator := workflowuc.NewUsecase(store.Repos.Tasks, store.Repos.Workflows, runtimes, workspacelock.New(store.Data.Paths.Locks), git)
	localApp := localapp.New(store, orchestrator, runtimes, git)
	defer localApp.Close()
	localApp.SetSettingsReload(func(value domainsettings.Settings) error { return managedLog.Reconfigure(logConfig(value)) })
	if err := localApp.InitializeSettings(context.Background()); err != nil {
		log.Fatal("initialize settings", zap.Error(err))
	}
	if err := localApp.RecoverInterruptedRuns(context.Background()); err != nil {
		log.Fatal("recover interrupted runs", zap.Error(err))
	}
	if err := localApp.EnsureBuiltinDefinitions(context.Background()); err != nil {
		log.Fatal("create builtin workflows", zap.Error(err))
	}
	var wailsApp *application.App
	workspaceBinding := bindings.NewWorkspaceBinding(localApp, func() *application.App { return wailsApp })

	wailsApp = application.New(application.Options{
		Name:        app.Name,
		Description: app.Description,
		Services: []application.Service{
			application.NewService(bindings.NewRuntimeBinding(localApp)),
			application.NewService(bindings.NewSettingsBinding(localApp)),
			application.NewService(workspaceBinding),
			application.NewService(bindings.NewWorkflowBinding(localApp)),
			application.NewService(bindings.NewTaskRunBinding(localApp)),
			application.NewService(bindings.NewWorkerBinding(localApp)),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})
	menu := wailsApp.NewMenu()
	menu.AddRole(application.AppMenu)
	menu.AddRole(application.EditMenu)
	menu.AddRole(application.WindowMenu)
	wailsApp.Menu.Set(menu)

	wailsApp.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            app.Name,
		Width:            1280,
		Height:           800,
		MinWidth:         1080,
		MinHeight:        720,
		BackgroundColour: application.NewRGB(245, 245, 247),
		URL:              "/",
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 38,
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar:                application.MacTitleBarHiddenInset,
		},
	})

	if err := wailsApp.Run(); err != nil {
		log.Fatal("wails app failed", zap.Error(err))
	}
}
