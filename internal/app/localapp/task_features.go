package localapp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	domaintasks "github.com/openmodu/oneshot/internal/domain/tasks"
	domainworkflows "github.com/openmodu/oneshot/internal/domain/workflows"
)

const (
	maxAttachmentCount = 8
	maxAttachmentSize  = 20 << 20
	maxAttachmentTotal = 50 << 20
)

func (a *App) RenameTask(ctx context.Context, taskID, title string) (domaintasks.Task, error) {
	task, err := a.store.Repos.Tasks.GetTask(ctx, strings.TrimSpace(taskID))
	if err != nil {
		return task, coded("task_not_found", "task was not found")
	}
	title = strings.TrimSpace(title)
	if title == "" || len([]rune(title)) > 160 {
		return task, coded("task_invalid", "task title must contain 1 to 160 characters")
	}
	task.Title = title
	task.UpdatedAt = time.Now().UTC()
	if err := a.store.Repos.Tasks.SaveTask(ctx, task); err != nil {
		return task, err
	}
	return task, nil
}

func (a *App) DeleteTask(ctx context.Context, taskID string) error {
	task, err := a.store.Repos.Tasks.GetTask(ctx, strings.TrimSpace(taskID))
	if err != nil {
		return coded("task_not_found", "task was not found")
	}
	runs, err := a.store.Repos.Workflows.ListRunsByTask(ctx, task.ID)
	if err != nil {
		return err
	}
	for _, run := range runs {
		if a.isActive(run.ID) || run.Status == domainworkflows.RunRunning {
			return coded("task_active", "interrupt the active run before deleting this task")
		}
	}
	workspace, _ := a.store.Repos.Tasks.GetWorkspace(ctx, task.WorkspaceID)
	if err := a.store.Repos.Tasks.DeleteTask(ctx, task.ID); err != nil {
		return err
	}
	if workspace.Path != "" {
		_ = os.RemoveAll(filepath.Join(workspace.Path, ".oneshot", "attachments", task.ID))
	}
	go a.reconcileWorkspaceQueue(task.WorkspaceID)
	return nil
}

func (a *App) EnqueueInstruction(ctx context.Context, runID string, input InstructionInput) (domainworkflows.Instruction, error) {
	return a.enqueueInstruction(ctx, runID, input, false)
}

func (a *App) enqueueInstruction(ctx context.Context, runID string, input InstructionInput, priority bool) (domainworkflows.Instruction, error) {
	run, err := a.store.Repos.Workflows.GetRun(ctx, strings.TrimSpace(runID))
	if err != nil {
		return domainworkflows.Instruction{}, coded("run_not_found", "run was not found")
	}
	if run.Status != domainworkflows.RunRunning && run.Status != domainworkflows.RunPaused {
		return domainworkflows.Instruction{}, coded("run_invalid_state", "instructions can only be queued for running or paused runs")
	}
	task, err := a.store.Repos.Tasks.GetTask(ctx, run.TaskID)
	if err != nil {
		return domainworkflows.Instruction{}, err
	}
	attachments, err := a.persistAttachments(ctx, task, input.AttachmentPaths)
	if err != nil {
		return domainworkflows.Instruction{}, err
	}
	paths := make([]string, 0, len(attachments))
	for _, attachment := range attachments {
		paths = append(paths, attachment.StoredPath)
	}
	instruction := domainworkflows.Instruction{ID: randomID("instruction"), Content: strings.TrimSpace(input.Content), Attachments: paths, Priority: priority, CreatedAt: time.Now().UTC()}
	if instruction.Content == "" && len(paths) > 0 {
		instruction.Content = "Review the attached files."
	}
	saved, err := a.store.Repos.Workflows.EnqueueInstruction(ctx, run.ID, instruction)
	if err != nil {
		return saved, mapError(err)
	}
	_, _ = a.store.Repos.Workflows.AppendEvent(ctx, domainworkflows.WorkflowEvent{RunID: run.ID, Type: "instruction.queued", StepID: run.CurrentStepID, Payload: localEventPayload(map[string]any{"instructionId": saved.ID, "priority": priority, "attachments": len(paths)}), At: time.Now().UTC()})
	return saved, nil
}

func (a *App) RemoveInstruction(ctx context.Context, runID, instructionID string) error {
	if err := a.store.Repos.Workflows.RemoveInstruction(ctx, runID, instructionID); err != nil {
		return mapError(err)
	}
	_, _ = a.store.Repos.Workflows.AppendEvent(ctx, domainworkflows.WorkflowEvent{RunID: runID, Type: "instruction.removed", Payload: localEventPayload(map[string]any{"instructionId": instructionID}), At: time.Now().UTC()})
	return nil
}

