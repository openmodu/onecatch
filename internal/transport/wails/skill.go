package wailstransport

import (
	"context"

	desktopservice "github.com/openmodu/onecatch/internal/service/desktop"
	"github.com/openmodu/onecatch/internal/service/skillmanager"
)

type SkillBinding struct{ service *desktopservice.Service }

func NewSkillBinding(service *desktopservice.Service) *SkillBinding {
	return &SkillBinding{service: service}
}

func (b *SkillBinding) ListSkills() ([]skillmanager.Skill, error) {
	return b.service.ListManagedSkills()
}
func (b *SkillBinding) ListFiles(directory string) ([]skillmanager.SkillFileEntry, error) {
	return b.service.ListSkillFiles(directory)
}
func (b *SkillBinding) GetSkill(name string) (skillmanager.SkillDocument, error) {
	return b.service.GetSkill(name)
}
func (b *SkillBinding) CreateSkill(input skillmanager.SaveSkillInput) (skillmanager.SkillDocument, error) {
	return b.service.CreateSkill(input)
}
func (b *SkillBinding) UpdateSkill(input skillmanager.SaveSkillInput) (skillmanager.SkillDocument, error) {
	return b.service.UpdateSkill(input)
}
func (b *SkillBinding) DeleteSkill(name string) error { return b.service.DeleteSkill(name) }
func (b *SkillBinding) ScanSyncTargets() ([]skillmanager.SyncTarget, error) {
	return b.service.ScanSkillSyncTargets()
}
func (b *SkillBinding) AddSyncTarget(input skillmanager.AddTargetInput) (skillmanager.SyncTarget, error) {
	return b.service.AddSkillSyncTarget(input)
}
func (b *SkillBinding) RemoveSyncTarget(id string) error {
	return b.service.RemoveSkillSyncTarget(id)
}
func (b *SkillBinding) Sync(id string) (skillmanager.SyncResult, error) {
	return b.service.SyncSkills(context.Background(), id)
}
func (b *SkillBinding) Debug(input desktopservice.SkillDebugInput) (desktopservice.SkillDebugResult, error) {
	return b.service.DebugSkill(context.Background(), input)
}
