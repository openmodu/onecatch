package worker

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/openmodu/onecatch/internal/usecase/agentrun"
	"github.com/openmodu/onecatch/pkg/localfile"
)

//go:embed sandboxfiles/helper.py
var sandboxFileHelper string

const syncFileLimit = 64 << 20
const syncChunkBytes = 192 << 10

// These exclusions apply even to tracked files. .gitignore supplies the
// project-specific exclusions; symlinks and special files never cross hosts.
var syncBlocked = []string{".git", ".onecatch", ".codex", ".claude", ".config", ".docker", ".ssh", ".aws", ".azure", ".gnupg", ".kube", ".npmrc", ".pypirc", ".netrc", "credentials", "credentials.json", "id_rsa", "id_ed25519", "node_modules", "vendor", ".venv", "venv", "__pycache__", ".next", ".cache", "dist", "build", "target", ".ds_store"}

type syncFile struct {
	Hash       string `json:"hash"`
	Size       int64  `json:"size"`
	Executable bool   `json:"executable"`
}
type syncManifest map[string]syncFile
type syncState struct {
	Generation string       `json:"generation"`
	Files      syncManifest `json:"files"`
	// Save owned shell IDs before any model turn so a restarted app can stop
	// orphaned processes before recovering their file changes.
	Sessions []string `json:"sessions,omitempty"`
}
type syncReply struct {
	Generation string       `json:"generation"`
	Error      string       `json:"error"`
	Files      syncManifest `json:"files"`
	Data       []byte       `json:"data"`
}
type sandboxFiles struct {
	stream io.ReadWriteCloser
	enc    *json.Encoder
	dec    *json.Decoder
}

