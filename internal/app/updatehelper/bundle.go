package updatehelper

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// replaceBundle stages beside the installed bundle, then uses atomic renames.
// Keep the old bundle until the new native window signals readiness.
func replaceBundle(payload, target, readyFile string) error {
	return replaceBundleWithTimeout(payload, target, readyFile, 45*time.Second)
}

func replaceBundleWithTimeout(payload, target, readyFile string, timeout time.Duration) error {
	staged, err := os.MkdirTemp(filepath.Dir(target), ".onecatch-update-")
	if err != nil {
		return err
	}
	preserveBackup := false
	defer func() {
		if !preserveBackup {
			_ = os.RemoveAll(staged)
		}
	}()
	var bundle string
	info, err := os.Stat(payload)
	if err != nil {
		return err
	}
	if info.IsDir() {
		bundle = filepath.Join(staged, filepath.Base(payload))
		if err := copyTree(payload, bundle); err != nil {
			return err
		}
	} else {
		if err := extractBundle(payload, staged); err != nil {
			return err
		}
		entries, err := os.ReadDir(staged)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if entry.IsDir() && strings.HasSuffix(entry.Name(), ".app") {
				if bundle != "" {
					return errors.New("update contains multiple application bundles")
				}
				bundle = filepath.Join(staged, entry.Name())
			}
		}
	}
	if bundle == "" {
		return errors.New("update contains no application bundle")
	}
	// The executable name is preserved by the native packager.
	entries, err := os.ReadDir(filepath.Join(bundle, "Contents", "MacOS"))
	if err != nil || len(entries) != 1 || entries[0].IsDir() {
		return errors.New("invalid application executable")
	}
	executable := filepath.Join(target, "Contents", "MacOS", entries[0].Name())
	backup := filepath.Join(staged, "previous.app")
	if err := os.Rename(target, backup); err != nil {
		return err
	}
	if err := os.Rename(bundle, target); err != nil {
		if restoreErr := os.Rename(backup, target); restoreErr != nil {
			preserveBackup = true
			return fmt.Errorf("activate failed (%v); backup retained at %s: %w", err, backup, restoreErr)
		}
		return err
	}
	if err := launchAndWait(executable, readyFile, timeout); err == nil {
		return nil
	}
	if err := os.RemoveAll(target); err != nil {
		preserveBackup = true
		return err
	}
	if err := os.Rename(backup, target); err != nil {
		preserveBackup = true
		return fmt.Errorf("restore previous bundle (backup at %s): %w", backup, err)
	}
	if err := launch(executable, ""); err != nil {
		return err
	}
	return errors.New("new version did not become ready; the previous application was restored")
}

func extractBundle(archive, destination string) error {
	reader, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer reader.Close()
	root, err := os.OpenRoot(destination)
	if err != nil {
		return err
	}
	defer root.Close()
	for _, entry := range reader.File {
		name := filepath.FromSlash(entry.Name)
		if !filepath.IsLocal(name) {
			return errors.New("unsafe update archive path")
		}
		if entry.FileInfo().IsDir() {
			if err := root.MkdirAll(name, 0o755); err != nil {
				return err
			}
			continue
		}
		if entry.Mode()&os.ModeSymlink != 0 {
			return errors.New("update archive contains unsupported symlink")
		}
		if !entry.Mode().IsRegular() {
			return errors.New("update archive contains special file")
		}
		if err := root.MkdirAll(filepath.Dir(name), 0o755); err != nil {
			return err
		}
		source, err := entry.Open()
		if err != nil {
			return err
		}
		target, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, entry.Mode().Perm()|0o600)
		if err != nil {
			source.Close()
			return err
		}
		_, copyErr := io.Copy(target, source)
		source.Close()
		closeErr := target.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}
