package keeldesktop

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/kit"
	"github.com/dyike/keel/ui/locale"
	"github.com/dyike/keel/ui/window"
	desktopservice "github.com/openmodu/onecatch/internal/service/desktop"
	"github.com/openmodu/onecatch/internal/service/skillmanager"
)

type skillFixtureService struct{ manager *skillmanager.Manager }

func (s *skillFixtureService) ListManagedSkills() ([]skillmanager.Skill, error) {
	return s.manager.List()
}
func (s *skillFixtureService) GetSkill(name string) (skillmanager.SkillDocument, error) {
	return s.manager.Get(name)
}
func (s *skillFixtureService) CreateSkill(input skillmanager.SaveSkillInput) (skillmanager.SkillDocument, error) {
	return s.manager.Create(input)
}
func (s *skillFixtureService) DeleteSkill(name string) error { return s.manager.Delete(name) }
func (s *skillFixtureService) ListSkillFiles(dir string) ([]skillmanager.SkillFileEntry, error) {
	return s.manager.ListFiles(dir)
}
func (s *skillFixtureService) ReadSkillFile(name string) (skillmanager.SkillFileContent, error) {
	return s.manager.ReadFile(name)
}
func (s *skillFixtureService) WriteSkillFile(input skillmanager.SaveSkillFileInput) (skillmanager.SkillFileContent, error) {
	return s.manager.WriteFile(input.Path, input.Content)
}
func (s *skillFixtureService) ScanSkillSyncTargets() ([]skillmanager.SyncTarget, error) {
	return nil, nil
}

