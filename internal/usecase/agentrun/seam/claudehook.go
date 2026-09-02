package seam

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/openmodu/onecatch/internal/remotefs"
)

type claudeHookEvent struct {
	HookEventName string          `json:"hook_event_name"`
	ToolName      string          `json:"tool_name"`
	ToolInput     json.RawMessage `json:"tool_input"`
}

type claudeHookReply struct {
	HookSpecificOutput *claudeHookOutput `json:"hookSpecificOutput,omitempty"`
}

type claudeHookOutput struct {
	HookEventName            string          `json:"hookEventName"`
	PermissionDecision       string          `json:"permissionDecision,omitempty"`
	PermissionDecisionReason string          `json:"permissionDecisionReason,omitempty"`
	UpdatedInput             json.RawMessage `json:"updatedInput,omitempty"`
	AdditionalContext        string          `json:"additionalContext,omitempty"`
}

type claudeMirrorFiles interface {
	Lstat(string) (os.FileInfo, error)
	OpenFile(string, int, os.FileMode) (remotefs.File, error)
	Mkdir(string, os.FileMode) error
	Remove(string) error
	Rename(string, string) error
	Close() error
}

type claudeMirrorDigester interface {
	Digest(context.Context, string) (string, error)
}

type localClaudeMirrorFiles struct{ root string }

func (f localClaudeMirrorFiles) local(name string) string {
	return filepath.Join(f.root, filepath.FromSlash(name))
}
func (f localClaudeMirrorFiles) Lstat(name string) (os.FileInfo, error) {
	return os.Lstat(f.local(name))
}
func (f localClaudeMirrorFiles) OpenFile(name string, flags int, mode os.FileMode) (remotefs.File, error) {
	return os.OpenFile(f.local(name), flags, mode)
}
func (f localClaudeMirrorFiles) Mkdir(name string, mode os.FileMode) error {
	return os.Mkdir(f.local(name), mode)
}
func (f localClaudeMirrorFiles) Remove(name string) error { return os.Remove(f.local(name)) }
func (f localClaudeMirrorFiles) Rename(oldName, newName string) error {
	return os.Rename(f.local(oldName), f.local(newName))
}
func (localClaudeMirrorFiles) Close() error { return nil }

func (f localClaudeMirrorFiles) Digest(_ context.Context, name string) (string, error) {
	data, err := os.ReadFile(f.local(name))
	if err != nil {
		return "", err
	}
	return digestBytes(data), nil
}

type sftpClaudeMirrorFiles struct {
	*remotefs.SFTPBackend
	target Target
}

// Digest hashes on the target, keeping large files off the network during the
// optimistic-concurrency check. Both common POSIX hash tools are supported;
// callers fall back to an SFTP read when neither exists.
func (f sftpClaudeMirrorFiles) Digest(ctx context.Context, name string) (string, error) {
	quoted := shellQuote(name)
	program := "if [ ! -f " + quoted + "]; then exit 44; " +
		"elif command -v sha256sum >/dev/null 2>&1; then sha256sum -- " + quoted + "; " +
		"elif command -v shasum >/dev/null 2>&1; then shasum -a 256 -- " + quoted + "; " +
		"else exit 45; fi"
	var output bytes.Buffer
	outcome, err := NewExecutor(f.target).Run(ctx, Command{
		Command: program, Dir: f.target.Root, Stdout: &output, Stderr: io.Discard,
	})
	if err != nil {
		return "", err
	}
	if outcome.ExitCode == 44 {
		return "", os.ErrNotExist
	}
	if outcome.ExitCode != 0 {
		return "", fmt.Errorf("target has no SHA-256 utility")
	}
	fields := strings.Fields(output.String())
	if len(fields) == 0 || len(fields[0]) != sha256.Size*2 {
		return "", fmt.Errorf("target returned an invalid SHA-256 digest")
	}
	if _, err := hex.DecodeString(fields[0]); err != nil {
		return "", fmt.Errorf("target returned an invalid SHA-256 digest")
	}
	return strings.ToLower(fields[0]), nil
}

type claudeMirrorBaseline struct {
	Exists bool   `json:"exists"`
	Digest string `json:"digest,omitempty"`
	Mode   uint32 `json:"mode,omitempty"`
}

