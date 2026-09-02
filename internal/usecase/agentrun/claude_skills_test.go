package agentrun

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestAdaptClaudeSkillMentionsUsesNativeSlashSyntax(t *testing.T) {
	tests := map[string]string{
		"$a-stock-data 昨天行情":      "/a-stock-data 昨天行情",
		"请使用 $telegram:access 检查": "请使用 /telegram:access 检查",
		"价格 price$usd，金额 $100":    "价格 price$usd，金额 /100",
		"没有 marker":               "没有 marker",
	}
	for input, want := range tests {
		if got := adaptClaudeSkillMentions(input); got != want {
			t.Errorf("adaptClaudeSkillMentions(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestClaudeRunnerListsPersonalProjectAndEnabledPluginSkills(t *testing.T) {
	home := t.TempDir()
	configRoot := filepath.Join(home, ".claude")
	workspace := filepath.Join(home, "repo")
	mustMkdirAll(t, filepath.Join(workspace, ".git"))

	writeClaudeSkill(t, filepath.Join(workspace, ".claude", "skills", "a-stock-data", "SKILL.md"), "project stock data", "")
	writeClaudeSkill(t, filepath.Join(workspace, ".claude", "skills", "project-only", "SKILL.md"), "project helper", "")
	writeClaudeSkill(t, filepath.Join(workspace, ".claude", "skills", "hidden", "SKILL.md"), "hidden helper", "user-invocable: false\n")
	writeClaudeSkill(t, filepath.Join(workspace, ".claude", "skills", "disabled", "SKILL.md"), "disabled helper", "")
	writeClaudeSkill(t, filepath.Join(configRoot, "skills", "a-stock-data", "SKILL.md"), "personal stock data", "name: A Stock Data\n")
	writeClaudeSkill(t, filepath.Join(configRoot, "skills", "synced", "synced-only", "SKILL.md"), "synced helper", "")

	settings := `{"enabledPlugins":{"telegram@claude-plugins-official":true},"skillOverrides":{"disabled":"off"}}`
	mustWriteFile(t, filepath.Join(configRoot, "settings.json"), settings)
	for _, version := range []string{"0.0.6", "0.0.7"} {
		pluginRoot := filepath.Join(configRoot, "plugins", "cache", "claude-plugins-official", "telegram", version)
		mustWriteFile(t, filepath.Join(pluginRoot, ".claude-plugin", "plugin.json"), `{"name":"telegram"}`)
		writeClaudeSkill(t, filepath.Join(pluginRoot, "skills", "access", "SKILL.md"), "plugin "+version, "")
	}

	runner := NewClaudeRunner("")
	skills, err := runner.ListSkills(context.Background(), workspace, []string{"HOME=" + home, "CLAUDE_CONFIG_DIR=" + configRoot})
	if err != nil {
		t.Fatal(err)
	}
	byName := make(map[string]Skill, len(skills))
	for _, skill := range skills {
		byName[skill.Name] = skill
	}
	if got := byName["a-stock-data"]; got.Scope != "personal" || got.Description != "personal stock data" || got.DisplayName != "A Stock Data" {
		t.Fatalf("personal precedence = %+v", got)
	}
	if got := byName["project-only"]; got.Scope != "project" || got.Description != "project helper" {
		t.Fatalf("project skill = %+v", got)
	}
	if got := byName["synced-only"]; got.Scope != "synced" {
		t.Fatalf("synced skill = %+v", got)
	}
	if got := byName["telegram:access"]; got.Scope != "plugin" || got.Description != "plugin 0.0.7" {
		t.Fatalf("namespaced latest plugin skill = %+v", got)
	}
	for _, hidden := range []string{"hidden", "disabled"} {
		if _, ok := byName[hidden]; ok {
			t.Fatalf("%q should not be user-invocable: %+v", hidden, byName[hidden])
		}
	}
}

func TestClaudeRunnerMergesSkillsObservedInSystemInit(t *testing.T) {
	home := t.TempDir()
	workspace := filepath.Join(home, "repo")
	mustMkdirAll(t, workspace)
	stream := `{"type":"system","subtype":"init","session_id":"skills-session","skills":["debug","remote-plugin:review"]}
{"type":"result","subtype":"success","is_error":false,"result":"done","session_id":"skills-session"}`
	runner := NewClaudeRunner(stubBinary(t, stream, "", 0))
	if _, err := runner.Run(context.Background(), Request{Workspace: workspace, Prompt: "go"}, nil); err != nil {
		t.Fatal(err)
	}
	skills, err := runner.ListSkills(context.Background(), workspace, []string{"HOME=" + home})
	if err != nil {
		t.Fatal(err)
	}
	byName := make(map[string]Skill, len(skills))
	for _, skill := range skills {
		byName[skill.Name] = skill
	}
	for _, name := range []string{"debug", "remote-plugin:review"} {
		if got := byName[name]; got.Scope != "runtime" {
			t.Fatalf("observed skill %q = %+v", name, got)
		}
	}
}

func writeClaudeSkill(t *testing.T, path, description, extraFrontmatter string) {
	t.Helper()
	mustWriteFile(t, path, "---\n"+extraFrontmatter+"description: "+description+"\n---\n\n# Instructions\n")
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustWriteFile(t *testing.T, path, contents string) {
	t.Helper()
	mustMkdirAll(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}
