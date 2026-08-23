package gitrepo

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	domainworkspaces "github.com/openmodu/onecatch/internal/domain/workspaces"
)

type Inspector struct {
	binary string
	runner CommandRunner
}

// CommandRunner executes Git on the machine that owns workspace. The desktop
// service supplies an SSH-backed runner for remote FS workspaces; keeping the
// Git parsing and validation here makes local and remote controls behave the
// same way.
type CommandRunner interface {
	Run(ctx context.Context, workspace string, args ...string) (string, error)
}

func New(binary string) *Inspector {
	if binary == "" {
		binary = "git"
	}
	return &Inspector{binary: binary}
}

func NewWithRunner(runner CommandRunner) *Inspector {
	return &Inspector{binary: "git", runner: runner}
}

func (i *Inspector) Inspect(ctx context.Context, workspace string) (domainworkspaces.GitSnapshot, error) {
	if _, err := exec.LookPath(i.binary); err != nil {
		return domainworkspaces.GitSnapshot{}, fmt.Errorf("git is unavailable: %w", err)
	}
	inside, err := i.run(ctx, workspace, "rev-parse", "--is-inside-work-tree")
	if err != nil {
		if commandExited(err) {
			return domainworkspaces.GitSnapshot{IsRepo: false}, nil
		}
		return domainworkspaces.GitSnapshot{}, err
	}
	if strings.TrimSpace(inside) != "true" {
		return domainworkspaces.GitSnapshot{IsRepo: false}, nil
	}
	status, err := i.run(ctx, workspace, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		return domainworkspaces.GitSnapshot{}, err
	}
	files := parseStatus(status)
	diffStat, err := i.run(ctx, workspace, "diff", "--stat", "--no-ext-diff")
	if err != nil {
		return domainworkspaces.GitSnapshot{}, err
	}
	stagedStat, err := i.run(ctx, workspace, "diff", "--cached", "--stat", "--no-ext-diff")
	if err != nil {
		return domainworkspaces.GitSnapshot{}, err
	}
	branch, _ := i.run(ctx, workspace, "symbolic-ref", "--short", "HEAD")
	ahead, behind := i.aheadBehind(ctx, workspace)
	head, err := i.run(ctx, workspace, "rev-parse", "--verify", "HEAD")
	if err != nil {
		if !commandExited(err) {
			return domainworkspaces.GitSnapshot{}, err
		}
		head = "" // valid repository with no commit yet
	}
	return domainworkspaces.GitSnapshot{
		IsRepo:     true,
		Head:       strings.TrimSpace(head),
		Branch:     strings.TrimSpace(branch),
		Ahead:      ahead,
		Behind:     behind,
		Status:     statusText(files),
		DiffStat:   strings.TrimSpace(diffStat),
		StagedStat: strings.TrimSpace(stagedStat),
		Files:      files,
	}, nil
}

func (i *Inspector) Diff(ctx context.Context, workspace string, staged bool) (string, error) {
	// core.quotePath=false keeps non-ASCII paths verbatim so diff headers match
	// the paths reported by parseStatus.
	args := []string{"-c", "core.quotePath=false", "diff", "--no-ext-diff", "--no-color"}
	if staged {
		args = append(args, "--cached")
	}
	return i.run(ctx, workspace, args...)
}

func (i *Inspector) StageAll(ctx context.Context, workspace string) error {
	_, err := i.run(ctx, workspace, "add", "--all")
	return err
}

func (i *Inspector) ListBranches(ctx context.Context, workspace string) ([]domainworkspaces.GitBranch, error) {
	output, err := i.run(ctx, workspace, "branch", "--format=%(refname:short)%00%(HEAD)%00%(upstream:short)")
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimRight(output, "\r\n"), "\n")
	branches := make([]domainworkspaces.GitBranch, 0, len(lines))
	for _, line := range lines {
		parts := strings.SplitN(strings.TrimSuffix(line, "\r"), "\x00", 3)
		if len(parts) < 2 || parts[0] == "" {
			continue
		}
		branch := domainworkspaces.GitBranch{Name: parts[0], Current: strings.TrimSpace(parts[1]) == "*"}
		if len(parts) == 3 {
			branch.Upstream = strings.TrimSpace(parts[2])
		}
		branches = append(branches, branch)
	}
	return branches, nil
}

