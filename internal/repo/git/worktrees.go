package gitrepo

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	domainworkspaces "github.com/openmodu/onecatch/internal/domain/workspaces"
)

// WorktreeIdentity delegates .git file and linked checkout resolution to Git.
func (i *Inspector) WorktreeIdentity(ctx context.Context, path string) (domainworkspaces.Worktree, error) {
	values := make([]string, 3)
	for n, arg := range []string{"--show-toplevel", "--absolute-git-dir", "--git-common-dir"} {
		out, err := i.run(ctx, path, "rev-parse", "--path-format=absolute", arg)
		if err != nil {
			return domainworkspaces.Worktree{}, err
		}
		// Remove Git's record terminator, not whitespace belonging to the path.
		values[n] = strings.TrimSuffix(out, "\n")
		if resolved, err := filepath.EvalSymlinks(values[n]); err == nil {
			values[n] = resolved
		}
	}
	return domainworkspaces.Worktree{Root: values[0], GitDir: values[1], CommonDir: values[2]}, nil
}

func (i *Inspector) ListWorktrees(ctx context.Context, workspace string) ([]domainworkspaces.Worktree, error) {
	current, err := i.WorktreeIdentity(ctx, workspace)
	if err != nil {
		return nil, err
	}
	cwd, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		return nil, err
	}
	relative, err := filepath.Rel(current.Root, cwd)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("workspace is outside its repository")
	}
	output, err := i.run(ctx, workspace, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return nil, err
	}
	items := parseWorktrees(output)
	for index := range items {
		item := &items[index]
		item.Main = index == 0
		item.Path = filepath.Join(item.Root, relative)
		if item.Unavailable {
			continue
		}
		info, statErr := os.Stat(item.Path)
		if statErr != nil || !info.IsDir() {
			item.Unavailable = true
			item.Reason = "directory_missing"
			continue
		}
		identity, inspectErr := i.WorktreeIdentity(ctx, item.Path)
		if inspectErr != nil || identity.CommonDir != current.CommonDir {
			item.Unavailable = true
			item.Reason = "repository_unavailable"
			continue
		}
		item.Root, item.GitDir, item.CommonDir = identity.Root, identity.GitDir, identity.CommonDir
		item.Path, err = filepath.EvalSymlinks(item.Path)
		if err != nil {
			item.Unavailable = true
			item.Reason = "directory_missing"
			continue
		}
		item.Current = identity.GitDir == current.GitDir
	}
	return items, nil
}

func parseWorktrees(value string) []domainworkspaces.Worktree {
	var items []domainworkspaces.Worktree
	var current *domainworkspaces.Worktree
	for _, record := range strings.Split(value, "\x00") {
		key, value, _ := strings.Cut(record, " ")
		if key == "worktree" {
			items = append(items, domainworkspaces.Worktree{Root: value})
			current = &items[len(items)-1]
			continue
		}
		if current == nil {
			continue
		}
		switch key {
		case "branch":
			current.Branch = strings.TrimPrefix(value, "refs/heads/")
		case "HEAD":
			current.Head = value
		case "bare", "prunable":
			current.Unavailable = true
			current.Reason = key
			// Locked worktrees can still be used. The lock prevents pruning/removal.
		}
	}
	return items
}

// CreateWorktree starts at the project's current committed HEAD. Uncommitted
// and ignored files remain in the source checkout. The caller owns the unique
// destination and branch name; Git's normal collision checks are retained.
func (i *Inspector) CreateWorktree(ctx context.Context, workspace, destination, branch string) (domainworkspaces.Worktree, error) {
	source, err := i.WorktreeIdentity(ctx, workspace)
	if err != nil {
		return domainworkspaces.Worktree{}, err
	}
	cwd, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		return domainworkspaces.Worktree{}, err
	}
	relative, err := filepath.Rel(source.Root, cwd)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return domainworkspaces.Worktree{}, fmt.Errorf("workspace is outside its repository")
	}
	head, err := i.run(ctx, workspace, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		return domainworkspaces.Worktree{}, fmt.Errorf("the project needs an initial commit: %w", err)
	}
	head = strings.TrimSpace(head)
	if relative != "." {
		kind, err := i.run(ctx, workspace, "cat-file", "-t", head+":"+filepath.ToSlash(relative))
		if err != nil || strings.TrimSpace(kind) != "tree" {
			return domainworkspaces.Worktree{}, fmt.Errorf("the project subdirectory is absent from the committed revision")
		}
	}
	branch, err = i.validBranchName(ctx, workspace, branch)
	if err != nil {
		return domainworkspaces.Worktree{}, err
	}
	if !filepath.IsAbs(destination) {
		return domainworkspaces.Worktree{}, fmt.Errorf("worktree destination must be absolute")
	}
	if _, err := os.Lstat(destination); !os.IsNotExist(err) {
		return domainworkspaces.Worktree{}, fmt.Errorf("worktree destination already exists or is inaccessible")
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0700); err != nil {
		return domainworkspaces.Worktree{}, err
	}
	if _, err := i.run(ctx, workspace, "worktree", "add", "-b", branch, "--", destination, head); err != nil {
		return domainworkspaces.Worktree{}, err
	}
	path := filepath.Join(destination, relative)
	identity, err := i.WorktreeIdentity(ctx, path)
	if err != nil {
		return domainworkspaces.Worktree{}, err
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return domainworkspaces.Worktree{}, err
	}
	if identity.CommonDir != source.CommonDir {
		return domainworkspaces.Worktree{}, fmt.Errorf("created worktree has unexpected repository identity")
	}
	identity.Path, identity.Branch, identity.Head = path, branch, head
	return identity, nil
}