func newSandboxFiles(stream io.ReadWriteCloser) *sandboxFiles {
	return &sandboxFiles{stream, json.NewEncoder(stream), json.NewDecoder(stream)}
}
func (f *sandboxFiles) call(request any) (syncReply, error) {
	if stream, ok := f.stream.(*sandboxStream); ok {
		_ = stream.conn.SetReadDeadline(time.Now().Add(5 * time.Minute))
	}
	if err := f.enc.Encode(request); err != nil {
		return syncReply{}, errors.New("sandbox file connection write failed")
	}
	var response syncReply
	if err := f.dec.Decode(&response); err != nil {
		return response, errors.New("sandbox file connection interrupted")
	}
	if response.Error != "" {
		return response, fmt.Errorf("sandbox sync: %s", response.Error)
	}
	return response, nil
}
func (f *sandboxFiles) manifest(base syncManifest) (syncManifest, error) {
	tracked := sortedSyncPaths(base)
	r, err := f.call(map[string]any{"op": "manifest", "tracked": tracked})
	if err != nil {
		return nil, err
	}
	if len(r.Files) > 100000 {
		return nil, errors.New("sandbox sync exceeds 100,000 files")
	}
	for name, item := range r.Files {
		if !syncAllowed(name) || item.Size < 0 || item.Size > syncFileLimit || len(item.Hash) != 64 {
			return nil, errors.New("invalid sandbox file manifest")
		}
		if _, err := hex.DecodeString(item.Hash); err != nil {
			return nil, errors.New("invalid sandbox file digest")
		}
	}
	return r.Files, nil
}
func (f *sandboxFiles) download(name string, expected syncFile) ([]byte, error) {
	data := make([]byte, 0, expected.Size)
	for int64(len(data)) < expected.Size {
		r, err := f.call(map[string]any{"op": "get", "path": name, "offset": len(data)})
		if err != nil {
			return nil, err
		}
		if len(r.Data) == 0 || len(r.Data) > syncChunkBytes || int64(len(data)+len(r.Data)) > expected.Size {
			return nil, errors.New("sandbox file changed during download")
		}
		data = append(data, r.Data...)
	}
	if digestSync(data) != expected.Hash {
		return nil, errors.New("sandbox file failed integrity check")
	}
	return data, nil
}
func (f *sandboxFiles) upload(name string, data []byte, file syncFile, before *syncFile) error {
	if _, err := f.call(map[string]any{"op": "begin", "path": name, "file": file, "before": before}); err != nil {
		return err
	}
	for start := 0; start < len(data); start += syncChunkBytes {
		if _, err := f.call(map[string]any{"op": "chunk", "path": name, "data": data[start:min(start+syncChunkBytes, len(data))]}); err != nil {
			return err
		}
	}
	_, err := f.call(map[string]any{"op": "finish", "path": name})
	return err
}
func syncAllowed(name string) bool {
	if name == "" || !utf8.ValidString(name) || !filepath.IsLocal(name) || strings.ContainsAny(name, "\\\x00\r\n") || path.Clean(name) != name {
		return false
	}
	for _, part := range strings.Split(name, "/") {
		low := strings.ToLower(part)
		if strings.HasPrefix(low, ".env") || strings.HasPrefix(low, ".onecatch-") || part == ".." || part == "." {
			return false
		}
		for _, blocked := range syncBlocked {
			if low == blocked {
				return false
			}
		}
		for _, suffix := range []string{".pem", ".key", ".p12", ".pfx"} {
			if strings.HasSuffix(low, suffix) {
				return false
			}
		}
	}
	return true
}
func digestSync(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
func syncFileAt(root *os.Root, name string) (*syncFile, []byte, error) {
	if !syncAllowed(name) {
		return nil, nil, fmt.Errorf("excluded sync path: %s", name)
	}
	// OpenRoot prevents traversal outside the selected project, including races.
	parts := strings.Split(name, "/")
	for i := range parts {
		info, err := root.Lstat(strings.Join(parts[:i+1], "/"))
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil, nil
		}
		if err != nil {
			return nil, nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, nil, fmt.Errorf("sync does not follow symlinks: %s", name)
		}
	}
	file, err := root.Open(name)
	if err != nil {
		return nil, nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, nil, fmt.Errorf("sync requires a regular file: %s", name)
	}
	if info.Size() > syncFileLimit {
		return nil, nil, fmt.Errorf("sync file exceeds 64 MiB: %s", name)
	}
	data, err := io.ReadAll(io.LimitReader(file, syncFileLimit+1))
	if err != nil {
		return nil, nil, err
	}
	after, err := file.Stat()
	if err != nil {
		return nil, nil, err
	}
	if len(data) > syncFileLimit || info.Size() != after.Size() || !info.ModTime().Equal(after.ModTime()) {
		return nil, nil, fmt.Errorf("file changed while reading: %s", name)
	}
	return &syncFile{digestSync(data), int64(len(data)), info.Mode().Perm()&0111 != 0}, data, nil
}
func syncGitEnv() []string {
	var env []string
	for _, v := range os.Environ() {
		if !strings.HasPrefix(v, "GIT_") {
			env = append(env, v)
		}
	}
	return append(env, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull)
}
func scanSyncLocal(ctx context.Context, root *os.Root, base syncManifest) (syncManifest, error) {
	// A private index evaluates nested ignore rules without invoking project Git
	// hooks, fsmonitor, credential helpers, or changing the user's Git metadata.
	index, err := os.MkdirTemp("", "onecatch-index-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(index)
	init := exec.CommandContext(ctx, "git", "init", "--bare", "--template=", index)
	init.Env = syncGitEnv()
	if err := init.Run(); err != nil {
		return nil, errors.New("file sync requires Git on this computer")
	}
	cmd := exec.CommandContext(ctx, "git", "--git-dir="+index, "--work-tree="+root.Name(), "-c", "core.bare=false", "ls-files", "--others", "--exclude-standard", "-z")
	cmd.Env = syncGitEnv()
	output, err := cmd.Output()
	if err != nil {
		return nil, errors.New("could not list project files for sync")
	}
	// Include tracked files even when a later ignore rule matches them. Disable
	// executable fsmonitor hooks in the user's repository explicitly.
	tracked := exec.CommandContext(ctx, "git", "-C", root.Name(), "-c", "core.fsmonitor=false", "ls-files", "--cached", "-z", "--", ".")
	tracked.Env = syncGitEnv()
	if trackedOutput, trackedErr := tracked.Output(); trackedErr == nil {
		output = append(output, trackedOutput...)
	}
	names := map[string]bool{}
	for _, name := range strings.Split(string(output), "\x00") {
		if syncAllowed(name) {
			names[name] = true
		}
	}
	for name := range base {
		if syncAllowed(name) {
			names[name] = true
		}
	}
	if len(names) > 100000 {
		return nil, errors.New("file sync exceeds 100,000 files")
	}
	manifest := syncManifest{}
	for name := range names {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		info, err := root.Lstat(name)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() {
			if _, wasSynced := base[name]; wasSynced {
				return nil, fmt.Errorf("previously synced file is no longer regular: %s", name)
			}
			continue
		}
		file, _, err := syncFileAt(root, name)
		if err != nil {
			return nil, err
		}
		if file != nil {
			manifest[name] = *file
		}
	}
	return manifest, nil
}
func sortedSyncPaths(manifests ...syncManifest) []string {
	set := map[string]bool{}
	for _, m := range manifests {
		for name := range m {
			set[name] = true
		}
	}
	paths := make([]string, 0, len(set))
	for name := range set {
		paths = append(paths, name)
	}
	sort.Strings(paths)
	return paths
}
func syncEntry(m syncManifest, name string) *syncFile {
	v, ok := m[name]
	if !ok {
		return nil
	}
	return &v
}
func syncEqual(a, b *syncFile) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
func setSyncEntry(m syncManifest, name string, value *syncFile) {
	if value == nil {
		delete(m, name)
	} else {
		m[name] = *value
	}
}

// Serialize all sync runs touching a local directory, even through different
// workers. The remote helper additionally holds a cross-process advisory lock.
var sandboxSyncLocks sync.Map

func takeSandboxSyncLock(key string) (func(), error) {
	ch := make(chan struct{}, 1)
	actual, _ := sandboxSyncLocks.LoadOrStore(key, ch)
	lock := actual.(chan struct{})
	select {
	case lock <- struct{}{}:
		return func() { <-lock }, nil
	default:
		return nil, errors.New("this project already has a sandbox sync run; wait for it to finish")
	}
}
func (r *Registry) SyncStateRoot() string { return filepath.Join(filepath.Dir(r.path), "worker-sync") }

// RunSandboxSynced keeps a reusable source mirror. Only file hashes/metadata and
// conflict recovery copies are stored outside the user's local project.
func (c *Client) RunSandboxSynced(ctx context.Context, config Config, req agentrun.Request, stateRoot string, sink agentrun.Sink) (agentrun.Result, error) {
	if req.Remote != nil || req.Runtime != agentrun.RuntimeCodex {
		return agentrun.Result{}, errors.New("sandbox file sync requires a local Codex project")
	}
	localPath, err := filepath.EvalSymlinks(req.Workspace)
	if err != nil {
		return agentrun.Result{}, err
	}
	localPath, err = filepath.Abs(localPath)
	if err != nil {
		return agentrun.Result{}, err
	}
	unlock, err := takeSandboxSyncLock(localPath)
	if err != nil {
		return agentrun.Result{}, err
	}
	defer unlock()
	root, err := os.OpenRoot(localPath)
	if err != nil {
		return agentrun.Result{}, err
	}
	defer root.Close()
	key := digestSync([]byte(config.ID + "\x00" + config.BaseURL + "\x00" + config.RemotePath + "\x00" + localPath))[:32]
	stateDir := filepath.Join(stateRoot, key)
	if err := os.MkdirAll(stateDir, 0700); err != nil {
		return agentrun.Result{}, err
	}
	statePath := filepath.Join(stateDir, "state.json")
	state := syncState{Files: syncManifest{}}
	if data, readErr := os.ReadFile(statePath); readErr == nil {
		if err := json.Unmarshal(data, &state); err != nil {
			return agentrun.Result{}, err
		}
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return agentrun.Result{}, readErr
	}
	if state.Files == nil {
		state.Files = syncManifest{}
	}
	for _, session := range state.Sessions {
		if err := stopSandboxSession(ctx, config, session); err != nil {
			return agentrun.Result{}, err
		}
	}
	state.Sessions = nil

	emit := func(kind agentrun.EventKind, text string, failed bool) {
		if sink != nil {
			sink(agentrun.Event{Kind: kind, Text: text, Failed: failed, At: time.Now().UTC()})
		}
	}
	emit(agentrun.KindToolUse, "同步本地项目", false)
	syncCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	process := "python3 -u -c " + sandboxQuote("import base64;exec(base64.b64decode("+fmt.Sprintf("%q", base64.StdEncoding.EncodeToString([]byte(sandboxFileHelper)))+"))")
	stream, err := c.openSandboxProcess(syncCtx, config, process)
	if err != nil {
		return agentrun.Result{}, err
	}
	defer stream.Close()
	state.Sessions = []string{stream.sessionID}
	if err = localfile.WriteJSONAtomic(statePath, state); err != nil {
		return agentrun.Result{}, err
	}
	files := newSandboxFiles(stream)
	remotePath := path.Join(config.RemotePath, ".onecatch-workspaces", key)
	initialized, err := files.call(map[string]any{"op": "init", "root": remotePath, "blocked": syncBlocked})
	if err != nil {
		return agentrun.Result{}, err
	}
	if len(initialized.Generation) != 32 {
		return agentrun.Result{}, errors.New("invalid sandbox workspace generation")
	}
	// A reset sandbox is an empty replica, not an instruction to delete every
	// local file. Start a full seed when the remote replica identity changes.
	if state.Generation != initialized.Generation {
		state.Generation = initialized.Generation
		state.Files = syncManifest{}
		if err = localfile.WriteJSONAtomic(statePath, state); err != nil {
			return agentrun.Result{}, err
		}
	}
	remote, err := files.manifest(state.Files)
	if err != nil {
		return agentrun.Result{}, err
	}
	// Recover remote changes from a previous completed turn before uploading.
	if err := pullSandboxFiles(root, files, &state, remote, stateDir, statePath); err != nil {
		return agentrun.Result{}, err
	}
	local, err := scanSyncLocal(ctx, root, state.Files)
	if err != nil {
		return agentrun.Result{}, err
	}
	sent := 0
	var batch []map[string]any
	batchBytes := 0
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		_, err := files.call(map[string]any{"op": "batch", "uploads": batch})
		batch = nil
		batchBytes = 0
		return err
	}
	for _, name := range sortedSyncPaths(local, remote) {
		target, before := syncEntry(local, name), syncEntry(remote, name)
		if syncEqual(target, before) {
			continue
		}
		if target == nil {
			if err = flush(); err != nil {
				return agentrun.Result{}, err
			}
			_, err = files.call(map[string]any{"op": "delete", "path": name, "before": before})
		} else {
			actual, data, readErr := syncFileAt(root, name)
			if readErr != nil {
				return agentrun.Result{}, readErr
			}
			if !syncEqual(actual, target) {
				return agentrun.Result{}, fmt.Errorf("本地文件正在变化，请保存后重试：%s", name)
			}
			if len(data) <= syncChunkBytes {
				if batchBytes+len(data) > syncChunkBytes || len(batch) >= 64 {
					if err = flush(); err != nil {
						return agentrun.Result{}, err
					}
				}
				batch = append(batch, map[string]any{"path": name, "data": data, "file": *target, "before": before})
				batchBytes += len(data)
			} else {
				if err = flush(); err != nil {
					return agentrun.Result{}, err
				}
				err = files.upload(name, data, *target, before)
			}
		}
		if err != nil {
			return agentrun.Result{}, err
		}
		sent++
	}
	if err = flush(); err != nil {
		return agentrun.Result{}, err
	}
	state.Files = local
	if err = localfile.WriteJSONAtomic(statePath, state); err != nil {
		return agentrun.Result{}, err
	}
	emit(agentrun.KindToolResult, fmt.Sprintf("已同步 · %d 个文件更新", sent), false)
	config.RemotePath = remotePath
	args := []string{}
	if req.ServiceTier == "fast" {
		args = append(args, "-c", "features.fast_mode=true")
	}
	codex, err := c.openSandboxCodex(ctx, config, args)
	if err != nil {
		return agentrun.Result{}, err
	}
	defer codex.Close()
	state.Sessions = append(state.Sessions, codex.sessionID)
	if err = localfile.WriteJSONAtomic(statePath, state); err != nil {
		return agentrun.Result{}, err
	}
	req.Workspace = remotePath
	req.Environment = nil
	req.EnvironmentAllowlist = nil
	result, runErr := agentrun.RunCodexStream(ctx, req, codex, sink)
	// Confirm termination rather than assuming a closed socket stopped the process.
	cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 15*time.Second)
	stopErr := stopSandboxSession(cleanupCtx, config, codex.sessionID)
	cleanupCancel()
	if stopErr != nil {
		return result, errors.Join(runErr, stopErr)
	}
	// On cancellation the file connection was closed too. Leave the durable
	// baseline for recovery before the next turn.
	if runErr != nil {
		return result, runErr
	}
	emit(agentrun.KindToolUse, "回传远端修改", false)
	remote, err = files.manifest(state.Files)
	if err == nil {
		err = pullSandboxFiles(root, files, &state, remote, stateDir, statePath)
	}
	if err != nil {
		emit(agentrun.KindToolResult, err.Error(), true)
		return result, err
	}
	_ = stream.Close()
	if err = stopSandboxSession(ctx, config, stream.sessionID); err != nil {
		return result, err
	}
	state.Sessions = nil
	if err = localfile.WriteJSONAtomic(statePath, state); err != nil {
		return result, err
	}
	emit(agentrun.KindToolResult, "远端修改已回传", false)
	return result, nil
}

