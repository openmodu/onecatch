package artifacts

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	domainorders "github.com/openmodu/oneshot/internal/domain/orders"
	"github.com/openmodu/oneshot/internal/domain/users"
	repoartifacts "github.com/openmodu/oneshot/internal/repo/artifacts"
	repoorders "github.com/openmodu/oneshot/internal/repo/orders"
)

func deliveredOrder() domainorders.Order {
	return domainorders.Order{
		ID:        "order_out_1",
		UserID:    users.DevUserID,
		AgentName: "行业研究分析师",
		Status:    domainorders.StatusDelivered,
	}
}

func newArtifactsUsecase(t *testing.T) (*Usecase, repoorders.OrdersRepo) {
	t.Helper()
	orderRepo := repoorders.NewOrdersRepo(nil)
	uc := NewUsecase(repoartifacts.NewArtifactsRepo(nil), orderRepo)
	return uc, orderRepo
}

func TestRecordWorkspaceOutputCollectsRealFilesAndSummary(t *testing.T) {
	ctx := context.Background()
	uc, orderRepo := newArtifactsUsecase(t)
	order := deliveredOrder()
	if err := orderRepo.SaveOrder(ctx, order); err != nil {
		t.Fatalf("save order: %v", err)
	}

	ws := t.TempDir()
	mustWrite(t, filepath.Join(ws, "report.md"), "# 报告")
	mustWrite(t, filepath.Join(ws, "data.csv"), "a,b\n1,2")
	// Scratch dirs and dotfiles must be ignored.
	if err := os.MkdirAll(filepath.Join(ws, "node_modules", "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(ws, "node_modules", "pkg", "index.js"), "x")
	mustWrite(t, filepath.Join(ws, ".hidden"), "secret")

	out, err := uc.RecordWorkspaceOutput(ctx, order, ws, "我整理了报告与数据")
	if err != nil {
		t.Fatalf("record: %v", err)
	}

	got := map[string]bool{}
	for _, a := range out {
		got[a.FileName] = true
	}
	for _, want := range []string{"report.md", "data.csv", summaryFileName} {
		if !got[want] {
			t.Fatalf("missing artifact %q in %v", want, got)
		}
	}
	if got["node_modules/pkg/index.js"] || got[".hidden"] {
		t.Fatalf("scratch/hidden files were not skipped: %v", got)
	}

	// Idempotent: a second call returns the same set without duplicating.
	again, err := uc.RecordWorkspaceOutput(ctx, order, ws, "再次")
	if err != nil {
		t.Fatalf("record again: %v", err)
	}
	if len(again) != len(out) {
		t.Fatalf("idempotency broken: first=%d second=%d", len(out), len(again))
	}
}

func TestDownloadServesRealFileBytes(t *testing.T) {
	ctx := context.Background()
	uc, orderRepo := newArtifactsUsecase(t)
	order := deliveredOrder()
	if err := orderRepo.SaveOrder(ctx, order); err != nil {
		t.Fatalf("save order: %v", err)
	}
	ws := t.TempDir()
	mustWrite(t, filepath.Join(ws, "report.md"), "真实内容")

	out, err := uc.RecordWorkspaceOutput(ctx, order, ws, "done")
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	var reportID string
	for _, a := range out {
		if a.FileName == "report.md" {
			reportID = a.ID
		}
	}
	if reportID == "" {
		t.Fatal("report.md not recorded")
	}

	dl, err := uc.Download(ctx, order.UserID, reportID)
	if err != nil {
		t.Fatalf("download: %v", err)
	}
	if string(dl.Content) != "真实内容" {
		t.Fatalf("content = %q, want 真实内容", string(dl.Content))
	}
	if dl.ContentType != "text/markdown; charset=utf-8" {
		t.Fatalf("content type = %q", dl.ContentType)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
