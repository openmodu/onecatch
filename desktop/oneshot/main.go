package main

import (
	"embed"
	"log"

	"github.com/openmodu/oneshot/desktop/oneshot/app"
	"github.com/openmodu/oneshot/desktop/oneshot/bindings"
	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	apiClient := initializeClient()

	wailsApp := application.New(application.Options{
		Name:        app.Name,
		Description: app.Description,
		Services: []application.Service{
			application.NewService(bindings.NewAuthBinding(apiClient)),
			application.NewService(bindings.NewAgentBinding(apiClient)),
			application.NewService(bindings.NewBillingBinding(apiClient)),
			application.NewService(bindings.NewOrderBinding(apiClient)),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	wailsApp.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            app.Name,
		Width:            1280,
		Height:           820,
		MinWidth:         960,
		MinHeight:        680,
		BackgroundColour: application.NewRGB(247, 248, 250),
		URL:              "/",
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 48,
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar:                application.MacTitleBarHiddenInset,
		},
	})

	if err := wailsApp.Run(); err != nil {
		log.Fatal(err)
	}
}
