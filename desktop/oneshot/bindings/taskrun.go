package bindings

import (
	"context"

	"github.com/openmodu/oneshot/internal/app/localapp"
	domaintasks "github.com/openmodu/oneshot/internal/domain/tasks"
	domainworkflows "github.com/openmodu/oneshot/internal/domain/workflows"
)

type TaskRunBinding struct{ app *localapp.App }

func NewTaskRunBinding(app *localapp.App) *TaskRunBinding { return &TaskRunBinding{app: app} }

func (b *TaskRunBinding) CreateTask(input localapp.CreateTaskInput) (domaintasks.Task, error) {
	return b.app.CreateTask(context.Background(), input)
}
func (b *TaskRunBinding) ListTasks(workspaceID string) ([]domaintasks.Task, error) {
	return b.app.ListTasks(context.Background(), workspaceID)
}
func (b *TaskRunBinding) StartRun(taskID string) (domainworkflows.Run, error) {
	return b.app.StartRun(context.Background(), taskID)
}
func (b *TaskRunBinding) GetRun(runID string) (localapp.RunDetail, error) {
	return b.app.GetRunDetail(context.Background(), runID)
}
func (b *TaskRunBinding) ListRunsByTask(taskID string) ([]domainworkflows.Run, error) {
	return b.app.ListRunsByTask(context.Background(), taskID)
}
func (b *TaskRunBinding) ListRunEvents(runID string, afterSeq int64) ([]localapp.WorkflowEventView, error) {
	return b.app.ListRunEvents(context.Background(), runID, afterSeq)
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
