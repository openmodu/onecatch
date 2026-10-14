package internal_test

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Check source imports (including platform-specific files) without starting a
// desktop or requiring the native graphics toolchain.
func TestDependencyBoundaries(t *testing.T) {
	const module = "github.com/openmodu/onecatch/internal/"
	forbidden := map[string][]string{
		"platform":  {"app/", "transport/", "service/", "usecase/", "repo/", "domain/"},
		"domain":    {"app/", "transport/", "service/", "usecase/", "repo/"},
		"usecase":   {"app/", "transport/", "service/", "repo/"},
		"repo":      {"app/", "transport/", "service/", "usecase/"},
		"service":   {"app/", "transport/"},
		"transport": {"app/"},
	}
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel := filepath.ToSlash(path)
		layer := strings.Split(rel, "/")[0]
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, spec := range file.Imports {
			dependency, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				return err
			}
			if layer != "app" && strings.HasPrefix(dependency, "github.com/dyike/keel/") {
				t.Errorf("%s imports UI toolkit %s", rel, dependency)
			}
			for _, prefix := range forbidden[layer] {
				if strings.HasPrefix(dependency, module+prefix) {
					t.Errorf("%s crosses %s boundary: %s", rel, layer, dependency)
				}
			}
			if strings.HasPrefix(rel, "app/keel/assets/") || strings.HasPrefix(rel, "app/keel/terminalview/") {
				if strings.HasPrefix(dependency, module) {
					t.Errorf("UI component %s imports application dependency %s", rel, dependency)
				}
			}
			// backend.go is the desktop composition root, not a view/controller.
			if strings.HasPrefix(rel, "app/keel/") && rel != "app/keel/backend.go" && strings.HasPrefix(dependency, module+"repo/") {
				t.Errorf("UI %s imports repository %s", rel, dependency)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
