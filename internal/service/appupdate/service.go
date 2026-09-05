package appupdate

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/openmodu/onecatch/internal/buildinfo"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/updater"
	"github.com/wailsapp/wails/v3/pkg/updater/providers/appcast"
)

const (
	readyEnvironment      = "ONECATCH_UPDATE_READY_FILE"
	updateDownloadTimeout = 10 * time.Minute
)

const EventStatusChanged = "onecatch:update:status-changed"

type Status struct {
	CurrentVersion      string    `json:"currentVersion"`
	State               string    `json:"state"`
	AvailableVersion    string    `json:"availableVersion,omitempty"`
	Name                string    `json:"name,omitempty"`
	Notes               string    `json:"notes,omitempty"`
	PublishedAt         time.Time `json:"publishedAt,omitempty"`
	AutomaticSupported  bool      `json:"automaticSupported"`
	VerificationEnabled bool      `json:"verificationEnabled"`
	FeedURL             string    `json:"feedUrl,omitempty"`
	DownloadPath        string    `json:"downloadPath,omitempty"`
	Error               string    `json:"error,omitempty"`
}

type Service struct {
	app         *application.App
	updater     *updater.Updater
	feedURL     string
	cacheRoot   string
	publicKey   []byte
	verified    bool
	operationMu sync.Mutex

	mu      sync.RWMutex
	release *updater.Release
	cached  *cachedUpdate
	lastErr string
	cancel  context.CancelFunc
	done    chan struct{}
}

func New(app *application.App, dataRoot string) (*Service, error) {
	if app == nil {
		return nil, errors.New("app update: application is nil")
	}
	cacheRoot, err := updateCacheRoot(dataRoot)
	if err != nil {
		return nil, err
	}
	service := &Service{app: app, updater: app.Updater, cacheRoot: cacheRoot}
	keyText := strings.TrimSpace(buildinfo.UpdatePublicKey)
	if keyText == "" || buildinfo.Version == "dev" {
		return service, nil
	}
	key, err := base64.StdEncoding.DecodeString(keyText)
	if err != nil || len(key) != 32 {
		return nil, errors.New("app update: embedded Ed25519 public key is invalid")
	}
	base := strings.TrimRight(strings.TrimSpace(buildinfo.UpdateFeedURL), "/")
	service.feedURL = fmt.Sprintf("%s/appcast-%s-%s.xml", base, runtime.GOOS, runtime.GOARCH)
	provider, err := appcast.New(appcast.Config{
		URL:        service.feedURL,
		HTTPClient: updateHTTPClient(),
	})
	if err != nil {
		return nil, err
	}
	if err := app.Updater.Init(updater.Config{
		CurrentVersion: buildinfo.Version,
		Providers:      []updater.Provider{provider},
		PublicKey:      key,
		Window:         updater.WindowNone,
	}); err != nil {
		return nil, err
	}
	service.publicKey = append([]byte(nil), key...)
	service.verified = true
	if cached, restoreErr := restoreCachedUpdate(service.cacheRoot, buildinfo.Version, service.publicKey); restoreErr == nil {
		service.cached = cached
		if cached != nil {
			release := cached.manifest.Release
			service.release = &release
		}
	} else {
		// A corrupt or incomplete cache is never allowed to become installable.
		// Remove it so the next check can offer a clean download.
		_ = removeCachedUpdate(service.cacheRoot)
	}
	return service, nil
}

func updateHTTPClient() *http.Client {
	return &http.Client{Timeout: updateDownloadTimeout}
}

func (s *Service) Close() {
	if s == nil {
		return
	}
	s.mu.Lock()
	cancel, done := s.cancel, s.done
	s.cancel, s.done = nil, nil
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
}

// Start periodically checks the signed feed without downloading anything.
// Downloads remain user-controlled, which avoids consuming bandwidth or
// prompting a restart while a long-running task is active.
func (s *Service) Start() {
	if s == nil || !s.verified {
		return
	}
	s.mu.Lock()
	if s.cancel != nil {
		s.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	s.cancel, s.done = cancel, done
	s.mu.Unlock()
	go func() {
		defer close(done)
		initial := time.NewTimer(12 * time.Second)
		defer initial.Stop()
		select {
		case <-ctx.Done():
			return
		case <-initial.C:
			_, _ = s.Check(ctx)
		}
		ticker := time.NewTicker(6 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_, _ = s.Check(ctx)
			}
		}
	}()
}

