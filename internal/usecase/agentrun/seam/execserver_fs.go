package seam

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/openmodu/onecatch/internal/remotefs"
)

type targetFiles interface {
	Lstat(string) (os.FileInfo, error)
	ReadDir(string) ([]os.FileInfo, error)
	ReadFile(string) ([]byte, error)
	OpenRead(string) (targetReadFile, error)
	WriteFile(string, []byte, os.FileMode) error
	MkdirAll(string, os.FileMode) error
	Remove(string, bool) error
	Copy(string, string, bool) error
	Canonicalize(string) (string, error)
	Close() error
}

type targetReadFile interface {
	io.ReaderAt
	io.Closer
}

type targetReadHandle struct {
	path string
	file targetReadFile
}

type localTargetFiles struct{}

func (localTargetFiles) Lstat(name string) (os.FileInfo, error) { return os.Lstat(name) }
func (localTargetFiles) ReadDir(name string) ([]os.FileInfo, error) {
	entries, err := os.ReadDir(name)
	if err != nil {
		return nil, err
	}
	infos := make([]os.FileInfo, 0, len(entries))
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		infos = append(infos, info)
	}
	return infos, nil
}
func (localTargetFiles) ReadFile(name string) ([]byte, error)         { return os.ReadFile(name) }
func (localTargetFiles) OpenRead(name string) (targetReadFile, error) { return os.Open(name) }
func (localTargetFiles) WriteFile(name string, data []byte, mode os.FileMode) error {
	return os.WriteFile(name, data, mode)
}
func (localTargetFiles) MkdirAll(name string, mode os.FileMode) error {
	return os.MkdirAll(name, mode)
}
func (localTargetFiles) Remove(name string, recursive bool) error {
	if recursive {
		return os.RemoveAll(name)
	}
	return os.Remove(name)
}
func (localTargetFiles) Copy(source, destination string, recursive bool) error {
	return copyLocalPath(source, destination, recursive)
}
func (localTargetFiles) Canonicalize(name string) (string, error) {
	return filepath.EvalSymlinks(name)
}
func (localTargetFiles) Close() error { return nil }

type remoteCanonicalizer interface {
	RealPath(string) (string, error)
}

type sftpTargetFiles struct {
	backend       remotefs.Backend
	canonicalizer remoteCanonicalizer
}

func sftpRelative(name string) (string, error) {
	if !path.IsAbs(name) {
		return "", fmt.Errorf("target path %q is not absolute", name)
	}
	return strings.TrimPrefix(path.Clean(name), "/"), nil
}

