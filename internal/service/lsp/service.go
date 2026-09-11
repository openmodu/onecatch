package lsp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	domainworkspaces "github.com/openmodu/onecatch/internal/domain/workspaces"
)

const (
	initializeTimeout = 20 * time.Second
	requestTimeout    = 15 * time.Second
)

type WorkspaceResolver func(context.Context, string) (domainworkspaces.Workspace, error)

type Position struct {
	Line      uint32 `json:"line"`
	Character uint32 `json:"character"`
}

type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

type Location struct {
	Path  string `json:"path"`
	Range Range  `json:"range"`
}

type SyncDocumentInput struct {
	WorkspaceID string `json:"workspaceId"`
	Path        string `json:"path"`
	Content     string `json:"content"`
}

type DefinitionInput struct {
	WorkspaceID string   `json:"workspaceId"`
	Path        string   `json:"path"`
	Content     string   `json:"content"`
	Position    Position `json:"position"`
}

type CloseDocumentInput struct {
	WorkspaceID string `json:"workspaceId"`
	Path        string `json:"path"`
}

type CloseWorkspaceInput struct {
	WorkspaceID string `json:"workspaceId"`
}

type DetectInput struct {
	WorkspaceID string `json:"workspaceId"`
	Path        string `json:"path"`
}

type Capability struct {
	Recognized  bool   `json:"recognized"`
	Available   bool   `json:"available"`
	LanguageID  string `json:"languageId"`
	ServerID    string `json:"serverId"`
	ServerName  string `json:"serverName"`
	Command     string `json:"command"`
	InstallHint string `json:"installHint"`
}

type sessionKey struct {
	workspaceID string
	serverID    string
}

type Service struct {
	resolver WorkspaceResolver
	mu       sync.Mutex
	sessions map[sessionKey]*session
	closed   bool
}

type documentState struct {
	content string
	version int
}

type session struct {
	workspace domainworkspaces.Workspace
	server    resolvedServer
	command   *exec.Cmd
	cancel    context.CancelFunc
	rpc       *rpcClient
	waitDone  chan struct{}
	syncKind  int

	mu        sync.Mutex
	documents map[string]documentState
}

type protocolLocation struct {
	URI   string `json:"uri"`
	Range Range  `json:"range"`
}

type protocolLocationLink struct {
	TargetURI            string `json:"targetUri"`
	TargetRange          Range  `json:"targetRange"`
	TargetSelectionRange Range  `json:"targetSelectionRange"`
}

func NewService(resolver WorkspaceResolver) *Service {
	return &Service{resolver: resolver, sessions: make(map[sessionKey]*session)}
}

func (s *Service) SyncDocument(ctx context.Context, input SyncDocumentInput) error {
	current, uri, languageID, err := s.documentSession(ctx, input.WorkspaceID, input.Path)
	if err != nil {
		return err
	}
	current.mu.Lock()
	defer current.mu.Unlock()
	return current.syncDocument(uri, languageID, input.Content)
}

func (s *Service) Definition(ctx context.Context, input DefinitionInput) ([]Location, error) {
	current, uri, languageID, err := s.documentSession(ctx, input.WorkspaceID, input.Path)
	if err != nil {
		return nil, err
	}
	current.mu.Lock()
	defer current.mu.Unlock()
	if err := current.syncDocument(uri, languageID, input.Content); err != nil {
		return nil, err
	}

	requestCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	var raw json.RawMessage
	if err := current.rpc.Request(requestCtx, "textDocument/definition", map[string]any{
		"textDocument": map[string]string{"uri": uri},
		"position":     input.Position,
	}, &raw); err != nil {
		return nil, err
	}
	locations, err := decodeDefinitionLocations(raw)
	if err != nil {
		return nil, err
	}
	return workspaceLocations(current.workspace.Path, locations), nil
}

