package keeldesktop

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/dyike/keel/ui/core"
	domainworkflows "github.com/openmodu/onecatch/internal/domain/workflows"
	desktopservice "github.com/openmodu/onecatch/internal/service/desktop"
	"github.com/openmodu/onecatch/internal/service/desktop/runstate"
)

func TestAttachmentSelectionDeduplicatesAndEnforcesBounds(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(path, []byte("test"), 0600); err != nil {
		t.Fatal(err)
	}
	selected, err := validateAttachmentFiles([]string{path, path}, nil)
	if err != nil || len(selected) != 1 || selected[0].Staged {
		t.Fatalf("selection: %+v, %v", selected, err)
	}
	if _, err = validateAttachmentFiles([]string{t.TempDir()}, selected); err == nil {
		t.Fatal("directory accepted as attachment")
	}
	big := filepath.Join(t.TempDir(), "large.bin")
	file, err := os.Create(big)
	if err != nil {
		t.Fatal(err)
	}
	file.Truncate(attachmentSize + 1)
	file.Close()
	if _, err = validateAttachmentFiles([]string{big}, nil); err == nil {
		t.Fatal("oversized attachment accepted")
	}
	existing := make([]selectedAttachment, attachmentCount)
	if _, err = validateAttachmentFiles([]string{path}, existing); err == nil {
		t.Fatal("too many attachments accepted")
	}
}
func TestPastedImagePersistsIntoTaskAndCleanupKeepsUserFile(t *testing.T) {
	ctx := context.Background()
	b, err := openBackendWithEngine(ctx, t.TempDir(), testEngine{})
	if err != nil {
		t.Fatal(err)
	}
	a := newApp(b)
	defer a.close()
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var encoded bytes.Buffer
	if err = png.Encode(&encoded, img); err != nil {
		t.Fatal(err)
	}
	data := core.ClipboardData{Images: []core.ClipboardImage{{MIME: "image/png", Data: encoded.Bytes()}}}
	items, err := stageClipboardAttachments(ctx, b.service, data, nil)
	if err != nil || len(items) != 1 || !items[0].Staged || items[0].Image == nil {
		t.Fatalf("pasted image: %+v, %v", items, err)
	}
	workspace, err := b.service.AddWorkspace(ctx, desktopservice.AddWorkspaceInput{Path: t.TempDir(), Name: "Attachments"})
	if err != nil {
		t.Fatal(err)
	}
	a.attachments.Items = items
	a.workspaceID = workspace.ID
	a.runtime.SetValue("codex")
	a.workbench.WorktreeMode = "project"
	input := a.createTaskInput("Read the image")
	input.Title = "Image task"
	task, err := b.service.CreateTask(ctx, input)
	if err != nil || len(task.Attachments) != 1 {
		t.Fatalf("task attachments: %+v, %v", task.Attachments, err)
	}
	stored := task.Attachments[0].StoredPath
	original := filepath.Join(t.TempDir(), "user.txt")
	os.WriteFile(original, []byte("keep"), 0600)
	a.attachments.Items = append(a.attachments.Items, selectedAttachment{Path: original})
	a.clearAttachments()
	if _, err = os.Stat(items[0].Path); !os.IsNotExist(err) {
		t.Fatal("pasted staging file leaked")
	}
	if _, err = os.Stat(original); err != nil {
		t.Fatal("cleanup deleted user file")
	}
	if _, err = os.Stat(stored); err != nil {
		t.Fatalf("persisted image lost: %v", err)
	}
}

func TestAttachmentContinuationPersistsOnceWhenResumeIsRejected(t *testing.T) {
	ctx := context.Background()
	b, err := openBackendWithEngine(ctx, t.TempDir(), testEngine{})
	if err != nil {
		t.Fatal(err)
	}
	defer b.close()
	workspace, err := b.service.AddWorkspace(ctx, desktopservice.AddWorkspaceInput{Path: t.TempDir(), Name: "Continuation"})
	if err != nil {
		t.Fatal(err)
	}
	task, err := b.service.CreateTask(ctx, desktopservice.CreateTaskInput{WorkspaceID: workspace.ID, WorktreeMode: "project", Title: "Continuation", Prompt: "Begin", WorkflowID: "single_agent", Harness: "codex", Sandbox: "read-only"})
	if err != nil {
		t.Fatal(err)
	}
	finished := make(chan runstate.View, 16)
	b.states.SetEmitter(func(view runstate.View) {
		if !view.Active && view.Run.Status == domainworkflows.RunCompleted {
			finished <- view
		}
	})
	run, err := b.service.StartRun(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	waitCompleted(t, finished, 0)
	note := filepath.Join(t.TempDir(), "note.md")
	if err = os.WriteFile(note, []byte("Follow-up note"), 0600); err != nil {
		t.Fatal(err)
	}
	persisted, err := b.service.SubmitRunMessage(ctx, run.ID, desktopservice.RunMessageInput{Resume: desktopservice.ResumeRunInput{Instruction: "Read this", Harness: "claude"}, AttachmentPaths: []string{note}})
	if !persisted || err == nil {
		t.Fatalf("resume rejection should retain persisted instruction: %v, %v", persisted, err)
	}
	detail, err := b.service.GetRunDetail(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Instructions) != 1 || detail.Instructions[0].Content != "Read this" || len(detail.Instructions[0].Attachments) != 1 {
		t.Fatalf("attachment instruction not persisted: %+v", detail.Instructions)
	}
	path := detail.Instructions[0].Attachments[0]
	if _, err = os.Stat(path); err != nil {
		t.Fatal(err)
	}
	// Retrying the persisted turn uses the empty instruction path; it must not
	// enqueue another instruction or duplicate the attachment.
	if _, err = b.service.SubmitRunMessage(ctx, run.ID, desktopservice.RunMessageInput{}); err != nil {
		t.Fatal(err)
	}
	waitCompleted(t, finished, detail.Run.Revision)
	resumed, err := b.service.GetRunDetail(ctx, run.ID)
	if err != nil || len(resumed.Instructions) != 1 {
		t.Fatalf("duplicate continuation: %d, %v", len(resumed.Instructions), err)
	}
}
