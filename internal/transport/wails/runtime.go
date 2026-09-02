package wailstransport

import (
	"context"

	desktopservice "github.com/openmodu/onecatch/internal/service/desktop"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
)

type RuntimeBinding struct{ service *desktopservice.Service }

func NewRuntimeBinding(service *desktopservice.Service) *RuntimeBinding {
	return &RuntimeBinding{service: service}
}

func (b *RuntimeBinding) ListRuntimes() []desktopservice.RuntimeInfo { return b.service.ListRuntimes() }
func (b *RuntimeBinding) ListCodexSkills(cwd string) ([]agentrun.CodexSkill, error) {
	return b.service.ListCodexSkills(context.Background(), cwd)
}
func (b *RuntimeBinding) CheckRuntime(runtime string) (desktopservice.RuntimeInfo, error) {
	return b.service.CheckRuntime(runtime)
}
func (b *RuntimeBinding) UpdateRuntimeConfig(input desktopservice.RuntimeConfigInput) (desktopservice.RuntimeInfo, error) {
	return b.service.UpdateRuntimeConfig(input)
}
