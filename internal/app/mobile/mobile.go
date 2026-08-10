package mobile

import (
	"os"
	"path/filepath"

	desktopassets "github.com/openmodu/oneshot/internal/app/desktop/assets"
	mobileservice "github.com/openmodu/oneshot/internal/service/mobile"
	wailstransport "github.com/openmodu/oneshot/internal/transport/wails"
	"github.com/openmodu/oneshot/pkg/logger"
	"github.com/wailsapp/wails/v3/pkg/application"
	"go.uber.org/zap"
)

const (
	Name        = "Oneshot"
	Description = "Remote Agent worker workbench"
)

func Run() {
	log := logger.MustNew(logger.Config{Service: "oneshot-mobile"})
	defer logger.Sync(log)

	root := application.Mobile.StoragePath()
	if root == "" {
		configRoot, err := os.UserConfigDir()
		if err != nil {
			log.Fatal("resolve mobile storage", zap.Error(err))
		}
		root = filepath.Join(configRoot, "oneshot-mobile")
	} else {
		root = filepath.Join(root, "oneshot")
	}
	service, err := mobileservice.NewService(root)
	if err != nil {
		log.Fatal("open mobile service", zap.Error(err))
	}
	defer service.Close()

	wailsApp := application.New(application.Options{
		Name:        Name,
		Description: Description,
		Icon:        desktopassets.AppIcon,
		Services: []application.Service{
			application.NewService(wailstransport.NewMobileBinding(service)),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(desktopassets.Frontend),
		},
		IOS: application.IOSOptions{
			DisableLinkPreview: true,
			BackgroundColour:   application.NewRGB(245, 245, 240),
		},
	})
	service.SetEmitter(func(frame mobileservice.RunFrame) {
		wailsApp.Event.Emit(mobileservice.RunEventName, frame)
	})

	wailsApp.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:             "mobile",
		Title:            Name,
		Width:            430,
		Height:           860,
		BackgroundColour: application.NewRGB(245, 245, 240),
		URL:              "/?platform=mobile",
	})
	if err := wailsApp.Run(); err != nil {
		log.Fatal("mobile app failed", zap.Error(err))
	}
}
