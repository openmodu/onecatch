package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	domainworkspaces "github.com/openmodu/onecatch/internal/domain/workspaces"
)

func TestRPCClientRequest(t *testing.T) {
	serverReader, clientWriter := io.Pipe()
	clientReader, serverWriter := io.Pipe()
	client := newRPCClient(clientReader, clientWriter)

	go func() {
		payload, err := readFrame(bufio.NewReader(serverReader))
		if err != nil {
			return
		}
		var request rpcMessage
		if json.Unmarshal(payload, &request) != nil {
			return
		}
		response, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "result": map[string]string{"value": "ok"}})
		_, _ = serverWriter.Write([]byte("Content-Length: " + stringInt(len(response)) + "\r\n\r\n"))
		_, _ = serverWriter.Write(response)
	}()

	var result map[string]string
	if err := client.Request(context.Background(), "test/method", map[string]string{"input": "x"}, &result); err != nil {
		t.Fatal(err)
	}
	if result["value"] != "ok" {
		t.Fatalf("result = %#v", result)
	}
}

func TestDefinitionLocationShapesAndWorkspaceFiltering(t *testing.T) {
	root := t.TempDir()
	inside := filepath.Join(root, "pkg", "hello.go")
	if err := os.MkdirAll(filepath.Dir(inside), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(inside, []byte("package pkg\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	insideURI := (&url.URL{Scheme: "file", Path: filepath.ToSlash(inside)}).String()
	raw := json.RawMessage(`[
		{"uri":` + quoted(insideURI) + `,"range":{"start":{"line":2,"character":3},"end":{"line":2,"character":8}}},
		{"targetUri":` + quoted(insideURI) + `,"targetRange":{"start":{"line":4,"character":0},"end":{"line":4,"character":9}},"targetSelectionRange":{"start":{"line":4,"character":5},"end":{"line":4,"character":9}}},
		{"uri":"file:///outside.go","range":{"start":{"line":0,"character":0},"end":{"line":0,"character":1}}}
	]`)
	decoded, err := decodeDefinitionLocations(raw)
	if err != nil {
		t.Fatal(err)
	}
	locations := workspaceLocations(root, decoded)
	if len(locations) != 2 {
		t.Fatalf("locations = %#v", locations)
	}
	if locations[0].Path != "pkg/hello.go" || locations[0].Range.Start.Line != 2 {
		t.Fatalf("first location = %#v", locations[0])
	}
	if locations[1].Range.Start.Character != 5 {
		t.Fatalf("location link did not use targetSelectionRange: %#v", locations[1])
	}
}

func TestIdentifyServer(t *testing.T) {
	tests := []struct {
		path       string
		serverID   string
		languageID string
	}{
		{path: "main.go", serverID: "go", languageID: "go"},
		{path: "frontend/App.tsx", serverID: "typescript", languageID: "typescriptreact"},
		{path: "Dockerfile", serverID: "docker", languageID: "dockerfile"},
		{path: "docker/Dockerfile.dev", serverID: "docker", languageID: "dockerfile"},
		{path: "Gemfile", serverID: "ruby", languageID: "ruby"},
		{path: "go.mod", serverID: "go", languageID: "go.mod"},
		{path: "CMakeLists.txt", serverID: "cmake", languageID: "cmake"},
		{path: "infra/main.tf.json", serverID: "terraform", languageID: "terraform"},
		{path: "schema.yaml", serverID: "yaml", languageID: "yaml"},
		{path: "README.unknown"},
	}
	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			definition, languageID := identifyServer(test.path)
			if test.serverID == "" {
				if definition != nil || languageID != "" {
					t.Fatalf("identifyServer(%q) = %#v, %q", test.path, definition, languageID)
				}
				return
			}
			if definition == nil || definition.id != test.serverID || languageID != test.languageID {
				t.Fatalf("identifyServer(%q) = %#v, %q", test.path, definition, languageID)
			}
		})
	}
}

func TestServerRegistryHasUniqueFileMappings(t *testing.T) {
	serverIDs := make(map[string]bool)
	extensions := make(map[string]string)
	filenames := make(map[string]string)
	for _, definition := range serverRegistry {
		if definition.id == "" || definition.name == "" || len(definition.candidates) == 0 {
			t.Fatalf("incomplete server definition: %#v", definition)
		}
		if serverIDs[definition.id] {
			t.Fatalf("duplicate server ID %q", definition.id)
		}
		serverIDs[definition.id] = true
		for extension := range definition.languages {
			if existing := extensions[extension]; existing != "" {
				t.Fatalf("extension %q is mapped by both %s and %s", extension, existing, definition.id)
			}
			extensions[extension] = definition.id
		}
		for filename := range definition.filenames {
			if existing := filenames[filename]; existing != "" {
				t.Fatalf("filename %q is mapped by both %s and %s", filename, existing, definition.id)
			}
			filenames[filename] = definition.id
		}
	}
}

