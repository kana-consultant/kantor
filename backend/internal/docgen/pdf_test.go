package docgen

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/kana-consultant/kantor/backend/internal/config"
)

var pdfPageRe = regexp.MustCompile(`/Type\s*/Page\b`)
var pdfCountRe = regexp.MustCompile(`/Type\s*/Pages\b[^>]*?/Count\s+(\d+)|/Count\s+(\d+)[^>]*?/Type\s*/Pages\b`)

// pdfPages counts the page objects of a PDF and cross-checks the page tree's
// /Count and PDFPageCount.
func pdfPages(t *testing.T, pdf []byte) int {
	t.Helper()
	n := len(pdfPageRe.FindAll(pdf, -1))
	if got := PDFPageCount(pdf); got != n {
		t.Fatalf("PDFPageCount = %d, page objects %d", got, n)
	}
	if m := pdfCountRe.FindSubmatch(pdf); m != nil {
		v := string(m[1])
		if v == "" {
			v = string(m[2])
		}
		if c, _ := strconv.Atoi(v); c != n {
			t.Fatalf("PDF has %d page objects but /Count %d", n, c)
		}
	}
	return n
}

func TestConverterUnavailable(t *testing.T) {
	c := NewConverter(ConverterConfig{})
	if c.Available() {
		t.Fatal("converter without Bin must be unavailable")
	}
	if _, err := c.Convert(context.Background(), []byte("x")); !errors.Is(err, ErrConverterUnavailable) {
		t.Fatalf("Convert: %v", err)
	}
	var nilConv *Converter
	if nilConv.Available() {
		t.Fatal("nil converter must be unavailable")
	}
}

// TestPDFTemplates converts the three templates with the real LibreOffice.
// It runs only when SOFFICE_BIN is set, e.g.
//
//	SOFFICE_BIN=/Applications/LibreOffice.app/Contents/MacOS/soffice \
//	DOCGEN_PDF_OUT=/tmp/pdf_out go test ./internal/docgen -run PDFTemplates -v
//
// With DOCGEN_PDF_OUT set the PDFs are written there for manual inspection.
func TestPDFTemplates(t *testing.T) {
	// Opt-in (a real conversion takes 10-20 s): the server auto-detects
	// LibreOffice, the tests only run it when SOFFICE_BIN names it.
	bin := os.Getenv("SOFFICE_BIN")
	if bin == "" || config.IsOffValue(bin) {
		t.Skip("SOFFICE_BIN not set")
	}

	var logoOpts []Option
	if logo, err := os.ReadFile(masterLogoPath()); masterLogoPath() != "" && err == nil {
		processed, err := ProcessLogo(logo)
		if err != nil {
			t.Fatalf("process logo: %v", err)
		}
		logoOpts = append(logoOpts, WithLogo(processed))
	} else {
		t.Log("master logo not available, rendering the slip without a logo")
	}

	slip := kamusPayload(t, TemplateSlip)
	if listLen(t, slip, "gaji") != 5 || listLen(t, slip, "potongan") != 4 || listLen(t, slip, "reimbursement") != 3 {
		t.Fatal("slip payload must carry 5 earnings, 4 deductions and 3 reimbursements")
	}

	type job struct {
		id    TemplateID
		docx  []byte
		pages int
	}
	jobs := []job{
		{id: TemplatePKWT, docx: mustRender(t, TemplatePKWT, kamusPayload(t, TemplatePKWT)), pages: 5},
		{id: TemplateNDA, docx: mustRender(t, TemplateNDA, kamusPayload(t, TemplateNDA)), pages: 5},
		{id: TemplateSlip, docx: mustRender(t, TemplateSlip, slip, logoOpts...), pages: 1},
	}
	docs := make([][]byte, len(jobs))
	for i, j := range jobs {
		docs[i] = j.docx
	}

	conv := NewConverter(ConverterConfig{Bin: bin, ProfileDir: filepath.Join(t.TempDir(), "lo-profile")})
	start := time.Now()
	pdfs, err := conv.ConvertBatch(context.Background(), docs) // one soffice call
	if err != nil {
		t.Fatalf("ConvertBatch: %v", err)
	}
	t.Logf("converted %d documents in one soffice call in %s", len(docs), time.Since(start).Round(time.Millisecond))

	outDir := os.Getenv("DOCGEN_PDF_OUT")
	if outDir != "" {
		if err := os.MkdirAll(outDir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for i, j := range jobs {
		pages := pdfPages(t, pdfs[i])
		t.Logf("%s: %d page(s), %d bytes", j.id, pages, len(pdfs[i]))
		if pages != j.pages {
			t.Errorf("%s: %d pages, want %d", j.id, pages, j.pages)
		}
		if outDir != "" {
			name := filepath.Join(outDir, j.id.baseName()+".pdf")
			if err := os.WriteFile(name, pdfs[i], 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func (id TemplateID) baseName() string {
	s := string(id)
	return s[:len(s)-len(filepath.Ext(s))]
}
