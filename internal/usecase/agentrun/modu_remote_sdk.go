//go:build !onecatch_worker

package agentrun

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	codingtools "github.com/openmodu/modu/pkg/coding_agent/tools"
	toolcommon "github.com/openmodu/modu/pkg/coding_agent/tools/common"
	"github.com/openmodu/modu/pkg/types"
	"github.com/openmodu/onecatch/internal/remotefs"
	"github.com/openmodu/onecatch/internal/usecase/agentrun/seam"
)

const remoteModuMaxReadBytes = 8 * 1024 * 1024

const remoteModuGuidance = `This session operates on a REMOTE target through OneCatch.

The read, write, edit, grep, find, ls and bash tools are redirected to the
remote workspace. They do not operate on the local harness directory. Paths
are the target's own absolute paths; keep all workspace operations inside the
configured remote root.`

type remoteModuFiles interface {
	Lstat(string) (os.FileInfo, error)
	ReadDir(string) ([]os.FileInfo, error)
	OpenFile(string, int, os.FileMode) (remotefs.File, error)
	Mkdir(string, os.FileMode) error
	Close() error
}

// remoteModuToolProvider keeps Modu's model and provider credentials local,
// while replacing every workspace-facing SDK tool with an SFTP/SSH-backed
// implementation. Tools that could silently create a local worktree are not
// exposed for a Remote FS run.
type remoteModuToolProvider struct {
	base     codingtools.DefaultProvider
	files    remoteModuFiles
	executor seam.Executor
	root     string
	readOnly bool

	readMu sync.Mutex
	reads  map[string]string
}

func newRemoteModuToolProvider(ctx context.Context, target seam.Target, readOnly bool) (*remoteModuToolProvider, error) {
	files, err := remotefs.NewSFTPBackend(ctx, remotefs.SFTPConfig{
		Host:          target.Host,
		Root:          target.Root,
		Username:      target.Username,
		CredentialID:  target.CredentialID,
		SSHBinary:     target.SSHBinary,
		AskPassBinary: target.AskPassBinary,
		SSHOptions:    target.SSHOptions,
	})
	if err != nil {
		return nil, fmt.Errorf("open remote workspace for Modu Code: %w", err)
	}
	set := codingtools.ToolSetCoding
	if readOnly {
		set = codingtools.ToolSetReadOnly
	}
	return &remoteModuToolProvider{
		base: codingtools.NewProvider(set), files: files, executor: seam.NewExecutor(target),
		root: path.Clean(target.Root), readOnly: readOnly, reads: make(map[string]string),
	}, nil
}

func (p *remoteModuToolProvider) Tools(ctx types.ToolContext) []types.Tool {
	ctx.Cwd = p.root
	input := p.base.Tools(ctx)
	output := make([]types.Tool, 0, len(input))
	for _, tool := range input {
		if adapted, ok := p.adapt(tool); ok {
			output = append(output, adapted)
		}
	}
	return output
}

func (p *remoteModuToolProvider) Rebind(tool types.Tool, ctx types.ToolContext) (types.Tool, bool) {
	ctx.Cwd = p.root
	if wrapped, ok := tool.(*remoteModuTool); ok {
		tool = wrapped.base
	}
	rebound, ok := p.base.Rebind(tool, ctx)
	if !ok {
		return nil, false
	}
	return p.adapt(rebound)
}

func (p *remoteModuToolProvider) adapt(tool types.Tool) (types.Tool, bool) {
	switch tool.Name() {
	case "read", "grep", "find", "ls":
		return &remoteModuTool{base: tool, provider: p}, true
	case "write", "edit", "bash":
		if p.readOnly {
			return nil, false
		}
		return &remoteModuTool{base: tool, provider: p}, true
	case "bash_output", "kill_bash", "enter_worktree", "exit_worktree":
		// Background process handles and local worktrees cannot be represented
		// safely across one-shot SSH commands.
		return nil, false
	default:
		// Memory, planning, web, context and trajectory tools are local Agent
		// state, not workspace operations, and remain available.
		return tool, true
	}
}

func (p *remoteModuToolProvider) ShutdownTools() {
	p.base.ShutdownTools()
	if p.files != nil {
		_ = p.files.Close()
	}
}

