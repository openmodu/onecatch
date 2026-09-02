package agentrun

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/openmodu/modu/pkg/coding_agent/foundation/resource"
	moduskills "github.com/openmodu/modu/pkg/skills"
)

func (r *ModuRunner) ListSkills(_ context.Context, cwd string, environment []string) ([]Skill, error) {
	return listModuSkills(r.agentDir, cwd, environment)
}

func listModuSkills(agentDir, cwd string, environment []string) ([]Skill, error) {
	home := environmentValue(environment, "HOME")
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	if strings.TrimSpace(agentDir) == "" {
		agentDir = filepath.Join(home, ".modu")
	}
	if strings.TrimSpace(cwd) == "" {
		cwd = home
	}

	manager := moduskills.NewManager(agentDir, cwd)
	snapshot := resource.NewLoader(agentDir, cwd).LoadResources()
	refs := make([]moduskills.PathRef, 0, len(snapshot.SkillPaths))
	for _, ref := range snapshot.SkillPaths {
		refs = append(refs, moduskills.PathRef{Path: ref.Path, Source: ref.Source})
	}
	manager.SetExtraPaths(refs)
	if err := manager.Discover(); err != nil {
		return nil, err
	}
	discovered := manager.List()
	items := make([]Skill, 0, len(discovered))
	for _, item := range discovered {
		items = append(items, Skill{Name: item.Name, DisplayName: item.Name, Description: item.Description, Path: item.FilePath, Scope: item.Source})
	}
	sort.Slice(items, func(i, j int) bool { return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name) })
	return items, nil
}
