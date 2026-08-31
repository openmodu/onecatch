// Package skillmanager owns OneCatch's user-authored Agent Skills library and
// the metadata for copies synchronized into external coding agents.
package skillmanager

import (
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

type SaveSkillInput struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

type AddTargetInput struct {
	Name string `json:"name"`
	Path string `json:"path"`
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
	definitions := append(m.builtinTargets(), metadata.CustomTargets...)
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
	definition, ok := findTarget(id, append(m.builtinTargets(), metadata.CustomTargets...))
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
	skills, err := m.listLocked()
	if err != nil {
		return SyncResult{}, err
	}
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
	target, scanErr := m.scanTargetLocked(metadata, definition, skills)
	if scanErr != nil {
		return SyncResult{}, scanErr
	}
	return SyncResult{Target: target, SyncedSkills: len(skills), Output: strings.Join(outputs, "\n")}, nil
}

func (m *Manager) scanTargetLocked(metadata metadataFile, definition targetDefinition, skills []Skill) (SyncTarget, error) {
	if skills == nil {
		var err error
		skills, err = m.listLocked()
		if err != nil {
			return SyncTarget{}, err
		}
	}
	path := expandHome(definition.Path, m.home)
	item := SyncTarget{ID: definition.ID, Name: definition.Name, Path: path, Builtin: definition.Builtin, TotalSkills: len(skills)}
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