type remoteModuTool struct {
	base     types.Tool
	provider *remoteModuToolProvider
}

func (t *remoteModuTool) Name() string    { return t.base.Name() }
func (t *remoteModuTool) Label() string   { return t.base.Label() }
func (t *remoteModuTool) Parameters() any { return t.base.Parameters() }
func (t *remoteModuTool) Parallel() bool {
	parallel, ok := t.base.(types.ParallelTool)
	return ok && parallel.Parallel()
}
func (t *remoteModuTool) Description() string {
	return "This tool operates on the configured remote workspace over SSH.\n\n" +
		strings.ReplaceAll(t.base.Description(), "local filesystem", "remote workspace")
}

func (t *remoteModuTool) Execute(ctx context.Context, toolCallID string, args map[string]any, onUpdate types.ToolUpdateCallback) (types.ToolResult, error) {
	switch t.Name() {
	case "read":
		return t.provider.read(args)
	case "write":
		return t.provider.write(args)
	case "edit":
		return t.provider.edit(args)
	case "ls":
		return t.provider.list(args)
	case "grep":
		return t.provider.grep(ctx, args)
	case "find":
		return t.provider.find(ctx, args)
	case "bash":
		return t.provider.bash(ctx, toolCallID, args)
	default:
		return toolcommon.ErrorResult("unsupported remote tool: " + t.Name()), nil
	}
}

func (p *remoteModuToolProvider) read(args map[string]any) (types.ToolResult, error) {
	display := stringArgument(args, "path", "file_path")
	if display == "" {
		return toolcommon.ErrorResult("path is required"), nil
	}
	relative, err := remoteModuPath(p.root, display)
	if err != nil {
		return toolcommon.ErrorResult(err.Error()), nil
	}
	info, err := p.files.Lstat(relative)
	if err != nil {
		return toolcommon.ErrorResult(fmt.Sprintf("failed to stat remote file: %v", err)), nil
	}
	if info.IsDir() {
		return toolcommon.ErrorResult(fmt.Sprintf("%s is a directory, not a file. Use ls to list directory contents.", display)), nil
	}
	if info.Size() > remoteModuMaxReadBytes {
		return toolcommon.ErrorResult(fmt.Sprintf("remote file is too large to read: %s", display)), nil
	}
	data, err := p.readFile(relative, info)
	if err != nil {
		return toolcommon.ErrorResult(fmt.Sprintf("failed to read remote file: %v", err)), nil
	}

	ext := strings.ToLower(path.Ext(relative))
	if mimeType := remoteImageMIME(ext); mimeType != "" {
		return types.ToolResult{Content: []types.ContentBlock{&types.ImageContent{
			Type: "image", Data: base64.StdEncoding.EncodeToString(data), MimeType: mimeType,
		}}}, nil
	}
	if bytes.IndexByte(data, 0) >= 0 {
		return toolcommon.ErrorResult("This tool cannot read binary files."), nil
	}
	if _, explicitLimit := args["limit"]; !explicitLimit && len(data) > toolcommon.ReadMaxBytes {
		return toolcommon.ErrorResult(fmt.Sprintf("file is too large to read at once: %s. Use offset and limit.", display)), nil
	}

	content := strings.TrimPrefix(string(data), "\xef\xbb\xbf")
	if content == "" {
		p.rememberRead(relative, string(data), true)
		return remoteTextResult("<system-reminder>Warning: the file exists but the contents are empty.</system-reminder>", map[string]any{"path": display, "size": info.Size()}), nil
	}
	lines := strings.Split(content, "\n")
	if strings.HasSuffix(content, "\n") {
		lines = lines[:len(lines)-1]
	}
	offset := 0
	if value, ok := args["offset"]; ok {
		offset, _ = toolcommon.ToSemanticInt(value)
		if offset < 0 {
			return toolcommon.ErrorResult("offset must be greater than or equal to 0"), nil
		}
		if offset > 0 {
			offset--
		}
	}
	limit := toolcommon.ReadMaxLines
	if value, ok := args["limit"]; ok {
		limit, _ = toolcommon.ToSemanticInt(value)
		if limit <= 0 {
			return toolcommon.ErrorResult("limit must be greater than 0"), nil
		}
	}
	if offset >= len(lines) {
		return remoteTextResult(fmt.Sprintf("<system-reminder>Warning: the file has only %d lines.</system-reminder>", len(lines)), map[string]any{"path": display, "lines": len(lines)}), nil
	}
	selected := lines[offset:]
	truncated := len(selected) > limit
	if truncated {
		selected = selected[:limit]
	}
	var output strings.Builder
	for index, line := range selected {
		fmt.Fprintf(&output, "%d\t%s\n", offset+index+1, line)
	}
	if truncated {
		fmt.Fprintf(&output, "\n... (%d more lines)", len(lines)-offset-limit)
	}
	p.rememberRead(relative, string(data), offset == 0 && !truncated)
	return remoteTextResult(output.String(), map[string]any{
		"path": display, "size": info.Size(), "lines": len(lines), "truncated": truncated,
	}), nil
}

