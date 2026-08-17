package artifacts

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	domainorders "github.com/openmodu/onecatch/internal/domain/orders"
)

// renderReport renders a structurally valid single-page PDF for the order, so
// the downloaded file opens in any PDF reader. ASCII fields render with the
// standard Helvetica font; rendering non-Latin glyphs (e.g. the Chinese
// requirement text) requires embedding a CJK font, which is left as future
// work — the bytes are still present and text-extractable.
func renderReport(order domainorders.Order) []byte {
	lines := []string{
		"OneCatch Delivery Report",
		"",
		"Order:   " + order.ID,
		"Agent:   " + order.AgentName,
		"Status:  " + string(order.Status),
		"Created: " + order.CreatedAt.UTC().Format("2006-01-02 15:04 UTC"),
		"",
		"Requirement:",
	}
	lines = append(lines, wrapRunes(order.Requirement.Prompt, 70)...)
	return buildPDF(lines)
}

// buildPDF assembles a minimal PDF 1.4 document with one Helvetica text page,
// computing object offsets and the xref table by hand.
func buildPDF(lines []string) []byte {
	var content bytes.Buffer
	content.WriteString("BT\n/F1 12 Tf\n72 770 Td\n16 TL\n")
	for i, line := range lines {
		content.WriteString("(" + escapePDFText(line) + ") Tj\n")
		if i < len(lines)-1 {
			content.WriteString("T*\n")
		}
	}
	content.WriteString("ET\n")

	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>",
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		"<< /Length " + strconv.Itoa(content.Len()) + " >>\nstream\n" + content.String() + "endstream",
	}

	var pdf bytes.Buffer
	pdf.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objects))
	for i, body := range objects {
		offsets[i] = pdf.Len()
		fmt.Fprintf(&pdf, "%d 0 obj\n%s\nendobj\n", i+1, body)
	}

	xrefStart := pdf.Len()
	fmt.Fprintf(&pdf, "xref\n0 %d\n", len(objects)+1)
	pdf.WriteString("0000000000 65535 f \n")
	for _, off := range offsets {
		fmt.Fprintf(&pdf, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&pdf, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xrefStart)
	return pdf.Bytes()
}

func escapePDFText(s string) string {
	return strings.NewReplacer(
		"\\", "\\\\",
		"(", "\\(",
		")", "\\)",
		"\r", "",
		"\n", "",
	).Replace(s)
}

// wrapRunes splits text into rune-bounded lines so multibyte content never
// breaks mid-character.
func wrapRunes(s string, width int) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return []string{"(none)"}
	}
	runes := []rune(s)
	var out []string
	for len(runes) > width {
		out = append(out, string(runes[:width]))
		runes = runes[width:]
	}
	return append(out, string(runes))
}
