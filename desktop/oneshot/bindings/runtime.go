package bindings

import "github.com/openmodu/oneshot/internal/app/localapp"

type RuntimeBinding struct{ app *localapp.App }

func NewRuntimeBinding(app *localapp.App) *RuntimeBinding { return &RuntimeBinding{app: app} }

func (b *RuntimeBinding) ListRuntimes() []localapp.RuntimeInfo { return b.app.ListRuntimes() }
func (b *RuntimeBinding) CheckRuntime(runtime string) (localapp.RuntimeInfo, error) {
	return b.app.CheckRuntime(runtime)
}
func (b *RuntimeBinding) UpdateRuntimeConfig(input localapp.RuntimeConfigInput) (localapp.RuntimeInfo, error) {
	return b.app.UpdateRuntimeConfig(input)
}
