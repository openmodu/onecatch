package desktop

import (
	"archive/zip"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	domainsettings "github.com/openmodu/oneshot/internal/domain/settings"
	domaintasks "github.com/openmodu/oneshot/internal/domain/tasks"
	domainworkflows "github.com/openmodu/oneshot/internal/domain/workflows"
)

type StorageCategory struct {
	Name  string `json:"name"`
	Bytes int64  `json:"bytes"`
	Files int64  `json:"files"`
}
type StorageUsage struct {
	Root         string            `json:"root"`
	TotalBytes   int64             `json:"totalBytes"`
	Categories   []StorageCategory `json:"categories"`
	CalculatedAt time.Time         `json:"calculatedAt"`
}
type CleanupPreviewInput struct {
	RetentionDays int `json:"retentionDays"`
}
type CleanupPreview struct {
	Token          string    `json:"token"`
	RunIDs         []string  `json:"runIds"`
	Count          int       `json:"count"`
	EstimatedBytes int64     `json:"estimatedBytes"`
	ExpiresAt      time.Time `json:"expiresAt"`
}
type CleanupResult struct {
	RemovedRunIDs []string `json:"removedRunIds"`
	SkippedRunIDs []string `json:"skippedRunIds"`
	RemovedBytes  int64    `json:"removedBytes"`
}
type DiagnosticsExportInput struct {
	Destination      string   `json:"destination"`
	RunIDs           []string `json:"runIds,omitempty"`
	IncludePrompt    bool     `json:"includePrompt"`
	IncludeRawEvents bool     `json:"includeRawEvents"`
}
type DiagnosticsExport struct {
	Path      string    `json:"path"`
	Bytes     int64     `json:"bytes"`
	CreatedAt time.Time `json:"createdAt"`
}

type cleanupCandidate struct {
	ID       string
	Revision int64
	Status   domainworkflows.RunStatus
	Bytes    int64
}
type cleanupPlan struct {
	Candidates []cleanupCandidate
	ExpiresAt  time.Time
	Running    bool
}

func (a *Service) GetStorageUsage() (StorageUsage, error) {
	return calculateStorageUsage(a.store.Data.Paths.Root)
}

func calculateStorageUsage(root string) (StorageUsage, error) {
	byName := map[string]*StorageCategory{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		parts := strings.Split(filepath.ToSlash(relative), "/")
		name := parts[0]
		if name == "runs" && (strings.HasSuffix(path, ".jsonl") || strings.Contains(filepath.ToSlash(relative), "/events/")) {
			name = "events"
		}
		if strings.HasPrefix(name, ".") {
			name = "other"
		}
		category := byName[name]
		if category == nil {
			category = &StorageCategory{Name: name}
			byName[name] = category
		}
		category.Bytes += info.Size()
		category.Files++
		return nil
	})
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return StorageUsage{}, fmt.Errorf("calculate storage usage: %w", err)
	}
	usage := StorageUsage{Root: root, CalculatedAt: time.Now().UTC()}
	for _, category := range byName {
		usage.TotalBytes += category.Bytes
		usage.Categories = append(usage.Categories, *category)
	}
	sort.Slice(usage.Categories, func(i, j int) bool { return usage.Categories[i].Name < usage.Categories[j].Name })
	return usage, nil
}

func (a *Service) RevealDataRoot() error {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		command = exec.Command("open", a.DataRoot())
	case "windows":
		command = exec.Command("explorer", a.DataRoot())
	default:
		command = exec.Command("xdg-open", a.DataRoot())
	}
	if err := command.Start(); err != nil {
		return coded("storage_reveal_failed", err.Error())
	}
	return nil
}

