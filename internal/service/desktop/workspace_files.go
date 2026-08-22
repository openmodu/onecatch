package desktop

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	pathpkg "path"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	domainworkspaces "github.com/openmodu/onecatch/internal/domain/workspaces"
	"github.com/openmodu/onecatch/internal/remotefs"
)

const maxWorkspaceFileBytes = 2 << 20

type WorkspaceFileEntry struct {
	Name       string `json:"name"`
	Path       string `json:"path"`
	Directory  bool   `json:"directory"`
	Size       int64  `json:"size,omitempty"`
	ModifiedAt string `json:"modifiedAt,omitempty"`
}

type WorkspaceFileDocument struct {
	Path       string `json:"path"`
	Content    string `json:"content"`
	Hash       string `json:"hash"`
	Size       int64  `json:"size"`
	ModifiedAt string `json:"modifiedAt"`
}

type WriteWorkspaceFileInput struct {
	WorkspaceID  string `json:"workspaceId"`
	Path         string `json:"path"`
	Content      string `json:"content"`
	ExpectedHash string `json:"expectedHash,omitempty"`
}

func (a *Service) ListWorkspaceFiles(ctx context.Context, workspaceID, directory string) ([]WorkspaceFileEntry, error) {
	workspace, relative, err := a.resolveWorkspaceFile(ctx, workspaceID, directory, true)
	if err != nil {
		return nil, err
	}
	if workspace.RemoteFS != nil {
		return listRemoteWorkspaceFiles(ctx, *workspace.RemoteFS, relative)
	}
	root, err := os.OpenRoot(workspace.Path)
	if err != nil {
		return nil, mapWorkspaceFileError(err)
	}
	defer root.Close()

	dir, err := root.Open(relative)
	if err != nil {
		return nil, mapWorkspaceFileError(err)
	}
	defer dir.Close()
	info, err := dir.Stat()
	if err != nil {
		return nil, mapWorkspaceFileError(err)
	}
	if !info.IsDir() {
		return nil, coded("workspace_file_invalid_path", "path is not a directory")
	}
	items, err := dir.ReadDir(-1)
	if err != nil {
		return nil, mapWorkspaceFileError(err)
	}
	entries := make([]WorkspaceFileEntry, 0, len(items))
	for _, item := range items {
		if blockedWorkspaceFileName(item.Name()) || item.Type()&os.ModeSymlink != 0 {
			continue
		}
		itemInfo, infoErr := item.Info()
		if infoErr != nil || (!itemInfo.IsDir() && !itemInfo.Mode().IsRegular()) {
			continue
		}
		path := item.Name()
		if relative != "." {
			path = filepath.Join(relative, item.Name())
		}
		entries = append(entries, WorkspaceFileEntry{
			Name:       item.Name(),
			Path:       filepath.ToSlash(path),
			Directory:  itemInfo.IsDir(),
			Size:       itemInfo.Size(),
			ModifiedAt: itemInfo.ModTime().UTC().Format(time.RFC3339Nano),
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Directory != entries[j].Directory {
			return entries[i].Directory
		}
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})
	return entries, nil
}

func (a *Service) ReadWorkspaceFile(ctx context.Context, workspaceID, path string) (WorkspaceFileDocument, error) {
	workspace, relative, err := a.resolveWorkspaceFile(ctx, workspaceID, path, false)
	if err != nil {
		return WorkspaceFileDocument{}, err
	}
	if workspace.RemoteFS != nil {
		return readRemoteWorkspaceFile(ctx, *workspace.RemoteFS, relative)
	}
	root, err := os.OpenRoot(workspace.Path)
	if err != nil {
		return WorkspaceFileDocument{}, mapWorkspaceFileError(err)
	}
	defer root.Close()
	return readWorkspaceFile(root, relative)
}

func (a *Service) WriteWorkspaceFile(ctx context.Context, input WriteWorkspaceFileInput) (WorkspaceFileDocument, error) {
	if len(input.Content) > maxWorkspaceFileBytes {
		return WorkspaceFileDocument{}, coded("workspace_file_too_large", "file exceeds the 2 MiB editor limit")
	}
	if !utf8.ValidString(input.Content) || strings.IndexByte(input.Content, 0) >= 0 {
		return WorkspaceFileDocument{}, coded("workspace_file_not_text", "only UTF-8 text files can be edited")
	}
	workspace, relative, err := a.resolveWorkspaceFile(ctx, input.WorkspaceID, input.Path, false)
	if err != nil {
		return WorkspaceFileDocument{}, err
	}
	if workspace.RemoteFS != nil {
		return writeRemoteWorkspaceFile(ctx, *workspace.RemoteFS, relative, input.Content, input.ExpectedHash)
	}
	root, err := os.OpenRoot(workspace.Path)
	if err != nil {
		return WorkspaceFileDocument{}, mapWorkspaceFileError(err)
	}
	defer root.Close()

	current, err := readWorkspaceFile(root, relative)
	if err != nil {
		return WorkspaceFileDocument{}, err
	}
	if input.ExpectedHash != "" && input.ExpectedHash != current.Hash {
		return WorkspaceFileDocument{}, coded("workspace_file_conflict", "file changed since it was opened")
	}
	info, err := root.Stat(relative)
	if err != nil {
		return WorkspaceFileDocument{}, mapWorkspaceFileError(err)
	}
	if err := replaceWorkspaceFile(root, relative, []byte(input.Content), info.Mode().Perm()); err != nil {
		return WorkspaceFileDocument{}, mapWorkspaceFileError(err)
	}
	return readWorkspaceFile(root, relative)
}

func (a *Service) openWorkspaceRoot(ctx context.Context, workspaceID, path string, allowRoot bool) (*os.Root, string, error) {
	workspace, relative, err := a.resolveWorkspaceFile(ctx, workspaceID, path, allowRoot)
	if err != nil {
		return nil, "", err
	}
	if workspace.RemoteFS != nil {
		return nil, "", coded("workspace_file_remote", "remote workspace requires the SFTP file adapter")
	}
	root, err := os.OpenRoot(workspace.Path)
	if err != nil {
		return nil, "", mapWorkspaceFileError(err)
	}
	return root, relative, nil
}

func (a *Service) resolveWorkspaceFile(ctx context.Context, workspaceID, path string, allowRoot bool) (domainworkspaces.Workspace, string, error) {
	if err := ctx.Err(); err != nil {
		return domainworkspaces.Workspace{}, "", err
	}
	workspace, err := a.GetWorkspace(ctx, strings.TrimSpace(workspaceID))
	if err != nil {
		return domainworkspaces.Workspace{}, "", err
	}
	relative, err := normalizeWorkspaceFilePath(path, allowRoot)
	if err != nil {
		return domainworkspaces.Workspace{}, "", err
	}
	return workspace, relative, nil
}

func openRemoteWorkspace(ctx context.Context, target domainworkspaces.RemoteFS) (*remotefs.SFTPBackend, error) {
	backend, err := remotefs.NewSFTPBackend(ctx, remotefs.SFTPConfig{
		Host: target.Host, Root: target.Root, Username: target.Username, CredentialID: target.CredentialID, SSHOptions: target.SSHOptions,
	})
	if err != nil {
		return nil, coded("remote_fs_unavailable", err.Error())
	}
	return backend, nil
}

func remoteRelativePath(relative string) string {
	if relative == "." {
		return "."
	}
	return filepath.ToSlash(relative)
}

func listRemoteWorkspaceFiles(ctx context.Context, target domainworkspaces.RemoteFS, relative string) ([]WorkspaceFileEntry, error) {
	backend, err := openRemoteWorkspace(ctx, target)
	if err != nil {
		return nil, err
	}
	defer backend.Close()
	items, err := backend.ReadDir(remoteRelativePath(relative))
	if err != nil {
		return nil, mapWorkspaceFileError(err)
	}
	entries := make([]WorkspaceFileEntry, 0, len(items))
	for _, item := range items {
		if blockedWorkspaceFileName(item.Name()) || item.Mode()&os.ModeSymlink != 0 || (!item.IsDir() && !item.Mode().IsRegular()) {
			continue
		}
		itemPath := item.Name()
		if relative != "." {
			itemPath = pathpkg.Join(remoteRelativePath(relative), item.Name())
		}
		entries = append(entries, WorkspaceFileEntry{
			Name:       item.Name(),
			Path:       itemPath,
			Directory:  item.IsDir(),
			Size:       item.Size(),
			ModifiedAt: item.ModTime().UTC().Format(time.RFC3339Nano),
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Directory != entries[j].Directory {
			return entries[i].Directory
		}
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})
	return entries, nil
}

func readRemoteWorkspaceFile(ctx context.Context, target domainworkspaces.RemoteFS, relative string) (WorkspaceFileDocument, error) {
	backend, err := openRemoteWorkspace(ctx, target)
	if err != nil {
		return WorkspaceFileDocument{}, err
	}
	defer backend.Close()
	return readRemoteFile(backend, remoteRelativePath(relative))
}

func readRemoteFile(backend *remotefs.SFTPBackend, relative string) (WorkspaceFileDocument, error) {
	info, err := backend.Lstat(relative)
	if err != nil {
		return WorkspaceFileDocument{}, mapWorkspaceFileError(err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return WorkspaceFileDocument{}, coded("workspace_file_invalid_path", "symbolic links are not editable")
	}
	if !info.Mode().IsRegular() {
		return WorkspaceFileDocument{}, coded("workspace_file_invalid_path", "path is not a regular file")
	}
	if info.Size() > maxWorkspaceFileBytes {
		return WorkspaceFileDocument{}, coded("workspace_file_too_large", "file exceeds the 2 MiB editor limit")
	}
	file, err := backend.OpenFile(relative, os.O_RDONLY, 0)
	if err != nil {
		return WorkspaceFileDocument{}, mapWorkspaceFileError(err)
	}
	defer file.Close()
	data, err := io.ReadAll(io.NewSectionReader(file, 0, info.Size()))
	if err != nil {
		return WorkspaceFileDocument{}, mapWorkspaceFileError(err)
	}
	if !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
		return WorkspaceFileDocument{}, coded("workspace_file_not_text", "only UTF-8 text files can be edited")
	}
	hash := sha256.Sum256(data)
	return WorkspaceFileDocument{
		Path:       relative,
		Content:    string(data),
		Hash:       hex.EncodeToString(hash[:]),
		Size:       int64(len(data)),
		ModifiedAt: info.ModTime().UTC().Format(time.RFC3339Nano),
	}, nil
}

func writeRemoteWorkspaceFile(ctx context.Context, target domainworkspaces.RemoteFS, relative, content, expectedHash string) (WorkspaceFileDocument, error) {
	backend, err := openRemoteWorkspace(ctx, target)
	if err != nil {
		return WorkspaceFileDocument{}, err
	}
	defer backend.Close()
	relative = remoteRelativePath(relative)
	current, err := readRemoteFile(backend, relative)
	if err != nil {
		return WorkspaceFileDocument{}, err
	}
	if expectedHash != "" && expectedHash != current.Hash {
		return WorkspaceFileDocument{}, coded("workspace_file_conflict", "file changed since it was opened")
	}
	info, err := backend.Lstat(relative)
	if err != nil {
		return WorkspaceFileDocument{}, mapWorkspaceFileError(err)
	}
	random := make([]byte, 8)
	if _, err := rand.Read(random); err != nil {
		return WorkspaceFileDocument{}, fmt.Errorf("create editor temporary name: %w", err)
	}
	temporary := pathpkg.Join(pathpkg.Dir(relative), ".onecatch-edit-"+hex.EncodeToString(random)+".tmp")
	file, err := backend.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
	if err != nil {
		return WorkspaceFileDocument{}, mapWorkspaceFileError(err)
	}
	keep := false
	defer func() {
		_ = file.Close()
		if !keep {
			_ = backend.Remove(temporary)
		}
	}()
	data := []byte(content)
	for offset := 0; offset < len(data); {
		written, writeErr := file.WriteAt(data[offset:], int64(offset))
		if writeErr != nil {
			return WorkspaceFileDocument{}, mapWorkspaceFileError(writeErr)
		}
		if written == 0 {
			return WorkspaceFileDocument{}, io.ErrShortWrite
		}
		offset += written
	}
	if err := file.Sync(); err != nil {
		return WorkspaceFileDocument{}, mapWorkspaceFileError(err)
	}
	if err := file.Close(); err != nil {
		return WorkspaceFileDocument{}, mapWorkspaceFileError(err)
	}
	if err := backend.Rename(temporary, relative); err != nil {
		return WorkspaceFileDocument{}, mapWorkspaceFileError(err)
	}
	keep = true
	return readRemoteFile(backend, relative)
}

func normalizeWorkspaceFilePath(path string, allowRoot bool) (string, error) {
	if strings.IndexByte(path, 0) >= 0 || filepath.IsAbs(path) || filepath.VolumeName(path) != "" {
		return "", coded("workspace_file_invalid_path", "path must be relative to the workspace")
	}
	relative := filepath.Clean(filepath.FromSlash(path))
	if relative == "." {
		if allowRoot {
			return relative, nil
		}
		return "", coded("workspace_file_invalid_path", "select a file")
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", coded("workspace_file_invalid_path", "path leaves the workspace")
	}
	for _, part := range strings.Split(relative, string(filepath.Separator)) {
		if blockedWorkspaceFileName(part) {
			return "", coded("workspace_file_invalid_path", "workspace metadata is not editable")
		}
	}
	return relative, nil
}

func blockedWorkspaceFileName(name string) bool {
	return strings.EqualFold(name, ".git") || strings.EqualFold(name, ".onecatch")
}

func readWorkspaceFile(root *os.Root, relative string) (WorkspaceFileDocument, error) {
	linkInfo, err := root.Lstat(relative)
	if err != nil {
		return WorkspaceFileDocument{}, mapWorkspaceFileError(err)
	}
	if linkInfo.Mode()&os.ModeSymlink != 0 {
		return WorkspaceFileDocument{}, coded("workspace_file_invalid_path", "symbolic links are not editable")
	}
	file, err := root.Open(relative)
	if err != nil {
		return WorkspaceFileDocument{}, mapWorkspaceFileError(err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return WorkspaceFileDocument{}, mapWorkspaceFileError(err)
	}
	if !info.Mode().IsRegular() {
		return WorkspaceFileDocument{}, coded("workspace_file_invalid_path", "path is not a regular file")
	}
	if info.Size() > maxWorkspaceFileBytes {
		return WorkspaceFileDocument{}, coded("workspace_file_too_large", "file exceeds the 2 MiB editor limit")
	}
	data, err := io.ReadAll(io.LimitReader(file, maxWorkspaceFileBytes+1))
	if err != nil {
		return WorkspaceFileDocument{}, mapWorkspaceFileError(err)
	}
	if len(data) > maxWorkspaceFileBytes {
		return WorkspaceFileDocument{}, coded("workspace_file_too_large", "file exceeds the 2 MiB editor limit")
	}
	if !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
		return WorkspaceFileDocument{}, coded("workspace_file_not_text", "only UTF-8 text files can be edited")
	}
	hash := sha256.Sum256(data)
	return WorkspaceFileDocument{
		Path:       filepath.ToSlash(relative),
		Content:    string(data),
		Hash:       hex.EncodeToString(hash[:]),
		Size:       int64(len(data)),
		ModifiedAt: info.ModTime().UTC().Format(time.RFC3339Nano),
	}, nil
}

func replaceWorkspaceFile(root *os.Root, relative string, data []byte, mode os.FileMode) error {
	random := make([]byte, 8)
	if _, err := rand.Read(random); err != nil {
		return fmt.Errorf("create editor temporary name: %w", err)
	}
	temporary := filepath.Join(filepath.Dir(relative), ".onecatch-edit-"+hex.EncodeToString(random)+".tmp")
	file, err := root.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	keep := false
	defer func() {
		_ = file.Close()
		if !keep {
			_ = root.Remove(temporary)
		}
	}()
	if _, err := file.Write(data); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := root.Rename(temporary, relative); err != nil {
		return err
	}
	keep = true
	parent, err := root.Open(filepath.Dir(relative))
	if err != nil {
		return err
	}
	defer parent.Close()
	return parent.Sync()
}

func mapWorkspaceFileError(err error) error {
	if errors.Is(err, os.ErrNotExist) {
		return coded("workspace_file_not_found", "file was not found")
	}
	if errors.Is(err, os.ErrPermission) {
		return coded("workspace_file_read_only", "file cannot be written")
	}
	return err
}
