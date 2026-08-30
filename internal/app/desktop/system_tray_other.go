//go:build !linux

package desktop

import (
	desktopservice "github.com/openmodu/onecatch/internal/service/desktop"
	"github.com/wailsapp/wails/v3/pkg/application"
	"go.uber.org/zap"
)

func installDesktopSystemTray(
	*application.App,
	*application.WebviewWindow,
	*desktopservice.Service,
	*zap.Logger,
	func(),
) func() {
	return nil
}
