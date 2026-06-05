package doctext

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractTextDocx(t *testing.T) {
	path := writeZip(t, "sample.docx", map[string]string{
		"word/document.xml": `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>Claims 1-3 rejected.</w:t></w:r></w:p></w:body></w:document>`,
	})

	text, err := ExtractText(path)
	if err != nil {
		t.Fatalf("ExtractText: %v", err)
	}
	if !strings.Contains(text, "Claims 1-3 rejected.") {
		t.Fatalf("text = %q", text)
	}
}

func TestExtractTextXlsx(t *testing.T) {
	path := writeZip(t, "sample.xlsx", map[string]string{
		"xl/sharedStrings.xml":     `<sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><si><t>Reference</t></si><si><t>Status</t></si></sst>`,
		"xl/worksheets/sheet1.xml": `<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData><row><c t="s"><v>0</v></c><c t="s"><v>1</v></c></row><row><c t="inlineStr"><is><t>US123</t></is></c><c><v>42</v></c><c t="b"><v>1</v></c></row></sheetData></worksheet>`,
	})

	text, err := ExtractText(path)
	if err != nil {
		t.Fatalf("ExtractText: %v", err)
	}
	for _, want := range []string{"Reference\tStatus", "US123\t42\tTRUE"} {
		if !strings.Contains(text, want) {
			t.Fatalf("text = %q, missing %q", text, want)
		}
	}
}

func TestExtractTextLegacyXLSReportsUnsupported(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.xls")
	if err := os.WriteFile(path, []byte("not parsed"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := ExtractText(path)
	if err == nil || !strings.Contains(err.Error(), "legacy .xls") {
		t.Fatalf("ExtractText error = %v, want legacy .xls unsupported", err)
	}
}

func TestNeedsOCR(t *testing.T) {
	if !NeedsOCR("scan.pdf", "") {
		t.Fatal("empty PDF should report OCR needed")
	}
	if !NeedsOCR("image-only.docx", "   ") {
		t.Fatal("empty DOCX should report OCR needed")
	}
	if NeedsOCR("digital.pdf", "Claims 1-3 rejected") {
		t.Fatal("PDF with embedded text should not report OCR needed")
	}
	if NeedsOCR("empty.txt", "") {
		t.Fatal("empty text file should not report OCR needed")
	}
	if NeedsOCR("legacy.xls", "") {
		t.Fatal("unsupported legacy .xls should report parser error, not OCR needed")
	}
}

func writeZip(t *testing.T, name string, files map[string]string) string {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for path, content := range files {
		w, err := zw.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