func (a *App) InterruptAndInsert(ctx context.Context, runID string, input InstructionInput) (domainworkflows.Instruction, error) {
	instruction, err := a.enqueueInstruction(ctx, runID, input, true)
	if err != nil {
		return instruction, err
	}
	if _, err := a.InterruptRun(ctx, runID); err != nil {
		_ = a.store.Repos.Workflows.RemoveInstruction(context.Background(), runID, instruction.ID)
		return instruction, err
	}
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		ticker := time.NewTicker(25 * time.Millisecond)
		defer ticker.Stop()
		timer := time.NewTimer(15 * time.Second)
		defer timer.Stop()
		for {
			select {
			case <-a.rootCtx.Done():
				return
			case <-timer.C:
				return
			case <-ticker.C:
				if a.isActive(runID) {
					continue
				}
				run, getErr := a.store.Repos.Workflows.GetRun(context.Background(), runID)
				if getErr == nil && run.Status == domainworkflows.RunPaused {
					_, _ = a.ResumeRun(context.Background(), runID, "")
				}
				return
			}
		}
	}()
	return instruction, nil
}

func (a *App) persistAttachments(ctx context.Context, task domaintasks.Task, paths []string) ([]domaintasks.Attachment, error) {
	if len(paths) == 0 {
		return nil, nil
	}
	if len(paths) > maxAttachmentCount {
		return nil, coded("attachment_limit", fmt.Sprintf("a maximum of %d attachments is allowed", maxAttachmentCount))
	}
	workspace, err := a.store.Repos.Tasks.GetWorkspace(ctx, task.WorkspaceID)
	if err != nil {
		return nil, err
	}
	root := filepath.Join(workspace.Path, ".oneshot", "attachments", task.ID)
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, fmt.Errorf("create attachment directory: %w", err)
	}
	_ = excludeLocalOneshot(workspace.Path)
	var total int64
	items := make([]domaintasks.Attachment, 0, len(paths))
	for _, source := range paths {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		abs, err := filepath.Abs(strings.TrimSpace(source))
		if err != nil {
			return nil, coded("attachment_invalid", "attachment path cannot be resolved")
		}
		info, err := os.Stat(abs)
		if err != nil || !info.Mode().IsRegular() {
			return nil, coded("attachment_invalid", "attachment must be a readable file")
		}
		if info.Size() > maxAttachmentSize || total+info.Size() > maxAttachmentTotal {
			return nil, coded("attachment_limit", "attachment size limit exceeded")
		}
		total += info.Size()
		id := randomID("attachment")
		name := safeAttachmentName(filepath.Base(abs))
		destination := filepath.Join(root, id+"-"+name)
		if err := copyFileAtomic(abs, destination); err != nil {
			return nil, err
		}
		items = append(items, domaintasks.Attachment{ID: id, Name: filepath.Base(abs), OriginalPath: abs, StoredPath: destination, MIMEType: mime.TypeByExtension(strings.ToLower(filepath.Ext(abs))), Size: info.Size(), CreatedAt: time.Now().UTC()})
	}
	return items, nil
}

func safeAttachmentName(value string) string {
	value = regexp.MustCompile(`[^a-zA-Z0-9._-]+`).ReplaceAllString(value, "-")
	value = strings.Trim(value, ".-")
	if value == "" {
		return "attachment"
	}
	return value
}

func copyFileAtomic(source, destination string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	temp := destination + ".tmp"
	output, err := os.OpenFile(temp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, io.LimitReader(input, maxAttachmentSize+1))
	closeErr := output.Close()
	if copyErr != nil {
		_ = os.Remove(temp)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(temp)
		return closeErr
	}
	return os.Rename(temp, destination)
}

func excludeLocalOneshot(workspace string) error {
	gitDir := filepath.Join(workspace, ".git")
	if info, err := os.Stat(gitDir); err != nil || !info.IsDir() {
		return nil
	}
	path := filepath.Join(gitDir, "info", "exclude")
	content, _ := os.ReadFile(path)
	if strings.Contains(string(content), "\n.oneshot/\n") || strings.HasPrefix(string(content), ".oneshot/\n") {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.WriteString("\n.oneshot/\n")
	return err
}

func localEventPayload(value any) json.RawMessage {
	payload, err := json.Marshal(value)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return payload
}
