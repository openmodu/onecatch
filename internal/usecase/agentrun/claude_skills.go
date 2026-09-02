package agentrun

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

	"go.yaml.in/yaml/v3"
	"golang.org/x/mod/semver"
)

type claudeSkillCatalog struct {
	mu       sync.RWMutex
	observed map[string][]string
}

func newClaudeSkillCatalog() *claudeSkillCatalog {
	return &claudeSkillCatalog{observed: make(map[string][]string)}
}

func (c *claudeSkillCatalog) remember(cwd string, names []string) {
	if c == nil || len(names) == 0 {
		return
	}
	key := cleanSkillWorkspace(cwd)
	items := make([]string, 0, len(names))
	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		name = strings.TrimPrefix(strings.TrimSpace(name), "/")
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		items = append(items, name)
	}
	c.mu.Lock()
	c.observed[key] = items
	c.mu.Unlock()
}

func (c *claudeSkillCatalog) names(cwd string) []string {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return append([]string(nil), c.observed[cleanSkillWorkspace(cwd)]...)
}

func cleanSkillWorkspace(cwd string) string {
	if absolute, err := filepath.Abs(cwd); err == nil {
		return filepath.Clean(absolute)
	}
	return filepath.Clean(cwd)
}

// ListSkills discovers Claude Code's user-invocable Skills without starting a
// model turn. Claude emits its authoritative list only after it receives the
// first message, so the pre-composer catalog is assembled from the documented
// project, personal, enterprise, synced, and enabled-plugin locations. Names
// observed in a later system/init event are merged in for subsequent turns.
func (r *ClaudeRunner) ListSkills(_ context.Context, cwd string, environment []string) ([]Skill, error) {
	home := environmentValue(environment, "HOME")
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	if strings.TrimSpace(cwd) == "" {
		cwd = home
	}
	configRoot := environmentValue(environment, "CLAUDE_CONFIG_DIR")
	if configRoot == "" {
		configRoot = filepath.Join(home, ".claude")
	}
	projectRoots := []string(nil)
	if info, err := os.Stat(cwd); err == nil && info.IsDir() {
		projectRoots = claudeProjectRoots(cwd)
	}
	settings := readClaudeSkillSettings(configRoot, projectRoots)
	byName := make(map[string]Skill)

	// Synced skills have the lowest filesystem precedence.
	mergeClaudeSkills(byName, scanClaudeSkillDirectory(filepath.Join(configRoot, "skills", "synced"), "synced", "", false), settings.SkillOverrides)
	for _, root := range projectRoots {
		mergeClaudeSkills(byName, scanClaudeSkillDirectory(filepath.Join(root, ".claude", "skills"), "project", "", false), settings.SkillOverrides)
	}
	// Personal skills override project skills. The reserved synced directory is
	// scanned separately above and is not itself a Skill.
	mergeClaudeSkills(byName, scanClaudeSkillDirectory(filepath.Join(configRoot, "skills"), "personal", "", true), settings.SkillOverrides)
	for _, root := range claudeEnterpriseSkillRoots() {
		mergeClaudeSkills(byName, scanClaudeSkillDirectory(root, "enterprise", "", false), settings.SkillOverrides)
	}
	for _, skill := range scanEnabledClaudePluginSkills(configRoot, settings.EnabledPlugins) {
		byName[skill.Name] = skill
	}
	for _, name := range r.skillCatalog.names(cwd) {
		if _, exists := byName[name]; !exists {
			byName[name] = Skill{Name: name, DisplayName: name, Scope: "runtime"}
		}
	}

	items := make([]Skill, 0, len(byName))
	for _, skill := range byName {
		items = append(items, skill)
	}
	sort.Slice(items, func(i, j int) bool {
		return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
	})
	return items, nil
}

// claudeProjectRoots returns the start directory and its parents through the
// repository root, ordered from the repository root down. This matches Claude
// Code's startup discovery for project Skills.
func claudeProjectRoots(cwd string) []string {
	start := filepath.Clean(cwd)
	chain := make([]string, 0)
	for current := start; ; current = filepath.Dir(current) {
		chain = append(chain, current)
		if _, err := os.Stat(filepath.Join(current, ".git")); err == nil {
			break
		}
		parent := filepath.Dir(current)
		if parent == current {
			// Outside a repository Claude treats the starting directory as the
			// project boundary; do not scan arbitrary filesystem ancestors.
			chain = chain[:1]
			break
		}
	}
	for left, right := 0, len(chain)-1; left < right; left, right = left+1, right-1 {
		chain[left], chain[right] = chain[right], chain[left]
	}
	return chain
}

type claudeSkillSettings struct {
	EnabledPlugins map[string]bool   `json:"enabledPlugins"`
	SkillOverrides map[string]string `json:"skillOverrides"`
}

func readClaudeSkillSettings(configRoot string, projectRoots []string) claudeSkillSettings {
	merged := claudeSkillSettings{EnabledPlugins: make(map[string]bool), SkillOverrides: make(map[string]string)}
	paths := []string{filepath.Join(configRoot, "settings.json")}
	for _, root := range projectRoots {
		paths = append(paths, filepath.Join(root, ".claude", "settings.json"), filepath.Join(root, ".claude", "settings.local.json"))
	}
	for _, path := range paths {
		contents, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var next claudeSkillSettings
		if json.Unmarshal(contents, &next) != nil {
			continue
		}
		for name, enabled := range next.EnabledPlugins {
			merged.EnabledPlugins[name] = enabled
		}
		for name, state := range next.SkillOverrides {
			merged.SkillOverrides[name] = state
		}
	}
	return merged
}

