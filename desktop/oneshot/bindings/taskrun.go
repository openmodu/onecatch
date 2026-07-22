package bindings

import (
	"context"

	"github.com/openmodu/oneshot/internal/app/localapp"
	domaintasks "github.com/openmodu/oneshot/internal/domain/tasks"
	domainworkflows "github.com/openmodu/oneshot/internal/domain/workflows"
	"github.com/openmodu/oneshot/internal/runstream"
)

type TaskRunBinding struct{ app *localapp.App }

func NewTaskRunBinding(app *localapp.App) *TaskRunBinding { return &TaskRunBinding{app: app} }

func (b *TaskRunBinding) CreateTask(input localapp.CreateTaskInput) (domaintasks.Task, error) {
	return b.app.CreateTask(context.Background(), input)
}
func (b *TaskRunBinding) ListTasks(workspaceID string) ([]domaintasks.Task, error) {
	return b.app.ListTasks(context.Background(), workspaceID)
}
func (b *TaskRunBinding) SearchTasks(input localapp.SearchTasksInput) (localapp.TaskSearchPage, error) {
	return b.app.SearchTasks(context.Background(), input)
}
func (b *TaskRunBinding) RenameTask(taskID, title string) (domaintasks.Task, error) {
	return b.app.RenameTask(context.Background(), taskID, title)
}
func (b *TaskRunBinding) DeleteTask(taskID string) error {
	return b.app.DeleteTask(context.Background(), taskID)
}
func (b *TaskRunBinding) EnqueueTask(taskID, confirmationToken string) (domaintasks.Task, error) {
	return b.app.EnqueueTask(context.Background(), taskID, confirmationToken)
}
func (b *TaskRunBinding) QueueSnapshot(workspaceID string) ([]domaintasks.Task, error) {
	return b.app.QueueSnapshot(context.Background(), workspaceID)
}
func (b *TaskRunBinding) PreviewRun(taskID string) (localapp.RunStartPreview, error) {
	return b.app.PreviewRun(context.Background(), taskID)
}
func (b *TaskRunBinding) StartRun(taskID, confirmationToken string) (domainworkflows.Run, error) {
	return b.app.StartRunConfirmed(context.Background(), taskID, confirmationToken)
}
func (b *TaskRunBinding) GetRun(runID string) (localapp.RunDetail, error) {
	return b.app.GetRunDetail(context.Background(), runID)
}
func (b *TaskRunBinding) GetRunStreamSnapshot(runID string) []runstream.Frame {
	return b.app.GetRunStreamSnapshot(runID)
}
func (b *TaskRunBinding) GetStepRunTranscript(runID, stepRunID string) ([]localapp.RuntimeEventView, error) {
	return b.app.GetStepRunTranscript(context.Background(), runID, stepRunID)
}
func (b *TaskRunBinding) ListRunsByTask(taskID string) ([]domainworkflows.Run, error) {
	return b.app.ListRunsByTask(context.Background(), taskID)
}
func (b *TaskRunBinding) ListRuns(input localapp.ListRunsInput) (localapp.RunListPage, error) {
	return b.app.ListRuns(context.Background(), input)
}
func (b *TaskRunBinding) ListRunEvents(runID string, afterSeq int64) ([]localapp.WorkflowEventView, error) {
	return b.app.ListRunEvents(context.Background(), runID, afterSeq)
}
func (b *TaskRunBinding) RespondPermission(input localapp.PermissionDecisionInput) error {
	return b.app.RespondPermission(input)
}
func (b *TaskRunBinding) InterruptRun(runID string) (domainworkflows.Run, error) {
	return b.app.InterruptRun(context.Background(), runID)
}
func (b *TaskRunBinding) ResumeRun(runID, instruction string) (domainworkflows.Run, error) {
	return b.app.ResumeRun(context.Background(), runID, instruction)
}
func (b *TaskRunBinding) CancelRun(runID string) (domainworkflows.Run, error) {
	return b.app.CancelRun(context.Background(), runID)
}
func (b *TaskRunBinding) EnqueueInstruction(runID string, input localapp.InstructionInput) (domainworkflows.Instruction, error) {
	return b.app.EnqueueInstruction(context.Background(), runID, input)
}
func (b *TaskRunBinding) RemoveInstruction(runID, instructionID string) error {
	return b.app.RemoveInstruction(context.Background(), runID, instructionID)
}
func (b *TaskRunBinding) InterruptAndInsert(runID string, input localapp.InstructionInput) (domainworkflows.Instruction, error) {
	return b.app.InterruptAndInsert(context.Background(), runID, input)
}
