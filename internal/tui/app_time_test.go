package tui

import "testing"

func TestParseTimeDuration(t *testing.T) {
	ok := map[string]int{
		"30m":     1800,
		"1h":      3600,
		"1h15m":   4500,
		"1:15":    4500,
		"0:45":    2700,
		"45":      2700, // plain minutes
		"90":      5400,
		" 30m ":   1800,
	}
	for in, want := range ok {
		got, err := parseTimeDuration(in)
		if err != nil {
			t.Errorf("parseTimeDuration(%q) error: %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("parseTimeDuration(%q) = %d, want %d", in, got, want)
		}
	}
	for _, bad := range []string{"", "0", "0m", "-5m", "abc", ":"} {
		if _, err := parseTimeDuration(bad); err == nil {
			t.Errorf("parseTimeDuration(%q): want error, got nil", bad)
		}
	}
}