type claudeSkillFrontmatter struct {
	Name          string `yaml:"name"`
	Description   string `yaml:"description"`
	WhenToUse     string `yaml:"when_to_use"`
	UserInvocable *bool  `yaml:"user-invocable"`
}

func scanClaudeSkillDirectory(root, scope, namespace string, skipSynced bool) []Skill {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	items := make([]Skill, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || skipSynced && strings.EqualFold(entry.Name(), "synced") {
			continue
		}
		path := filepath.Join(root, entry.Name(), "SKILL.md")
		frontmatter, body, ok := readClaudeSkillFile(path)
		if !ok || frontmatter.UserInvocable != nil && !*frontmatter.UserInvocable {
			continue
		}
		commandName := entry.Name()
		if namespace != "" && strings.TrimSpace(frontmatter.Name) != "" {
			commandName = strings.TrimSpace(frontmatter.Name)
		}
		commandName = strings.TrimPrefix(commandName, namespace+":")
		name := commandName
		if namespace != "" {
			name = namespace + ":" + commandName
		}
		description := strings.TrimSpace(frontmatter.Description)
		if when := strings.TrimSpace(frontmatter.WhenToUse); when != "" {
			if description != "" {
				description += " "
			}
			description += when
		}
		if description == "" {
			description = firstClaudeSkillParagraph(body)
		}
		displayName := strings.TrimSpace(frontmatter.Name)
		if displayName == "" {
			displayName = entry.Name()
		}
		items = append(items, Skill{Name: name, DisplayName: displayName, Description: truncateRunes(description, 1536), ShortDescription: truncateRunes(description, 180), Path: path, Scope: scope})
	}
	return items
}

func readClaudeSkillFile(path string) (claudeSkillFrontmatter, string, bool) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return claudeSkillFrontmatter{}, "", false
	}
	text := strings.ReplaceAll(string(contents), "\r\n", "\n")
	if !strings.HasPrefix(text, "---\n") {
		return claudeSkillFrontmatter{}, text, true
	}
	end := strings.Index(text[4:], "\n---\n")
	if end < 0 {
		return claudeSkillFrontmatter{}, "", false
	}
	end += 4
	var frontmatter claudeSkillFrontmatter
	if yaml.Unmarshal([]byte(text[4:end]), &frontmatter) != nil {
		return claudeSkillFrontmatter{}, "", false
	}
	return frontmatter, text[end+5:], true
}

func firstClaudeSkillParagraph(body string) string {
	paragraphs := strings.Split(strings.TrimSpace(body), "\n\n")
	for _, paragraph := range paragraphs {
		lines := strings.Split(strings.TrimSpace(paragraph), "\n")
		plain := make([]string, 0, len(lines))
		for _, line := range lines {
			line = strings.TrimSpace(strings.TrimLeft(line, "#>-* "))
			if line != "" {
				plain = append(plain, line)
			}
		}
		if len(plain) > 0 {
			return strings.Join(plain, " ")
		}
	}
	return ""
}

func truncateRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}

func mergeClaudeSkills(target map[string]Skill, skills []Skill, overrides map[string]string) {
	for _, skill := range skills {
		switch overrides[skill.Name] {
		case "off":
			continue
		case "name-only":
			skill.Description = ""
			skill.ShortDescription = ""
		}
		target[skill.Name] = skill
	}
}

func claudeEnterpriseSkillRoots() []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{filepath.Join(string(filepath.Separator), "Library", "Application Support", "ClaudeCode", ".claude", "skills")}
	case "linux":
		return []string{filepath.Join(string(filepath.Separator), "etc", "claude-code", ".claude", "skills")}
	default:
		return nil
	}
}

func scanEnabledClaudePluginSkills(configRoot string, enabled map[string]bool) []Skill {
	items := make([]Skill, 0)
	for identifier, isEnabled := range enabled {
		if !isEnabled {
			continue
		}
		pluginName, marketplace, ok := strings.Cut(identifier, "@")
		if !ok || pluginName == "" || marketplace == "" {
			continue
		}
		root := newestClaudePluginVersion(filepath.Join(configRoot, "plugins", "cache", marketplace, pluginName))
		if root == "" {
			continue
		}
		namespace := pluginName
		manifestPath := filepath.Join(root, ".claude-plugin", "plugin.json")
		if contents, err := os.ReadFile(manifestPath); err == nil {
			var manifest struct {
				Name string `json:"name"`
			}
			if json.Unmarshal(contents, &manifest) == nil && strings.TrimSpace(manifest.Name) != "" {
				namespace = strings.TrimSpace(manifest.Name)
			}
		}
		items = append(items, scanClaudeSkillDirectory(filepath.Join(root, "skills"), "plugin", namespace, false)...)
	}
	return items
}

func newestClaudePluginVersion(root string) string {
	entries, err := os.ReadDir(root)
	if err != nil {
		return ""
	}
	best := ""
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if best == "" || compareClaudePluginVersions(entry.Name(), best) > 0 {
			best = entry.Name()
		}
	}
	if best == "" {
		return ""
	}
	return filepath.Join(root, best)
}

func compareClaudePluginVersions(left, right string) int {
	leftSemver, rightSemver := left, right
	if !strings.HasPrefix(leftSemver, "v") {
		leftSemver = "v" + leftSemver
	}
	if !strings.HasPrefix(rightSemver, "v") {
		rightSemver = "v" + rightSemver
	}
	if semver.IsValid(leftSemver) && semver.IsValid(rightSemver) {
		return semver.Compare(leftSemver, rightSemver)
	}
	return strings.Compare(left, right)
}