func TestDetectProbesPATHWithoutStartingServer(t *testing.T) {
	bin := t.TempDir()
	launcher := filepath.Join(bin, "typescript-language-server")
	if err := os.WriteFile(launcher, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	workspace := domainworkspaces.Workspace{ID: "test", Name: "test", Path: t.TempDir()}
	service := NewService(func(context.Context, string) (domainworkspaces.Workspace, error) { return workspace, nil })
	capability, err := service.Detect(context.Background(), DetectInput{WorkspaceID: workspace.ID, Path: "App.tsx"})
	if err != nil {
		t.Fatal(err)
	}
	if !capability.Recognized || !capability.Available || capability.ServerID != "typescript" || capability.LanguageID != "typescriptreact" || capability.Command != "typescript-language-server" {
		t.Fatalf("capability = %#v", capability)
	}
	localBin := filepath.Join(workspace.Path, "node_modules", ".bin")
	if err := os.MkdirAll(localBin, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(localBin, "yaml-language-server"), []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	local, err := service.Detect(context.Background(), DetectInput{WorkspaceID: workspace.ID, Path: "config.yaml"})
	if err != nil {
		t.Fatal(err)
	}
	if !local.Available || local.Command != "yaml-language-server" {
		t.Fatalf("local capability = %#v", local)
	}
	missing, err := service.Detect(context.Background(), DetectInput{WorkspaceID: workspace.ID, Path: "main.go"})
	if err != nil {
		t.Fatal(err)
	}
	if !missing.Recognized || missing.Available || missing.InstallHint == "" {
		t.Fatalf("missing capability = %#v", missing)
	}
	unsupported, err := service.Detect(context.Background(), DetectInput{WorkspaceID: workspace.ID, Path: "notes.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if unsupported.Recognized || unsupported.Available {
		t.Fatalf("unsupported capability = %#v", unsupported)
	}
}

func TestTextDocumentSyncKind(t *testing.T) {
	if kind := textDocumentSyncKind(json.RawMessage(`2`)); kind != 2 {
		t.Fatalf("numeric sync kind = %d", kind)
	}
	if kind := textDocumentSyncKind(json.RawMessage(`{"openClose":true,"change":2}`)); kind != 2 {
		t.Fatalf("options sync kind = %d", kind)
	}
	if kind := textDocumentSyncKind(nil); kind != 1 {
		t.Fatalf("default sync kind = %d", kind)
	}
}

func TestIncrementalContentChangeUsesUTF16Range(t *testing.T) {
	change := incrementalContentChange("first\n😀 old value\n", "first\n😀 new value\n")
	rangeValue, ok := change["range"].(Range)
	if !ok {
		t.Fatalf("range = %#v", change["range"])
	}
	if rangeValue.Start != (Position{Line: 1, Character: 3}) || rangeValue.End != (Position{Line: 1, Character: 6}) {
		t.Fatalf("range = %#v", rangeValue)
	}
	if change["text"] != "new" {
		t.Fatalf("text = %#v", change["text"])
	}
}

func TestDefinitionWithGopls(t *testing.T) {
	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip("gopls is not installed")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.test/navigation\n\ngo 1.24\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	source := "package navigation\n\nfunc Hello() {}\n\nfunc call() { Hello() }\n"
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	workspace := domainworkspaces.Workspace{ID: "test", Name: "test", Path: root}
	service := NewService(func(context.Context, string) (domainworkspaces.Workspace, error) { return workspace, nil })
	defer service.Close()
	locations, err := service.Definition(context.Background(), DefinitionInput{
		WorkspaceID: workspace.ID,
		Path:        "main.go",
		Content:     source,
		Position:    Position{Line: 4, Character: 15},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(locations) != 1 || locations[0].Path != "main.go" || locations[0].Range.Start.Line != 2 {
		t.Fatalf("locations = %#v", locations)
	}

	unsaved := "package navigation\n\nfunc Renamed() {}\n\nfunc call() { Renamed() }\n"
	locations, err = service.Definition(context.Background(), DefinitionInput{
		WorkspaceID: workspace.ID,
		Path:        "main.go",
		Content:     unsaved,
		Position:    Position{Line: 4, Character: 17},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(locations) != 1 || locations[0].Range.Start.Line != 2 || locations[0].Range.Start.Character != 5 {
		t.Fatalf("unsaved locations = %#v", locations)
	}
	if err := service.CloseWorkspace(context.Background(), CloseWorkspaceInput{WorkspaceID: workspace.ID}); err != nil {
		t.Fatal(err)
	}
	if len(service.sessions) != 0 {
		t.Fatalf("sessions remained after closing workspace: %#v", service.sessions)
	}
}

func TestDefinitionWithClangd(t *testing.T) {
	if _, err := exec.LookPath("clangd"); err != nil {
		t.Skip("clangd is not installed")
	}
	root := t.TempDir()
	source := "int target() { return 1; }\nint main() { return target(); }\n"
	if err := os.WriteFile(filepath.Join(root, "main.cpp"), []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	workspace := domainworkspaces.Workspace{ID: "clangd-test", Name: "clangd-test", Path: root}
	service := NewService(func(context.Context, string) (domainworkspaces.Workspace, error) { return workspace, nil })
	defer service.Close()
	locations, err := service.Definition(context.Background(), DefinitionInput{
		WorkspaceID: workspace.ID,
		Path:        "main.cpp",
		Content:     source,
		Position:    Position{Line: 1, Character: 22},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(locations) != 1 || locations[0].Path != "main.cpp" || locations[0].Range.Start.Line != 0 || locations[0].Range.Start.Character != 4 {
		t.Fatalf("locations = %#v", locations)
	}
}

func quoted(value string) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

func stringInt(value int) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}
