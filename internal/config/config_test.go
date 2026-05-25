package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveEnvRefsAndFileRefs(t *testing.T) {
	dir := t.TempDir()
	secretPath := filepath.Join(dir, "b2_key_id")
	if err := os.WriteFile(secretPath, []byte("key-from-file\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATENTMINE_CREDENTIALS_DIR", dir)
	t.Setenv("PATENTMINE_BACKUP_B2_KEY_ID", "file:${PATENTMINE_CREDENTIALS_DIR}/b2_key_id")

	keys := []string{"PATENTMINE_BACKUP_B2_KEY_ID"}
	resolveEnvRefs(keys)
	resolveFileRefs(keys)

	if got := os.Getenv("PATENTMINE_BACKUP_B2_KEY_ID"); got != "key-from-file" {
		t.Fatalf("resolved key = %q, want key-from-file", got)
	}
}

func TestLoadDotEnvDoesNotOverrideExistingEnv(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(envPath, []byte("PATENTMINE_BACKUP_B2_KEY_ID=file:/tmp/should-not-read\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATENTMINE_BACKUP_B2_KEY_ID", "ci-value")
	loaded := loadDotEnv(envPath)
	resolveEnvRefs(loaded)
	resolveFileRefs(loaded)

	if got := os.Getenv("PATENTMINE_BACKUP_B2_KEY_ID"); got != "ci-value" {
		t.Fatalf("key = %q, want ci-value", got)
	}
}