func (s *sftpTargetFiles) Lstat(name string) (os.FileInfo, error) {
	relative, err := sftpRelative(name)
	if err != nil {
		return nil, err
	}
	return s.backend.Lstat(relative)
}
func (s *sftpTargetFiles) ReadDir(name string) ([]os.FileInfo, error) {
	relative, err := sftpRelative(name)
	if err != nil {
		return nil, err
	}
	return s.backend.ReadDir(relative)
}
func (s *sftpTargetFiles) ReadFile(name string) ([]byte, error) {
	file, err := s.OpenRead(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return io.ReadAll(io.NewSectionReader(file, 0, 1<<63-1))
}
func (s *sftpTargetFiles) OpenRead(name string) (targetReadFile, error) {
	relative, err := sftpRelative(name)
	if err != nil {
		return nil, err
	}
	return s.backend.OpenFile(relative, os.O_RDONLY, 0)
}
func (s *sftpTargetFiles) WriteFile(name string, data []byte, mode os.FileMode) error {
	relative, err := sftpRelative(name)
	if err != nil {
		return err
	}
	file, err := s.backend.OpenFile(relative, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := file.WriteAt(data, 0); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}
func (s *sftpTargetFiles) MkdirAll(name string, mode os.FileMode) error {
	relative, err := sftpRelative(name)
	if err != nil {
		return err
	}
	if relative == "" {
		return nil
	}
	current := ""
	for _, part := range strings.Split(relative, "/") {
		current = path.Join(current, part)
		if err := s.backend.Mkdir(current, mode); err != nil {
			if info, statErr := s.backend.Lstat(current); statErr == nil && info.IsDir() {
				continue
			}
			return err
		}
	}
	return nil
}
func (s *sftpTargetFiles) Remove(name string, recursive bool) error {
	relative, err := sftpRelative(name)
	if err != nil {
		return err
	}
	if relative == "" {
		return fmt.Errorf("refusing to remove the target filesystem root")
	}
	info, err := s.backend.Lstat(relative)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return s.backend.Remove(relative)
	}
	if !recursive {
		return s.backend.RemoveDirectory(relative)
	}
	entries, err := s.backend.ReadDir(relative)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := s.Remove("/"+path.Join(relative, entry.Name()), true); err != nil {
			return err
		}
	}
	return s.backend.RemoveDirectory(relative)
}
func (s *sftpTargetFiles) Copy(source, destination string, recursive bool) error {
	info, err := s.Lstat(source)
	if err != nil {
		return err
	}
	if info.IsDir() {
		if !recursive {
			return fmt.Errorf("%s is a directory", source)
		}
		if err := s.MkdirAll(destination, info.Mode().Perm()); err != nil {
			return err
		}
		entries, err := s.ReadDir(source)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if err := s.Copy(path.Join(source, entry.Name()), path.Join(destination, entry.Name()), true); err != nil {
				return err
			}
		}
		return nil
	}
	data, err := s.ReadFile(source)
	if err != nil {
		return err
	}
	return s.WriteFile(destination, data, info.Mode().Perm())
}
func (s *sftpTargetFiles) Canonicalize(name string) (string, error) {
	relative, err := sftpRelative(name)
	if err != nil {
		return "", err
	}
	return s.canonicalizer.RealPath(relative)
}
func (s *sftpTargetFiles) Close() error { return s.backend.Close() }

func readAt(reader io.ReaderAt, offset, length int64) ([]byte, error) {
	if offset < 0 || length < 0 {
		return nil, fmt.Errorf("negative read offset or length")
	}
	if length == 0 {
		return []byte{}, nil
	}
	data := make([]byte, length)
	read, err := reader.ReadAt(data, offset)
	if errors.Is(err, io.EOF) {
		err = nil
	}
	return data[:read], err
}

func copyLocalPath(source, destination string, recursive bool) error {
	info, err := os.Lstat(source)
	if err != nil {
		return err
	}
	if info.IsDir() {
		if !recursive {
			return fmt.Errorf("%s is a directory", source)
		}
		if err := os.MkdirAll(destination, info.Mode().Perm()); err != nil {
			return err
		}
		entries, err := os.ReadDir(source)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if err := copyLocalPath(filepath.Join(source, entry.Name()), filepath.Join(destination, entry.Name()), true); err != nil {
				return err
			}
		}
		return nil
	}
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	return os.WriteFile(destination, data, info.Mode().Perm())
}

type fsPathParams struct {
	Path string `json:"path"`
}

func (s *ExecServer) fsTarget(raw json.RawMessage, method string) (string, *rpcError) {
	var params fsPathParams
	if err := json.Unmarshal(raw, &params); err != nil {
		return "", invalidParams("%s: %v", method, err)
	}
	return s.mapURI(params.Path)
}

func (s *ExecServer) handleFSReadFile(ctx context.Context, raw json.RawMessage) (any, *rpcError) {
	target, rpcErr := s.fsTarget(raw, "fs/readFile")
	if rpcErr != nil {
		return nil, rpcErr
	}
	operationCtx, cancel := s.operationContext(ctx)
	defer cancel()
	if err := operationCtx.Err(); err != nil {
		return nil, internalError("%v", err)
	}
	data, err := s.files.ReadFile(target)
	if err != nil {
		return nil, internalError("read %s: %v", target, err)
	}
	return map[string]any{"dataBase64": base64.StdEncoding.EncodeToString(data)}, nil
}

