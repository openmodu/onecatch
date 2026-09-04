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
func (b *SkillBinding) ReadFile(path string) (skillmanager.SkillFileContent, error) {
	return b.service.ReadSkillFile(path)
}
func (b *SkillBinding) WriteFile(input skillmanager.SaveSkillFileInput) (skillmanager.SkillFileContent, error) {
	return b.service.WriteSkillFile(input)
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
func (b *SkillBinding) UpdateSyncTarget(input skillmanager.UpdateTargetInput) (skillmanager.SyncTarget, error) {
	return b.service.UpdateSkillSyncTarget(input)
}
func (b *SkillBinding) SetSyncTargetSkills(input skillmanager.SetTargetSkillsInput) (skillmanager.SyncTarget, error) {
	return b.service.SetSkillSyncTargetSkills(input)
}
func (b *SkillBinding) RemoveSyncTarget(id string) error {
	return b.service.RemoveSkillSyncTarget(id)
}
func (b *SkillBinding) Sync(id string) (skillmanager.SyncResult, error) {
	return b.service.SyncSkills(context.Background(), id)
}
func (b *SkillBinding) SyncSkill(name string) (skillmanager.SyncSkillResult, error) {
	return b.service.SyncSkill(context.Background(), name)
}
func (b *SkillBinding) Debug(input desktopservice.SkillDebugInput) (desktopservice.SkillDebugResult, error) {
	return b.service.DebugSkill(context.Background(), input)
}
func (b *SkillBinding) StopDebug(runID string) { b.service.StopSkillDebug(runID) }
func (b *SkillBinding) DebugHistory(name string) ([]desktopservice.SkillDebugRecord, error) {
	return b.service.ListSkillDebugRuns(name)
}
func (b *SkillBinding) ClearDebugHistory(name string) error {
	return b.service.ClearSkillDebugRuns(name)
}