func (p *remoteModuToolProvider) write(args map[string]any) (types.ToolResult, error) {
	display := stringArgument(args, "path", "file_path")
	content, hasContent := args["content"].(string)
	if display == "" {
		return toolcommon.ErrorResult("path is required"), nil
	}
	if !hasContent {
		return toolcommon.ErrorResult("content is required"), nil
	}
	relative, err := remoteModuPath(p.root, display)
	if err != nil || relative == "" {
		return toolcommon.ErrorResult(remotePathError(err, "a file path is required")), nil
	}
	writeType := "create"
	if info, statErr := p.files.Lstat(relative); statErr == nil {
		if info.IsDir() {
			return toolcommon.ErrorResult(display + " is a directory, not a file"), nil
		}
		current, readErr := p.readFile(relative, info)
		if readErr != nil {
			return toolcommon.ErrorResult(fmt.Sprintf("failed to verify remote file: %v", readErr)), nil
		}
		if !p.matchesRememberedRead(relative, string(current)) {
			return toolcommon.ErrorResult("File has not been fully read, or has changed since it was read. Read it before overwriting."), nil
		}
		writeType = "update"
	} else if !os.IsNotExist(statErr) {
		return toolcommon.ErrorResult(fmt.Sprintf("failed to stat remote file: %v", statErr)), nil
	}
	if err := p.writeFile(relative, []byte(content)); err != nil {
		return toolcommon.ErrorResult(fmt.Sprintf("failed to write remote file: %v", err)), nil
	}
	p.rememberRead(relative, content, true)
	message := fmt.Sprintf("File created successfully at: %s", display)
	if writeType == "update" {
		message = fmt.Sprintf("The file %s has been updated successfully.", display)
	}
	return remoteTextResult(message, map[string]any{"path": display, "bytes": len([]byte(content)), "type": writeType}), nil
}

