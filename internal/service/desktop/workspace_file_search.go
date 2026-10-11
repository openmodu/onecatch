package desktop

import (
	"context"
	"path"
	"sort"
	"strings"
)

type WorkspaceFileMatch struct {
	Path string
	Line int
	Text string
}
type WorkspaceFileSearch struct {
	Matches   []WorkspaceFileMatch
	Truncated bool
}

// SearchWorkspaceFiles uses Git's file inventory when available. A leading ?
// searches text instead of paths. Results and content reads are bounded.
func (a *Service) SearchWorkspaceFiles(ctx context.Context, id, query string) (WorkspaceFileSearch, error) {
	result := WorkspaceFileSearch{}
	content := strings.HasPrefix(strings.TrimSpace(query), "?")
	query = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(query), "?")))
	if query == "" {
		return result, nil
	}
	workspace, err := a.GetWorkspace(ctx, id)
	if err != nil {
		return result, err
	}
	paths, err := a.gitForWorkspace(workspace).FilePaths(ctx, workspace.Path)
	if err != nil {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		paths = nil
		queue := []string{""}
		visited := 0
		for len(queue) > 0 && len(paths) < 10000 {
			if ctx.Err() != nil {
				return result, ctx.Err()
			}
			visited++
			if visited > 2000 {
				result.Truncated = true
				break
			}
			dir := queue[0]
			queue = queue[1:]
			entries, listErr := a.ListWorkspaceFiles(ctx, id, dir)
			if listErr != nil {
				if dir == "" {
					return result, listErr
				}
				continue
			}
			for _, entry := range entries {
				if entry.Directory {
					if entry.Name != "node_modules" && entry.Name != "vendor" {
						queue = append(queue, entry.Path)
					}
				} else {
					paths = append(paths, entry.Path)
				}
			}
			if len(queue) > 2000 {
				result.Truncated = true
				break
			}
		}
		result.Truncated = result.Truncated || len(paths) >= 10000
	}
	sort.Strings(paths)
	scanned := 0
	remainingBytes := 16 << 20
	for _, name := range paths {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		blocked := false
		for _, part := range strings.Split(name, "/") {
			if blockedWorkspaceFileName(part) {
				blocked = true
				break
			}
		}
		if blocked {
			continue
		}
		if !content {
			if strings.Contains(strings.ToLower(name), query) {
				result.Matches = append(result.Matches, WorkspaceFileMatch{Path: name})
			}
		} else {
			if scanned >= 2000 || remainingBytes <= 0 {
				result.Truncated = true
				break
			}
			scanned++
			doc, readErr := a.ReadWorkspaceFile(ctx, id, name)
			if readErr != nil {
				continue
			}
			remainingBytes -= len(doc.Content)
			for i, line := range strings.Split(doc.Content, "\n") {
				if strings.Contains(strings.ToLower(line), query) {
					result.Matches = append(result.Matches, WorkspaceFileMatch{Path: name, Line: i + 1, Text: strings.TrimSpace(line)})
					break
				}
			}
		}
		if len(result.Matches) >= 200 {
			result.Truncated = true
			break
		}
	}
	// Exact base-name matches lead partial path matches.
	sort.SliceStable(result.Matches, func(i, j int) bool {
		return strings.ToLower(path.Base(result.Matches[i].Path)) == query && strings.ToLower(path.Base(result.Matches[j].Path)) != query
	})
	return result, nil
}