func (a *Service) PreviewCleanup(ctx context.Context, input CleanupPreviewInput) (CleanupPreview, error) {
	settings, err := a.settings.Get(ctx)
	if err != nil {
		return CleanupPreview{}, mapSettingsError(err)
	}
	days := input.RetentionDays
	if days == 0 {
		days = settings.Storage.CompletedRunRetentionDays
	}
	if days == 0 {
		return CleanupPreview{RunIDs: []string{}, ExpiresAt: time.Now().UTC().Add(5 * time.Minute)}, nil
	}
	if days != 30 && days != 90 && days != 180 {
		return CleanupPreview{}, coded("settings_invalid", "retentionDays must be 30, 90, or 180")
	}
	runs, err := a.store.Repos.Workflows.ListRunsByTask(ctx, "")
	if err != nil {
		return CleanupPreview{}, err
	}
	cutoff := time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour)
	plan := cleanupPlan{ExpiresAt: time.Now().UTC().Add(5 * time.Minute)}
	for _, run := range runs {
		if run.Status != domainworkflows.RunCompleted && run.Status != domainworkflows.RunCancelled {
			continue
		}
		completed := run.CompletedAt
		if completed.IsZero() {
			completed = run.UpdatedAt
		}
		if completed.After(cutoff) {
			continue
		}
		bytes, _ := directorySize(filepath.Join(a.store.Data.Paths.Runs, run.ID))
		plan.Candidates = append(plan.Candidates, cleanupCandidate{ID: run.ID, Revision: run.Revision, Status: run.Status, Bytes: bytes})
	}
	token := randomToken()
	preview := CleanupPreview{Token: token, ExpiresAt: plan.ExpiresAt}
	for _, item := range plan.Candidates {
		preview.RunIDs = append(preview.RunIDs, item.ID)
		preview.EstimatedBytes += item.Bytes
	}
	preview.Count = len(preview.RunIDs)
	a.cleanupMu.Lock()
	a.cleanupPlans[token] = plan
	a.cleanupMu.Unlock()
	return preview, nil
}

func (a *Service) ExecuteCleanup(ctx context.Context, token string) (CleanupResult, error) {
	a.cleanupMu.Lock()
	plan, ok := a.cleanupPlans[token]
	if ok && plan.Running {
		a.cleanupMu.Unlock()
		return CleanupResult{}, coded("cleanup_state_changed", "cleanup is already running")
	}
	if ok {
		plan.Running = true
		a.cleanupPlans[token] = plan
	}
	a.cleanupMu.Unlock()
	if !ok || time.Now().UTC().After(plan.ExpiresAt) {
		a.cleanupMu.Lock()
		delete(a.cleanupPlans, token)
		a.cleanupMu.Unlock()
		return CleanupResult{}, coded("cleanup_preview_expired", "cleanup preview is missing or expired")
	}
	trashRoot := filepath.Join(a.DataRoot(), ".trash", randomToken())
	if err := os.MkdirAll(trashRoot, 0o700); err != nil {
		a.resetCleanupPlan(token)
		return CleanupResult{}, err
	}
	result := CleanupResult{}
	var moved []cleanupCandidate
	for _, candidate := range plan.Candidates {
		if err := ctx.Err(); err != nil {
			rollbackCleanupMoves(a.store.Data.Paths.Runs, trashRoot, moved)
			a.resetCleanupPlan(token)
			return CleanupResult{}, err
		}
		if a.isActive(candidate.ID) {
			result.SkippedRunIDs = append(result.SkippedRunIDs, candidate.ID)
			continue
		}
		current, err := a.store.Repos.Workflows.GetRun(ctx, candidate.ID)
		if err != nil || current.Revision != candidate.Revision || current.Status != candidate.Status {
			result.SkippedRunIDs = append(result.SkippedRunIDs, candidate.ID)
			continue
		}
		source := filepath.Join(a.store.Data.Paths.Runs, candidate.ID)
		destination := filepath.Join(trashRoot, candidate.ID)
		if err := os.Rename(source, destination); err != nil {
			if rollbackErr := rollbackCleanupMoves(a.store.Data.Paths.Runs, trashRoot, moved); rollbackErr != nil {
				a.cleanupMu.Lock()
				delete(a.cleanupPlans, token)
				a.cleanupMu.Unlock()
				return CleanupResult{}, fmt.Errorf("move run to cleanup trash: %v (rollback failed: %v)", err, rollbackErr)
			}
			a.resetCleanupPlan(token)
			return CleanupResult{}, fmt.Errorf("move run to cleanup trash: %w", err)
		}
		moved = append(moved, candidate)
		result.RemovedRunIDs = append(result.RemovedRunIDs, candidate.ID)
		result.RemovedBytes += candidate.Bytes
	}
	if err := os.RemoveAll(trashRoot); err != nil {
		a.cleanupMu.Lock()
		delete(a.cleanupPlans, token)
		a.cleanupMu.Unlock()
		return result, fmt.Errorf("remove cleanup trash: %w", err)
	}
	a.cleanupMu.Lock()
	delete(a.cleanupPlans, token)
	a.cleanupMu.Unlock()
	return result, nil
}