func (s *ExecServer) handleFSWriteFile(ctx context.Context, raw json.RawMessage) (any, *rpcError) {
	var params struct {
		Path string `json:"path"`
		Data string `json:"dataBase64"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, invalidParams("fs/writeFile: %v", err)
	}
	target, rpcErr := s.mapURI(params.Path)
	if rpcErr != nil {
		return nil, rpcErr
	}
	data, err := base64.StdEncoding.DecodeString(params.Data)
	if err != nil {
		return nil, invalidParams("fs/writeFile: invalid base64: %v", err)
	}
	operationCtx, cancel := s.operationContext(ctx)
	defer cancel()
	if err := operationCtx.Err(); err != nil {
		return nil, internalError("%v", err)
	}
	if err := s.files.WriteFile(target, data, 0o644); err != nil {
		return nil, internalError("write %s: %v", target, err)
	}
	return map[string]any{}, nil
}

func (s *ExecServer) handleFSCreateDirectory(ctx context.Context, raw json.RawMessage) (any, *rpcError) {
	target, rpcErr := s.fsTarget(raw, "fs/createDirectory")
	if rpcErr != nil {
		return nil, rpcErr
	}
	operationCtx, cancel := s.operationContext(ctx)
	defer cancel()
	if err := operationCtx.Err(); err != nil {
		return nil, internalError("%v", err)
	}
	if err := s.files.MkdirAll(target, 0o755); err != nil {
		return nil, internalError("mkdir %s: %v", target, err)
	}
	return map[string]any{}, nil
}

func (s *ExecServer) handleFSGetMetadata(ctx context.Context, raw json.RawMessage) (any, *rpcError) {
	target, rpcErr := s.fsTarget(raw, "fs/getMetadata")
	if rpcErr != nil {
		return nil, rpcErr
	}
	operationCtx, cancel := s.operationContext(ctx)
	defer cancel()
	if err := operationCtx.Err(); err != nil {
		return nil, internalError("%v", err)
	}
	info, err := s.files.Lstat(target)
	if err != nil {
		return nil, internalError("stat %s: %v", target, err)
	}
	return map[string]any{
		"isDirectory": info.IsDir(), "isFile": info.Mode().IsRegular(),
		"isSymlink": info.Mode()&os.ModeSymlink != 0, "size": uint64(max(info.Size(), 0)),
		"createdAtMs": 0, "modifiedAtMs": info.ModTime().UnixMilli(),
	}, nil
}

func (s *ExecServer) handleFSCanonicalize(ctx context.Context, raw json.RawMessage) (any, *rpcError) {
	target, rpcErr := s.fsTarget(raw, "fs/canonicalize")
	if rpcErr != nil {
		return nil, rpcErr
	}
	operationCtx, cancel := s.operationContext(ctx)
	defer cancel()
	if err := operationCtx.Err(); err != nil {
		return nil, internalError("%v", err)
	}
	canonical, err := s.files.Canonicalize(target)
	if err != nil {
		return nil, internalError("canonicalize %s: %v", target, err)
	}
	return map[string]any{"path": pathToURI(canonical)}, nil
}

func (s *ExecServer) handleFSReadDirectory(ctx context.Context, raw json.RawMessage) (any, *rpcError) {
	target, rpcErr := s.fsTarget(raw, "fs/readDirectory")
	if rpcErr != nil {
		return nil, rpcErr
	}
	operationCtx, cancel := s.operationContext(ctx)
	defer cancel()
	if err := operationCtx.Err(); err != nil {
		return nil, internalError("%v", err)
	}
	infos, err := s.files.ReadDir(target)
	if err != nil {
		return nil, internalError("list %s: %v", target, err)
	}
	entries := make([]map[string]any, 0, len(infos))
	for _, info := range infos {
		entries = append(entries, map[string]any{"fileName": info.Name(), "isDirectory": info.IsDir(), "isFile": info.Mode().IsRegular()})
	}
	return map[string]any{"entries": entries}, nil
}

type fsWalkOptions struct {
	MaxDepth                int  `json:"maxDepth"`
	MaxDirectories          int  `json:"maxDirectories"`
	MaxEntries              int  `json:"maxEntries"`
	FollowDirectorySymlinks bool `json:"followDirectorySymlinks"`
	PruneHiddenDirectories  bool `json:"pruneHiddenDirectories"`
}
type fsWalkParams struct {
	Path    string        `json:"path"`
	Options fsWalkOptions `json:"options"`
}
type fsWalker struct {
	server      *ExecServer
	options     fsWalkOptions
	entries     []map[string]any
	errs        []map[string]any
	directories int
	examined    int
	truncated   bool
}

func (s *ExecServer) handleFSWalk(ctx context.Context, raw json.RawMessage) (any, *rpcError) {
	var params fsWalkParams
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, invalidParams("fs/walk: %v", err)
	}
	root, rpcErr := s.mapURI(params.Path)
	if rpcErr != nil {
		return nil, rpcErr
	}
	operationCtx, cancel := s.operationContext(ctx)
	defer cancel()
	walker := &fsWalker{server: s, options: params.Options, entries: []map[string]any{}, errs: []map[string]any{}, directories: 1}
	walker.walk(operationCtx, root, 0)
	return map[string]any{"entries": walker.entries, "errors": walker.errs, "truncated": walker.truncated}, nil
}
func (w *fsWalker) walk(ctx context.Context, directory string, depth int) {
	if w.truncated || ctx.Err() != nil || (w.options.MaxDepth > 0 && depth >= w.options.MaxDepth) {
		return
	}
	infos, err := w.server.files.ReadDir(directory)
	if err != nil {
		w.errs = append(w.errs, map[string]any{"path": pathToURI(directory), "message": err.Error()})
		return
	}
	for _, info := range infos {
		if w.options.MaxEntries > 0 && w.examined >= w.options.MaxEntries {
			w.truncated = true
			return
		}
		w.examined++
		full := path.Join(directory, info.Name())
		isDirectory := info.IsDir()
		kind := "file"
		if isDirectory {
			kind = "directory"
		}
		w.entries = append(w.entries, map[string]any{"path": pathToURI(full), "kind": kind})
		if !isDirectory || (w.options.PruneHiddenDirectories && strings.HasPrefix(info.Name(), ".")) {
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 && !w.options.FollowDirectorySymlinks {
			continue
		}
		if w.options.MaxDirectories > 0 && w.directories >= w.options.MaxDirectories {
			w.truncated = true
			return
		}
		w.directories++
		w.walk(ctx, full, depth+1)
		if w.truncated {
			return
		}
	}
}

func (s *ExecServer) handleFSRemove(ctx context.Context, raw json.RawMessage) (any, *rpcError) {
	var params struct {
		Path      string `json:"path"`
		Recursive bool   `json:"recursive"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, invalidParams("fs/remove: %v", err)
	}
	target, rpcErr := s.mapURI(params.Path)
	if rpcErr != nil {
		return nil, rpcErr
	}
	operationCtx, cancel := s.operationContext(ctx)
	defer cancel()
	if err := operationCtx.Err(); err != nil {
		return nil, internalError("%v", err)
	}
	if err := s.files.Remove(target, params.Recursive); err != nil {
		return nil, internalError("remove %s: %v", target, err)
	}
	return map[string]any{}, nil
}

