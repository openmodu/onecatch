// Package skillmanager owns OneCatch's user-authored Agent Skills library and
// the metadata for copies synchronized into external coding agents.
package skillmanager

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/openmodu/modu/pkg/utils"
	"github.com/openmodu/onecatch/pkg/localfile"
)

const (
	metadataVersion = 1
	maxSkillBytes   = 2 << 20
	// Resources beside SKILL.md are edited in a plain textarea, so the cap is
	// about what a person can reasonably review rather than what fits in RAM.
	maxSkillFileBytes = 1 << 20
)

var skillNamePattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// Skill is the lightweight list representation of one directory-backed skill.
type Skill struct {
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Path        string    `json:"path"`
	UpdatedAt   time.Time `json:"updatedAt"`
	SizeBytes   int64     `json:"sizeBytes"`
	Digest      string    `json:"digest"`
}

// SkillDocument includes the editable SKILL.md source.
type SkillDocument struct {
	Skill
	Content string `json:"content"`
}

// SkillFileEntry is one direct child of a directory inside the managed Skills
// root. Paths are always relative to ~/.onecatch/skills and symlinks are never
// followed or returned.
type SkillFileEntry struct {
	Name       string `json:"name"`
	Path       string `json:"path"`
	Directory  bool   `json:"directory"`
	Size       int64  `json:"size,omitempty"`
	ModifiedAt string `json:"modifiedAt,omitempty"`
}

// SkillFileContent is one editable text file inside the managed Skills root.
// It is how the Skills inspector reads and writes the resources that sit
// beside SKILL.md — references, scripts, prompts — without handing the UI a
// general-purpose filesystem.
type SkillFileContent struct {
	Path       string `json:"path"`
	Name       string `json:"name"`
	Content    string `json:"content"`
	SizeBytes  int64  `json:"sizeBytes"`
	ModifiedAt string `json:"modifiedAt,omitempty"`
}

// SaveSkillFileInput is a single-argument shape so the generated Wails binding
// keeps a stable signature as the editor grows.
type SaveSkillFileInput struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type SaveSkillInput struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

type AddTargetInput struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// UpdateTargetInput repoints an existing target. A built-in target keeps its
// name — only the directory is the user's to choose, which is how Settings
// points Codex or Claude Code at a non-default install.
type UpdateTargetInput struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
	Path string `json:"path"`
}

// SetTargetSkillsInput narrows what a target receives. An empty list means the
// whole library, which is what every target did before selection existed and
// what a newly added target still does.
type SetTargetSkillsInput struct {
	ID     string   `json:"id"`
	Skills []string `json:"skills"`
}

// SyncTarget is a scanned external Agent Skills directory.
type SyncTarget struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	Path           string     `json:"path"`
	Builtin        bool       `json:"builtin"`
	Exists         bool       `json:"exists"`
	Status         string     `json:"status"`
	SyncedSkills   int        `json:"syncedSkills"`
	TotalSkills    int        `json:"totalSkills"`
	LastSyncedAt   *time.Time `json:"lastSyncedAt,omitempty"`
	LastError      string     `json:"lastError,omitempty"`
	RsyncAvailable bool       `json:"rsyncAvailable"`
	// Skills is the configured subset this target receives. Empty means the
	// whole library; TotalSkills already counts only what will be sent, so the
	// two together say both "3 of 5 chosen" and "2 of those 3 are current".
	Skills []string `json:"skills,omitempty"`
	// LibrarySkills is the size of the whole library, so a partial selection
	// can be described without a second call.
	LibrarySkills int `json:"librarySkills"`
}

type SyncResult struct {
	Target       SyncTarget `json:"target"`
	SyncedSkills int        `json:"syncedSkills"`
	Output       string     `json:"output,omitempty"`
}

type targetDefinition struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Path    string `json:"path"`
	Builtin bool   `json:"builtin,omitempty"`
}

type targetMetadata struct {
	Path         string            `json:"path"`
	LastSyncedAt time.Time         `json:"lastSyncedAt"`
	Skills       map[string]string `json:"skills"`
	LastError    string            `json:"lastError,omitempty"`
}

