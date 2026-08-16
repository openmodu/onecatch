package wailstransport

import (
	"context"

	domaintasks "github.com/openmodu/oneshot/internal/domain/tasks"
	domainworkflows "github.com/openmodu/oneshot/internal/domain/workflows"
	desktopservice "github.com/openmodu/oneshot/internal/service/desktop"
	"github.com/openmodu/oneshot/internal/service/desktop/runstream"
)

type TaskRunBinding struct{ service *desktopservice.Service }

func NewTaskRunBinding(service *desktopservice.Service) *TaskRunBinding {
	return &TaskRunBinding{service: service}
}

func (b *TaskRunBinding) CreateTask(input desktopservice.CreateTaskInput) (domaintasks.Task, error) {
	return b.service.CreateTask(context.Background(), input)
}
func (b *TaskRunBinding) ListTasks(workspaceID string) ([]domaintasks.Task, error) {
	return b.service.ListTasks(context.Background(), workspaceID)
}
func (b *TaskRunBinding) SearchTasks(input desktopservice.SearchTasksInput) (desktopservice.TaskSearchPage, error) {
	return b.service.SearchTasks(context.Background(), input)
}
func (b *TaskRunBinding) RenameTask(taskID, title string) (domaintasks.Task, error) {
	return b.service.RenameTask(context.Background(), taskID, title)
}
func (b *TaskRunBinding) SetTaskPinned(taskID string, pinned bool) (domaintasks.Task, error) {
	return b.service.SetTaskPinned(context.Background(), taskID, pinned)
}
func (b *TaskRunBinding) DeleteTask(taskID string) error {
	return b.service.DeleteTask(context.Background(), taskID)
}
func (b *TaskRunBinding) EnqueueTask(taskID, confirmationToken string) (domaintasks.Task, error) {
	return b.service.EnqueueTask(context.Background(), taskID, confirmationToken)
}
func (b *TaskRunBinding) QueueSnapshot(workspaceID string) ([]domaintasks.Task, error) {
	return b.service.QueueSnapshot(context.Background(), workspaceID)
}
func (b *TaskRunBinding) PreviewRun(taskID string) (desktopservice.RunStartPreview, error) {
	return b.service.PreviewRun(context.Background(), taskID)
}
func (b *TaskRunBinding) StartRun(taskID, confirmationToken string) (domainworkflows.Run, error) {
	return b.service.StartRunConfirmed(context.Background(), taskID, confirmationToken)
}
func (b *TaskRunBinding) GetRun(runID string) (desktopservice.RunDetail, error) {
	return b.service.GetRunDetail(context.Background(), runID)
}
func (b *TaskRunBinding) GetRunStreamSnapshot(runID string) []runstream.Frame {
	return b.service.GetRunStreamSnapshot(runID)
}
func (b *TaskRunBinding) ListRunsByTask(taskID string) ([]domainworkflows.Run, error) {
	return b.service.ListRunsByTask(context.Background(), taskID)
}
func (b *TaskRunBinding) ListRuns(input desktopservice.ListRunsInput) (desktopservice.RunListPage, error) {
	return b.service.ListRuns(context.Background(), input)
}
func (b *TaskRunBinding) ListRunEvents(runID string, afterSeq int64) ([]desktopservice.WorkflowEventView, error) {
	return b.service.ListRunEvents(context.Background(), runID, afterSeq)
}
func (b *TaskRunBinding) RespondPermission(input desktopservice.PermissionDecisionInput) error {
	return b.service.RespondPermission(input)
}
func (b *TaskRunBinding) InterruptRun(runID string) (domainworkflows.Run, error) {
	return b.service.InterruptRun(context.Background(), runID)
}
func (b *TaskRunBinding) ResumeRun(runID, instruction string) (domainworkflows.Run, error) {
	return b.service.ResumeRun(context.Background(), runID, instruction)
}
func (b *TaskRunBinding) ResumeRunConfigured(runID string, input desktopservice.ResumeRunInput) (domainworkflows.Run, error) {
	return b.service.ResumeRunConfigured(context.Background(), runID, input)
}
func (b *TaskRunBinding) CancelRun(runID string) (domainworkflows.Run, error) {
	return b.service.CancelRun(context.Background(), runID)
}
func (b *TaskRunBinding) EnqueueInstruction(runID string, input desktopservice.InstructionInput) (domainworkflows.Instruction, error) {
	return b.service.EnqueueInstruction(context.Background(), runID, input)
}
func (b *TaskRunBinding) RemoveInstruction(runID, instructionID string) error {
	return b.service.RemoveInstruction(context.Background(), runID, instructionID)
}
func (b *TaskRunBinding) InterruptAndInsert(runID string, input desktopservice.InstructionInput) (domainworkflows.Instruction, error) {
	return b.service.InterruptAndInsert(context.Background(), runID, input)
}