func (p *remoteModuToolProvider) edit(args map[string]any) (types.ToolResult, error) {
	display := stringArgument(args, "path", "file_path")
	oldText, hasOld := stringArgumentPresent(args, "old_text", "old_string")
	newText, hasNew := stringArgumentPresent(args, "new_text", "new_string")
	if display == "" || !hasOld || !hasNew {
		return toolcommon.ErrorResult("path, old_text and new_text are required"), nil
	}
	if oldText == newText {
		return toolcommon.ErrorResult("No changes to make: old_text and new_text are exactly the same."), nil
	}
	relative, err := remoteModuPath(p.root, display)
	if err != nil || relative == "" {
		return toolcommon.ErrorResult(remotePathError(err, "a file path is required")), nil
	}
	info, statErr := p.files.Lstat(relative)
	if statErr != nil {
		if os.IsNotExist(statErr) && oldText == "" {
			if err := p.writeFile(relative, []byte(newText)); err != nil {
				return toolcommon.ErrorResult(fmt.Sprintf("failed to create remote file: %v", err)), nil
			}
			p.rememberRead(relative, newText, true)
			return remoteTextResult("File created successfully at: "+display, map[string]any{"path": display, "replacements": 1}), nil
		}
		return toolcommon.ErrorResult(fmt.Sprintf("failed to stat remote file: %v", statErr)), nil
	}
	if info.IsDir() {
		return toolcommon.ErrorResult(display + " is a directory, not a file"), nil
	}
	data, err := p.readFile(relative, info)
	if err != nil {
		return toolcommon.ErrorResult(fmt.Sprintf("failed to read remote file: %v", err)), nil
	}
	current := string(data)
	if !p.matchesRememberedRead(relative, current) {
		return toolcommon.ErrorResult("File has not been fully read, or has changed since it was read. Read it before editing."), nil
	}
	count := strings.Count(current, oldText)
	if count == 0 {
		return toolcommon.ErrorResult("old_text was not found in " + display), nil
	}
	replaceAll, _ := toolcommon.ToSemanticBool(args["replace_all"])
	if count > 1 && !replaceAll {
		return toolcommon.ErrorResult(fmt.Sprintf("old_text appears %d times; provide more context or use replace_all=true", count)), nil
	}
	replacements := 1
	limit := 1
	if replaceAll {
		replacements = count
		limit = -1
	}
	updated := strings.Replace(current, oldText, newText, limit)
	if err := p.writeFile(relative, []byte(updated)); err != nil {
		return toolcommon.ErrorResult(fmt.Sprintf("failed to edit remote file: %v", err)), nil
	}
	p.rememberRead(relative, updated, true)
	return remoteTextResult(fmt.Sprintf("Updated %s successfully.", display), map[string]any{"path": display, "replacements": replacements}), nil
}

func (p *remoteModuToolProvider) list(args map[string]any) (types.ToolResult, error) {
	display, _ := args["path"].(string)
	relative, err := remoteModuPath(p.root, display)
	if err != nil {
		return toolcommon.ErrorResult(err.Error()), nil
	}
	entries, err := p.files.ReadDir(relative)
	if err != nil {
		return toolcommon.ErrorResult(fmt.Sprintf("failed to list remote directory: %v", err)), nil
	}
	limit := 500
	if value, ok := args["limit"]; ok {
		if parsed, valid := toolcommon.ToSemanticInt(value); valid && parsed > 0 {
			limit = parsed
		}
	}
	ignore := remoteIgnorePatterns(args["ignore"])
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			name += "/"
		}
		if remoteIgnored(name, ignore) {
			continue
		}
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool { return strings.ToLower(names[i]) < strings.ToLower(names[j]) })
	total := len(names)
	if len(names) > limit {
		names = names[:limit]
	}
	text := strings.Join(names, "\n")
	if text == "" {
		text = "(empty directory)"
	} else if total > len(names) {
		text += fmt.Sprintf("\n\n... (%d entries total, showing first %d)", total, len(names))
	}
	return remoteTextResult(text, map[string]any{"path": display, "totalEntries": total}), nil
}