func (s *Service) Status() Status {
	status := Status{
		CurrentVersion:      buildinfo.Version,
		State:               string(updater.StateUnconfigured),
		AutomaticSupported:  automaticSupported(),
		VerificationEnabled: s != nil && s.verified,
	}
	if s == nil {
		return status
	}
	status.FeedURL = s.feedURL
	if s.verified {
		status.State = string(s.updater.State())
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	status.Error = s.lastErr
	release := s.release
	if s.cached != nil {
		status.State = string(updater.StateReady)
		status.DownloadPath = s.cached.payload
		release = &s.cached.manifest.Release
	} else if status.State == string(updater.StateReady) {
		status.DownloadPath = s.updater.DownloadedPath()
	}
	if release != nil {
		status.AvailableVersion = release.Version
		status.Name = release.Name
		status.Notes = release.Notes
		status.PublishedAt = release.PublishedAt
	}
	return status
}

func (s *Service) Check(ctx context.Context) (Status, error) {
	if err := s.requireConfigured(); err != nil {
		return s.Status(), err
	}
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	release, err := s.updater.Check(ctx)
	s.mu.Lock()
	s.release = release
	removeStaleCache := release != nil && s.cached != nil && release.Version != s.cached.manifest.Release.Version
	if removeStaleCache {
		s.cached = nil
	}
	if err != nil {
		s.lastErr = err.Error()
	} else {
		s.lastErr = ""
	}
	s.mu.Unlock()
	if removeStaleCache {
		_ = removeCachedUpdate(s.cacheRoot)
	}
	status := s.Status()
	s.app.Event.Emit(EventStatusChanged, status)
	return status, err
}

func (s *Service) Download(ctx context.Context) (Status, error) {
	if err := s.requireConfigured(); err != nil {
		return s.Status(), err
	}
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	s.mu.RLock()
	release := s.release
	s.mu.RUnlock()
	if err := validateSignedRelease(release); err != nil {
		s.rememberError(err)
		return s.Status(), err
	}
	err := s.updater.DownloadAndInstall(ctx)
	if err == nil {
		cached, cacheErr := persistCachedUpdate(s.cacheRoot, release, s.updater.DownloadedPath())
		if cacheErr != nil {
			err = cacheErr
		} else {
			s.mu.Lock()
			s.cached = cached
			s.mu.Unlock()
			discardTemporaryDownload(s.updater.DownloadedPath())
		}
	}
	s.rememberError(err)
	status := s.Status()
	s.app.Event.Emit(EventStatusChanged, status)
	return status, err
}

func (s *Service) Apply(ctx context.Context) error {
	if err := s.requireConfigured(); err != nil {
		return err
	}
	if !automaticSupported() {
		return errors.New("app update: this installation cannot update itself; download the package from the release page")
	}
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	s.mu.RLock()
	cached := s.cached
	s.mu.RUnlock()
	if cached != nil {
		restored, err := restoreCachedUpdate(s.cacheRoot, buildinfo.Version, s.publicKey)
		if err != nil || restored == nil {
			_ = removeCachedUpdate(s.cacheRoot)
			s.mu.Lock()
			s.cached = nil
			s.release = nil
			s.mu.Unlock()
			if err == nil {
				err = errors.New("app update: cached artifact is no longer newer than this application")
			}
			return err
		}
		if runtime.GOOS == "darwin" {
			return s.restartCachedDarwin(restored.payload)
		}
		return s.startUpdaterHelper(restored.payload)
	}
	if s.updater.State() != updater.StateReady {
		return errors.New("app update: no verified update is ready")
	}
	if runtime.GOOS == "darwin" {
		return s.updater.Restart(ctx)
	}
	staged := s.updater.DownloadedPath()
	if staged == "" {
		return errors.New("app update: verified artifact is missing")
	}
	return s.startUpdaterHelper(staged)
}

func (s *Service) startUpdaterHelper(staged string) error {
	mode, target := updateTarget()
	helper, err := copyUpdaterHelper()
	if err != nil {
		return err
	}
	readyFile := filepath.Join(os.TempDir(), fmt.Sprintf("onecatch-update-ready-%d", os.Getpid()))
	command := exec.Command(helper,
		"--mode", mode,
		"--parent-pid", fmt.Sprint(os.Getpid()),
		"--payload", staged,
		"--target", target,
		"--ready-file", readyFile,
	)
	command.Stdin, command.Stdout, command.Stderr = nil, nil, nil
	if err := command.Start(); err != nil {
		_ = os.Remove(helper)
		return fmt.Errorf("app update: start updater helper: %w", err)
	}
	s.app.Quit()
	return nil
}

func (s *Service) restartCachedDarwin(staged string) error {
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("app update: resolve executable: %w", err)
	}
	target := darwinBundleTarget(executable)
	if target == executable {
		return errors.New("app update: application is not running from a macOS bundle")
	}
	logPath := filepath.Join(os.TempDir(), fmt.Sprintf("wails-update-%d.log", os.Getpid()))
	command := exec.Command(executable)
	command.Stdin, command.Stdout, command.Stderr = nil, nil, nil
	command.Env = append(os.Environ(),
		"WAILS_UPDATER_HELPER=1",
		"WAILS_UPDATER_HELPER_TARGET="+target,
		"WAILS_UPDATER_HELPER_NEW="+staged,
		"WAILS_UPDATER_HELPER_PID="+fmt.Sprint(os.Getpid()),
		"WAILS_UPDATER_HELPER_LOG="+logPath,
	)
	if err := command.Start(); err != nil {
		return fmt.Errorf("app update: start macOS updater helper: %w", err)
	}
	s.app.Quit()
	return nil
}