// Pull only changes made remotely. Local-only edits remain queued for the next
// turn. Each replacement rechecks the local hash immediately before rename.
func pullSandboxFiles(root *os.Root, files *sandboxFiles, state *syncState, remote syncManifest, stateDir, statePath string) error {
	var conflicts []string
	recovery := filepath.Join(stateDir, "conflicts", time.Now().UTC().Format("20060102T150405.000000000"))
	for _, name := range sortedSyncPaths(state.Files, remote) {
		base, target := syncEntry(state.Files, name), syncEntry(remote, name)
		if syncEqual(base, target) {
			continue
		}
		current, _, err := syncFileAt(root, name)
		if err != nil {
			return err
		}
		if syncEqual(current, target) {
			setSyncEntry(state.Files, name, target)
			continue
		}
		var data []byte
		if target != nil {
			data, err = files.download(name, *target)
			if err != nil {
				return err
			}
		}
		if !syncEqual(current, base) {
			// Preserve the remote alternative before advancing the base. The next
			// explicitly submitted turn uses the user's retained local version.
			backup := filepath.Join(recovery, filepath.FromSlash(name)+".remote")
			if err = os.MkdirAll(filepath.Dir(backup), 0700); err != nil {
				return err
			}
			if target == nil {
				backup += "-deleted"
				data = []byte("Remote deleted this file. Local version was kept.\n")
			}
			if err = os.WriteFile(backup, data, 0600); err != nil {
				return err
			}
			conflicts = append(conflicts, name)
			setSyncEntry(state.Files, name, target)
			continue
		}
		if err = applySyncFile(root, name, current, target, data); err != nil {
			return err
		}
		setSyncEntry(state.Files, name, target)
	}
	if err := localfile.WriteJSONAtomic(statePath, *state); err != nil {
		return err
	}
	if len(conflicts) > 0 {
		return fmt.Errorf("同步冲突：%s。本地版本已保留，远端版本备份在 %s。检查并合并后，再次发送将使用本地版本", strings.Join(conflicts, ", "), recovery)
	}
	return nil
}
func applySyncFile(root *os.Root, name string, before, after *syncFile, data []byte) error {
	current, _, err := syncFileAt(root, name)
	if err != nil {
		return err
	}
	if !syncEqual(current, before) {
		return fmt.Errorf("本地文件在回传时发生修改：%s；远端版本已保留", name)
	}
	if after == nil {
		if current == nil {
			return nil
		}
		return root.Remove(name)
	}
	if digestSync(data) != after.Hash {
		return errors.New("invalid sync file digest")
	}
	if err = root.MkdirAll(filepath.Dir(name), 0755); err != nil {
		return err
	}
	var nonce [16]byte
	// Random temp names also avoid overwriting user files if a previous write died.
	if _, err = rand.Read(nonce[:]); err != nil {
		return err
	}
	temp := path.Join(path.Dir(name), ".onecatch-sync-"+hex.EncodeToString(nonce[:]))
	mode := os.FileMode(0644)
	if info, statErr := root.Lstat(name); statErr == nil {
		mode = info.Mode().Perm() &^ 0111
	}
	if after.Executable {
		mode |= 0111
	}
	f, err := root.OpenFile(temp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	defer root.Remove(temp)
	_, writeErr := f.Write(data)
	if writeErr == nil {
		writeErr = f.Sync()
	}
	closeErr := f.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	current, _, err = syncFileAt(root, name)
	if err != nil {
		return err
	}
	if !syncEqual(current, before) {
		return fmt.Errorf("本地文件在回传时发生修改：%s", name)
	}
	return root.Rename(temp, name)
}

func stopSandboxSession(ctx context.Context, config Config, session string) error {
	if session == "" {
		return errors.New("sandbox session identity missing; cannot safely recover sync")
	}
	endpoint, err := sandboxURL(config, "/v1/shell/sessions/"+url.PathEscape(session), false)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return errors.New("invalid sandbox cleanup request")
	}
	response, err := sandboxHTTPClient().Do(request)
	if err != nil {
		return errors.New("cannot confirm sandbox process stopped; retry when connected")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusNoContent && response.StatusCode != http.StatusNotFound {
		return fmt.Errorf("sandbox process cleanup returned HTTP %d", response.StatusCode)
	}
	return nil
}