func (p *remoteModuToolProvider) grep(ctx context.Context, args map[string]any) (types.ToolResult, error) {
	pattern, _ := args["pattern"].(string)
	if pattern == "" {
		return toolcommon.ErrorResult("pattern is required"), nil
	}
	display, _ := args["path"].(string)
	relative, err := remoteModuPath(p.root, display)
	if err != nil {
		return toolcommon.ErrorResult(err.Error()), nil
	}
	if relative == "" {
		relative = "."
	}
	mode, _ := args["output_mode"].(string)
	if mode == "" {
		mode = "files_with_matches"
	}
	rgArgs := []string{"rg", "--color", "never", "--hidden", "--glob", "!.git/**"}
	grepArgs := []string{"grep", "-R"}
	if mode == "files_with_matches" {
		rgArgs = append(rgArgs, "-l")
		grepArgs = append(grepArgs, "-l")
	} else if mode == "count" {
		rgArgs = append(rgArgs, "-c")
		grepArgs = append(grepArgs, "-c")
	} else if mode == "content" {
		rgArgs = append(rgArgs, "-n")
		grepArgs = append(grepArgs, "-n")
	} else {
		return toolcommon.ErrorResult("invalid output_mode; use content, files_with_matches, or count"), nil
	}
	ignoreCase, _ := toolcommon.ToSemanticBool(args["ignore_case"])
	if !ignoreCase {
		ignoreCase, _ = toolcommon.ToSemanticBool(args["-i"])
	}
	if ignoreCase {
		rgArgs = append(rgArgs, "-i")
		grepArgs = append(grepArgs, "-i")
	}
	literal, _ := toolcommon.ToSemanticBool(args["literal"])
	if literal {
		rgArgs = append(rgArgs, "-F")
		grepArgs = append(grepArgs, "-F")
	}
	if glob, _ := args["glob"].(string); strings.TrimSpace(glob) != "" {
		for _, item := range strings.FieldsFunc(glob, func(r rune) bool { return r == ',' || r == ' ' }) {
			rgArgs = append(rgArgs, "--glob", item)
		}
	}
	rgArgs = append(rgArgs, "--", pattern, relative)
	grepArgs = append(grepArgs, "--", pattern, relative)
	command := "if command -v rg >/dev/null 2>&1; then " + remoteShellJoin(rgArgs) + "; else " + remoteShellJoin(grepArgs) + "; fi"
	stdout, stderr, outcome, runErr := p.runRemote(ctx, command, 2*time.Minute)
	if runErr != nil {
		return toolcommon.ErrorResult(fmt.Sprintf("remote grep transport failed: %v", runErr)), nil
	}
	if outcome.ExitCode > 1 {
		return toolcommon.ErrorResult(strings.TrimSpace(stdout + "\n" + stderr)), nil
	}
	lines := nonEmptyLines(stdout)
	offset, limit := remotePage(args, 250)
	lines = pageLines(lines, offset, limit)
	text := strings.Join(lines, "\n")
	if text == "" {
		text = "(no matches)"
	}
	return remoteTextResult(text, map[string]any{"path": display, "outputMode": mode}), nil
}

func (p *remoteModuToolProvider) find(ctx context.Context, args map[string]any) (types.ToolResult, error) {
	pattern, _ := args["pattern"].(string)
	if pattern == "" {
		return toolcommon.ErrorResult("pattern is required"), nil
	}
	display, _ := args["path"].(string)
	relative, err := remoteModuPath(p.root, display)
	if err != nil {
		return toolcommon.ErrorResult(err.Error()), nil
	}
	if relative == "" {
		relative = "."
	}
	command := remoteShellJoin([]string{"find", relative, "-type", "f", "!", "-path", "*/.git/*", "-print"})
	stdout, stderr, outcome, runErr := p.runRemote(ctx, command, 2*time.Minute)
	if runErr != nil {
		return toolcommon.ErrorResult(fmt.Sprintf("remote find transport failed: %v", runErr)), nil
	}
	if outcome.ExitCode != 0 {
		return toolcommon.ErrorResult(strings.TrimSpace(stdout + "\n" + stderr)), nil
	}
	var matches []string
	for _, name := range nonEmptyLines(stdout) {
		name = strings.TrimPrefix(strings.TrimSpace(name), "./")
		candidate := name
		if relative != "." {
			candidate = strings.TrimPrefix(candidate, strings.TrimSuffix(relative, "/")+"/")
		}
		if remoteGlobMatch(pattern, candidate) || remoteGlobMatch(pattern, name) {
			matches = append(matches, name)
		}
	}
	sort.Strings(matches)
	_, limit := remotePage(args, 100)
	if len(matches) > limit {
		matches = matches[:limit]
	}
	text := strings.Join(matches, "\n")
	if text == "" {
		text = "(no files found)"
	}
	return remoteTextResult(text, map[string]any{"path": display, "pattern": pattern}), nil
}

