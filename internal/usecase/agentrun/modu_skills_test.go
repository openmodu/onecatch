package agentrun

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestModuRunnerListsProjectUserAndPackageSkills(t *testing.T) {
	home := t.TempDir()
	agentDir := filepath.Join(home, ".modu")
	workspace := filepath.Join(home, "repo")
	mustWriteFile(t, filepath.Join(agentDir, "skills", "shared", "SKILL.md"), "---\ndescription: user copy\n---\n")
	mustWriteFile(t, filepath.Join(workspace, ".coding_agent", "skills", "shared", "SKILL.md"), "---\ndescription: project copy\n---\n")
	mustWriteFile(t, filepath.Join(agentDir, "packages", "team", "package.json"), `{"name":"team","skills":["skills/*/SKILL.md"]}`)
	mustWriteFile(t, filepath.Join(agentDir, "packages", "team", "skills", "packaged", "SKILL.md"), "---\ndescription: packaged helper\n---\n")

	runner := NewModuRunner("/bin/true")
	runner.agentDir = agentDir
	skills, err := runner.ListSkills(context.Background(), workspace, []string{"HOME=" + home})
	if err != nil {
		t.Fatal(err)
	}
	byName := make(map[string]Skill, len(skills))
	for _, skill := range skills {
		byName[skill.Name] = skill
	}
	if got := byName["shared"]; got.Scope != "project" || got.Description != "project copy" || got.Path != filepath.Join(workspace, ".coding_agent", "skills", "shared", "SKILL.md") {
		t.Fatalf("project precedence = %+v", got)
	}
	if got := byName["packaged"]; got.Scope != "user/team" || got.Description != "packaged helper" {
		t.Fatalf("package skill = %+v", got)
	}
	if _, ok := byName["skill-creator"]; !ok {
		t.Fatalf("built-in skill missing: %+v", skills)
	}
}

func TestModuRunnerDefaultsSkillRootFromProvidedHome(t *testing.T) {
	home := t.TempDir()
	mustWriteFile(t, filepath.Join(home, ".modu", "skills", "home-skill", "SKILL.md"), "---\ndescription: home helper\n---\n")
	skills, err := NewModuRunner("/bin/true").ListSkills(context.Background(), t.TempDir(), []string{"HOME=" + home})
	if err != nil {
		t.Fatal(err)
	}
	for _, skill := range skills {
		if skill.Name == "home-skill" {
			return
		}
	}
	t.Fatalf("home skill missing: %+v (process HOME %q)", skills, os.Getenv("HOME"))
}