func (s *Service) Detect(ctx context.Context, input DetectInput) (Capability, error) {
	workspace, err := s.resolver(ctx, strings.TrimSpace(input.WorkspaceID))
	if err != nil {
		return Capability{}, err
	}
	definition, languageID := identifyServer(input.Path)
	if definition == nil {
		return Capability{}, nil
	}
	capability := Capability{
		Recognized:  true,
		LanguageID:  languageID,
		ServerID:    definition.id,
		ServerName:  definition.name,
		InstallHint: definition.installHint,
	}
	if workspace.RemoteFS != nil {
		return capability, nil
	}
	resolved, available := resolveServer(workspace.Path, input.Path)
	if available {
		capability.Available = true
		capability.Command = resolved.candidate.command
	}
	return capability, nil
}

func (s *Service) CloseDocument(ctx context.Context, input CloseDocumentInput) error {
	workspace, err := s.resolver(ctx, strings.TrimSpace(input.WorkspaceID))
	if err != nil {
		return err
	}
	if workspace.RemoteFS != nil {
		return nil
	}
	definition, _ := identifyServer(input.Path)
	if definition == nil {
		return nil
	}
	_, uri, err := workspaceDocument(workspace.Path, input.Path)
	if err != nil {
		return err
	}
	s.mu.Lock()
	current := s.sessions[sessionKey{workspaceID: workspace.ID, serverID: definition.id}]
	s.mu.Unlock()
	if current == nil {
		return nil
	}
	current.mu.Lock()
	defer current.mu.Unlock()
	if _, ok := current.documents[uri]; !ok {
		return nil
	}
	delete(current.documents, uri)
	return current.rpc.Notify("textDocument/didClose", map[string]any{"textDocument": map[string]string{"uri": uri}})
}

func (s *Service) CloseWorkspace(_ context.Context, input CloseWorkspaceInput) error {
	workspaceID := strings.TrimSpace(input.WorkspaceID)
	if workspaceID == "" {
		return nil
	}
	s.mu.Lock()
	sessions := make([]*session, 0)
	for key, current := range s.sessions {
		if key.workspaceID == workspaceID {
			sessions = append(sessions, current)
			delete(s.sessions, key)
		}
	}
	s.mu.Unlock()
	for _, current := range sessions {
		current.close()
	}
	return nil
}

func (s *Service) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	sessions := make([]*session, 0, len(s.sessions))
	for _, current := range s.sessions {
		sessions = append(sessions, current)
	}
	s.sessions = make(map[sessionKey]*session)
	s.mu.Unlock()
	for _, current := range sessions {
		current.close()
	}
	return nil
}

func (s *Service) documentSession(ctx context.Context, workspaceID, path string) (*session, string, string, error) {
	workspace, err := s.resolver(ctx, strings.TrimSpace(workspaceID))
	if err != nil {
		return nil, "", "", err
	}
	if workspace.RemoteFS != nil {
		return nil, "", "", errors.New("LSP navigation is not available for Remote FS workspaces yet")
	}
	resolved, available := resolveServer(workspace.Path, path)
	if resolved.definition == nil {
		return nil, "", "", fmt.Errorf("no language server is registered for %s; editing remains available", displayFileType(path))
	}
	if !available {
		return nil, "", "", fmt.Errorf("%s is not available; %s", resolved.definition.name, resolved.definition.installHint)
	}
	_, uri, err := workspaceDocument(workspace.Path, path)
	if err != nil {
		return nil, "", "", err
	}
	current, err := s.getSession(ctx, workspace, resolved)
	return current, uri, resolved.languageID, err
}

func (s *Service) getSession(ctx context.Context, workspace domainworkspaces.Workspace, server resolvedServer) (*session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, errors.New("LSP service is closed")
	}
	key := sessionKey{workspaceID: workspace.ID, serverID: server.definition.id}
	if current := s.sessions[key]; current != nil && current.rpc.Err() == nil {
		return current, nil
	}
	if current := s.sessions[key]; current != nil {
		current.close()
		delete(s.sessions, key)
	}
	current, err := startSession(ctx, workspace, server)
	if err != nil {
		return nil, err
	}
	s.sessions[key] = current
	return current, nil
}

