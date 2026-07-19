package localapp

import (
	"archive/zip"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	localdata "github.com/openmodu/oneshot/internal/data/local"
	domainsettings "github.com/openmodu/oneshot/internal/domain/settings"
	domainworkflows "github.com/openmodu/oneshot/internal/domain/workflows"
	"github.com/openmodu/oneshot/internal/gitinspect"
	repoworkflows "github.com/openmodu/oneshot/internal/repo/workflows"
	workflowuc "github.com/openmodu/oneshot/internal/usecase/workflows"
	"github.com/openmodu/oneshot/internal/workspacelock"
)

func TestStorageUsageDoesNotFollowSymlinks(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret.bin")
	if err := os.WriteFile(outside, make([]byte, 4096), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "local.bin"), make([]byte, 32), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "linked.bin")); err != nil {
		t.Fatal(err)
	}
	usage, err := calculateStorageUsage(root)
	if err != nil {
		t.Fatal(err)
	}
	if usage.TotalBytes != 32 {
		t.Fatalf("total = %d, followed symlink", usage.TotalBytes)
	}
}

func TestCleanupPreviewRechecksAndRemovesEligibleRuns(t *testing.T) {
	app, orchestrator := newStorageTestApp(t)
	ctx := context.Background()
	if err := app.InitializeSettings(ctx); err != nil {
		t.Fatal(err)
	}
	if err := app.EnsureBuiltinDefinitions(ctx); err != nil {
		t.Fatal(err)
	}
	workspace, err := app.AddWorkspace(ctx, AddWorkspaceInput{Path: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	task, err := app.CreateTask(ctx, CreateTaskInput{WorkspaceID: workspace.ID, WorkflowID: "single_agent", Title: "old", Prompt: "old"})
	if err != nil {
		t.Fatal(err)
	}
	run, err := orchestrator.StartTask(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	run.Status = "completed"
	run.CompletedAt = time.Now().UTC().Add(-45 * 24 * time.Hour)
	run.UpdatedAt = run.CompletedAt
	run, err = app.store.Repos.Workflows.UpdateRun(ctx, run, run.Revision)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := app.PreviewCleanup(ctx, CleanupPreviewInput{RetentionDays: 30})
	if err != nil {
		t.Fatal(err)
	}
	if preview.Count != 1 || preview.RunIDs[0] != run.ID {
		t.Fatalf("preview = %+v", preview)
	}
	result, err := app.ExecuteCleanup(ctx, preview.Token)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.RemovedRunIDs) != 1 {
		t.Fatalf("result = %+v", result)
	}
	if _, err := app.store.Repos.Workflows.GetRun(ctx, run.ID); !errors.Is(err, repoworkflows.ErrRunNotFound) {
		t.Fatalf("run still exists: %v", err)
	}
	if _, err := app.ExecuteCleanup(ctx, preview.Token); errorCode(err) != "cleanup_preview_expired" {
		t.Fatalf("token was reusable: %v", err)
	}
}

func TestDiagnosticsNeverPersistsAllowedEnvironmentValue(t *testing.T) {
	app, _ := newStorageTestApp(t)
	ctx := context.Background()
	if err := app.InitializeSettings(ctx); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ONESHOT_TEST_SECRET", "do-not-export-this-value")
	settings, err := app.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	runtime := settings.Runtimes["codex"]
	runtime.EnvironmentAllowlist = []string{"ONESHOT_TEST_SECRET"}
	settings.Runtimes["codex"] = runtime
	if _, err := app.UpdateRuntimeSettings(ctx, settings.Runtimes, settings.Revision); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(t.TempDir(), "diagnostics.zip")
	if _, err := app.ExportDiagnostics(ctx, DiagnosticsExportInput{Destination: destination}); err != nil {
		t.Fatal(err)
	}
	archive, err := zip.OpenReader(destination)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	var all strings.Builder
	for _, item := range archive.File {
		file, err := item.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(file)
		_ = file.Close()
		if err != nil {
			t.Fatal(err)
		}
		all.Write(data)
	}
	if strings.Contains(all.String(), "do-not-export-this-value") {
		t.Fatal("diagnostics contained an environment value")
	}
}

func TestConfigureModuProviderOverridesInheritedValue(t *testing.T) {
	got := configureModuProvider([]string{"PATH=/bin", "MODU_CODE_PROVIDER=openai"}, "anthropic")
	if strings.Join(got, "|") != "PATH=/bin|MODU_CODE_PROVIDER=anthropic" {
		t.Fatalf("environment = %v", got)
	}
	got = configureModuProvider(got, "auto")
	if strings.Join(got, "|") != "PATH=/bin" {
		t.Fatalf("auto environment = %v", got)
	}
}

func TestRunFreezesResolvedSettings(t *testing.T) {
	app, _ := newStorageTestApp(t)
	ctx := context.Background()
	if err := app.InitializeSettings(ctx); err != nil {
		t.Fatal(err)
	}
	if err := app.EnsureBuiltinDefinitions(ctx); err != nil {
		t.Fatal(err)
	}
	settings, err := app.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	runtime := settings.Runtimes["codex"]
	runtime.DefaultModel = "snapshot-model"
	runtime.ReasoningEffort = "xhigh"
	runtime.ServiceTier = "priority"
	runtime.EnvironmentAllowlist = []string{"ONESHOT_TEST_ENV"}
	settings.Runtimes["codex"] = runtime
	claude := settings.Runtimes["claude"]
	claude.ReasoningEffort = "high"
	settings.Runtimes["claude"] = claude
	modu := settings.Runtimes["modu"]
	modu.Provider = "anthropic"
	settings.Runtimes["modu"] = modu
	saved, err := app.UpdateRuntimeSettings(ctx, settings.Runtimes, settings.Revision)
	if err != nil {
		t.Fatal(err)
	}
	saved.Execution.MaxLocalDAGConcurrency = 7
	saved.Execution.InterruptGraceSeconds = 22
	saved, err = app.UpdateExecutionSettings(ctx, saved.Execution, saved.Revision)
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := app.AddWorkspace(ctx, AddWorkspaceInput{Path: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	task, err := app.CreateTask(ctx, CreateTaskInput{WorkspaceID: workspace.ID, WorkflowID: "single_agent", Title: "snapshot", Prompt: "snapshot"})
	if err != nil {
		t.Fatal(err)
	}
	run, err := app.StartRun(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := app.store.Repos.Workflows.GetRunDefinition(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Steps[0].Model != "snapshot-model" || run.MaxLocalDAGConcurrency != 7 || run.InterruptGraceSeconds != 22 {
		t.Fatalf("resolved snapshot missing: run=%+v definition=%+v", run, snapshot)
	}
	if got := run.RuntimeSettings["codex"].EnvironmentAllowlist; len(got) != 1 || got[0] != "ONESHOT_TEST_ENV" {
		t.Fatalf("environment snapshot = %#v", got)
	}
	if got := run.RuntimeSettings["codex"]; got.ReasoningEffort != "xhigh" || got.ServiceTier != "priority" {
		t.Fatalf("Codex model settings snapshot = %#v", got)
	}
	if got := run.RuntimeSettings["claude"].ReasoningEffort; got != "high" {
		t.Fatalf("Claude Code effort snapshot = %q", got)
	}
	if got := run.RuntimeSettings["modu"].Provider; got != "anthropic" {
		t.Fatalf("modu provider snapshot = %q", got)
	}
}

func TestFullSandboxRequiresGrantConfirmationAndBlocksResumeAfterDowngrade(t *testing.T) {
	app, orchestrator := newStorageTestApp(t)
	ctx := context.Background()
	if err := app.InitializeSettings(ctx); err != nil {
		t.Fatal(err)
	}
	settings, err := app.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	settings.Security.AllowFullSandbox = true
	settings, err = app.UpdateSecuritySettings(ctx, settings.Security, settings.Revision)
	if err != nil {
		t.Fatal(err)
	}
	definition := domainworkflows.Definition{ID: "full_flow", Name: "Full", EntryStepID: "run", Steps: []domainworkflows.Step{{ID: "run", Name: "Run", Runtime: "codex", Sandbox: "full", WorkerID: "local", RolePrompt: "Run", Instruction: "Run", Transitions: map[string]string{"completed": domainworkflows.TargetDone}}}}
	if _, err := app.SaveDefinition(ctx, definition); err != nil {
		t.Fatal(err)
	}
	workspace, err := app.AddWorkspace(ctx, AddWorkspaceInput{Path: t.TempDir(), DefaultSandbox: "full"})
	if err != nil {
		t.Fatal(err)
	}
	task, err := app.CreateTask(ctx, CreateTaskInput{WorkspaceID: workspace.ID, WorkflowID: definition.ID, Title: "full", Prompt: "full"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.StartRun(ctx, task.ID); errorCode(err) != "security_full_sandbox_confirmation_required" {
		t.Fatalf("start without confirmation = %v", err)
	}
	preview, err := app.PreviewRun(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !preview.RequiresFullSandboxConfirmation || preview.ConfirmationToken == "" {
		t.Fatalf("preview = %+v", preview)
	}
	resolved, resolution, err := app.resolveRunSettings(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	run, err := orchestrator.StartTaskResolved(ctx, task.ID, resolved, resolution)
	if err != nil {
		t.Fatal(err)
	}
	if err := app.RecoverInterruptedRuns(ctx); err != nil {
		t.Fatal(err)
	}
	settings, err = app.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	settings.Security = domainsettings.Defaults().Security
	if _, err := app.UpdateSecuritySettings(ctx, settings.Security, settings.Revision); err != nil {
		t.Fatal(err)
	}
	if _, err := app.ResumeRun(ctx, run.ID, ""); errorCode(err) != "security_full_sandbox_disabled" {
		t.Fatalf("resume after downgrade = %v", err)
	}
}

func newStorageTestApp(t *testing.T) (*App, *workflowuc.Usecase) {
	t.Helper()
	root := t.TempDir()
	store, err := localdata.OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	runtimes, err := NewRuntimeRegistry(root)
	if err != nil {
		t.Fatal(err)
	}
	git := gitinspect.New("")
	orchestrator := workflowuc.NewUsecase(store.Repos.Tasks, store.Repos.Workflows, completingEngine{}, workspacelock.New(store.Data.Paths.Locks), git)
	app := New(store, orchestrator, runtimes, git)
	t.Cleanup(func() { _ = app.Close() })
	return app, orchestrator
}

func errorCode(err error) string {
	var codedError Error
	if errors.As(err, &codedError) {
		return codedError.Code
	}
	return ""
}