func (p *remoteModuToolProvider) bash(ctx context.Context, toolCallID string, args map[string]any) (types.ToolResult, error) {
	command, _ := args["command"].(string)
	if strings.TrimSpace(command) == "" {
		return toolcommon.ErrorResult("command is required"), nil
	}
	background, _ := toolcommon.ToSemanticBool(args["background"])
	if !background {
		background, _ = toolcommon.ToSemanticBool(args["run_in_background"])
	}
	if background {
		return toolcommon.ErrorResult("background commands are not supported for a Remote FS Modu run"), nil
	}
	timeout := remoteBashTimeout(args)
	stdout, stderr, outcome, err := p.runRemote(ctx, command, timeout)
	if err != nil {
		return toolcommon.ErrorResult(fmt.Sprintf("remote command transport failed: %v", err)), nil
	}
	combined := stdout
	if stderr != "" {
		if combined != "" {
			combined += "\n"
		}
		combined += stderr
	}
	preview := toolcommon.PreviewText(combined, toolcommon.TextPreviewOptions{
		ToolCallID: toolCallID, ArtifactName: "remote-output", Strategy: toolcommon.PreviewTail,
		MaxLines: toolcommon.BashMaxLines, MaxBytes: toolcommon.DefaultMaxBytes,
	})
	text := preview.Text
	if outcome.ExitCode != 0 {
		text += fmt.Sprintf("\n\nExit code: %d", outcome.ExitCode)
	}
	if strings.TrimSpace(text) == "" {
		text = "(no output)"
	}
	return remoteTextResult(text, map[string]any{"exitCode": outcome.ExitCode, "remote": true}), nil
}

func (p *remoteModuToolProvider) runRemote(ctx context.Context, command string, timeout time.Duration) (string, string, seam.Outcome, error) {
	var stdout, stderr bytes.Buffer
	outcome, err := p.executor.Run(ctx, seam.Command{
		Command: command, Dir: p.root, Stdout: &stdout, Stderr: &stderr, Timeout: timeout,
	})
	return stdout.String(), stderr.String(), outcome, err
}

