package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExportKeymapCSV(t *testing.T) {
	dir := t.TempDir()
	modePath := filepath.Join(dir, "keymap_mode.csv")
	keyPath := filepath.Join(dir, "keymap_key.csv")
	if err := exportKeymapCSV(modePath, keyPath); err != nil {
		t.Fatalf("exportKeymapCSV: %v", err)
	}
	for _, path := range []string{modePath, keyPath} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		content := string(data)
		if !strings.Contains(content, "label") {
			t.Errorf("expected CSV header in %s, got: %s", path, content)
		}
		if !strings.Contains(content, "Move down") {
			t.Errorf("expected global key in %s, got: %s", path, content)
		}
	}
}
