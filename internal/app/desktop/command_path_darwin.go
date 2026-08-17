//go:build darwin

package desktop

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	loginPathStart = "__ONECATCH_LOGIN_PATH_START__"
	loginPathEnd   = "__ONECATCH_LOGIN_PATH_END__"
)

// prepareCommandEnvironment restores the command search path that Finder does
// not pass to GUI applications. Runtime CLIs are commonly installed under a
// user's npm, Homebrew, Cargo, Go, or Nix profile rather than /usr/bin.
func prepareCommandEnvironment() {
	home, _ := os.UserHomeDir()
	username := os.Getenv("USER")
	path := mergeCommandPaths(
		loginShellPath(),
		strings.Join(macOSFallbackCommandPaths(home, username), string(os.PathListSeparator)),
		os.Getenv("PATH"),
	)
	if path != "" {
		_ = os.Setenv("PATH", path)
	}
}

func loginShellPath() string {
	shell := strings.TrimSpace(os.Getenv("SHELL"))
	if !isExecutableFile(shell) {
		shell = "/bin/zsh"
	}
	if !isExecutableFile(shell) {
		return ""
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	command := `printf '\n__ONECATCH_LOGIN_PATH_START__%s__ONECATCH_LOGIN_PATH_END__\n' "$PATH"`
	output, err := exec.CommandContext(ctx, shell, "-ilc", command).CombinedOutput()
	if err != nil {
		return ""
	}
	return extractLoginShellPath(string(output))
}

func extractLoginShellPath(output string) string {
	start := strings.LastIndex(output, loginPathStart)
	if start < 0 {
		return ""
	}
	valueStart := start + len(loginPathStart)
	end := strings.Index(output[valueStart:], loginPathEnd)
	if end < 0 {
		return ""
	}
	return strings.TrimSpace(output[valueStart : valueStart+end])
}

func mergeCommandPaths(groups ...string) string {
	seen := make(map[string]struct{})
	merged := make([]string, 0)
	for _, group := range groups {
		for _, entry := range filepath.SplitList(group) {
			entry = strings.TrimSpace(entry)
			if entry == "" {
				continue
			}
			entry = filepath.Clean(entry)
			if _, ok := seen[entry]; ok {
				continue
			}
			seen[entry] = struct{}{}
			merged = append(merged, entry)
		}
	}
	return strings.Join(merged, string(os.PathListSeparator))
}

func macOSFallbackCommandPaths(home, username string) []string {
	paths := make([]string, 0, 11)
	if home != "" {
		paths = append(paths,
			filepath.Join(home, ".local", "bin"),
			filepath.Join(home, ".npm-global", "bin"),
			filepath.Join(home, ".cargo", "bin"),
			filepath.Join(home, "go", "bin"),
			filepath.Join(home, ".nix-profile", "bin"),
		)
	}
	paths = append(paths, "/opt/homebrew/bin", "/opt/homebrew/sbin", "/usr/local/bin")
	if username != "" {
		paths = append(paths, filepath.Join("/etc/profiles/per-user", username, "bin"))
	}
	return append(paths, "/run/current-system/sw/bin", "/nix/var/nix/profiles/default/bin")
}

func isExecutableFile(path string) bool {
	if path == "" || !filepath.IsAbs(path) {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && info.Mode().Perm()&0o111 != 0
}