// RunClaudeHook handles Claude Code's PreToolUse and PostToolUse events for
// native file tools. It materializes one requested remote file locally and
// writes it back only when the remote baseline still matches.
func RunClaudeHook(ctx context.Context, session *Session, in io.Reader, out io.Writer) error {
	var event claudeHookEvent
	if err := json.NewDecoder(io.LimitReader(in, 16<<20)).Decode(&event); err != nil {
		return json.NewEncoder(out).Encode(claudeHookReply{})
	}
	reply := claudeHookReply{}
	if event.HookEventName == "PreToolUse" && (event.ToolName == "Grep" || event.ToolName == "Glob") {
		reply = claudeHookDeny(event, "remote search must use Bash with rg, grep, or find")
		return json.NewEncoder(out).Encode(reply)
	}
	if event.ToolName != "Read" && event.ToolName != "Write" && event.ToolName != "Edit" && event.ToolName != "NotebookEdit" {
		return json.NewEncoder(out).Encode(reply)
	}
	var input map[string]any
	if json.Unmarshal(event.ToolInput, &input) != nil {
		return json.NewEncoder(out).Encode(reply)
	}
	rawPath, _ := input["file_path"].(string)
	if strings.TrimSpace(rawPath) == "" {
		return json.NewEncoder(out).Encode(reply)
	}

	files, err := openClaudeMirrorFiles(ctx, session)
	if err != nil {
		reply = claudeHookFailure(event, err)
		return json.NewEncoder(out).Encode(reply)
	}
	defer func() { _ = files.Close() }()
	mirrorRoot, err := ClaudeMirrorRoot(session.Name)
	if err != nil {
		reply = claudeHookFailure(event, err)
		return json.NewEncoder(out).Encode(reply)
	}
	operationCtx, cancel := context.WithTimeout(ctx, remoteOperationTimeout)
	defer cancel()
	mirror := claudeMirror{root: mirrorRoot, targetRoot: path.Clean(session.Target.Root), files: files}
	reply = mirror.handle(operationCtx, event, input, rawPath)
	return json.NewEncoder(out).Encode(reply)
}

func openClaudeMirrorFiles(ctx context.Context, session *Session) (claudeMirrorFiles, error) {
	if strings.TrimSpace(session.Target.Host) == "" {
		return localClaudeMirrorFiles{root: filepath.Clean(session.Target.Root)}, nil
	}
	options := append(SSHMultiplexOptions(session.Target), session.Target.SSHOptions...)
	backend, err := remotefs.NewSFTPBackend(ctx, remotefs.SFTPConfig{
		Host: session.Target.Host, Root: session.Target.Root, Username: session.Target.Username,
		CredentialID: session.Target.CredentialID, SSHBinary: session.Target.SSHBinary,
		AskPassBinary: session.Target.AskPassBinary, SSHOptions: options, Stderr: os.Stderr,
	})
	if err != nil {
		return nil, err
	}
	return sftpClaudeMirrorFiles{SFTPBackend: backend, target: session.Target}, nil
}

type claudeMirror struct {
	root       string
	targetRoot string
	files      claudeMirrorFiles
}

func (m claudeMirror) handle(ctx context.Context, event claudeHookEvent, input map[string]any, rawPath string) claudeHookReply {
	target, relative, err := m.resolve(rawPath)
	if err != nil {
		return claudeHookDeny(event, err.Error())
	}
	local := m.local(relative)
	switch event.HookEventName {
	case "PreToolUse":
		if event.ToolName == "Write" {
			err = m.prepare(ctx, relative, local)
		} else {
			err = m.fetch(ctx, relative, local)
		}
		if err != nil {
			return claudeHookDeny(event, fmt.Sprintf("could not fetch %s from the remote target: %v", target, err))
		}
		input["file_path"] = local
		updated, _ := json.Marshal(input)
		return claudeHookReply{HookSpecificOutput: &claudeHookOutput{
			HookEventName: "PreToolUse", PermissionDecision: "allow", UpdatedInput: updated,
		}}
	case "PostToolUse":
		if event.ToolName == "Read" {
			return claudeHookReply{}
		}
		if err := m.push(ctx, relative, local); err != nil {
			return claudeHookReply{HookSpecificOutput: &claudeHookOutput{
				HookEventName:     "PostToolUse",
				AdditionalContext: "OneCatch: THE CHANGE WAS NOT SAVED TO THE REMOTE TARGET. " + err.Error(),
			}}
		}
	}
	return claudeHookReply{}
}

