// Package localfile provides business-agnostic primitives for Oneshot's local
// JSON/JSONL persistence.
package localfile

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

var safeID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

func ValidID(value string) bool {
	return value != "." && value != ".." && safeID.MatchString(value)
}

func ReadJSON(path string, target any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	return nil
}

// WriteJSONAtomic writes an indented snapshot through a temporary file in the
// same directory, fsyncs it, renames it over the destination, then fsyncs the
// directory. A crash therefore exposes either the old complete snapshot or the
// new complete snapshot, never a partially written JSON document.
func WriteJSONAtomic(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encode %s: %w", path, err)
	}
	data = append(data, '\n')
	return writeAtomic(path, data)
}

func WriteTextAtomic(path, content string) error {
	return writeAtomic(path, []byte(content))
}

func writeAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create directory %s: %w", dir, err)
	}
	temporary, err := os.CreateTemp(dir, ".oneshot-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary snapshot: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	closeWithError := func(current error) error {
		if closeErr := temporary.Close(); current == nil && closeErr != nil {
			return closeErr
		}
		return current
	}
	if err := temporary.Chmod(0o600); err != nil {
		return closeWithError(fmt.Errorf("secure temporary snapshot: %w", err))
	}
	if _, err := temporary.Write(data); err != nil {
		return closeWithError(fmt.Errorf("write temporary snapshot: %w", err))
	}
	if err := temporary.Sync(); err != nil {
		return closeWithError(fmt.Errorf("sync temporary snapshot: %w", err))
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary snapshot: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("replace snapshot: %w", err)
	}
	if err := syncDirectory(dir); err != nil {
		return err
	}
	return nil
}

// WriteJSONExclusive creates a new JSON file and fails with os.ErrExist when
// the path is already present. It is used for lock acquisition.
func WriteJSONExclusive(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encode %s: %w", path, err)
	}
	data = append(data, '\n')
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create directory %s: %w", dir, err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create exclusive JSON file: %w", err)
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return fmt.Errorf("write exclusive JSON file: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return fmt.Errorf("sync exclusive JSON file: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return fmt.Errorf("close exclusive JSON file: %w", err)
	}
	return syncDirectory(dir)
}

func RemoveAndSync(path string) error {
	if err := os.Remove(path); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(path))
}

func AppendJSONLine(path string, value any) error {
	line, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode JSONL event: %w", err)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create JSONL directory: %w", err)
	}
	_, statErr := os.Stat(path)
	created := errors.Is(statErr, os.ErrNotExist)
	if statErr != nil && !created {
		return fmt.Errorf("stat JSONL stream: %w", statErr)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open JSONL stream: %w", err)
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return fmt.Errorf("secure JSONL stream: %w", err)
	}
	if _, err := file.Write(append(line, '\n')); err != nil {
		_ = file.Close()
		return fmt.Errorf("append JSONL event: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("sync JSONL stream: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close JSONL stream: %w", err)
	}
	if created {
		if err := syncDirectory(dir); err != nil {
			return err
		}
	}
	return nil
}

// TrimIncompleteJSONLTail removes a final partial line left by a crash during
// append. Complete lines always end in a newline; an invalid line that does end
// in a newline is preserved so callers can report real corruption.
func TrimIncompleteJSONLTail(path string) error {
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("open JSONL stream for recovery: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("stat JSONL stream for recovery: %w", err)
	}
	size := info.Size()
	if size == 0 {
		return nil
	}
	last := []byte{0}
	if _, err := file.ReadAt(last, size-1); err != nil {
		return fmt.Errorf("read JSONL tail: %w", err)
	}
	if last[0] == '\n' {
		return nil
	}
	truncateAt := int64(0)
	buffer := make([]byte, 4096)
	for end := size; end > 0; {
		start := end - int64(len(buffer))
		if start < 0 {
			start = 0
		}
		chunk := buffer[:end-start]
		if _, err := file.ReadAt(chunk, start); err != nil {
			return fmt.Errorf("scan JSONL tail: %w", err)
		}
		if index := bytes.LastIndexByte(chunk, '\n'); index >= 0 {
			truncateAt = start + int64(index) + 1
			break
		}
		end = start
	}
	if err := file.Truncate(truncateAt); err != nil {
		return fmt.Errorf("truncate incomplete JSONL tail: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync recovered JSONL stream: %w", err)
	}
	return nil
}

func syncDirectory(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open directory for sync: %w", err)
	}
	defer dir.Close()
	if err := dir.Sync(); err != nil && !errors.Is(err, os.ErrInvalid) {
		return fmt.Errorf("sync directory: %w", err)
	}
	return nil
}