func startSession(parent context.Context, workspace domainworkspaces.Workspace, server resolvedServer) (*session, error) {
	processCtx, cancel := context.WithCancel(context.Background())
	command := exec.CommandContext(processCtx, server.binary, server.candidate.args...)
	command.Dir = workspace.Path
	stdin, err := command.StdinPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("open %s stdin: %w", server.definition.name, err)
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("open %s stdout: %w", server.definition.name, err)
	}
	stderr, err := command.StderrPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("open %s stderr: %w", server.definition.name, err)
	}
	if err := command.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("start %s: %w", server.definition.name, err)
	}
	go func() { _, _ = io.Copy(io.Discard, stderr) }()

	current := &session{
		workspace: workspace,
		server:    server,
		command:   command,
		cancel:    cancel,
		rpc:       newRPCClient(stdout, stdin),
		waitDone:  make(chan struct{}),
		documents: make(map[string]documentState),
	}
	go func() {
		_ = command.Wait()
		close(current.waitDone)
	}()

	rootURI, err := fileURI(workspace.Path)
	if err != nil {
		current.close()
		return nil, err
	}
	workspaceFolders := []map[string]string{{"uri": rootURI, "name": workspace.Name}}
	current.rpc.SetWorkspaceFolders(workspaceFolders)
	initializeCtx, initializeCancel := context.WithTimeout(parent, initializeTimeout)
	defer initializeCancel()
	var initializeResult struct {
		Capabilities struct {
			TextDocumentSync json.RawMessage `json:"textDocumentSync"`
		} `json:"capabilities"`
	}
	if err := current.rpc.Request(initializeCtx, "initialize", map[string]any{
		"processId":        os.Getpid(),
		"clientInfo":       map[string]string{"name": "OneCatch"},
		"rootUri":          rootURI,
		"workspaceFolders": workspaceFolders,
		"capabilities": map[string]any{
			"general":   map[string]any{"positionEncodings": []string{"utf-16"}},
			"workspace": map[string]any{"configuration": true, "workspaceFolders": true},
			"textDocument": map[string]any{
				"definition":      map[string]any{"linkSupport": true},
				"synchronization": map[string]any{"didSave": true},
			},
		},
	}, &initializeResult); err != nil {
		current.close()
		return nil, fmt.Errorf("initialize %s: %w", server.definition.name, err)
	}
	current.syncKind = textDocumentSyncKind(initializeResult.Capabilities.TextDocumentSync)
	if err := current.rpc.Notify("initialized", map[string]any{}); err != nil {
		current.close()
		return nil, err
	}
	return current, nil
}

func (s *session) syncDocument(uri, languageID, content string) error {
	state, opened := s.documents[uri]
	if !opened {
		state = documentState{content: content, version: 1}
		s.documents[uri] = state
		return s.rpc.Notify("textDocument/didOpen", map[string]any{
			"textDocument": map[string]any{"uri": uri, "languageId": languageID, "version": state.version, "text": content},
		})
	}
	if state.content == content {
		return nil
	}
	previous := state.content
	state.content = content
	state.version++
	s.documents[uri] = state
	change := map[string]any{"text": content}
	if s.syncKind == 2 {
		change = incrementalContentChange(previous, content)
	}
	return s.rpc.Notify("textDocument/didChange", map[string]any{
		"textDocument":   map[string]any{"uri": uri, "version": state.version},
		"contentChanges": []map[string]any{change},
	})
}

func textDocumentSyncKind(raw json.RawMessage) int {
	var kind int
	if json.Unmarshal(raw, &kind) == nil && kind != 0 {
		return kind
	}
	var options struct {
		Change int `json:"change"`
	}
	if json.Unmarshal(raw, &options) == nil && options.Change != 0 {
		return options.Change
	}
	return 1
}

func incrementalContentChange(before, after string) map[string]any {
	prefix := 0
	for prefix < len(before) && prefix < len(after) && before[prefix] == after[prefix] {
		prefix++
	}
	for prefix > 0 && prefix < len(before) && !utf8.RuneStart(before[prefix]) {
		prefix--
	}

	suffix := 0
	for suffix < len(before)-prefix && suffix < len(after)-prefix && before[len(before)-1-suffix] == after[len(after)-1-suffix] {
		suffix++
	}
	for suffix > 0 && !utf8.RuneStart(before[len(before)-suffix]) {
		suffix--
	}
	return map[string]any{
		"range": Range{
			Start: positionAtByte(before, prefix),
			End:   positionAtByte(before, len(before)-suffix),
		},
		"text": after[prefix : len(after)-suffix],
	}
}

