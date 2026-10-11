package keeldesktop

import (
	"github.com/dyike/keel/ui/core"
	"github.com/openmodu/onecatch/internal/service/appupdate"
)

type appUpdates struct {
	service *appupdate.Service
	status  appupdate.Status
	busy    bool
	err     string
}

type nativeUpdateHost struct{ app *app }

func (h nativeUpdateHost) Quit() {
	core.Update(func() {
		if h.app.window != nil {
			h.app.window.Close()
		}
	})
}
func (a *app) initUpdates() {
	host := nativeUpdateHost{a}
	service, err := appupdate.New(a.backend.service.DataRoot(), func(status appupdate.Status) {
		core.Update(func() {
			if !a.closed {
				a.updates.status = status
			}
		})
	}, host.Quit)
	if err != nil {
		a.updates.err = err.Error()
		return
	}
	a.updates.service = service
	a.updates.status = service.Status()
	// Use the existing check interval, never a frame-driven network request.
	service.Start()
}