func (p *remoteModuToolProvider) readFile(relative string, info os.FileInfo) ([]byte, error) {
	file, err := p.files.OpenFile(relative, os.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	if info == nil {
		info, err = file.Stat()
		if err != nil {
			return nil, err
		}
	}
	reader := io.NewSectionReader(file, 0, info.Size())
	return io.ReadAll(reader)
}

func (p *remoteModuToolProvider) writeFile(relative string, data []byte) error {
	if err := p.ensureParent(path.Dir(relative)); err != nil {
		return err
	}
	file, err := p.files.OpenFile(relative, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	for offset := 0; offset < len(data); {
		written, writeErr := file.WriteAt(data[offset:], int64(offset))
		offset += written
		if writeErr != nil {
			return writeErr
		}
		if written == 0 {
			return io.ErrShortWrite
		}
	}
	return file.Sync()
}

func (p *remoteModuToolProvider) ensureParent(relative string) error {
	if relative == "." || relative == "" {
		return nil
	}
	current := ""
	for _, part := range strings.Split(relative, "/") {
		current = path.Join(current, part)
		info, err := p.files.Lstat(current)
		if err == nil {
			if !info.IsDir() {
				return fmt.Errorf("%s is not a directory", current)
			}
			continue
		}
		if !os.IsNotExist(err) {
			return err
		}
		if err := p.files.Mkdir(current, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func (p *remoteModuToolProvider) rememberRead(relative, content string, full bool) {
	p.readMu.Lock()
	defer p.readMu.Unlock()
	if full {
		p.reads[relative] = content
	} else {
		delete(p.reads, relative)
	}
}

func (p *remoteModuToolProvider) matchesRememberedRead(relative, current string) bool {
	p.readMu.Lock()
	defer p.readMu.Unlock()
	remembered, ok := p.reads[relative]
	return ok && remembered == current
}

func remoteModuPath(root, input string) (string, error) {
	input = strings.TrimSpace(strings.ReplaceAll(input, "\\", "/"))
	if input == "" || input == "." {
		return "", nil
	}
	cleaned := path.Clean(input)
	if path.IsAbs(cleaned) {
		root = path.Clean(root)
		if cleaned == root {
			return "", nil
		}
		if root != "/" && !strings.HasPrefix(cleaned, root+"/") {
			return "", fmt.Errorf("path %q is outside the remote workspace %q", input, root)
		}
		if root == "/" {
			return strings.TrimPrefix(cleaned, "/"), nil
		}
		return strings.TrimPrefix(cleaned, root+"/"), nil
	}
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("path %q escapes the remote workspace", input)
	}
	return cleaned, nil
}

func remoteTextResult(text string, details map[string]any) types.ToolResult {
	return types.ToolResult{Content: []types.ContentBlock{&types.TextContent{Type: "text", Text: text}}, Details: details}
}

func remotePathError(err error, fallback string) string {
	if err != nil {
		return err.Error()
	}
	return fallback
}

func stringArgument(args map[string]any, keys ...string) string {
	value, _ := stringArgumentPresent(args, keys...)
	return value
}

func stringArgumentPresent(args map[string]any, keys ...string) (string, bool) {
	for _, key := range keys {
		if value, ok := args[key].(string); ok {
			return value, true
		}
	}
	return "", false
}

func remoteImageMIME(extension string) string {
	switch extension {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	default:
		return ""
	}
}

func remoteIgnorePatterns(value any) []string {
	var values []string
	switch typed := value.(type) {
	case string:
		values = []string{typed}
	case []string:
		values = typed
	case []any:
		for _, item := range typed {
			if text, ok := item.(string); ok {
				values = append(values, text)
			}
		}
	}
	for index := range values {
		values[index] = strings.TrimSpace(values[index])
	}
	return values
}

func remoteIgnored(name string, patterns []string) bool {
	for _, pattern := range patterns {
		if pattern == "" {
			continue
		}
		matched, _ := path.Match(strings.TrimSuffix(pattern, "/"), strings.TrimSuffix(name, "/"))
		if matched || strings.TrimSuffix(pattern, "/**") == strings.TrimSuffix(name, "/") {
			return true
		}
	}
	return false
}

func remoteShellJoin(arguments []string) string {
	quoted := make([]string, len(arguments))
	for index, argument := range arguments {
		quoted[index] = remoteShellQuote(argument)
	}
	return strings.Join(quoted, " ")
}

func remoteShellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func nonEmptyLines(value string) []string {
	var output []string
	for _, line := range strings.Split(strings.TrimSpace(value), "\n") {
		if strings.TrimSpace(line) != "" {
			output = append(output, line)
		}
	}
	return output
}

func remotePage(args map[string]any, defaultLimit int) (int, int) {
	offset := 0
	if value, ok := args["offset"]; ok {
		offset, _ = toolcommon.ToSemanticInt(value)
		if offset < 0 {
			offset = 0
		}
	}
	limit := defaultLimit
	for _, key := range []string{"limit", "head_limit"} {
		if value, ok := args[key]; ok {
			if parsed, valid := toolcommon.ToSemanticInt(value); valid && parsed > 0 {
				limit = parsed
			}
		}
	}
	return offset, limit
}

func pageLines(lines []string, offset, limit int) []string {
	if offset >= len(lines) {
		return nil
	}
	lines = lines[offset:]
	if len(lines) > limit {
		lines = lines[:limit]
	}
	return lines
}

func remoteGlobMatch(pattern, name string) bool {
	pattern = strings.TrimPrefix(path.Clean(pattern), "./")
	name = strings.TrimPrefix(path.Clean(name), "./")
	if matched, _ := path.Match(pattern, name); matched {
		return true
	}
	if strings.HasPrefix(pattern, "**/") {
		pattern = strings.TrimPrefix(pattern, "**/")
		parts := strings.Split(name, "/")
		for index := range parts {
			if matched, _ := path.Match(pattern, strings.Join(parts[index:], "/")); matched {
				return true
			}
		}
	}
	return false
}

func remoteBashTimeout(args map[string]any) time.Duration {
	const defaultTimeout = 120 * time.Second
	if value, ok := args["timeout_ms"]; ok {
		if parsed, valid := toolcommon.ToSemanticInt(value); valid && parsed > 0 {
			return min(time.Duration(parsed)*time.Millisecond, 10*time.Minute)
		}
	}
	if value, ok := args["timeout"]; ok {
		if parsed, valid := toolcommon.ToSemanticInt(value); valid && parsed > 0 {
			if parsed <= 600 {
				return time.Duration(parsed) * time.Second
			}
			return min(time.Duration(parsed)*time.Millisecond, 10*time.Minute)
		}
	}
	return defaultTimeout
}
