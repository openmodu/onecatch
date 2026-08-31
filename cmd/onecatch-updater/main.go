// onecatch-updater is deliberately a small, UI-free process. The desktop app
// copies it to a temporary path before exiting, so it can replace the complete
// installed unit without trying to overwrite the updater that is doing the
// work.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const readyEnvironment = "ONECATCH_UPDATE_READY_FILE"

func main() {
	mode := flag.String("mode", "", "installer or replace-file")
	parentPID := flag.Int("parent-pid", 0, "desktop process to wait for")
	payload := flag.String("payload", "", "verified update artifact")
	target := flag.String("target", "", "installed executable or AppImage")
	readyFile := flag.String("ready-file", "", "new-process readiness marker")
	flag.Parse()

	if *mode == "" || *payload == "" || *target == "" || *parentPID <= 0 || *readyFile == "" {
		fatal(errors.New("missing required updater arguments"))
	}
	if err := waitForExit(*parentPID, 45*time.Second); err != nil {
		fatal(err)
	}
	defer cleanupPayload(*payload)

	var err error
	switch *mode {
	case "installer":
		err = applyInstaller(*payload, *target, *readyFile)
	case "replace-file":
		err = replaceFile(*payload, *target, *readyFile)
	default:
		err = fmt.Errorf("unsupported updater mode %q", *mode)
	}
	if err != nil {
		fatal(err)
	}
}

func cleanupPayload(payload string) {
	directory := filepath.Dir(payload)
	if filepath.Clean(filepath.Dir(directory)) != filepath.Clean(os.TempDir()) || !strings.HasPrefix(filepath.Base(directory), "wails-update-") {
		return
	}
	_ = os.RemoveAll(directory)
}

func applyInstaller(installer, executable, readyFile string) error {
	installDir := filepath.Dir(executable)
	backup, err := os.MkdirTemp("", "onecatch-update-backup-*")
	if err != nil {
		return fmt.Errorf("create rollback directory: %w", err)
	}
	defer os.RemoveAll(backup)
	backupInstall := filepath.Join(backup, filepath.Base(installDir))
	if err := copyTree(installDir, backupInstall); err != nil {
		return fmt.Errorf("backup installation: %w", err)
	}

	cmd := exec.Command(installer, "/S")
	if output, err := cmd.CombinedOutput(); err != nil {
		_ = os.RemoveAll(installDir)
		if restoreErr := copyTree(backupInstall, installDir); restoreErr != nil {
			return fmt.Errorf("installer failed (%v: %s) and rollback failed: %w", err, output, restoreErr)
		}
		if launchErr := launch(executable, ""); launchErr != nil {
			return fmt.Errorf("installer failed (%v: %s); rollback succeeded but relaunch failed: %w", err, output, launchErr)
		}
		return fmt.Errorf("installer failed and the previous installation was restored: %w: %s", err, output)
	}
	if err := launchAndWait(executable, readyFile, 45*time.Second); err == nil {
		return nil
	}

	_ = os.RemoveAll(installDir)
	if err := copyTree(backupInstall, installDir); err != nil {
		return fmt.Errorf("new version did not become ready and rollback failed: %w", err)
	}
	if err := launch(executable, ""); err != nil {
		return fmt.Errorf("relaunch rolled-back version: %w", err)
	}
	return errors.New("new version did not become ready; the previous installation was restored")
}

func replaceFile(payload, target, readyFile string) error {
	info, err := os.Stat(target)
	if err != nil {
		return fmt.Errorf("inspect installed AppImage: %w", err)
	}
	newPath := target + ".new"
	backup := target + ".old"
	_ = os.Remove(newPath)
	_ = os.Remove(backup)
	if err := copyFile(payload, newPath, info.Mode().Perm()|0o111); err != nil {
		return fmt.Errorf("stage replacement: %w", err)
	}
	if err := os.Rename(target, backup); err != nil {
		return fmt.Errorf("backup installed AppImage: %w", err)
	}
	if err := os.Rename(newPath, target); err != nil {
		_ = os.Rename(backup, target)
		return fmt.Errorf("activate AppImage: %w", err)
	}
	if err := launchAndWait(target, readyFile, 45*time.Second); err == nil {
		return os.Remove(backup)
	}
	_ = os.Remove(target)
	if err := os.Rename(backup, target); err != nil {
		return fmt.Errorf("new version did not become ready and rollback failed: %w", err)
	}
	if err := launch(target, ""); err != nil {
		return fmt.Errorf("relaunch rolled-back version: %w", err)
	}
	return errors.New("new version did not become ready; the previous AppImage was restored")
}

func launchAndWait(executable, readyFile string, timeout time.Duration) error {
	_ = os.Remove(readyFile)
	process, err := start(executable, readyFile)
	if err != nil {
		return err
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(readyFile); err == nil {
			_ = os.Remove(readyFile)
			_ = process.Release()
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	_ = process.Kill()
	_, _ = process.Wait()
	return errors.New("timed out waiting for the updated application")
}

func launch(executable, readyFile string) error {
	process, err := start(executable, readyFile)
	if err != nil {
		return err
	}
	return process.Release()
}

func start(executable, readyFile string) (*os.Process, error) {
	cmd := exec.Command(executable)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, nil, nil
	cmd.Env = os.Environ()
	if readyFile != "" {
		cmd.Env = append(cmd.Env, readyEnvironment+"="+readyFile)
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd.Process, nil
}

func waitForExit(pid int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !processAlive(pid) {
			return nil
		}
		time.Sleep(150 * time.Millisecond)
	}
	return fmt.Errorf("desktop process %d did not exit within %s", pid, timeout)
}

func copyTree(source, destination string) error {
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm())
		}
		if entry.Type()&os.ModeSymlink != 0 {
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		}
		return copyFile(path, target, info.Mode().Perm())
	})
}

func copyFile(source, destination string, mode fs.FileMode) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Sync(); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

func fatal(err error) {
	logPath := filepath.Join(os.TempDir(), "onecatch-updater-"+strconv.Itoa(os.Getpid())+".log")
	_ = os.WriteFile(logPath, []byte(time.Now().Format(time.RFC3339)+" "+err.Error()+"\n"), 0o600)
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