func (i *Inspector) SwitchBranch(ctx context.Context, workspace, name string) error {
	name, err := i.validBranchName(ctx, workspace, name)
	if err != nil {
		return err
	}
	branches, err := i.ListBranches(ctx, workspace)
	if err != nil {
		return err
	}
	for _, branch := range branches {
		if branch.Name != name {
			continue
		}
		if branch.Current {
			return nil
		}
		_, err = i.run(ctx, workspace, "switch", "--", name)
		return err
	}
	return fmt.Errorf("git branch %q does not exist", name)
}

func (i *Inspector) CreateBranch(ctx context.Context, workspace, name string) error {
	name, err := i.validBranchName(ctx, workspace, name)
	if err != nil {
		return err
	}
	branches, err := i.ListBranches(ctx, workspace)
	if err != nil {
		return err
	}
	for _, branch := range branches {
		if branch.Name == name {
			return fmt.Errorf("git branch %q already exists", name)
		}
	}
	_, err = i.run(ctx, workspace, "switch", "-c", name)
	return err
}

func (i *Inspector) Commit(ctx context.Context, workspace, message string) (string, error) {
	message = strings.TrimSpace(message)
	if message == "" || strings.ContainsAny(message, "\r\n\x00") {
		return "", errors.New("commit message must be a single non-empty line")
	}
	if _, err := i.run(ctx, workspace, "commit", "-m", message); err != nil {
		return "", err
	}
	head, err := i.run(ctx, workspace, "rev-parse", "HEAD")
	return strings.TrimSpace(head), err
}

func (i *Inspector) Push(ctx context.Context, workspace, remote string) error {
	remote = strings.TrimSpace(remote)
	if remote == "" {
		remote = "origin"
	}
	if strings.HasPrefix(remote, "-") || strings.ContainsAny(remote, "\r\n\x00") {
		return errors.New("git remote is invalid")
	}
	_, err := i.run(ctx, workspace, "push", remote, "HEAD")
	return err
}

func (i *Inspector) aheadBehind(ctx context.Context, workspace string) (int, int) {
	value, err := i.run(ctx, workspace, "rev-list", "--left-right", "--count", "@{upstream}...HEAD")
	if err != nil {
		return 0, 0
	}
	var behind, ahead int
	_, _ = fmt.Sscanf(strings.TrimSpace(value), "%d %d", &behind, &ahead)
	return ahead, behind
}

func (i *Inspector) validBranchName(ctx context.Context, workspace, name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || strings.ContainsAny(name, "\r\n\x00") {
		return "", errors.New("git branch name is invalid")
	}
	if _, err := i.run(ctx, workspace, "check-ref-format", "--branch", name); err != nil {
		return "", errors.New("git branch name is invalid")
	}
	return name, nil
}

// parseStatus reads NUL separated `git status --porcelain=v1 -z` records. The
// record separator matters: the newline format C-quotes paths that are not
// plain ASCII, and its "XY path" prefix starts with a space whenever a change
// is unstaged, so trimming the payload would eat the first path's first byte.
func parseStatus(value string) []domainworkspaces.GitFile {
	records := strings.Split(value, "\x00")
	files := make([]domainworkspaces.GitFile, 0, len(records))
	for position := 0; position < len(records); position++ {
		record := records[position]
		if len(record) < 4 || record[2] != ' ' {
			continue
		}
		index, worktree := record[0], record[1]
		if index == 'R' || index == 'C' || worktree == 'R' || worktree == 'C' {
			position++ // renames and copies emit the original path as the next record
		}
		files = append(files, domainworkspaces.GitFile{Path: record[3:], Index: string(index), Worktree: string(worktree)})
	}
	return files
}

func statusText(files []domainworkspaces.GitFile) string {
	lines := make([]string, 0, len(files))
	for _, file := range files {
		lines = append(lines, fmt.Sprintf("%s%s %s", file.Index, file.Worktree, file.Path))
	}
	return strings.Join(lines, "\n")
}

func (i *Inspector) run(ctx context.Context, workspace string, args ...string) (string, error) {
	if i.runner != nil {
		return i.runner.Run(ctx, workspace, args...)
	}
	commandArgs := append([]string{"-C", workspace}, args...)
	output, err := exec.CommandContext(ctx, i.binary, commandArgs...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return string(output), nil
}

type exitCoder interface {
	ExitCode() int
}

func commandExited(err error) bool {
	var exitErr exitCoder
	return errors.As(err, &exitErr)
}