type metadataFile struct {
	Version       int                       `json:"version"`
	Targets       map[string]targetMetadata `json:"targets"`
	CustomTargets []targetDefinition        `json:"customTargets,omitempty"`
	// TargetPaths overrides a built-in target's default directory. Built-ins
	// are defined in code, so an override is the only place a user-chosen path
	// for one can live.
	TargetPaths map[string]string `json:"targetPaths,omitempty"`
	// Selections is the subset of the library each target receives, keyed by
	// target id. A target with no entry receives everything.
	Selections map[string][]string `json:"selections,omitempty"`
}

type commandRunner func(context.Context, string, ...string) ([]byte, error)

type Manager struct {
	root         string
	metadataPath string
	home         string
	now          func() time.Time
	lookPath     func(string) (string, error)
	run          commandRunner
	mu           sync.Mutex
}

// DefaultRoot resolves the product-wide skill library required by the desktop
// app. It is deliberately independent of the platform-specific application
// data directory.
func DefaultRoot() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".onecatch", "skills"), nil
}

func New(root string) (*Manager, error) {
	if strings.TrimSpace(root) == "" {
		var err error
		root, err = DefaultRoot()
		if err != nil {
			return nil, err
		}
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve skills root: %w", err)
	}
	home, _ := os.UserHomeDir()
	m := &Manager{
		root:         filepath.Clean(absolute),
		metadataPath: filepath.Join(filepath.Dir(filepath.Clean(absolute)), "skill-sync.json"),
		home:         filepath.Clean(home),
		now:          time.Now,
		lookPath:     exec.LookPath,
		run: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			return exec.CommandContext(ctx, name, args...).CombinedOutput()
		},
	}
	if err := os.MkdirAll(m.root, 0o700); err != nil {
		return nil, fmt.Errorf("create skills root: %w", err)
	}
	return m, nil
}

func (m *Manager) Root() string { return m.root }

