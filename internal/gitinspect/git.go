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
	head, err := i.run(ctx, workspace, "rev-parse", "--verify", "HEAD")
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			return domainworkspaces.GitSnapshot{}, err
		}
		head = "" // valid repository with no commit yet
	}
	return domainworkspaces.GitSnapshot{
		IsRepo:   true,
		Head:     strings.TrimSpace(head),
		Status:   strings.TrimSpace(status),
		DiffStat: strings.TrimSpace(diffStat),
	}, nil
}

func (i *Inspector) run(ctx context.Context, workspace string, args ...string) (string, error) {
	commandArgs := append([]string{"-C", workspace}, args...)
	output, err := exec.CommandContext(ctx, i.binary, commandArgs...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return string(output), nil
}
