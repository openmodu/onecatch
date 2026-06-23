package artifacts

import (
	"context"
	"io/fs"
	"mime"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	domainartifacts "github.com/openmodu/oneshot/internal/domain/artifacts"
	domainorders "github.com/openmodu/oneshot/internal/domain/orders"
)

type Repository interface {
	NextArtifactID(context.Context) (string, error)
	NextShareToken(context.Context) (string, error)
	SaveArtifact(context.Context, domainartifacts.Artifact) error
	ListArtifacts(context.Context, string, string) ([]domainartifacts.Artifact, error)
	GetArtifact(context.Context, string, string) (domainartifacts.Artifact, error)
	SaveShare(context.Context, domainartifacts.Share) (domainartifacts.Share, error)
}

type OrderRepository interface {
	GetOrder(context.Context, string, string) (domainorders.Order, error)
}

type Usecase struct {
	repo   Repository
	orders OrderRepository
	now    func() time.Time
}

func NewUsecase(repo Repository, orders OrderRepository) *Usecase {
	return &Usecase{
		repo:   repo,
		orders: orders,
		now:    time.Now,
	}
}

// maxArtifactsPerOrder bounds how many produced files we surface, so a runaway
// agent (or a workspace that accreted a dependency tree) cannot flood the order
// with thousands of artifacts.
const maxArtifactsPerOrder = 100

// summaryFileName holds the agent's closing message, guaranteeing every
// delivered order has at least one human-readable deliverable.
const summaryFileName = "SUMMARY.md"

// RecordWorkspaceOutput is the simple form used when the agent works in the
// managed workspace: the summary and produced files live in the same dir.
func (s *Usecase) RecordWorkspaceOutput(ctx context.Context, order domainorders.Order, workspaceDir string, finalMessage string) ([]domainartifacts.Artifact, error) {
	return s.RecordRunOutput(ctx, order, workspaceDir, workspaceDir, true, finalMessage)
}

// RecordRunOutput turns what an agent produced into the order's deliverables.
// The agent's final message is written to SUMMARY.md in metaDir so there is
// always something to show; when collectFiles is true, every regular file in
// workspaceDir is also registered as an artifact whose bytes live on disk.
//
// It is safe across multi-turn resumes: files already recorded (by name) are
// skipped, so only new deliverables are added each turn — never duplicated.
func (s *Usecase) RecordRunOutput(ctx context.Context, order domainorders.Order, metaDir string, workspaceDir string, collectFiles bool, finalMessage string) ([]domainartifacts.Artifact, error) {
	if order.Status != domainorders.StatusDelivering && order.Status != domainorders.StatusDelivered {
		return nil, domainartifacts.ErrNotReady
	}

	existing, err := s.repo.ListArtifacts(ctx, order.UserID, order.ID)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool, len(existing))
	for _, a := range existing {
		seen[a.FileName] = true
	}

	if err := s.writeSummary(metaDir, order, finalMessage); err != nil {
		return nil, err
	}

	// The summary lives in metaDir; gather it plus any deliverables.
	type candidate struct{ rel, abs string }
	var candidates []candidate
	candidates = append(candidates, candidate{rel: summaryFileName, abs: filepath.Join(metaDir, summaryFileName)})
	if collectFiles {
		files, err := walkWorkspaceFiles(workspaceDir)
		if err != nil {
			return nil, err
		}
		for _, rel := range files {
			if rel == summaryFileName {
				continue // already added from metaDir
			}
			candidates = append(candidates, candidate{rel: rel, abs: filepath.Join(workspaceDir, rel)})
		}
	}

	out := make([]domainartifacts.Artifact, 0, len(candidates))
	for _, c := range candidates {
		if seen[filepath.ToSlash(c.rel)] {
			continue // recorded on a previous turn
		}
		info, statErr := os.Stat(c.abs)
		if statErr != nil {
			continue
		}
		id, idErr := s.repo.NextArtifactID(ctx)
		if idErr != nil {
			return nil, idErr
		}
		artifact := domainartifacts.Artifact{
			ID:         id,
			OrderID:    order.ID,
			UserID:     order.UserID,
			FileName:   filepath.ToSlash(c.rel),
			FileType:   fileType(c.rel),
			SizeBytes:  info.Size(),
			Preview:    previewLabel(c.rel),
			StorageURI: c.abs,
			CreatedAt:  s.now(),
		}
		if err := s.repo.SaveArtifact(ctx, artifact); err != nil {
			return nil, err
		}
		seen[artifact.FileName] = true
		out = append(out, artifact)
	}
	return out, nil
}

func (s *Usecase) writeSummary(workspaceDir string, order domainorders.Order, finalMessage string) error {
	body := strings.TrimSpace(finalMessage)
	if body == "" {
		body = "（本次任务未返回总结文本。）"
	}
	content := "# 任务总结\n\n- 订单：" + order.ID + "\n- Agent：" + order.AgentName + "\n\n" + body + "\n"
	return os.WriteFile(filepath.Join(workspaceDir, summaryFileName), []byte(content), 0o644)
}