func (s *ExecServer) handleFSCopy(ctx context.Context, raw json.RawMessage) (any, *rpcError) {
	var params struct {
		Source      string `json:"sourcePath"`
		Destination string `json:"destinationPath"`
		Recursive   bool   `json:"recursive"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, invalidParams("fs/copy: %v", err)
	}
	source, rpcErr := s.mapURI(params.Source)
	if rpcErr != nil {
		return nil, rpcErr
	}
	destination, rpcErr := s.mapURI(params.Destination)
	if rpcErr != nil {
		return nil, rpcErr
	}
	operationCtx, cancel := s.operationContext(ctx)
	defer cancel()
	if err := operationCtx.Err(); err != nil {
		return nil, internalError("%v", err)
	}
	if err := s.files.Copy(source, destination, params.Recursive); err != nil {
		return nil, internalError("copy %s to %s: %v", source, destination, err)
	}
	return map[string]any{}, nil
}

func (s *ExecServer) handleFSOpen(ctx context.Context, raw json.RawMessage) (any, *rpcError) {
	var params struct {
		HandleID string `json:"handleId"`
		Path     string `json:"path"`
	}
	if err := json.Unmarshal(raw, &params); err != nil || params.HandleID == "" {
		return nil, invalidParams("fs/open: handleId and path are required")
	}
	target, rpcErr := s.mapURI(params.Path)
	if rpcErr != nil {
		return nil, rpcErr
	}
	operationCtx, cancel := s.operationContext(ctx)
	defer cancel()
	if err := operationCtx.Err(); err != nil {
		return nil, internalError("%v", err)
	}
	file, err := s.files.OpenRead(target)
	if err != nil {
		return nil, internalError("open %s: %v", target, err)
	}
	s.mu.Lock()
	if _, exists := s.handles[params.HandleID]; exists {
		s.mu.Unlock()
		_ = file.Close()
		return nil, invalidParams("fs/open: handleId %q is already in use", params.HandleID)
	}
	s.handles[params.HandleID] = &targetReadHandle{path: target, file: file}
	s.mu.Unlock()
	return map[string]any{"handleId": params.HandleID}, nil
}

func (s *ExecServer) handleFSReadBlock(ctx context.Context, raw json.RawMessage) (any, *rpcError) {
	var params struct {
		HandleID string `json:"handleId"`
		Offset   uint64 `json:"offset"`
		Length   int64  `json:"len"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, invalidParams("fs/readBlock: %v", err)
	}
	s.mu.Lock()
	handle, exists := s.handles[params.HandleID]
	s.mu.Unlock()
	if !exists {
		return nil, invalidRequest("fs/readBlock: unknown handle %q", params.HandleID)
	}
	operationCtx, cancel := s.operationContext(ctx)
	defer cancel()
	if err := operationCtx.Err(); err != nil {
		return nil, internalError("%v", err)
	}
	data, err := readAt(handle.file, int64(params.Offset), params.Length)
	if err != nil {
		return nil, internalError("read %s: %v", handle.path, err)
	}
	return map[string]any{"chunk": base64.StdEncoding.EncodeToString(data), "eof": int64(len(data)) < params.Length}, nil
}

func (s *ExecServer) handleFSClose(raw json.RawMessage) (any, *rpcError) {
	var params struct {
		HandleID string `json:"handleId"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, invalidParams("fs/close: %v", err)
	}
	s.mu.Lock()
	handle := s.handles[params.HandleID]
	delete(s.handles, params.HandleID)
	s.mu.Unlock()
	if handle != nil {
		if err := handle.file.Close(); err != nil {
			return nil, internalError("close %s: %v", handle.path, err)
		}
	}
	return map[string]any{}, nil
}

func (s *ExecServer) handleCapabilityDiscover(raw json.RawMessage) (any, *rpcError) {
	var params struct {
		Roots []struct {
			ID   string `json:"id"`
			Path string `json:"path"`
		} `json:"roots"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, invalidParams("capabilityRoots/discoverV1: %v", err)
	}
	roots := make([]map[string]any, 0, len(params.Roots))
	for _, root := range params.Roots {
		roots = append(roots, map[string]any{"id": root.ID, "path": root.Path, "plugin": nil, "skills": []any{}, "namespaceManifests": []any{}, "warnings": []any{}, "error": nil})
	}
	return map[string]any{"roots": roots}, nil
}