func (m claudeMirror) resolve(raw string) (target, relative string, err error) {
	if rel, ok := m.fromLocal(raw); ok {
		return path.Join(m.targetRoot, rel), rel, nil
	}
	if path.IsAbs(raw) {
		target = path.Clean(raw)
	} else {
		target = path.Join(m.targetRoot, raw)
	}
	if target == m.targetRoot {
		return target, "", nil
	}
	prefix := strings.TrimSuffix(m.targetRoot, "/") + "/"
	if !strings.HasPrefix(target, prefix) {
		return "", "", fmt.Errorf("refusing native file access outside remote workspace %s; use Bash for remote system paths", m.targetRoot)
	}
	return target, strings.TrimPrefix(target, prefix), nil
}

func (m claudeMirror) local(relative string) string {
	return filepath.Join(m.root, filepath.FromSlash(relative))
}

func (m claudeMirror) fromLocal(value string) (string, bool) {
	relative, err := filepath.Rel(m.root, value)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", false
	}
	return filepath.ToSlash(relative), true
}

func (m claudeMirror) fetch(ctx context.Context, relative, local string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := m.validatePath(relative, false); err != nil {
		return err
	}
	info, err := m.files.Lstat(relative)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("path is a directory")
	}
	if baseline, ok := m.baseline(relative); ok && baseline.Exists {
		if localData, localErr := os.ReadFile(local); localErr == nil && digestBytes(localData) == baseline.Digest {
			if remoteDigest, remoteErr := m.digest(ctx, relative); remoteErr == nil && remoteDigest == baseline.Digest {
				return nil
			}
		}
	}
	data, mode, err := m.read(relative)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(local), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(local, data, 0o600); err != nil {
		return err
	}
	return m.record(relative, claudeMirrorBaseline{Exists: true, Digest: digestBytes(data), Mode: uint32(mode.Perm())})
}

func (m claudeMirror) prepare(ctx context.Context, relative, local string) error {
	if err := m.validatePath(relative, true); err != nil {
		return err
	}
	if _, err := m.files.Lstat(relative); err == nil {
		return m.fetch(ctx, relative, local)
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(local), 0o700); err != nil {
		return err
	}
	_ = os.Remove(local)
	return m.record(relative, claudeMirrorBaseline{Exists: false, Mode: uint32(0o644)})
}

func (m claudeMirror) push(ctx context.Context, relative, local string) error {
	baseline, ok := m.baseline(relative)
	if !ok {
		return fmt.Errorf("no verified remote baseline exists; re-read the file and retry")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := m.validatePath(relative, !baseline.Exists); err != nil {
		return err
	}
	currentDigest, err := m.digest(ctx, relative)
	switch {
	case baseline.Exists && err != nil:
		return fmt.Errorf("remote file can no longer be verified: %w", err)
	case !baseline.Exists && err == nil:
		return fmt.Errorf("refusing to overwrite: the file was created remotely after this edit began")
	case !baseline.Exists && !os.IsNotExist(err):
		return fmt.Errorf("verify new remote file: %w", err)
	case baseline.Exists && currentDigest != baseline.Digest:
		return fmt.Errorf("refusing to overwrite: the remote file changed after it was read")
	}
	data, err := os.ReadFile(local)
	if err != nil {
		return err
	}
	mode := os.FileMode(baseline.Mode)
	if mode == 0 {
		mode = 0o644
	}
	if err := m.atomicWrite(relative, data, mode); err != nil {
		return err
	}
	return m.record(relative, claudeMirrorBaseline{Exists: true, Digest: digestBytes(data), Mode: uint32(mode.Perm())})
}

// validatePath refuses symbolic links anywhere below the declared workspace.
// The SFTP backend canonicalizes paths as well, but the local target used by
// conformance runs has normal OS symlink semantics. Keeping this check here
// makes the hook's boundary independent of its transport and also avoids
// surprising edits through an in-workspace link.
func (m claudeMirror) validatePath(relative string, allowAbsent bool) error {
	if _, local := m.files.(localClaudeMirrorFiles); !local {
		// SFTPBackend canonicalizes every existing path and rejects a resolved
		// path outside its root. Avoid a separate Lstat round trip per path
		// component on high-latency links.
		return nil
	}
	current := ""
	parts := strings.Split(path.Clean(relative), "/")
	for index, part := range parts {
		if part == "." || part == "" {
			continue
		}
		current = path.Join(current, part)
		info, err := m.files.Lstat(current)
		if err != nil {
			if allowAbsent && os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing symbolic link in remote workspace path %s", current)
		}
		if index < len(parts)-1 && !info.IsDir() {
			return fmt.Errorf("%s is not a directory", current)
		}
	}
	return nil
}

func (m claudeMirror) digest(ctx context.Context, relative string) (string, error) {
	if digester, ok := m.files.(claudeMirrorDigester); ok {
		if digest, err := digester.Digest(ctx, relative); err == nil || os.IsNotExist(err) {
			return digest, err
		}
	}
	data, _, err := m.read(relative)
	if err != nil {
		return "", err
	}
	return digestBytes(data), nil
}

func (m claudeMirror) read(relative string) ([]byte, os.FileMode, error) {
	file, err := m.files.OpenFile(relative, os.O_RDONLY, 0)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return nil, 0, err
	}
	data, err := io.ReadAll(io.NewSectionReader(file, 0, info.Size()))
	return data, info.Mode(), err
}