func positionAtByte(content string, offset int) Position {
	if offset < 0 {
		offset = 0
	}
	if offset > len(content) {
		offset = len(content)
	}
	var position Position
	for _, character := range content[:offset] {
		if character == '\n' {
			position.Line++
			position.Character = 0
			continue
		}
		position.Character++
		if character > 0xffff {
			position.Character++
		}
	}
	return position
}

func displayFileType(path string) string {
	extension := strings.ToLower(filepath.Ext(strings.TrimSpace(path)))
	if extension != "" {
		return extension + " files"
	}
	return filepath.Base(path)
}

func (s *session) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	_ = s.rpc.Request(shutdownCtx, "shutdown", nil, nil)
	cancel()
	_ = s.rpc.Notify("exit", nil)
	s.cancel()
	select {
	case <-s.waitDone:
	case <-time.After(time.Second):
		if s.command.Process != nil {
			_ = s.command.Process.Kill()
		}
	}
}

func workspaceDocument(root, relative string) (string, string, error) {
	clean := filepath.Clean(filepath.FromSlash(strings.TrimSpace(relative)))
	if clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", "", errors.New("LSP document path is outside the workspace")
	}
	abs, err := filepath.Abs(filepath.Join(root, clean))
	if err != nil {
		return "", "", err
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", "", errors.New("LSP document path is outside the workspace")
	}
	uri, err := fileURI(abs)
	return abs, uri, err
}

func fileURI(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	slashed := filepath.ToSlash(abs)
	if runtime.GOOS == "windows" && len(slashed) >= 2 && slashed[1] == ':' {
		slashed = "/" + slashed
	}
	return (&url.URL{Scheme: "file", Path: slashed}).String(), nil
}

func decodeDefinitionLocations(raw json.RawMessage) ([]protocolLocation, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	items := []json.RawMessage{raw}
	if raw[0] == '[' {
		if err := json.Unmarshal(raw, &items); err != nil {
			return nil, fmt.Errorf("decode definition locations: %w", err)
		}
	}
	locations := make([]protocolLocation, 0, len(items))
	for _, item := range items {
		var probe struct {
			URI       string `json:"uri"`
			TargetURI string `json:"targetUri"`
		}
		if err := json.Unmarshal(item, &probe); err != nil {
			return nil, fmt.Errorf("decode definition location: %w", err)
		}
		if probe.TargetURI != "" {
			var link protocolLocationLink
			if err := json.Unmarshal(item, &link); err != nil {
				return nil, err
			}
			locations = append(locations, protocolLocation{URI: link.TargetURI, Range: link.TargetSelectionRange})
			continue
		}
		var location protocolLocation
		if err := json.Unmarshal(item, &location); err != nil {
			return nil, err
		}
		if location.URI != "" {
			locations = append(locations, location)
		}
	}
	return locations, nil
}

func workspaceLocations(root string, protocolLocations []protocolLocation) []Location {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return nil
	}
	if canonical, canonicalErr := filepath.EvalSymlinks(rootAbs); canonicalErr == nil {
		rootAbs = canonical
	}
	locations := make([]Location, 0, len(protocolLocations))
	for _, item := range protocolLocations {
		uri, err := url.Parse(item.URI)
		if err != nil || uri.Scheme != "file" {
			continue
		}
		target := filepath.FromSlash(uri.Path)
		if runtime.GOOS == "windows" && len(target) >= 3 && target[0] == filepath.Separator && target[2] == ':' {
			target = target[1:]
		}
		if canonical, canonicalErr := filepath.EvalSymlinks(target); canonicalErr == nil {
			target = canonical
		}
		relative, err := filepath.Rel(rootAbs, target)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			continue
		}
		locations = append(locations, Location{Path: filepath.ToSlash(relative), Range: item.Range})
	}
	return locations
}