func (a *Service) resetCleanupPlan(token string) {
	a.cleanupMu.Lock()
	plan, ok := a.cleanupPlans[token]
	if ok {
		plan.Running = false
		a.cleanupPlans[token] = plan
	}
	a.cleanupMu.Unlock()
}

func rollbackCleanupMoves(runsRoot, trashRoot string, moved []cleanupCandidate) error {
	var first error
	for index := len(moved) - 1; index >= 0; index-- {
		candidate := moved[index]
		if err := os.Rename(filepath.Join(trashRoot, candidate.ID), filepath.Join(runsRoot, candidate.ID)); err != nil && first == nil {
			first = err
		}
	}
	_ = os.RemoveAll(trashRoot)
	return first
}

func (a *Service) ExportDiagnostics(ctx context.Context, input DiagnosticsExportInput) (DiagnosticsExport, error) {
	settings, err := a.settings.Get(ctx)
	if err != nil {
		return DiagnosticsExport{}, mapSettingsError(err)
	}
	if input.IncludePrompt && !settings.Security.DiagnosticsIncludePrompt {
		return DiagnosticsExport{}, coded("diagnostics_export_failed", "Prompt export is disabled in Settings")
	}
	if input.IncludeRawEvents && !settings.Security.DiagnosticsIncludeRawEvents {
		return DiagnosticsExport{}, coded("diagnostics_export_failed", "raw event export is disabled in Settings")
	}
	destination := strings.TrimSpace(input.Destination)
	if destination == "" {
		destination = filepath.Join(a.DataRoot(), fmt.Sprintf("oneshot-diagnostics-%s.zip", time.Now().Format("20060102-150405")))
	}
	if filepath.Ext(destination) != ".zip" {
		destination += ".zip"
	}
	file, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return DiagnosticsExport{}, coded("diagnostics_export_failed", err.Error())
	}
	archive := zip.NewWriter(file)
	closeAll := func() error {
		if err := archive.Close(); err != nil {
			_ = file.Close()
			return err
		}
		return file.Close()
	}
	settings.UpdatedAt = settings.UpdatedAt.UTC()
	secrets := inheritedSecretValues(settings)
	settings = redactSettingsPaths(settings)
	usage, _ := a.GetStorageUsage()
	usage.Root = "$HOME/.oneshot"
	if err := zipRedactedJSON(archive, "settings.json", settings, secrets); err != nil {
		_ = closeAll()
		return DiagnosticsExport{}, coded("diagnostics_export_failed", err.Error())
	}
	if err := zipRedactedJSON(archive, "storage.json", usage, secrets); err != nil {
		_ = closeAll()
		return DiagnosticsExport{}, coded("diagnostics_export_failed", err.Error())
	}
	if err := zipRedactedJSON(archive, "runtimes.json", a.ListRuntimes(), secrets); err != nil {
		_ = closeAll()
		return DiagnosticsExport{}, coded("diagnostics_export_failed", err.Error())
	}
	for _, runID := range input.RunIDs {
		run, err := a.store.Repos.Workflows.GetRun(ctx, runID)
		if err != nil {
			continue
		}
		if err := zipRedactedJSON(archive, filepath.Join("runs", runID, "state.json"), run, secrets); err != nil {
			_ = closeAll()
			return DiagnosticsExport{}, coded("diagnostics_export_failed", err.Error())
		}
		if input.IncludePrompt {
			if task, err := a.store.Repos.Tasks.GetTask(ctx, run.TaskID); err == nil {
				task = redactTask(task)
				_ = zipRedactedJSON(archive, filepath.Join("runs", runID, "task.json"), task, secrets)
			}
		}
		if input.IncludeRawEvents {
			events, _ := a.store.Repos.Workflows.ListEvents(ctx, runID, 0, 10000)
			_ = zipRedactedJSON(archive, filepath.Join("runs", runID, "events.json"), events, secrets)
			stepRuns, _ := a.store.Repos.Workflows.ListStepRuns(ctx, runID)
			for _, stepRun := range stepRuns {
				runtimeEvents, _ := a.store.Repos.Workflows.ListRuntimeEvents(ctx, runID, stepRun.ID, 0, 10000)
				_ = zipRedactedJSON(archive, filepath.Join("runs", runID, "runtime-events", stepRun.ID+".json"), runtimeEvents, secrets)
			}
		}
	}
	_ = addLogFiles(archive, a.store.Data.Paths.Logs, secrets)
	if err := closeAll(); err != nil {
		return DiagnosticsExport{}, coded("diagnostics_export_failed", err.Error())
	}
	info, err := os.Stat(destination)
	if err != nil {
		return DiagnosticsExport{}, coded("diagnostics_export_failed", err.Error())
	}
	return DiagnosticsExport{Path: destination, Bytes: info.Size(), CreatedAt: time.Now().UTC()}, nil
}

