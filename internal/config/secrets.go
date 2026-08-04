package config

import (
	"os"
	"path/filepath"
)

// Secret basenames under CredentialsDir. Keep every credential filename here so
// config defaults, docs, setup checks, and UI helpers cannot drift.
const (
	SecretUSPTOODPKey          = "uspto_odp_key"
	SecretEPOOPSConsumerKey    = "epo_ops_consumer_key"
	SecretEPOOPSConsumerSecret = "epo_ops_consumer_secret"
	SecretReminderSMTPPassword = "smtp_password"
	SecretBackupB2KeyID        = "b2_key_id"
	SecretBackupB2Secret       = "b2_secret"
)

// CredentialsDir is the secure local directory for operator-managed secrets.
// PATENTMINE_CREDENTIALS_DIR overrides it; otherwise ~/.ssh/patentmine is used.
func CredentialsDir() string {
	if dir := expandHomePath(expandBraceEnv(firstNonEmptyEnv("PATENTMINE_CREDENTIALS_DIR"))); dir != "" {
		return dir
	}
	return expandHomePath("~/.ssh/patentmine")
}

// SecretFile returns CredentialsDir/<basename> using host-native separators.
func SecretFile(basename string) string {
	return filepath.Join(CredentialsDir(), basename)
}

func firstNonEmptyEnv(keys ...string) string {
	for _, key := range keys {
		if v := os.Getenv(key); v != "" {
			return v
		}
	}
	return ""
}
