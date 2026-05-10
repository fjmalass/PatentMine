package config

import (
	"testing"
)

func TestApplyCLI(t *testing.T) {
	tests := []struct {
		name            string
		initialSource   ImportSource
		cliSource       string
		cliKey          string
		expectedSource  ImportSource
		expectedKey     string
	}{
		{
			name:           "CLI overrides default",
			initialSource:  ImportSourceGoogle,
			cliSource:      "uspto",
			cliKey:         "test-key",
			expectedSource: ImportSourceUSPTO,
			expectedKey:    "test-key",
		},
		{
			name:           "Empty CLI source keeps initial",
			initialSource:  ImportSourceUSPTO,
			cliSource:      "",
			cliKey:         "new-key",
			expectedSource: ImportSourceUSPTO,
			expectedKey:    "new-key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{ImportSource: tt.initialSource}
			ApplyCLI(&cfg, tt.cliSource, tt.cliKey, "")
			if cfg.ImportSource != tt.expectedSource {
				t.Errorf("expected source %v, got %v", tt.expectedSource, cfg.ImportSource)
			}
			if cfg.USPTO.APIKey != tt.expectedKey {
				t.Errorf("expected key %v, got %v", tt.expectedKey, cfg.USPTO.APIKey)
			}
		})
	}
}
