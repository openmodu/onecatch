package desktop

import (
	"bytes"
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/openmodu/onecatch/internal/service/mobile"
	"github.com/openmodu/onecatch/internal/service/worker"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
)

func TestPhoneReadsConversationImagesWithPairedCredentials(t *testing.T) {
	ctx := context.Background()
	engine := &fifoEngine{started: make(chan agentrun.Request, 4), release: make(chan struct{}, 4)}
	app, store := newLocalTestApp(t, engine)
	binary, err := exec.LookPath("go")
	if err != nil {
		t.Fatal(err)
	}
	app.runtimes.engine = agentrun.NewEngine(agentrun.Config{ModuIntegration: "cli", Binaries: map[string]string{"modu": binary}})
	root := t.TempDir()
	workspace, err := app.AddWorkspace(ctx, AddWorkspaceInput{Path: root})
	if err != nil {
		t.Fatal(err)
	}
	png, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")
	imagePath := filepath.Join(root, "photo with space.png")
	outside := filepath.Join(t.TempDir(), "outside.png")
	for _, path := range []string{imagePath, outside} {
		if err := os.WriteFile(path, png, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "fake.png"), []byte("not an image"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape.png")); err != nil {
		t.Fatal(err)
	}
	task, err := app.CreateTask(ctx, CreateTaskInput{WorkspaceID: workspace.ID, WorkflowID: directAgentWorkflowID, Title: "image", Prompt: "inspect", Harness: "modu", AttachmentPaths: []string{imagePath}})
	if err != nil {
		t.Fatal(err)
	}
	run, err := app.StartRun(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	awaitHostedRequest(t, engine)
	defer func() { engine.release <- struct{}{} }()
	server := worker.NewServer("image-host", "Desktop", "secret", nil, app.runtimes, 1)
	server.SetSharedRuns(&hostedRuns{app: app})
	server.EnablePairing("PAIR1234", time.Now().Add(time.Minute), true)
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()
	phone, err := mobile.NewService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer phone.Close()
	if _, err := phone.PairWorker(ctx, httpServer.URL, "PAIR1234"); err != nil {
		t.Fatal(err)
	}
	views := phone.ListRuns()
	if len(views) != 1 || len(views[0].Attachments) != 1 {
		t.Fatalf("missing attachment metadata: %+v", views)
	}
	instruction, err := app.EnqueueInstruction(ctx, run.ID, InstructionInput{Content: "follow-up photo", AttachmentPaths: []string{imagePath}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Repos.Workflows.ClaimInstructions(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	detail, err := phone.GetRun(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, event := range detail.Events {
		if event.Kind == "user_message" && event.Text == "follow-up photo" {
			found = len(event.Attachments) == 1 && event.Attachments[0] == instruction.Attachments[0]
		}
	}
	if !found {
		t.Fatal("follow-up attachment metadata was lost")
	}
	for _, path := range []string{imagePath, "photo with space.png", "file://" + imagePath, task.Attachments[0].StoredPath, instruction.Attachments[0]} {
		got, mime, err := phone.ReadConversationImage(ctx, run.ID, path)
		if err != nil || mime != "image/png" || !bytes.Equal(got, png) {
			t.Errorf("image %q: mime=%s err=%v", path, mime, err)
		}
	}
	for _, path := range []string{outside, "escape.png", "fake.png", "https://example.com/image.png", "file://other-host/photo.png", "", "../outside.png"} {
		if _, _, err := phone.ReadConversationImage(ctx, run.ID, path); err == nil {
			t.Errorf("unexpected access to %q", path)
		}
	}
	if _, _, err := phone.ReadConversationImage(ctx, "unknown", imagePath); err == nil {
		t.Fatal("unknown run allowed")
	}
	response, err := http.Get(httpServer.URL + "/v1/shared-runs/" + run.ID + "/image?path=photo.png")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d", response.StatusCode)
	}
}