func (s *Usecase) ListForOrder(ctx context.Context, userID string, orderID string) ([]domainartifacts.Artifact, error) {
	order, err := s.orders.GetOrder(ctx, userID, orderID)
	if err != nil {
		return nil, err
	}
	if order.Status != domainorders.StatusDelivered {
		return nil, domainartifacts.ErrNotReady
	}
	return s.repo.ListArtifacts(ctx, userID, orderID)
}

func (s *Usecase) Download(ctx context.Context, userID string, artifactID string) (domainartifacts.Download, error) {
	artifact, err := s.repo.GetArtifact(ctx, userID, artifactID)
	if err != nil {
		return domainartifacts.Download{}, err
	}
	order, err := s.orders.GetOrder(ctx, userID, artifact.OrderID)
	if err != nil {
		return domainartifacts.Download{}, err
	}
	if order.Status != domainorders.StatusDelivered {
		return domainartifacts.Download{}, domainartifacts.ErrNotReady
	}

	if artifact.StorageURI == "" {
		// Legacy/fallback path: synthesize a report when no real file is backing
		// the artifact (kept for resilience; the worker always stores files).
		return domainartifacts.Download{
			Artifact:    artifact,
			ContentType: "application/pdf",
			Content:     renderReport(order),
		}, nil
	}
	content, err := os.ReadFile(artifact.StorageURI)
	if err != nil {
		return domainartifacts.Download{}, err
	}
	return domainartifacts.Download{
		Artifact:    artifact,
		ContentType: contentType(artifact.FileName),
		Content:     content,
	}, nil
}

func (s *Usecase) Share(ctx context.Context, userID string, artifactID string) (domainartifacts.Share, error) {
	artifact, err := s.repo.GetArtifact(ctx, userID, artifactID)
	if err != nil {
		return domainartifacts.Share{}, err
	}
	order, err := s.orders.GetOrder(ctx, userID, artifact.OrderID)
	if err != nil {
		return domainartifacts.Share{}, err
	}
	if order.Status != domainorders.StatusDelivered {
		return domainartifacts.Share{}, domainartifacts.ErrNotReady
	}
	token, err := s.repo.NextShareToken(ctx)
	if err != nil {
		return domainartifacts.Share{}, err
	}
	return s.repo.SaveShare(ctx, domainartifacts.Share{
		ArtifactID: artifact.ID,
		Token:      token,
		URL:        "https://oneshot.local/share/" + token,
		CreatedAt:  s.now(),
	})
}

// skipDirs are directories an agent might create as scratch/dependency space
// that are not deliverables worth surfacing to the user.
var skipDirs = map[string]bool{
	".git":         true,
	"node_modules": true,
	".venv":        true,
	"__pycache__":  true,
	".cache":       true,
}

// collectFiles returns workspace-relative paths of regular files, skipping
// hidden files and known scratch directories, capped and sorted for stability.
func walkWorkspaceFiles(workspaceDir string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(workspaceDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			if path == workspaceDir {
				return nil
			}
			if skipDirs[name] || strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(name, ".") {
			return nil
		}
		rel, relErr := filepath.Rel(workspaceDir, path)
		if relErr != nil {
			return relErr
		}
		files = append(files, rel)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(files, func(i, j int) bool {
		// Surface the summary first, then everything else alphabetically.
		if files[i] == summaryFileName {
			return true
		}
		if files[j] == summaryFileName {
			return false
		}
		return files[i] < files[j]
	})
	if len(files) > maxArtifactsPerOrder {
		files = files[:maxArtifactsPerOrder]
	}
	return files, nil
}

// fileType is the short, user-facing label for an artifact's kind.
func fileType(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".md", ".markdown":
		return "Markdown"
	case ".pdf":
		return "PDF"
	case ".csv":
		return "CSV"
	case ".json":
		return "JSON"
	case ".txt":
		return "Text"
	case ".html", ".htm":
		return "HTML"
	case ".py":
		return "Python"
	case ".go":
		return "Go"
	case ".js", ".ts", ".jsx", ".tsx":
		return "Code"
	case "":
		return "File"
	default:
		return strings.ToUpper(strings.TrimPrefix(filepath.Ext(name), "."))
	}
}

func previewLabel(name string) string {
	return fileType(name) + " · " + filepath.Base(name)
}

// contentType resolves a MIME type for download, with sensible defaults for the
// text formats agents most often produce.
func contentType(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".md", ".markdown":
		return "text/markdown; charset=utf-8"
	case ".csv":
		return "text/csv; charset=utf-8"
	case ".txt":
		return "text/plain; charset=utf-8"
	case ".json":
		return "application/json"
	}
	if ct := mime.TypeByExtension(filepath.Ext(name)); ct != "" {
		return ct
	}
	return "application/octet-stream"
}
