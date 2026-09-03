package skillmanager

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func skillSource(name, description string) string {
	return "---\nname: " + name + "\ndescription: " + description + "\n---\n\n# " + name + "\n\nDo the thing.\n"
}

func TestManagerCreatesUpdatesListsAndDeletesSkills(t *testing.T) {
	manager, err := New(filepath.Join(t.TempDir(), ".onecatch", "skills"))
	if err != nil {
		t.Fatal(err)
	}
	created, err := manager.Create(SaveSkillInput{Name: "release-notes", Content: skillSource("release-notes", "Write release notes")})
	if err != nil {
		t.Fatal(err)
	}
	if created.Name != "release-notes" || created.Description != "Write release notes" {
		t.Fatalf("unexpected created skill: %#v", created)
	}
	if _, err := os.Stat(filepath.Join(manager.Root(), "release-notes", "SKILL.md")); err != nil {
		t.Fatal(err)
	}

	updated, err := manager.Update(SaveSkillInput{Name: "release-notes", Content: skillSource("release-notes", "Updated description")})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Description != "Updated description" || updated.Digest == created.Digest {
		t.Fatalf("update did not refresh metadata: %#v", updated)
	}
	items, err := manager.List()
	if err != nil || len(items) != 1 || items[0].Name != "release-notes" {
		t.Fatalf("unexpected list: %#v, %v", items, err)
	}
	if err := manager.Delete("release-notes"); err != nil {
		t.Fatal(err)
	}
	items, err = manager.List()
	if err != nil || len(items) != 0 {
		t.Fatalf("expected empty list: %#v, %v", items, err)
	}
}

func TestManagerRejectsUnsafeAndMalformedSkills(t *testing.T) {
	manager, err := New(filepath.Join(t.TempDir(), "skills"))
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []SaveSkillInput{
		{Name: "../escape", Content: skillSource("escape", "bad")},
		{Name: "UPPER", Content: skillSource("UPPER", "bad")},
		{Name: "valid", Content: "# no frontmatter"},
		{Name: "valid", Content: "---\nname: other\ndescription: bad\n---\nbody"},
	} {
		if _, err := manager.Create(input); err == nil {
			t.Fatalf("expected input to fail: %#v", input)
		}
	}
}