func (m claudeMirror) atomicWrite(relative string, data []byte, mode os.FileMode) error {
	if err := m.ensureParent(path.Dir(relative)); err != nil {
		return err
	}
	temporary := path.Join(path.Dir(relative), ".onecatch-mirror-"+randomHex(8))
	file, err := m.files.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	remove := true
	defer func() {
		_ = file.Close()
		if remove {
			_ = m.files.Remove(temporary)
		}
	}()
	for offset := 0; offset < len(data); {
		n, writeErr := file.WriteAt(data[offset:], int64(offset))
		offset += n
		if writeErr != nil {
			return writeErr
		}
		if n == 0 {
			return io.ErrShortWrite
		}
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := m.files.Rename(temporary, relative); err != nil {
		return err
	}
	remove = false
	return nil
}

func (m claudeMirror) ensureParent(relative string) error {
	if relative == "." || relative == "" {
		return nil
	}
	current := ""
	for _, part := range strings.Split(relative, "/") {
		current = path.Join(current, part)
		info, err := m.files.Lstat(current)
		if err == nil {
			if !info.IsDir() {
				return fmt.Errorf("%s is not a directory", current)
			}
			continue
		}
		if !os.IsNotExist(err) {
			return err
		}
		if err := m.files.Mkdir(current, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func (m claudeMirror) recordPath(relative string) string {
	sum := sha256.Sum256([]byte(relative))
	return filepath.Join(m.root, ".baselines", hex.EncodeToString(sum[:])+".json")
}

func (m claudeMirror) record(relative string, baseline claudeMirrorBaseline) error {
	directory := filepath.Join(m.root, ".baselines")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	data, _ := json.Marshal(baseline)
	temporary, err := os.CreateTemp(directory, ".baseline-")
	if err != nil {
		return err
	}
	name := temporary.Name()
	defer func() { _ = os.Remove(name) }()
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(name, m.recordPath(relative))
}

func (m claudeMirror) baseline(relative string) (claudeMirrorBaseline, bool) {
	data, err := os.ReadFile(m.recordPath(relative))
	if err != nil {
		return claudeMirrorBaseline{}, false
	}
	var baseline claudeMirrorBaseline
	if json.Unmarshal(data, &baseline) != nil {
		return claudeMirrorBaseline{}, false
	}
	return baseline, true
}

func digestBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func ClaudeMirrorRoot(sessionName string) (string, error) {
	if !sessionNameRE.MatchString(sessionName) {
		return "", fmt.Errorf("invalid mirror session name")
	}
	directory, err := SessionDir()
	if err != nil {
		return "", err
	}
	root := filepath.Join(directory, "claude-mirror", sessionName)
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", err
	}
	return root, nil
}

// RemoveClaudeMirror removes exactly one session's private mirror without
// creating it as a side effect when no native file tool was used.
func RemoveClaudeMirror(sessionName string) error {
	if !sessionNameRE.MatchString(sessionName) {
		return fmt.Errorf("invalid mirror session name")
	}
	directory, err := SessionDir()
	if err != nil {
		return err
	}
	return os.RemoveAll(filepath.Join(directory, "claude-mirror", sessionName))
}

func claudeHookDeny(event claudeHookEvent, reason string) claudeHookReply {
	return claudeHookReply{HookSpecificOutput: &claudeHookOutput{
		HookEventName: event.HookEventName, PermissionDecision: "deny", PermissionDecisionReason: "OneCatch: " + reason,
	}}
}

func claudeHookFailure(event claudeHookEvent, err error) claudeHookReply {
	if event.HookEventName == "PostToolUse" {
		return claudeHookReply{HookSpecificOutput: &claudeHookOutput{
			HookEventName: "PostToolUse", AdditionalContext: "OneCatch: THE CHANGE WAS NOT SAVED TO THE REMOTE TARGET. " + err.Error(),
		}}
	}
	return claudeHookDeny(event, err.Error())
}
