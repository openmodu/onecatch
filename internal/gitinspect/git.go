package gitinspect

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	domainworkspaces "github.com/openmodu/oneshot/internal/domain/workspaces"
)

type Inspector struct {
	binary string
}

func New(binary string) *Inspector {
	if binary == "" {
		binary = "git"
	}
	return &Inspector{binary: binary}
}

func (i *Inspector) Inspect(ctx context.Context, workspace string) (domainworkspaces.GitSnapshot, error) {
	if _, err := exec.LookPath(i.binary); err != nil {
		return domainworkspaces.GitSnapshot{}, fmt.Errorf("git is unavailable: %w", err)
	}
	inside, err := i.run(ctx, workspace, "rev-parse", "--is-inside-work-tree")
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return domainworkspaces.GitSnapshot{IsRepo: false}, nil
		}
		return domainworkspaces.GitSnapshot{}, err
	}
	if strings.TrimSpace(inside) != "true" {
		return domainworkspaces.GitSnapshot{IsRepo: false}, nil
	}
	status, err := i.run(ctx, workspace, "status", "--porcelain=v1", "--untracked-files=all")
	if err != nil {
		return domainworkspaces.GitSnapshot{}, err
	}
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
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
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
		Status:     strings.TrimSpace(status),
		DiffStat:   strings.TrimSpace(diffStat),
		StagedStat: strings.TrimSpace(stagedStat),
		Files:      parseStatus(status),
	}, nil
}

func (i *Inspector) Diff(ctx context.Context, workspace string, staged bool) (string, error) {
	args := []string{"diff", "--no-ext-diff", "--no-color"}
	if staged {
		args = append(args, "--cached")
	}
	return i.run(ctx, workspace, args...)
}

func (i *Inspector) StageAll(ctx context.Context, workspace string) error {
	_, err := i.run(ctx, workspace, "add", "--all")
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

func parseStatus(value string) []domainworkspaces.GitFile {
	lines := strings.Split(strings.TrimSpace(value), "\n")
	files := make([]domainworkspaces.GitFile, 0, len(lines))
	for _, line := range lines {
		if len(line) < 3 {
			continue
		}
		path := strings.TrimSpace(line[3:])
		if index := strings.LastIndex(path, " -> "); index >= 0 {
			path = path[index+4:]
		}
		files = append(files, domainworkspaces.GitFile{Path: path, Index: string(line[0]), Worktree: string(line[1])})
	}
	return files
}

func (i *Inspector) run(ctx context.Context, workspace string, args ...string) (string, error) {
	commandArgs := append([]string{"-C", workspace}, args...)
	output, err := exec.CommandContext(ctx, i.binary, commandArgs...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return string(output), nil
}
