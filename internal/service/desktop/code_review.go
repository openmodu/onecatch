package desktop

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	pathpkg "path"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	domainworkspaces "github.com/openmodu/onecatch/internal/domain/workspaces"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
)

const (
	maxCodeReviewInputBytes = 240 * 1024
	maxCodeReviewFindings   = 50
	codeReviewTimeout       = 5 * time.Minute
)

type CodeReviewInput struct {
	WorkspaceID string `json:"workspaceId"`
	Runtime     string `json:"runtime"`
	Language    string `json:"language,omitempty"`
}

type CodeReviewFinding struct {
	Priority  int    `json:"priority"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	File      string `json:"file"`
	StartLine int    `json:"startLine"`
	EndLine   int    `json:"endLine"`
}

type CodeReviewResult struct {
	Runtime    string              `json:"runtime"`
	Summary    string              `json:"summary"`
	Findings   []CodeReviewFinding `json:"findings"`
	Truncated  bool                `json:"truncated,omitempty"`
	ChangeHash string              `json:"changeHash"`
	ReviewedAt time.Time           `json:"reviewedAt"`
}

type codeReviewPayload struct {
	Files        []domainworkspaces.GitFile `json:"files"`
	StagedDiff   string                     `json:"stagedDiff,omitempty"`
	WorktreeDiff string                     `json:"worktreeDiff,omitempty"`
	Untracked    []codeReviewUntracked      `json:"untracked,omitempty"`
}

type codeReviewUntracked struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

func (a *Service) ReviewChanges(ctx context.Context, input CodeReviewInput) (CodeReviewResult, error) {
	workspace, err := a.GetWorkspace(ctx, strings.TrimSpace(input.WorkspaceID))
	if err != nil {
		return CodeReviewResult{}, err
	}
	runtime := agentrun.Runtime(strings.TrimSpace(input.Runtime))
	if !runtime.Valid() {
		return CodeReviewResult{}, coded("runtime_invalid", "select a valid Agent for review")
	}
	settings, err := a.settings.Get(ctx)
	if err != nil {
		return CodeReviewResult{}, mapSettingsError(err)
	}
	if !settings.HarnessEnabled(string(runtime)) {
		return CodeReviewResult{}, coded("runtime_disabled", fmt.Sprintf("Agent %q is disabled in Settings", runtime))
	}
	if !a.runtimes.Available(runtime) {
		return CodeReviewResult{}, coded("runtime_unavailable", fmt.Sprintf("Agent %q is not available", runtime))
	}

	git := a.gitForWorkspace(workspace)
	snapshot, err := git.Inspect(ctx, workspace.Path)
	if err != nil {
		return CodeReviewResult{}, err
	}
	if !snapshot.IsRepo {
		return CodeReviewResult{}, coded("git_not_repository", "the workspace is not a Git repository")
	}
	if len(snapshot.Files) == 0 {
		return CodeReviewResult{}, coded("git_no_changes", "there are no changes to review")
	}
	staged, err := git.Diff(ctx, workspace.Path, true)
	if err != nil {
		return CodeReviewResult{}, err
	}
	worktree, err := git.Diff(ctx, workspace.Path, false)
	if err != nil {
		return CodeReviewResult{}, err
	}
	payload, partial := a.buildCodeReviewPayload(ctx, workspace, snapshot.Files, staged, worktree)
	payloadJSON, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return CodeReviewResult{}, fmt.Errorf("encode review changes: %w", err)
	}
	digest := sha256.Sum256(payloadJSON)

	temporary, err := os.MkdirTemp("", "onecatch-code-review-")
	if err != nil {
		return CodeReviewResult{}, fmt.Errorf("create code-review workspace: %w", err)
	}
	defer os.RemoveAll(temporary)

	runCtx, cancel := context.WithTimeout(ctx, codeReviewTimeout)
	defer cancel()
	result, err := a.runtimes.Run(runCtx, agentrun.Request{
		Runtime:   runtime,
		Workspace: temporary,
		Prompt:    codeReviewPrompt(payloadJSON, input.Language),
		Sandbox:   agentrun.SandboxReadOnly,
	}, nil)
	if err != nil {
		return CodeReviewResult{}, err
	}
	if !result.Succeeded {
		return CodeReviewResult{}, errors.New("Agent did not complete the code review")
	}
	allowed := make(map[string]struct{}, len(snapshot.Files))
	for _, file := range snapshot.Files {
		allowed[pathpkg.Clean(filepathSlash(file.Path))] = struct{}{}
	}
	summary, findings, err := parseCodeReviewResult(result.FinalMessage, allowed, input.Language)
	if err != nil {
		return CodeReviewResult{}, coded("code_review_invalid_response", err.Error())
	}
	return CodeReviewResult{
		Runtime:    string(runtime),
		Summary:    summary,
		Findings:   findings,
		Truncated:  partial,
		ChangeHash: hex.EncodeToString(digest[:]),
		ReviewedAt: time.Now().UTC(),
	}, nil
}

func (a *Service) buildCodeReviewPayload(ctx context.Context, workspace domainworkspaces.Workspace, files []domainworkspaces.GitFile, staged, worktree string) (codeReviewPayload, bool) {
	payload := codeReviewPayload{Files: append([]domainworkspaces.GitFile{}, files...), StagedDiff: staged, WorktreeDiff: worktree}
	partial := false
	for _, file := range files {
		if !strings.Contains(file.Index+file.Worktree, "?") {
			continue
		}
		document, err := a.ReadWorkspaceFile(ctx, workspace.ID, file.Path)
		if err != nil {
			partial = true
			continue
		}
		payload.Untracked = append(payload.Untracked, codeReviewUntracked{Path: file.Path, Content: document.Content})
	}
	components := []*string{&payload.StagedDiff, &payload.WorktreeDiff}
	for index := range payload.Untracked {
		components = append(components, &payload.Untracked[index].Content)
	}
	return payload, truncateCodeReviewComponents(components, maxCodeReviewInputBytes) || partial
}

func truncateCodeReviewComponents(components []*string, budget int) bool {
	truncated := false
	remaining := budget
	for index, component := range components {
		left := len(components) - index
		limit := 0
		if left > 0 && remaining > 0 {
			limit = remaining / left
		}
		value := *component
		if len(value) > limit {
			const marker = "\n[... OneCatch review input truncated ...]\n"
			contentLimit := limit - len(marker)
			if contentLimit < 0 {
				contentLimit = 0
			}
			value = utf8Prefix(value, contentLimit) + marker
			truncated = true
		}
		*component = value
		remaining -= len(value)
		if remaining < 0 {
			remaining = 0
		}
	}
	return truncated
}

func utf8Prefix(value string, limit int) string {
	if limit >= len(value) {
		return value
	}
	if limit <= 0 {
		return ""
	}
	for limit > 0 && !utf8.ValidString(value[:limit]) {
		limit--
	}
	return value[:limit]
}

func codeReviewPrompt(payload []byte, language string) string {
	responseLanguage := "English"
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(language)), "zh") {
		responseLanguage = "Simplified Chinese"
	}
	return `Review the supplied current Git changes. Do not modify files and do not use tools.

Find only concrete, actionable problems introduced by these changes: correctness bugs, regressions, security issues, data loss, concurrency problems, or material performance issues. Do not report style preferences, speculative concerns, or pre-existing problems.

Prioritize every finding:
- P0: release-blocking or catastrophic
- P1: high-impact bug that should be fixed immediately
- P2: normal actionable bug
- P3: low-impact but real bug

Each finding must name one changed file and the tightest relevant line range. Lines refer to the post-change file when available. If there are no actionable findings, return an empty findings array.

Return only one JSON object with this exact shape, without Markdown fences:
{"summary":"short overall assessment","findings":[{"priority":1,"title":"imperative concise title","body":"why this is a problem and when it occurs","file":"relative/path.go","startLine":12,"endLine":12}]}

Write summary, title, and body in ` + responseLanguage + `.

Git change payload:
` + string(payload)
}

func parseCodeReviewResult(message string, allowed map[string]struct{}, language string) (string, []CodeReviewFinding, error) {
	objects := jsonObjects(message)
	for _, object := range objects {
		summary, findings, content, ok := decodeCodeReviewObject(object, allowed)
		if ok {
			if summary == "" {
				if strings.HasPrefix(strings.ToLower(strings.TrimSpace(language)), "zh") {
					summary = map[bool]string{true: "未发现需要处理的问题。", false: "发现可处理的问题。"}[len(findings) == 0]
				} else if len(findings) == 0 {
					summary = "No actionable findings."
				} else {
					summary = "Actionable findings were identified."
				}
			}
			return summary, findings, nil
		}
		if content != "" && content != message {
			if summary, findings, err := parseCodeReviewResult(content, allowed, language); err == nil {
				return summary, findings, nil
			}
		}
	}
	return "", nil, errors.New("Agent returned no valid structured review")
}

func decodeCodeReviewObject(object string, allowed map[string]struct{}) (string, []CodeReviewFinding, string, bool) {
	var value map[string]json.RawMessage
	if json.Unmarshal([]byte(object), &value) != nil {
		return "", nil, "", false
	}
	var content string
	if raw, ok := value["content"]; ok {
		_ = json.Unmarshal(raw, &content)
	}
	rawFindings, ok := value["findings"]
	if !ok {
		return "", nil, content, false
	}
	var summary string
	_ = json.Unmarshal(value["summary"], &summary)
	summary = trimReviewText(summary, 2000)
	var items []map[string]json.RawMessage
	if json.Unmarshal(rawFindings, &items) != nil {
		return "", nil, content, false
	}
	findings := make([]CodeReviewFinding, 0, len(items))
	seen := make(map[string]struct{})
	for _, item := range items {
		finding, valid := normalizeCodeReviewFinding(item, allowed)
		if !valid {
			continue
		}
		key := fmt.Sprintf("%d\x00%s\x00%d\x00%s", finding.Priority, finding.File, finding.StartLine, finding.Title)
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		findings = append(findings, finding)
		if len(findings) == maxCodeReviewFindings {
			break
		}
	}
	if len(items) > 0 && len(findings) == 0 {
		return "", nil, content, false
	}
	sort.SliceStable(findings, func(i, j int) bool {
		if findings[i].Priority != findings[j].Priority {
			return findings[i].Priority < findings[j].Priority
		}
		if findings[i].File != findings[j].File {
			return findings[i].File < findings[j].File
		}
		return findings[i].StartLine < findings[j].StartLine
	})
	return summary, findings, content, true
}

func normalizeCodeReviewFinding(item map[string]json.RawMessage, allowed map[string]struct{}) (CodeReviewFinding, bool) {
	priority, ok := codeReviewPriority(item["priority"])
	if !ok {
		priority, ok = codeReviewPriority(item["severity"])
	}
	if !ok {
		return CodeReviewFinding{}, false
	}
	file := rawReviewString(item, "file", "path")
	file = pathpkg.Clean(filepathSlash(strings.TrimSpace(file)))
	if _, exists := allowed[file]; !exists {
		for _, prefix := range []string{"a/", "b/"} {
			candidate := strings.TrimPrefix(file, prefix)
			if _, candidateExists := allowed[candidate]; candidateExists {
				file = candidate
				exists = true
				break
			}
		}
		if !exists {
			return CodeReviewFinding{}, false
		}
	}
	if pathpkg.IsAbs(file) || file == "." || file == ".." || strings.HasPrefix(file, "../") {
		return CodeReviewFinding{}, false
	}
	startLine := rawReviewInt(item, "startLine", "start_line", "line")
	endLine := rawReviewInt(item, "endLine", "end_line")
	if startLine < 1 {
		return CodeReviewFinding{}, false
	}
	if endLine < startLine {
		endLine = startLine
	}
	if endLine-startLine > 20 {
		endLine = startLine + 20
	}
	title := trimReviewText(rawReviewString(item, "title"), 160)
	body := trimReviewText(rawReviewString(item, "body", "description"), 2000)
	if title == "" && body != "" {
		title = trimReviewText(body, 160)
	}
	if title == "" || body == "" {
		return CodeReviewFinding{}, false
	}
	return CodeReviewFinding{Priority: priority, Title: title, Body: body, File: file, StartLine: startLine, EndLine: endLine}, true
}

func codeReviewPriority(raw json.RawMessage) (int, bool) {
	if len(raw) == 0 {
		return 0, false
	}
	var number int
	if json.Unmarshal(raw, &number) == nil && number >= 0 && number <= 3 {
		return number, true
	}
	var value string
	if json.Unmarshal(raw, &value) != nil {
		return 0, false
	}
	value = strings.ToLower(strings.TrimSpace(value))
	if strings.HasPrefix(value, "p") {
		if parsed, err := strconv.Atoi(strings.TrimPrefix(value, "p")); err == nil && parsed >= 0 && parsed <= 3 {
			return parsed, true
		}
	}
	priorities := map[string]int{"critical": 0, "high": 1, "medium": 2, "normal": 2, "low": 3}
	priority, ok := priorities[value]
	return priority, ok
}

func rawReviewString(item map[string]json.RawMessage, keys ...string) string {
	for _, key := range keys {
		var value string
		if raw, ok := item[key]; ok && json.Unmarshal(raw, &value) == nil {
			return value
		}
	}
	return ""
}

func rawReviewInt(item map[string]json.RawMessage, keys ...string) int {
	for _, key := range keys {
		raw, ok := item[key]
		if !ok {
			continue
		}
		var value int
		if json.Unmarshal(raw, &value) == nil {
			return value
		}
	}
	return 0
}

func trimReviewText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) <= limit {
		return value
	}
	return strings.TrimSpace(utf8Prefix(value, limit))
}

func filepathSlash(value string) string {
	return strings.ReplaceAll(value, "\\", "/")
}

func jsonObjects(value string) []string {
	var objects []string
	start, depth := -1, 0
	inString, escaped := false, false
	for index, char := range value {
		if start < 0 {
			if char == '{' {
				start, depth = index, 1
				inString, escaped = false, false
			}
			continue
		}
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if char == '\\' {
				escaped = true
			} else if char == '"' {
				inString = false
			}
			continue
		}
		switch char {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				objects = append(objects, value[start:index+1])
				start = -1
			}
		}
	}
	return objects
}
