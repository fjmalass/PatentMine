package pdf

import (
	"os"
	"path/filepath"
	"testing"
)

func TestClean(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"hyphen break", "exam-\nple text", "example text"},
		{"runs of spaces", "a    b\tc", "a b c"},
		{"trailing spaces", "line one   \nline two", "line one\nline two"},
		{"collapse blank lines", "a\n\n\n\n\nb", "a\n\nb"},
		{"crlf to lf", "a\r\nb", "a\nb"},
		{"trim surrounding", "\n\n  hello  \n\n", "hello"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := clean(c.in); got != c.want {
				t.Fatalf("clean(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestExtractTextMissingFile(t *testing.T) {
	if _, err := ExtractText(filepath.Join(t.TempDir(), "nope.pdf")); err == nil {
		t.Fatal("ExtractText on a missing file: want error, got nil")
	}
}

// TestExtractTextMalformedDoesNotPanic feeds non-PDF bytes (a .txt renamed) and
// asserts the recover guard turns the underlying reader's panic/error into a
// returned error rather than crashing the process.
func TestExtractTextMalformedDoesNotPanic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fake.pdf")
	if err := os.WriteFile(path, []byte("this is not a pdf at all\n"), 0o644); err != nil {
		t.Fatalf("write fake pdf: %v", err)
	}
	text, err := ExtractText(path)
	if err == nil {
		t.Fatal("ExtractText on malformed input: want error, got nil")
	}
	if text != "" {
		t.Fatalf("ExtractText on malformed input: want empty text, got %q", text)
	}
}