func skillTestApp(t *testing.T) (*app, *skillFixtureService) {
	t.Helper()
	a := dagTestApp(t)
	icon, err := png.Decode(bytes.NewReader(appIcon))
	if err != nil {
		t.Fatal(err)
	}
	a.logo = kit.Image(icon, "OneCatch").Size(22, 22).Rounded(5)
	manager, err := skillmanager.New(filepath.Join(t.TempDir(), "skills"))
	if err != nil {
		t.Fatal(err)
	}
	api := &skillFixtureService{manager}
	a.skills.api = api
	a.page = "skills"
	return a, api
}
func TestSkillCreateTemplateAndNativeDelete(t *testing.T) {
	a, api := skillTestApp(t)
	doc, err := api.CreateSkill(skillmanager.SaveSkillInput{Name: "release-notes", Content: newManagedSkillTemplate("release-notes", "版本: \"桌面版\"\n 检查发布变化")})
	if err != nil || doc.Description != "版本: \"桌面版\" 检查发布变化" {
		t.Fatalf("YAML description: %q %v", doc.Description, err)
	}
	a.library.SkillName = "release-notes"
	a.library.SkillDoc = &doc
	a.confirmDeleteSkill()
	root := el.Root(a)
	var router input.Router
	var ops op.Ops
	frame := func() {
		ops.Reset()
		root.Layout(layout.Context{Ops: &ops, Now: time.Now(), Source: router.Source(), Constraints: layout.Exact(image.Pt(1120, 760)), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}})
		router.Frame(&ops)
	}
	frame()
	box, ok := findSkillControl(router.AppendSemantics(nil), "删除")
	if !ok {
		t.Fatal("native delete confirmation omitted action")
	}
	pos := f32.Pt(float32(box.Min.X+box.Dx()/2), float32(box.Min.Y+box.Dy()/2))
	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: pos})
	frame()
	router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: pos})
	frame()
	if a.skills.Busy != "delete" {
		t.Fatal("native confirmation did not serialize deletion")
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		_, err := api.GetSkill(doc.Name)
		if err != nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("native deletion did not remove temp Skill")
		}
		time.Sleep(time.Millisecond * 5)
	}
}
func findSkillControl(nodes []input.SemanticNode, name string) (image.Rectangle, bool) {
	for _, n := range nodes {
		if n.Desc.Label == name && !n.Desc.Disabled && n.Desc.Gestures&input.ClickGesture != 0 {
			return n.Desc.Bounds, true
		}
		if box, ok := findSkillControl(n.Children, name); ok {
			return box, true
		}
	}
	return image.Rectangle{}, false
}
func TestSkillFileSavePreservesNewDraftAndFailure(t *testing.T) {
	a, api := skillTestApp(t)
	doc, err := api.CreateSkill(skillmanager.SaveSkillInput{Name: "review", Content: newManagedSkillTemplate("review", "Review changes")})
	if err != nil {
		t.Fatal(err)
	}
	a.library.SkillName = "review"
	a.library.SkillDoc = &doc
	file, err := api.ReadSkillFile("review/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	b := &skillFileBuffer{File: file, Draft: file.Content + "\nSaved paragraph.\n", Saving: true, Version: 1}
	a.skills.Buffers[file.Path] = b
	saved, err := api.WriteSkillFile(skillmanager.SaveSkillFileInput{Path: file.Path, Content: b.Draft})
	if err != nil {
		t.Fatal(err)
	}
	newer := b.Draft + "\nTyped during save.\n"
	b.Draft = newer
	updated, err := api.GetSkill("review")
	if err != nil {
		t.Fatal(err)
	}
	a.applySkillFileSave(file.Path, b, 1, saved, &updated, nil)
	if b.Saving || b.Draft != newer || b.File.Content != saved.Content || a.skillPreviewContent() != newer {
		t.Fatal("save replaced a newer draft or lost the live preview")
	}
	b.Saving = true
	b.Version++
	a.applySkillFileSave(file.Path, b, 2, skillmanager.SkillFileContent{}, nil, errors.New("validation failed"))
	if b.Saving || b.Error == "" || b.Draft != newer || b.File.Content != saved.Content {
		t.Fatal("failed save lost retryable draft")
	}
	replacement := &skillFileBuffer{Draft: "new buffer"}
	a.skills.Buffers[file.Path] = replacement
	a.applySkillFileSave(file.Path, b, 2, saved, &doc, nil)
	if replacement.Draft != "new buffer" {
		t.Fatal("late save modified replacement buffer")
	}
	if _, err := api.WriteSkillFile(skillmanager.SaveSkillFileInput{Path: "../outside.txt", Content: "escaped"}); err == nil {
		t.Fatal("file inspector escaped managed root")
	}
}
func TestSkillDocumentRejectsLateSelection(t *testing.T) {
	a, _ := skillTestApp(t)
	a.library.SkillName = "new"
	a.library.SkillVersion = 5
	a.applySkillDocument(4, "old", skillmanager.SkillDocument{Skill: skillmanager.Skill{Name: "old"}}, nil)
	if a.library.SkillDoc != nil {
		t.Fatal("previous selection replaced current Skill")
	}
	a.applySkillDocument(5, "new", skillmanager.SkillDocument{Skill: skillmanager.Skill{Name: "new"}, Content: "# New"}, nil)
	if a.library.SkillDoc == nil || a.library.SkillDoc.Name != "new" {
		t.Fatal("current selection ignored")
	}
	metadata, body := parseManagedSkill("---\r\nname: new\r\ndescription: \"版本: 变化\"\r\nlicense: MIT\r\n---\r\n\r\n# New\r\n")
	if metadata["description"] != "版本: 变化" || metadata["license"] != "MIT" || strings.Contains(body, "description:") || !strings.Contains(body, "# New") {
		t.Fatal("frontmatter leaked into Markdown preview")
	}
}

type skillTargetFixture struct{ *skillFixtureService }

func (s *skillTargetFixture) ScanSkillSyncTargets() ([]skillmanager.SyncTarget, error) {
	return s.manager.ScanTargets()
}
func (s *skillTargetFixture) AddSkillSyncTarget(input skillmanager.AddTargetInput) (skillmanager.SyncTarget, error) {
	return s.manager.AddTarget(input)
}
func (s *skillTargetFixture) UpdateSkillSyncTarget(input skillmanager.UpdateTargetInput) (skillmanager.SyncTarget, error) {
	return s.manager.UpdateTarget(input)
}
func (s *skillTargetFixture) RemoveSkillSyncTarget(id string) error {
	return s.manager.RemoveTarget(id)
}
func TestSkillTargetSettingsUsesManagedMetadata(t *testing.T) {
	a, base := skillTestApp(t)
	api := &skillTargetFixture{base}
	p := workerTestSettings(a)
	p.section = "skills"
	p.skillTargets.api = api
	p.initSkillTargetSettings()
	s := &p.skillTargets
	s.Name = "Temporary Agent"
	s.Path = filepath.Join(t.TempDir(), "agent-skills")
	p.mutateSkillTarget("add", "")
	var target skillmanager.SyncTarget
	deadline := time.Now().Add(2 * time.Second)
	for {
		items, err := api.ScanSkillSyncTargets()
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range items {
			if item.Name == s.Name {
				target = item
			}
		}
		if target.ID != "" {
			p.applySkillTargets(s.Version, items, nil)
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("new target did not reach metadata")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if target.Builtin || target.Path != s.Path {
		t.Fatal("custom target identity/path mismatch")
	}
	next := filepath.Join(t.TempDir(), "next-skills")
	*s.Paths[target.ID] = next
	p.commitSkillTargetPath(target.ID)
	deadline = time.Now().Add(2 * time.Second)
	for {
		items, err := api.ScanSkillSyncTargets()
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range items {
			if item.ID == target.ID {
				target = item
			}
		}
		if target.Path == next {
			p.applySkillTargets(s.Version, items, nil)
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("path change did not persist")
		}
		time.Sleep(5 * time.Millisecond)
	}
	p.mutateSkillTarget("remove", target.ID)
	deadline = time.Now().Add(2 * time.Second)
	for {
		items, err := api.ScanSkillSyncTargets()
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, item := range items {
			found = found || item.ID == target.ID
		}
		if !found {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("custom target configuration was not removed")
		}
		time.Sleep(5 * time.Millisecond)
	}
	s.Version++
	s.Busy = "sentinel"
	p.applySkillTargets(s.Version-1, nil, nil)
	if s.Busy != "sentinel" {
		t.Fatal("late target reply reset current request")
	}
	p.workers.closed = true
	p.applySkillTargets(s.Version, nil, nil)
	if s.Busy != "sentinel" {
		t.Fatal("closed settings accepted target reply")
	}
}

type skillDebugFixture struct{}

func (skillDebugFixture) DebugSkill(context.Context, desktopservice.SkillDebugInput) (desktopservice.SkillDebugResult, error) {
	return desktopservice.SkillDebugResult{}, errors.New("configure Modu")
}
func (skillDebugFixture) StopSkillDebug(string)                                     {}
func (skillDebugFixture) SetSkillDebugEmitter(func(desktopservice.SkillDebugFrame)) {}
func (skillDebugFixture) ListSkillDebugRuns(string) ([]desktopservice.SkillDebugRecord, error) {
	return nil, nil
}
func (skillDebugFixture) ClearSkillDebugRuns(string) error { return nil }
func TestSkillDebugFramesAndRetry(t *testing.T) {
	a, _ := skillTestApp(t)
	d := &a.skills.Debug
	d.api = skillDebugFixture{}
	d.Name = "review"
	d.RunID = "run-1"
	d.Running = true
	d.Prompt = "Review safely"
	a.applySkillDebugFrame(desktopservice.SkillDebugFrame{RunID: "run-2", Index: 0, Event: desktopservice.SkillDebugEvent{Text: "wrong"}})
	if len(d.Events) != 0 {
		t.Fatal("other run leaked into transcript")
	}
	a.applySkillDebugFrame(desktopservice.SkillDebugFrame{RunID: "run-1", Index: 0, Event: desktopservice.SkillDebugEvent{Kind: "message", Text: "partial", Streaming: true}})
	a.applySkillDebugFrame(desktopservice.SkillDebugFrame{RunID: "run-1", Index: 0, Event: desktopservice.SkillDebugEvent{Kind: "message", Text: "final"}})
	if len(d.Events) != 1 || d.Events[0].Text != "final" {
		t.Fatal("token frames duplicated messages")
	}
	a.finishSkillDebug("run-2", desktopservice.SkillDebugResult{}, nil)
	if !d.Running {
		t.Fatal("stale completion stopped current run")
	}
	a.library.SkillName = "review"
	a.finishSkillDebug("run-1", desktopservice.SkillDebugResult{}, errors.New("configure Modu"))
	if d.Running || d.Error == "" || d.Prompt != "Review safely" || len(d.Events) != 1 {
		t.Fatal("failed run lost prompt or partial output")
	}
}

type controlledSkillDebug struct {
	skillDebugFixture
	started chan desktopservice.SkillDebugInput
	stopped chan string
	release chan struct{}
}

func (s *controlledSkillDebug) DebugSkill(ctx context.Context, input desktopservice.SkillDebugInput) (desktopservice.SkillDebugResult, error) {
	s.started <- input
	select {
	case <-s.release:
		return desktopservice.SkillDebugResult{Stopped: true, Output: "partial"}, nil
	case <-ctx.Done():
		return desktopservice.SkillDebugResult{}, ctx.Err()
	}
}
func (s *controlledSkillDebug) StopSkillDebug(id string) { s.stopped <- id; close(s.release) }
func TestSkillDebugShortcutAndStopUseCurrentRun(t *testing.T) {
	a, _ := skillTestApp(t)
	api := &controlledSkillDebug{started: make(chan desktopservice.SkillDebugInput, 1), stopped: make(chan string, 1), release: make(chan struct{})}
	a.skills.Debug.api = api
	a.skills.Visible = true
	a.skills.Tab = "debug"
	a.skills.Debug.Prompt = "Use this Skill"
	a.library.SkillName = "review"
	a.send()
	select {
	case <-api.started:
		t.Fatal("unfocused shortcut started a debug run")
	default:
	}
	a.skills.Debug.PromptFocused = true
	a.send()
	var input desktopservice.SkillDebugInput
	select {
	case input = <-api.started:
	case <-time.After(time.Second):
		t.Fatal("debug shortcut did not reach the service")
	}
	if input.Name != "review" || input.Prompt != "Use this Skill" || input.RunID == "" {
		t.Fatal("debug request lost selected skill/input/identity")
	}
	a.stopSkillDebug()
	select {
	case id := <-api.stopped:
		if id != input.RunID {
			t.Fatal("stop targeted another run")
		}
	case <-time.After(time.Second):
		t.Fatal("stop did not reach service")
	}
	if !a.skills.Debug.Running {
		t.Fatal("stop optimistically erased the pending transcript")
	}
}

type isolatedSkillSync struct {
	*skillTargetFixture
	targetID string
}

func (s *isolatedSkillSync) SetSkillSyncTargetSkills(input skillmanager.SetTargetSkillsInput) (skillmanager.SyncTarget, error) {
	return s.manager.SetTargetSkills(input)
}
func (s *isolatedSkillSync) SyncSkills(ctx context.Context, id string) (skillmanager.SyncResult, error) {
	if id != s.targetID {
		return skillmanager.SyncResult{}, errors.New("test refuses non-temporary target")
	}
	return s.manager.Sync(ctx, id)
}
func (s *isolatedSkillSync) SyncSkill(ctx context.Context, name string) (skillmanager.SyncSkillResult, error) {
	if _, err := s.manager.SetTargetSkills(skillmanager.SetTargetSkillsInput{ID: s.targetID, Skills: []string{name}}); err != nil {
		return skillmanager.SyncSkillResult{}, err
	}
	result, err := s.SyncSkills(ctx, s.targetID)
	return skillmanager.SyncSkillResult{Skill: name, Targets: []skillmanager.SyncTarget{result.Target}, Synced: result.SyncedSkills}, err
}
func TestSkillSyncActionWritesOnlyTemporaryTarget(t *testing.T) {
	a, base := skillTestApp(t)
	doc, err := base.CreateSkill(skillmanager.SaveSkillInput{Name: "review", Content: newManagedSkillTemplate("review", "Review changes")})
	if err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(t.TempDir(), "agent-skills")
	target, err := base.manager.AddTarget(skillmanager.AddTargetInput{Name: "Temporary Sync", Path: directory})
	if err != nil {
		t.Fatal(err)
	}
	if !target.RsyncAvailable {
		t.Skip("rsync is unavailable")
	}
	a.skills.api = &isolatedSkillSync{skillTargetFixture: &skillTargetFixture{base}, targetID: target.ID}
	a.skills.Targets = []skillmanager.SyncTarget{target}
	a.library.Skills = []skillmanager.Skill{doc.Skill}
	a.syncManagedSkills(doc.Name)
	if a.skills.Busy != "sync/review" {
		t.Fatal("sync action did not serialize")
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		bytes, err := os.ReadFile(filepath.Join(directory, "review", "SKILL.md"))
		if err == nil && string(bytes) == doc.Content {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("sync did not copy the selected Skill to temporary directory")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestRenderSkillsPreview(t *testing.T) {
	dir := os.Getenv("ONECATCH_KEEL_PREVIEWS")
	if dir == "" {
		t.Skip("set ONECATCH_KEEL_PREVIEWS to render Skills")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	a, _ := skillTestApp(t)
	locale.Apply(locale.Chinese())
	applyPalette(false)
	names := []string{"release-notes", "code-review", "commit-message", "api-docs", "incident-report"}
	descriptions := []string{"Write concise, user-facing release notes from a real diff.", "Review a diff for correctness and report only what is verified.", "Draft a Conventional Commits message from the staged changes.", "Document an HTTP endpoint from its handler and its tests.", "Turn a run log into a timeline and a root-cause summary."}
	a.library.Skills = nil
	for i, name := range names {
		a.library.Skills = append(a.library.Skills, skillmanager.Skill{Name: name, Description: descriptions[i], Path: "~/.onecatch/skills/" + name + "/SKILL.md", SizeBytes: 284, UpdatedAt: time.Date(2026, 10, 9, 5, 42, 51, 0, time.Local)})
	}
	a.library.SkillName = names[0]
	a.library.SkillDoc = &skillmanager.SkillDocument{Skill: a.library.Skills[0], Content: newManagedSkillTemplate(names[0], descriptions[0])}
	a.skills.Targets = []skillmanager.SyncTarget{{ID: "codex", Name: "Codex", Builtin: true, Path: "~/.codex/skills", Status: "synced", SyncedSkills: 5, TotalSkills: 5, LibrarySkills: 5, SyncedNames: names, RsyncAvailable: true}, {ID: "claude", Name: "Claude Code", Builtin: true, Path: "~/.claude/skills", Status: "missing", Skills: names[:2], TotalSkills: 2, LibrarySkills: 5, RsyncAvailable: true}, {ID: "modu", Name: "Modu", Builtin: true, Path: "~/.modu/skills", Status: "out-of-sync", SyncedSkills: 3, TotalSkills: 5, LibrarySkills: 5, SyncedNames: names[:3], RsyncAvailable: true}}
	a.rebuildSkillTargetMenus()
	a.skills.Debug.api = skillDebugFixture{}
	shot := func(name string) {
		t.Helper()
		if err := window.Screenshot(el.Root(a), 1120, 760, filepath.Join(dir, name+".png")); err != nil {
			t.Fatal(err)
		}
	}
	shot("skills")
	a.skills.Create.SetValue(true)
	shot("skill-create")
	a.skills.Create.SetValue(false)
	a.skills.Visible = true
	a.skills.Tab = "files"
	a.skills.Tree[""] = []skillmanager.SkillFileEntry{{Name: "release-notes", Path: "release-notes", Directory: true}, {Name: "code-review", Path: "code-review", Directory: true}}
	a.skills.Tree["release-notes"] = []skillmanager.SkillFileEntry{{Name: "references", Path: "release-notes/references", Directory: true}, {Name: "SKILL.md", Path: "release-notes/SKILL.md"}}
	a.skills.Expanded["release-notes"] = true
	source := newManagedSkillTemplate("release-notes", "Demo content for release-notes.")
	file := skillmanager.SkillFileContent{Path: "release-notes/SKILL.md", Name: "SKILL.md", Content: source, SizeBytes: int64(len(source))}
	a.skills.Active = file.Path
	a.skills.Buffers[file.Path] = &skillFileBuffer{File: file, Draft: source}
	shot("skill-files")
	a.skills.Tab = "debug"
	a.skills.Debug.Name = "release-notes"
	shot("skill-debug")
	a.skills.Visible = false
	a.skills.Pane = "sync"
	shot("skill-sync")
	a.dark = true
	applyPalette(true)
	shot("skill-sync-dark")
	a.skills.Visible = true
	a.skills.Tab = "files"
	a.skills.Pane = "skill"
	shot("skill-files-dark")
	a.dark = false
	applyPalette(false)
	if err := window.Screenshot(el.Root(a), 860, 560, filepath.Join(dir, "skill-files-narrow.png")); err != nil {
		t.Fatal(err)
	}
	p := workerTestSettings(a)
	p.section = "skills"
	p.initSkillTargetSettings()
	p.skillTargets.Loaded = true
	p.applySkillTargets(0, a.skills.Targets, nil)
	if err := window.Screenshot(el.Root(p), 960, 800, filepath.Join(dir, "skill-settings.png")); err != nil {
		t.Fatal(err)
	}
	p.skillTargets.Add.SetValue(true)
	if err := window.Screenshot(el.Root(p), 960, 800, filepath.Join(dir, "skill-target-add.png")); err != nil {
		t.Fatal(err)
	}
}