func directorySize(root string) (int64, error) {
	var total int64
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err == nil {
			total += info.Size()
		}
		return err
	})
	return total, err
}
func randomToken() string {
	data := make([]byte, 16)
	if _, err := rand.Read(data); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(data)
}
func zipRedactedJSON(archive *zip.Writer, name string, value any, secrets []string) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	text := string(data)
	home, _ := os.UserHomeDir()
	if home != "" {
		text = strings.ReplaceAll(text, home, "$HOME")
	}
	for _, secret := range secrets {
		if secret != "" {
			text = strings.ReplaceAll(text, secret, "<redacted>")
		}
	}
	writer, err := archive.Create(filepath.ToSlash(name))
	if err != nil {
		return err
	}
	_, err = writer.Write(append([]byte(text), '\n'))
	return err
}
func addLogFiles(archive *zip.Writer, root string, secrets []string) error {
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		source, err := os.Open(filepath.Join(root, entry.Name()))
		if err != nil {
			continue
		}
		writer, err := archive.Create(filepath.Join("logs", filepath.Base(entry.Name())))
		if err == nil {
			data, readErr := io.ReadAll(io.LimitReader(source, 10<<20))
			if readErr != nil {
				err = readErr
			} else {
				home, _ := os.UserHomeDir()
				text := strings.ReplaceAll(string(data), home, "$HOME")
				for _, secret := range secrets {
					if secret != "" {
						text = strings.ReplaceAll(text, secret, "<redacted>")
					}
				}
				_, err = writer.Write([]byte(text))
			}
		}
		_ = source.Close()
		if err != nil {
			return err
		}
	}
	return nil
}
func redactTask(task domaintasks.Task) domaintasks.Task { return task }

func redactSettingsPaths(settings domainsettings.Settings) domainsettings.Settings {
	home, _ := os.UserHomeDir()
	for id, runtime := range settings.Runtimes {
		if home != "" {
			runtime.Binary = strings.ReplaceAll(runtime.Binary, home, "$HOME")
		}
		settings.Runtimes[id] = runtime
	}
	return settings
}

func inheritedSecretValues(settings domainsettings.Settings) []string {
	var values []string
	seen := map[string]struct{}{}
	for _, runtime := range settings.Runtimes {
		for _, key := range runtime.EnvironmentAllowlist {
			if value, ok := os.LookupEnv(key); ok {
				if _, exists := seen[value]; !exists {
					seen[value] = struct{}{}
					values = append(values, value)
				}
			}
		}
	}
	return values
}