// ListFiles returns the immediate children of a directory under the managed
// Skills root. Keeping this API rooted in the manager prevents the Skills UI
// from accidentally browsing the active project or escaping through a symlink.
func (m *Manager) ListFiles(directory string) ([]SkillFileEntry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	relative, err := cleanRelativeDirectory(directory)
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(m.root)
	if err != nil {
		return nil, fmt.Errorf("open skills root: %w", err)
	}
	defer root.Close()
	dir, err := root.Open(relative)
	if err != nil {
		return nil, fmt.Errorf("open skills directory: %w", err)
	}
	defer dir.Close()
	info, err := dir.Stat()
	if err != nil {
		return nil, fmt.Errorf("inspect skills directory: %w", err)
	}
	if !info.IsDir() {
		return nil, errors.New("skill file path is not a directory")
	}
	children, err := dir.ReadDir(-1)
	if err != nil {
		return nil, fmt.Errorf("read skills directory: %w", err)
	}
	entries := make([]SkillFileEntry, 0, len(children))
	for _, child := range children {
		if child.Type()&os.ModeSymlink != 0 {
			continue
		}
		childInfo, infoErr := child.Info()
		if infoErr != nil || (!childInfo.IsDir() && !childInfo.Mode().IsRegular()) {
			continue
		}
		path := child.Name()
		if relative != "." {
			path = filepath.Join(relative, child.Name())
		}
		entries = append(entries, SkillFileEntry{
			Name:       child.Name(),
			Path:       filepath.ToSlash(path),
			Directory:  childInfo.IsDir(),
			Size:       childInfo.Size(),
			ModifiedAt: childInfo.ModTime().UTC().Format(time.RFC3339Nano),
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

// ReadFile returns one text file under the managed Skills root. Binary
// payloads are refused rather than mangled: the caller is a text editor, and
// round-tripping arbitrary bytes through a JSON string would corrupt them.
func (m *Manager) ReadFile(path string) (SkillFileContent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	relative, err := cleanRelativeFile(path)
	if err != nil {
		return SkillFileContent{}, err
	}
	root, err := os.OpenRoot(m.root)
	if err != nil {
		return SkillFileContent{}, fmt.Errorf("open skills root: %w", err)
	}
	defer root.Close()
	file, err := root.Open(relative)
	if err != nil {
		return SkillFileContent{}, fmt.Errorf("open skill file: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return SkillFileContent{}, fmt.Errorf("inspect skill file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return SkillFileContent{}, errors.New("skill file path is not a regular file")
	}
	if info.Size() > maxSkillFileBytes {
		return SkillFileContent{}, fmt.Errorf("%s exceeds the %d MiB editor limit", filepath.Base(relative), maxSkillFileBytes>>20)
	}
	data, err := io.ReadAll(file)
	if err != nil {
		return SkillFileContent{}, fmt.Errorf("read skill file: %w", err)
	}
	if bytes.IndexByte(data, 0) >= 0 {
		return SkillFileContent{}, fmt.Errorf("%s is not a text file", filepath.Base(relative))
	}
	return SkillFileContent{
		Path:       filepath.ToSlash(relative),
		Name:       filepath.Base(relative),
		Content:    string(data),
		SizeBytes:  info.Size(),
		ModifiedAt: info.ModTime().UTC().Format(time.RFC3339Nano),
	}, nil
}

// WriteFile replaces the contents of a file that already exists under the
// Skills root. Creating files is deliberately not exposed here — the editor
// edits what the library already contains, and Create owns new skills.
//
// A skill's own SKILL.md still goes through the frontmatter validation Update
// applies, so the library cannot be left with a skill no runtime can load.
func (m *Manager) WriteFile(path, content string) (SkillFileContent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	relative, err := cleanRelativeFile(path)
	if err != nil {
		return SkillFileContent{}, err
	}
	if len(content) > maxSkillFileBytes {
		return SkillFileContent{}, fmt.Errorf("%s exceeds the %d MiB editor limit", filepath.Base(relative), maxSkillFileBytes>>20)
	}
	if skill, ok := skillDocumentPath(relative); ok {
		if _, err := validateSkillContent(skill, content); err != nil {
			return SkillFileContent{}, err
		}
	}
	root, err := os.OpenRoot(m.root)
	if err != nil {
		return SkillFileContent{}, fmt.Errorf("open skills root: %w", err)
	}
	defer root.Close()
	// Opening through the root is the containment check: it refuses to
	// traverse a symlink or leave the directory. Only once that succeeds is
	// the resolved path safe to hand to the atomic writer.
	existing, err := root.Open(relative)
	if err != nil {
		return SkillFileContent{}, fmt.Errorf("open skill file: %w", err)
	}
	info, statErr := existing.Stat()
	existing.Close()
	if statErr != nil {
		return SkillFileContent{}, fmt.Errorf("inspect skill file: %w", statErr)
	}
	if !info.Mode().IsRegular() {
		return SkillFileContent{}, errors.New("skill file path is not a regular file")
	}
	if err := localfile.WriteTextAtomic(filepath.Join(m.root, relative), content); err != nil {
		return SkillFileContent{}, fmt.Errorf("write skill file: %w", err)
	}
	return SkillFileContent{
		Path:       filepath.ToSlash(relative),
		Name:       filepath.Base(relative),
		Content:    content,
		SizeBytes:  int64(len(content)),
		ModifiedAt: m.now().UTC().Format(time.RFC3339Nano),
	}, nil
}

// skillDocumentPath reports whether a relative path is a skill's own SKILL.md,
// which is the one file in the tree that carries library-wide invariants.
func skillDocumentPath(relative string) (string, bool) {
	directory, file := filepath.Split(relative)
	if file != "SKILL.md" {
		return "", false
	}
	name := strings.Trim(filepath.ToSlash(directory), "/")
	if name == "" || strings.Contains(name, "/") {
		return "", false
	}
	return name, true
}

func cleanRelativeFile(path string) (string, error) {
	relative, err := cleanRelativeDirectory(path)
	if err != nil {
		return "", err
	}
	if relative == "." {
		return "", errors.New("skill file path is required")
	}
	return relative, nil
}

func cleanRelativeDirectory(directory string) (string, error) {
	directory = strings.TrimSpace(directory)
	if directory == "" {
		return ".", nil
	}
	relative := filepath.Clean(filepath.FromSlash(directory))
	if filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", errors.New("skill file path must stay inside ~/.onecatch/skills")
	}
	return relative, nil
}

// ValidSkillName reports whether a name is one the library could hold. It is
// exported so callers that key their own per-skill storage off a name can
// refuse a value that would escape their directory, without duplicating the
// pattern.
func ValidSkillName(name string) bool {
	return validateSkillName(strings.TrimSpace(name)) == nil
}

func validateSkillName(name string) error {
	if len(name) > 64 || !skillNamePattern.MatchString(name) {
		return errors.New("skill name must use lowercase alphanumeric segments separated by hyphens")
	}
	return nil
}

func validateSkillContent(name, content string) (string, error) {
	if len(content) > maxSkillBytes {
		return "", fmt.Errorf("SKILL.md exceeds the %d MiB editor limit", maxSkillBytes>>20)
	}
	fields, _, ok := utils.ParseFrontmatter(content)
	if !ok {
		return "", errors.New("SKILL.md must start with YAML frontmatter")
	}
	description := strings.TrimSpace(fields["description"])
	if description == "" {
		return "", errors.New("SKILL.md frontmatter must include a description")
	}
	if len(description) > 1024 {
		return "", errors.New("skill description exceeds 1024 bytes")
	}
	if declared := strings.TrimSpace(fields["name"]); declared != "" && declared != name {
		return "", fmt.Errorf("frontmatter name %q must match directory name %q", declared, name)
	}
	return description, nil
}

func (m *Manager) skillDir(name string) (string, error) {
	name = strings.TrimSpace(name)
	if err := validateSkillName(name); err != nil {
		return "", err
	}
	return filepath.Join(m.root, name), nil
}

func (m *Manager) List() ([]Skill, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.listLocked()
}

func (m *Manager) listLocked() ([]Skill, error) {
	entries, err := os.ReadDir(m.root)
	if err != nil {
		return nil, fmt.Errorf("read skills root: %w", err)
	}
	items := make([]Skill, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || validateSkillName(entry.Name()) != nil {
			continue
		}
		item, err := m.readSummaryLocked(entry.Name())
		if err == nil {
			items = append(items, item)
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return items, nil
}

func (m *Manager) readSummaryLocked(name string) (Skill, error) {
	dir, err := m.skillDir(name)
	if err != nil {
		return Skill{}, err
	}
	path := filepath.Join(dir, "SKILL.md")
	data, err := os.ReadFile(path)
	if err != nil {
		return Skill{}, fmt.Errorf("read %s: %w", path, err)
	}
	description, err := validateSkillContent(name, string(data))
	if err != nil {
		return Skill{}, err
	}
	digest, size, updatedAt, err := digestTree(dir)
	if err != nil {
		return Skill{}, err
	}
	return Skill{Name: name, Description: description, Path: path, UpdatedAt: updatedAt, SizeBytes: size, Digest: digest}, nil
}

func (m *Manager) Get(name string) (SkillDocument, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	item, err := m.readSummaryLocked(name)
	if err != nil {
		return SkillDocument{}, err
	}
	data, err := os.ReadFile(item.Path)
	if err != nil {
		return SkillDocument{}, fmt.Errorf("read skill: %w", err)
	}
	return SkillDocument{Skill: item, Content: string(data)}, nil
}

func (m *Manager) Create(input SaveSkillInput) (SkillDocument, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	name := strings.TrimSpace(input.Name)
	dir, err := m.skillDir(name)
	if err != nil {
		return SkillDocument{}, err
	}
	if _, err := validateSkillContent(name, input.Content); err != nil {
		return SkillDocument{}, err
	}
	if _, err := os.Lstat(dir); err == nil {
		return SkillDocument{}, fmt.Errorf("skill %q already exists", name)
	} else if !errors.Is(err, os.ErrNotExist) {
		return SkillDocument{}, fmt.Errorf("inspect skill: %w", err)
	}
	if err := os.Mkdir(dir, 0o700); err != nil {
		return SkillDocument{}, fmt.Errorf("create skill directory: %w", err)
	}
	path := filepath.Join(dir, "SKILL.md")
	if err := localfile.WriteTextAtomic(path, input.Content); err != nil {
		_ = os.Remove(dir)
		return SkillDocument{}, fmt.Errorf("create skill: %w", err)
	}
	item, err := m.readSummaryLocked(name)
	return SkillDocument{Skill: item, Content: input.Content}, err
}

func (m *Manager) Update(input SaveSkillInput) (SkillDocument, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	name := strings.TrimSpace(input.Name)
	dir, err := m.skillDir(name)
	if err != nil {
		return SkillDocument{}, err
	}
	if _, err := validateSkillContent(name, input.Content); err != nil {
		return SkillDocument{}, err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return SkillDocument{}, fmt.Errorf("inspect skill: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return SkillDocument{}, errors.New("skill path is not a managed directory")
	}
	if err := localfile.WriteTextAtomic(filepath.Join(dir, "SKILL.md"), input.Content); err != nil {
		return SkillDocument{}, fmt.Errorf("update skill: %w", err)
	}
	item, err := m.readSummaryLocked(name)
	return SkillDocument{Skill: item, Content: input.Content}, err
}

func (m *Manager) Delete(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	dir, err := m.skillDir(name)
	if err != nil {
		return err
	}
	info, err := os.Lstat(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect skill: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("skill path is not a managed directory")
	}
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("delete skill: %w", err)
	}
	return nil
}

func (m *Manager) AddTarget(input AddTargetInput) (SyncTarget, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	name := strings.TrimSpace(input.Name)
	if name == "" || len(name) > 64 {
		return SyncTarget{}, errors.New("target name is required and must not exceed 64 characters")
	}
	path, err := m.validateTargetPath(input.Path)
	if err != nil {
		return SyncTarget{}, err
	}
	metadata, err := m.loadMetadataLocked()
	if err != nil {
		return SyncTarget{}, err
	}
	base := "custom-" + slug(name)
	id := base
	for suffix := 2; targetIDExists(id, metadata.CustomTargets); suffix++ {
		id = fmt.Sprintf("%s-%d", base, suffix)
	}
	metadata.CustomTargets = append(metadata.CustomTargets, targetDefinition{ID: id, Name: name, Path: path})
	if err := m.saveMetadataLocked(metadata); err != nil {
		return SyncTarget{}, err
	}
	return m.scanTargetLocked(metadata, targetDefinition{ID: id, Name: name, Path: path}, nil)
}

func (m *Manager) RemoveTarget(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if isBuiltinTarget(id) {
		return errors.New("built-in sync targets cannot be removed")
	}
	metadata, err := m.loadMetadataLocked()
	if err != nil {
		return err
	}
	filtered := metadata.CustomTargets[:0]
	found := false
	for _, target := range metadata.CustomTargets {
		if target.ID == id {
			found = true
			continue
		}
		filtered = append(filtered, target)
	}
	if !found {
		return fmt.Errorf("sync target %q not found", id)
	}
	metadata.CustomTargets = filtered
	delete(metadata.Targets, id)
	return m.saveMetadataLocked(metadata)
}

func (m *Manager) ScanTargets() ([]SyncTarget, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	metadata, err := m.loadMetadataLocked()
	if err != nil {
		return nil, err
	}
	skills, err := m.listLocked()
	if err != nil {
		return nil, err
	}
	definitions := targetDefinitions(m, metadata)
	items := make([]SyncTarget, 0, len(definitions))
	for _, definition := range definitions {
		item, scanErr := m.scanTargetLocked(metadata, definition, skills)
		if scanErr != nil {
			item.LastError = scanErr.Error()
			item.Status = "error"
		}
		items = append(items, item)
	}
	return items, nil
}

func (m *Manager) Sync(ctx context.Context, id string) (SyncResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	metadata, err := m.loadMetadataLocked()
	if err != nil {
		return SyncResult{}, err
	}
	definition, ok := findTarget(id, targetDefinitions(m, metadata))
	if !ok {
		return SyncResult{}, fmt.Errorf("sync target %q not found", id)
	}
	definition.Path, err = m.validateTargetPath(definition.Path)
	if err != nil {
		return SyncResult{}, err
	}
	rsync, err := m.lookPath("rsync")
	if err != nil {
		return SyncResult{}, errors.New("rsync is not available on PATH")
	}
	library, err := m.listLocked()
	if err != nil {
		return SyncResult{}, err
	}
	// Only the chosen subset is copied. Skills the target is not configured to
	// receive are left alone rather than deleted: another tool may own them.
	skills := selectSkills(metadata.Selections[id], library)
	if err := os.MkdirAll(definition.Path, 0o700); err != nil {
		return SyncResult{}, fmt.Errorf("create sync target: %w", err)
	}
	record := targetMetadata{Path: definition.Path, Skills: make(map[string]string)}
	var outputs []string
	for _, skill := range skills {
		if err := ctx.Err(); err != nil {
			return SyncResult{}, err
		}
		source := filepath.Join(m.root, skill.Name) + string(filepath.Separator)
		destination := filepath.Join(definition.Path, skill.Name) + string(filepath.Separator)
		if err := os.MkdirAll(destination, 0o700); err != nil {
			return SyncResult{}, fmt.Errorf("prepare %s destination: %w", skill.Name, err)
		}
		output, runErr := m.run(ctx, rsync, "-a", "--delete", source, destination)
		if text := strings.TrimSpace(string(output)); text != "" {
			outputs = append(outputs, text)
		}
		if runErr != nil {
			record.LastError = fmt.Sprintf("sync %s: %v", skill.Name, runErr)
			metadata.Targets[id] = record
			_ = m.saveMetadataLocked(metadata)
			return SyncResult{}, fmt.Errorf("%s%s", record.LastError, commandOutputSuffix(output))
		}
		record.Skills[skill.Name] = skill.Digest
	}
	record.LastSyncedAt = m.now().UTC()
	metadata.Targets[id] = record
	if err := m.saveMetadataLocked(metadata); err != nil {
		return SyncResult{}, err
	}
	target, scanErr := m.scanTargetLocked(metadata, definition, library)
	if scanErr != nil {
		return SyncResult{}, scanErr
	}
	return SyncResult{Target: target, SyncedSkills: len(skills), Output: strings.Join(outputs, "\n")}, nil
}

// scanTargetLocked reports a target against the subset it is configured to
// receive: `skills` is the whole library, and everything counted below is what
// this target actually expects to hold.
func (m *Manager) scanTargetLocked(metadata metadataFile, definition targetDefinition, skills []Skill) (SyncTarget, error) {
	if skills == nil {
		var err error
		skills, err = m.listLocked()
		if err != nil {
			return SyncTarget{}, err
		}
	}
	library := len(skills)
	selection := metadata.Selections[definition.ID]
	skills = selectSkills(selection, skills)
	path := expandHome(definition.Path, m.home)
	item := SyncTarget{ID: definition.ID, Name: definition.Name, Path: path, Builtin: definition.Builtin, TotalSkills: len(skills), LibrarySkills: library, Skills: selection}
	_, rsyncErr := m.lookPath("rsync")
	item.RsyncAvailable = rsyncErr == nil
	info, statErr := os.Stat(path)
	item.Exists = statErr == nil && info.IsDir()
	if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		item.Status, item.LastError = "error", statErr.Error()
		return item, statErr
	}
	record, recorded := metadata.Targets[definition.ID]
	if recorded && !record.LastSyncedAt.IsZero() {
		stamp := record.LastSyncedAt
		item.LastSyncedAt = &stamp
		item.LastError = record.LastError
	}
	if !item.RsyncAvailable {
		item.Status = "rsync-unavailable"
		return item, nil
	}
	if !item.Exists {
		item.Status = "missing"
		return item, nil
	}
	for _, skill := range skills {
		digest, _, _, err := digestTree(filepath.Join(path, skill.Name))
		if err == nil && digest == skill.Digest {
			item.SyncedSkills++
		}
	}
	switch {
	case len(skills) == 0:
		item.Status = "empty"
	case item.SyncedSkills == len(skills):
		item.Status = "synced"
	case item.SyncedSkills > 0:
		item.Status = "partial"
	case recorded:
		item.Status = "out-of-sync"
	default:
		item.Status = "ready"
	}
	return item, nil
}

func (m *Manager) validateTargetPath(value string) (string, error) {
	if strings.ContainsAny(value, "\x00\r\n") {
		return "", errors.New("sync target contains invalid characters")
	}
	path := expandHome(strings.TrimSpace(value), m.home)
	if path == "" || !filepath.IsAbs(path) {
		return "", errors.New("sync target must be an absolute path or start with ~/")
	}
	path = filepath.Clean(path)
	if path == filepath.VolumeName(path)+string(filepath.Separator) || path == m.home {
		return "", errors.New("sync target is too broad")
	}
	if pathsOverlap(path, m.root) {
		return "", errors.New("sync target must not contain or be inside the OneCatch skills root")
	}
	return path, nil
}

func (m *Manager) builtinTargets() []targetDefinition {
	return []targetDefinition{
		{ID: "codex", Name: "Codex", Path: filepath.Join(m.home, ".codex", "skills"), Builtin: true},
		{ID: "claude", Name: "Claude Code", Path: filepath.Join(m.home, ".claude", "skills"), Builtin: true},
		{ID: "modu", Name: "Modu", Path: filepath.Join(m.home, ".modu", "skills"), Builtin: true},
	}
}

// targetDefinitions is every target the library knows about, with a built-in's
// directory replaced by the user's override when one was configured.
func targetDefinitions(m *Manager, metadata metadataFile) []targetDefinition {
	builtins := m.builtinTargets()
	for index, definition := range builtins {
		if path := strings.TrimSpace(metadata.TargetPaths[definition.ID]); path != "" {
			builtins[index].Path = path
		}
	}
	return append(builtins, metadata.CustomTargets...)
}

// selectSkills narrows the library to what a target is configured to receive.
// Names that no longer exist simply drop out, so deleting a skill never leaves
// a target pointing at nothing.
func selectSkills(selection []string, skills []Skill) []Skill {
	if len(selection) == 0 {
		return skills
	}
	wanted := make(map[string]struct{}, len(selection))
	for _, name := range selection {
		wanted[strings.TrimSpace(name)] = struct{}{}
	}
	filtered := make([]Skill, 0, len(selection))
	for _, skill := range skills {
		if _, ok := wanted[skill.Name]; ok {
			filtered = append(filtered, skill)
		}
	}
	return filtered
}

// UpdateTarget repoints a target at a different directory. The path is
// validated the same way an added target's is, so Settings cannot aim the
// library at the home directory or at itself.
func (m *Manager) UpdateTarget(input UpdateTargetInput) (SyncTarget, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id := strings.TrimSpace(input.ID)
	metadata, err := m.loadMetadataLocked()
	if err != nil {
		return SyncTarget{}, err
	}
	definition, ok := findTarget(id, targetDefinitions(m, metadata))
	if !ok {
		return SyncTarget{}, fmt.Errorf("sync target %q not found", id)
	}
	path, err := m.validateTargetPath(input.Path)
	if err != nil {
		return SyncTarget{}, err
	}
	if definition.Builtin {
		if metadata.TargetPaths == nil {
			metadata.TargetPaths = make(map[string]string)
		}
		metadata.TargetPaths[id] = path
	} else {
		name := strings.TrimSpace(input.Name)
		for index, custom := range metadata.CustomTargets {
			if custom.ID != id {
				continue
			}
			metadata.CustomTargets[index].Path = path
			if name != "" {
				metadata.CustomTargets[index].Name = name
			}
		}
	}
	if err := m.saveMetadataLocked(metadata); err != nil {
		return SyncTarget{}, err
	}
	updated, _ := findTarget(id, targetDefinitions(m, metadata))
	return m.scanTargetLocked(metadata, updated, nil)
}

// SetTargetSkills chooses which skills a target receives. Passing no names
// restores "the whole library" rather than recording an empty selection, so a
// target can always be put back the way it started.
func (m *Manager) SetTargetSkills(input SetTargetSkillsInput) (SyncTarget, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id := strings.TrimSpace(input.ID)
	metadata, err := m.loadMetadataLocked()
	if err != nil {
		return SyncTarget{}, err
	}
	definition, ok := findTarget(id, targetDefinitions(m, metadata))
	if !ok {
		return SyncTarget{}, fmt.Errorf("sync target %q not found", id)
	}
	skills, err := m.listLocked()
	if err != nil {
		return SyncTarget{}, err
	}
	// Storing only names that exist keeps the record honest; a stale name
	// would read as a chosen skill that never arrives.
	known := make(map[string]struct{}, len(skills))
	for _, skill := range skills {
		known[skill.Name] = struct{}{}
	}
	selection := make([]string, 0, len(input.Skills))
	seen := make(map[string]struct{}, len(input.Skills))
	for _, name := range input.Skills {
		name = strings.TrimSpace(name)
		if _, ok := known[name]; !ok {
			continue
		}
		if _, duplicate := seen[name]; duplicate {
			continue
		}
		seen[name] = struct{}{}
		selection = append(selection, name)
	}
	if metadata.Selections == nil {
		metadata.Selections = make(map[string][]string)
	}
	if len(selection) == 0 || len(selection) == len(skills) {
		delete(metadata.Selections, id)
	} else {
		metadata.Selections[id] = selection
	}
	if err := m.saveMetadataLocked(metadata); err != nil {
		return SyncTarget{}, err
	}
	return m.scanTargetLocked(metadata, definition, skills)
}

func (m *Manager) loadMetadataLocked() (metadataFile, error) {
	value := metadataFile{Version: metadataVersion, Targets: make(map[string]targetMetadata)}
	if err := localfile.ReadJSON(m.metadataPath, &value); errors.Is(err, os.ErrNotExist) {
		return value, nil
	} else if err != nil {
		return metadataFile{}, fmt.Errorf("read skill sync metadata: %w", err)
	}
	if value.Version != metadataVersion {
		return metadataFile{}, fmt.Errorf("unsupported skill sync metadata version %d", value.Version)
	}
	if value.Targets == nil {
		value.Targets = make(map[string]targetMetadata)
	}
	return value, nil
}

func (m *Manager) saveMetadataLocked(value metadataFile) error {
	value.Version = metadataVersion
	if value.Targets == nil {
		value.Targets = make(map[string]targetMetadata)
	}
	if err := localfile.WriteJSONAtomic(m.metadataPath, value); err != nil {
		return fmt.Errorf("save skill sync metadata: %w", err)
	}
	return nil
}

func digestTree(root string) (string, int64, time.Time, error) {
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		if err == nil {
			err = errors.New("not a directory")
		}
		return "", 0, time.Time{}, err
	}
	hash := sha256.New()
	var size int64
	updatedAt := info.ModTime()
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.ModTime().After(updatedAt) {
			updatedAt = info.ModTime()
		}
		_, _ = io.WriteString(hash, filepath.ToSlash(relative)+"\x00"+info.Mode().String()+"\x00")
		if entry.Type()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			_, _ = io.WriteString(hash, target)
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("unsupported file type in skill: %s", relative)
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		written, copyErr := io.Copy(hash, file)
		closeErr := file.Close()
		size += written
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
	if err != nil {
		return "", 0, time.Time{}, err
	}
	return hex.EncodeToString(hash.Sum(nil)), size, updatedAt.UTC(), nil
}

func expandHome(path, home string) string {
	path = strings.TrimSpace(path)
	if path == "~" {
		return home
	}
	if strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
		return filepath.Join(home, path[2:])
	}
	return filepath.Clean(path)
}

func pathsOverlap(left, right string) bool {
	contains := func(parent, child string) bool {
		relative, err := filepath.Rel(parent, child)
		return err == nil && (relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))))
	}
	return contains(left, right) || contains(right, left)
}

func slug(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var out strings.Builder
	previousDash := false
	for _, char := range value {
		alphanumeric := char >= 'a' && char <= 'z' || char >= '0' && char <= '9'
		if alphanumeric {
			out.WriteRune(char)
			previousDash = false
		} else if !previousDash && out.Len() > 0 {
			out.WriteByte('-')
			previousDash = true
		}
	}
	result := strings.Trim(out.String(), "-")
	if result == "" {
		return "target"
	}
	return result
}

func targetIDExists(id string, targets []targetDefinition) bool {
	if isBuiltinTarget(id) {
		return true
	}
	_, ok := findTarget(id, targets)
	return ok
}

func isBuiltinTarget(id string) bool { return id == "codex" || id == "claude" || id == "modu" }

func findTarget(id string, targets []targetDefinition) (targetDefinition, bool) {
	for _, target := range targets {
		if target.ID == id {
			return target, true
		}
	}
	return targetDefinition{}, false
}

func commandOutputSuffix(output []byte) string {
	text := strings.TrimSpace(string(output))
	if text == "" {
		return ""
	}
	if len(text) > 1000 {
		text = text[len(text)-1000:]
	}
	return ": " + text
}