func darwinBundleTarget(executable string) string {
	parts := strings.Split(filepath.Clean(executable), string(os.PathSeparator))
	for index, part := range parts {
		if strings.HasSuffix(part, ".app") {
			return string(os.PathSeparator) + filepath.Join(parts[1:index+1]...)
		}
	}
	return executable
}

func discardTemporaryDownload(staged string) {
	directory := filepath.Dir(filepath.Clean(staged))
	if filepath.Clean(filepath.Dir(directory)) != filepath.Clean(os.TempDir()) || !strings.HasPrefix(filepath.Base(directory), "wails-update-") {
		return
	}
	_ = os.RemoveAll(directory)
}

func (s *Service) requireConfigured() error {
	if s == nil || !s.verified {
		return errors.New("app update: disabled in development or this build has no pinned update key")
	}
	return nil
}

func (s *Service) rememberError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err == nil {
		s.lastErr = ""
	} else {
		s.lastErr = err.Error()
	}
}

func automaticSupported() bool {
	switch runtime.GOOS {
	case "darwin":
		executable, err := os.Executable()
		return err == nil && strings.Contains(executable, ".app/Contents/MacOS/")
	case "windows":
		return true
	case "linux":
		return strings.TrimSpace(os.Getenv("APPIMAGE")) != ""
	default:
		return false
	}
}

func updateTarget() (mode, target string) {
	if runtime.GOOS == "windows" {
		executable, _ := os.Executable()
		return "installer", executable
	}
	return "replace-file", strings.TrimSpace(os.Getenv("APPIMAGE"))
}

func copyUpdaterHelper() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("app update: resolve executable: %w", err)
	}
	name := "onecatch-updater"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	source := filepath.Join(filepath.Dir(executable), name)
	if runtime.GOOS == "darwin" {
		source = filepath.Join(filepath.Dir(filepath.Dir(executable)), "Resources", "bin", name)
	}
	in, err := os.Open(source)
	if err != nil {
		return "", fmt.Errorf("app update: open updater helper: %w", err)
	}
	defer in.Close()
	destination := filepath.Join(os.TempDir(), fmt.Sprintf("onecatch-updater-%d%s", os.Getpid(), filepath.Ext(name)))
	out, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o700)
	if err != nil {
		return "", fmt.Errorf("app update: create temporary helper: %w", err)
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return "", fmt.Errorf("app update: copy updater helper: %w", err)
	}
	if err := out.Close(); err != nil {
		return "", err
	}
	return destination, nil
}

// SignalReadyFromEnvironment is called only after the native webview reports
// runtime-ready. The external helper treats this as the commit point; until
// then it retains the previous installation for rollback.
func SignalReadyFromEnvironment() error {
	path := strings.TrimSpace(os.Getenv(readyEnvironment))
	if path == "" {
		return nil
	}
	if filepath.Clean(filepath.Dir(path)) != filepath.Clean(os.TempDir()) || !strings.HasPrefix(filepath.Base(path), "onecatch-update-ready-") {
		return errors.New("app update: refusing unsafe readiness path")
	}
	if err := os.WriteFile(path, []byte(buildinfo.Version+"\n"), 0o600); err != nil {
		return err
	}
	return os.Unsetenv(readyEnvironment)
}
