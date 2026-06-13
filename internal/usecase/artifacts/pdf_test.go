package artifacts

import (
	"bufio"
	"bytes"
	"strconv"
	"strings"
	"testing"
	"time"

	domainorders "github.com/openmodu/oneshot/internal/domain/orders"
)

func TestRenderReportProducesValidPDF(t *testing.T) {
	pdf := renderReport(domainorders.Order{
		ID:          "order_delivered",
		AgentName:   "行业研究分析师",
		Status:      domainorders.StatusDelivered,
		Requirement: domainorders.Requirement{Prompt: "请帮我完成 2026 年中国 AI Agent 服务市场研究。"},
		CreatedAt:   time.Date(2026, 6, 13, 10, 30, 0, 0, time.UTC),
	})

	if !bytes.HasPrefix(pdf, []byte("%PDF-1.")) {
		t.Fatalf("missing PDF header: %q", pdf[:min(16, len(pdf))])
	}
	if !bytes.HasSuffix(pdf, []byte("%%EOF\n")) {
		t.Fatal("missing EOF trailer")
	}
	if !bytes.Contains(pdf, []byte("order_delivered")) {
		t.Fatal("PDF content missing order id")
	}

	// startxref must point at the xref table.
	startxref := lastInt(t, pdf, "startxref")
	if startxref < 0 || startxref >= len(pdf) || !bytes.HasPrefix(pdf[startxref:], []byte("xref")) {
		t.Fatalf("startxref %d does not point at xref table", startxref)
	}

	// Every in-use xref offset must point at the start of its object.
	offsets := parseXrefOffsets(t, pdf[startxref:])
	for obj, off := range offsets {
		if off == 0 {
			continue // free object 0
		}
		want := strconv.Itoa(obj) + " 0 obj"
		if off >= len(pdf) || !bytes.HasPrefix(pdf[off:], []byte(want)) {
			t.Fatalf("xref offset for obj %d (=%d) does not point at %q", obj, off, want)
		}
	}
}

func lastInt(t *testing.T, pdf []byte, keyword string) int {
	t.Helper()
	idx := bytes.LastIndex(pdf, []byte(keyword))
	if idx < 0 {
		t.Fatalf("keyword %q not found", keyword)
	}
	rest := strings.TrimSpace(string(pdf[idx+len(keyword):]))
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		t.Fatalf("no value after %q", keyword)
	}
	n, err := strconv.Atoi(fields[0])
	if err != nil {
		t.Fatalf("value after %q is not an int: %v", keyword, err)
	}
	return n
}

func parseXrefOffsets(t *testing.T, xref []byte) []int {
	t.Helper()
	scanner := bufio.NewScanner(bytes.NewReader(xref))
	scanner.Scan() // "xref"
	scanner.Scan() // "0 N"
	header := strings.Fields(scanner.Text())
	if len(header) != 2 {
		t.Fatalf("bad xref subsection header: %q", scanner.Text())
	}
	count, err := strconv.Atoi(header[1])
	if err != nil {
		t.Fatalf("bad xref count: %v", err)
	}
	offsets := make([]int, 0, count)
	for i := range count {
		if !scanner.Scan() {
			t.Fatalf("xref truncated at entry %d", i)
		}
		fields := strings.Fields(scanner.Text())
		off, err := strconv.Atoi(fields[0])
		if err != nil {
			t.Fatalf("bad xref offset %q: %v", scanner.Text(), err)
		}
		offsets = append(offsets, off)
	}
	return offsets
}