func TestManagerListsOnlyFilesInsideSkillsRoot(t *testing.T) {
	base := t.TempDir()
	manager, err := New(filepath.Join(base, "skills"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Create(SaveSkillInput{Name: "release-notes", Content: skillSource("release-notes", "Write release notes")}); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(manager.Root(), "release-notes", "scripts"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(manager.Root(), "release-notes", "scripts", "render.sh"), []byte("#!/bin/sh\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(base, filepath.Join(manager.Root(), "outside")); err != nil {
		t.Fatal(err)
	}

	rootEntries, err := manager.ListFiles("")
	if err != nil {
		t.Fatal(err)
	}
	if len(rootEntries) != 1 || rootEntries[0].Path != "release-notes" || !rootEntries[0].Directory {
		t.Fatalf("unexpected root entries: %#v", rootEntries)
	}
	scriptEntries, err := manager.ListFiles("release-notes/scripts")
	if err != nil {
		t.Fatal(err)
	}
	if len(scriptEntries) != 1 || scriptEntries[0].Path != "release-notes/scripts/render.sh" || scriptEntries[0].Directory {
		t.Fatalf("unexpected script entries: %#v", scriptEntries)
	}
	for _, unsafe := range []string{"../", "release-notes/../../../"} {
		if _, err := manager.ListFiles(unsafe); err == nil {
			t.Fatalf("expected %q to be rejected", unsafe)
		}
	}
}

func TestManagerSyncsEachSkillWithRsyncAndRecordsMetadata(t *testing.T) {
	base := t.TempDir()
	manager, err := New(filepath.Join(base, ".onecatch", "skills"))
	if err != nil {
		t.Fatal(err)
	}
	manager.home = filepath.Join(base, "home")
	manager.now = func() time.Time { return time.Date(2026, 8, 31, 4, 5, 6, 0, time.UTC) }
	manager.lookPath = func(name string) (string, error) { return "/test/rsync", nil }
	var calls [][]string
	manager.run = func(_ context.Context, name string, args ...string) ([]byte, error) {
		calls = append(calls, append([]string{name}, args...))
		source, destination := args[len(args)-2], args[len(args)-1]
		if err := copyTreeForTest(strings.TrimSuffix(source, string(filepath.Separator)), strings.TrimSuffix(destination, string(filepath.Separator))); err != nil {
			return nil, err
		}
		return []byte("copied"), nil
	}
	if _, err := manager.Create(SaveSkillInput{Name: "helper", Content: skillSource("helper", "Help with a task")}); err != nil {
		t.Fatal(err)
	}
	targetPath := filepath.Join(base, "agent", "skills")
	target, err := manager.AddTarget(AddTargetInput{Name: "Test Agent", Path: targetPath})
	if err != nil {
		t.Fatal(err)
	}
	result, err := manager.Sync(context.Background(), target.ID)
	if err != nil {
		t.Fatal(err)
	}
	if result.SyncedSkills != 1 || result.Target.Status != "synced" {
		t.Fatalf("unexpected sync result: %#v", result)
	}
	want := []string{"/test/rsync", "-a", "--delete", filepath.Join(manager.Root(), "helper") + string(filepath.Separator), filepath.Join(targetPath, "helper") + string(filepath.Separator)}
	if !reflect.DeepEqual(calls, [][]string{want}) {
		t.Fatalf("unexpected rsync invocation: %#v", calls)
	}
	var metadata metadataFile
	if err := localfileReadJSON(manager.metadataPath, &metadata); err != nil {
		t.Fatal(err)
	}
	if got := metadata.Targets[target.ID]; got.LastSyncedAt != manager.now() || got.Skills["helper"] == "" {
		t.Fatalf("metadata not recorded: %#v", got)
	}
}

func TestManagerScansMissingTargetsAndRsyncAvailability(t *testing.T) {
	base := t.TempDir()
	manager, err := New(filepath.Join(base, "skills"))
	if err != nil {
		t.Fatal(err)
	}
	manager.home = filepath.Join(base, "home")
	manager.lookPath = func(string) (string, error) { return "", os.ErrNotExist }
	targets, err := manager.ScanTargets()
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 3 {
		t.Fatalf("expected built-in targets, got %#v", targets)
	}
	for _, target := range targets {
		if target.Status != "rsync-unavailable" || target.RsyncAvailable {
			t.Fatalf("unexpected target state: %#v", target)
		}
	}
}

func copyTreeForTest(source, destination string) error {
	if err := os.RemoveAll(destination); err != nil {
		return err
	}
	return filepath.WalkDir(source, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o600)
	})
}

// Kept local so tests exercise the on-disk representation without reaching
// into the manager's read path.
func localfileReadJSON(path string, target any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

func TestManagerReadsAndWritesSkillResources(t *testing.T) {
	manager, err := New(filepath.Join(t.TempDir(), ".onecatch", "skills"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Create(SaveSkillInput{Name: "release-notes", Content: skillSource("release-notes", "Write release notes")}); err != nil {
		t.Fatal(err)
	}
	references := filepath.Join(manager.Root(), "release-notes", "references")
	if err := os.MkdirAll(references, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(references, "style.md"), []byte("# Style\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	file, err := manager.ReadFile("release-notes/references/style.md")
	if err != nil {
		t.Fatal(err)
	}
	if file.Name != "style.md" || file.Content != "# Style\n" {
		t.Fatalf("unexpected file: %#v", file)
	}

	saved, err := manager.WriteFile("release-notes/references/style.md", "# Style\n\nPrefer the imperative mood.\n")
	if err != nil {
		t.Fatal(err)
	}
	if saved.SizeBytes != int64(len(saved.Content)) {
		t.Fatalf("size does not match written content: %#v", saved)
	}
	reread, err := manager.ReadFile("release-notes/references/style.md")
	if err != nil {
		t.Fatal(err)
	}
	if reread.Content != saved.Content {
		t.Fatalf("write did not land on disk: %q", reread.Content)
	}
}

func TestManagerGuardsSkillFileEdits(t *testing.T) {
	manager, err := New(filepath.Join(t.TempDir(), ".onecatch", "skills"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Create(SaveSkillInput{Name: "release-notes", Content: skillSource("release-notes", "Write release notes")}); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(filepath.Dir(manager.Root()), "escape.md")
	if err := os.WriteFile(outside, []byte("secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{"", "../escape.md", "release-notes/missing.md", "release-notes"} {
		if _, err := manager.ReadFile(path); err == nil {
			t.Fatalf("expected ReadFile(%q) to fail", path)
		}
		if _, err := manager.WriteFile(path, "x"); err == nil {
			t.Fatalf("expected WriteFile(%q) to fail", path)
		}
	}

	// A skill's own SKILL.md keeps the frontmatter contract the library relies
	// on, even when it is edited as a plain file rather than through Update.
	if _, err := manager.WriteFile("release-notes/SKILL.md", "no frontmatter here\n"); err == nil {
		t.Fatal("expected SKILL.md without frontmatter to be rejected")
	}
	if _, err := manager.WriteFile("release-notes/SKILL.md", skillSource("release-notes", "Rewritten through the file editor")); err != nil {
		t.Fatal(err)
	}
	document, err := manager.Get("release-notes")
	if err != nil {
		t.Fatal(err)
	}
	if document.Description != "Rewritten through the file editor" {
		t.Fatalf("SKILL.md edit did not refresh the summary: %#v", document.Skill)
	}

	binary := filepath.Join(manager.Root(), "release-notes", "logo.bin")
	if err := os.WriteFile(binary, []byte{0x00, 0x01, 0x02}, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.ReadFile("release-notes/logo.bin"); err == nil {
		t.Fatal("expected a binary file to be refused")
	}
}

func TestManagerSyncsOnlyTheSelectedSkills(t *testing.T) {
	manager, err := New(filepath.Join(t.TempDir(), ".onecatch", "skills"))
	if err != nil {
		t.Fatal(err)
	}
	manager.lookPath = func(string) (string, error) { return "/usr/bin/rsync", nil }
	var copied []string
	manager.run = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		source := args[len(args)-2]
		destination := args[len(args)-1]
		copied = append(copied, filepath.Base(filepath.Clean(source)))
		if err := os.MkdirAll(destination, 0o700); err != nil {
			return nil, err
		}
		entries, err := os.ReadDir(filepath.Clean(source))
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			data, readErr := os.ReadFile(filepath.Join(filepath.Clean(source), entry.Name()))
			if readErr != nil {
				return nil, readErr
			}
			if err := os.WriteFile(filepath.Join(filepath.Clean(destination), entry.Name()), data, 0o600); err != nil {
				return nil, err
			}
		}
		return nil, nil
	}
	for _, name := range []string{"alpha", "beta", "gamma"} {
		if _, err := manager.Create(SaveSkillInput{Name: name, Content: skillSource(name, "Do "+name)}); err != nil {
			t.Fatal(err)
		}
	}

	destination := filepath.Join(t.TempDir(), "agent", "skills")
	target, err := manager.AddTarget(AddTargetInput{Name: "Agent", Path: destination})
	if err != nil {
		t.Fatal(err)
	}
	if target.TotalSkills != 3 || len(target.Skills) != 0 {
		t.Fatalf("a new target receives the whole library: %#v", target)
	}

	narrowed, err := manager.SetTargetSkills(SetTargetSkillsInput{ID: target.ID, Skills: []string{"alpha", "gamma", "ghost", "alpha"}})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(narrowed.Skills, []string{"alpha", "gamma"}) {
		t.Fatalf("unknown and duplicate names must not be stored: %#v", narrowed.Skills)
	}
	if narrowed.TotalSkills != 2 || narrowed.LibrarySkills != 3 {
		t.Fatalf("a target counts its own subset against the library: %#v", narrowed)
	}

	result, err := manager.Sync(context.Background(), target.ID)
	if err != nil {
		t.Fatal(err)
	}
	if result.SyncedSkills != 2 {
		t.Fatalf("only the selection is copied: %d", result.SyncedSkills)
	}
	if !reflect.DeepEqual(copied, []string{"alpha", "gamma"}) {
		t.Fatalf("beta must not be copied: %#v", copied)
	}
	if _, err := os.Stat(filepath.Join(destination, "beta")); !os.IsNotExist(err) {
		t.Fatal("an unselected skill must not appear at the target")
	}
	if result.Target.Status != "synced" {
		t.Fatalf("a fully copied selection is synced, not partial: %q", result.Target.Status)
	}

	// Clearing the selection restores the whole library.
	restored, err := manager.SetTargetSkills(SetTargetSkillsInput{ID: target.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(restored.Skills) != 0 || restored.TotalSkills != 3 {
		t.Fatalf("an empty selection means everything: %#v", restored)
	}
}

func TestManagerRepointsBuiltinAndCustomTargets(t *testing.T) {
	manager, err := New(filepath.Join(t.TempDir(), ".onecatch", "skills"))
	if err != nil {
		t.Fatal(err)
	}
	moved := filepath.Join(t.TempDir(), "elsewhere", "skills")
	updated, err := manager.UpdateTarget(UpdateTargetInput{ID: "codex", Path: moved})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Path != moved || !updated.Builtin {
		t.Fatalf("a built-in target keeps its identity and takes the new path: %#v", updated)
	}
	targets, err := manager.ScanTargets()
	if err != nil {
		t.Fatal(err)
	}
	codex, ok := findSyncTarget(targets, "codex")
	if !ok || codex.Path != moved {
		t.Fatalf("the override must survive a rescan: %#v", targets)
	}
	if len(targets) != 3 {
		t.Fatalf("repointing a built-in must not add a second entry: %d targets", len(targets))
	}

	// The same validation an added target gets applies to a repointed one.
	if _, err := manager.UpdateTarget(UpdateTargetInput{ID: "codex", Path: manager.Root()}); err == nil {
		t.Fatal("a target inside the library must be refused")
	}
	if _, err := manager.UpdateTarget(UpdateTargetInput{ID: "codex", Path: "relative/path"}); err == nil {
		t.Fatal("a relative target must be refused")
	}
	if _, err := manager.UpdateTarget(UpdateTargetInput{ID: "missing", Path: moved}); err == nil {
		t.Fatal("an unknown target must be refused")
	}
}

func findSyncTarget(targets []SyncTarget, id string) (SyncTarget, bool) {
	for _, target := range targets {
		if target.ID == id {
			return target, true
		}
	}
	return SyncTarget{}, false
}
