package tui

import (
	"fmt"
	"time"
)

func formatDurationShort(d time.Duration) string {
	if d <= 0 {
		return "0ms"
	}
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Round(time.Millisecond).Milliseconds())
	}
	d = d.Round(time.Second)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	if m >= 60 {
		h := m / 60
		m = m % 60
		return fmt.Sprintf("%dh%dm", h, m)
	}
	return fmt.Sprintf("%dm%ds", m, s)
}
