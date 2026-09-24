package gitrepo

import "testing"

func TestParseWorktreesNULPathsAndFlags(t *testing.T) {
	items := parseWorktrees("worktree /repo with\nnewline\x00HEAD abc\x00branch refs/heads/main\x00\x00worktree /linked 中文\x00HEAD def\x00detached\x00locked travelling disk\x00\x00worktree /gone\x00prunable missing gitdir\x00\x00worktree /bare\x00bare\x00\x00")
	if len(items) != 4 {
		t.Fatalf("records: %+v", items)
	}
	if items[0].Root != "/repo with\nnewline" || items[0].Branch != "main" {
		t.Fatalf("path parse: %+v", items[0])
	}
	if items[1].Branch != "" || items[1].Head != "def" || items[1].Unavailable {
		t.Fatalf("locked checkout should remain usable: %+v", items[1])
	}
	if !items[2].Unavailable || !items[3].Unavailable {
		t.Fatalf("invalid directories: %+v", items)
	}
}
