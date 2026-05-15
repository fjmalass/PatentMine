package tui

import (
	"fmt"
	"strings"

	ansi "github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/lipgloss"
)

// overlayBase returns a base lipgloss style used by all overlay popup views.
func overlayBase() lipgloss.Style {
	return lipgloss.NewStyle()
}

func (m *Model) overlayBackdropCacheToken() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}
	idx := len(m.backStack) - 1
	for idx >= 0 {
		if !mustModeSpec(m.backStack[idx].mode).isOverlay {
			break
		}
		idx--
	}
	if idx < 0 {
		return fmt.Sprintf("empty:%d:%d", m.width, m.height)
	}
	snap := m.backStack[idx]
	return fmt.Sprintf("%d:%d:%s:%s:%s:%d", m.width, m.height, snap.mode, snap.current.Number, snap.ProjectID, idx)
}

func (m *Model) overlayBackdrop() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}
	cacheKey := m.overlayBackdropCacheToken()
	if cacheKey != "" && m.overlayBackdropCacheKey == cacheKey && m.overlayBackdropCache != "" {
		return m.overlayBackdropCache
	}

	idx := len(m.backStack) - 1
	for idx >= 0 {
		if !mustModeSpec(m.backStack[idx].mode).isOverlay {
			break
		}
		idx--
	}
	if idx < 0 {
		backdrop := lipgloss.Place(m.width, m.height, lipgloss.Left, lipgloss.Top, "")
		m.overlayBackdropCacheKey = cacheKey
		m.overlayBackdropCache = backdrop
		return backdrop
	}

	backdrop := *m
	backdrop.restore(m.backStack[idx])
	backdrop.width = m.width
	backdrop.height = m.height
	backdrop.backStack = append([]navSnapshot(nil), m.backStack[:idx]...)
	backdrop.jumpLabelsCache = nil

	plain := ansi.Strip(backdrop.renderView())
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color(ColorDim))
	lines := strings.Split(lipgloss.Place(m.width, m.height, lipgloss.Left, lipgloss.Top, plain), "\n")
	for i := range lines {
		lines[i] = dim.Render(lines[i])
	}
	backdropRendered := strings.Join(lines, "\n")
	m.overlayBackdropCacheKey = cacheKey
	m.overlayBackdropCache = backdropRendered
	return backdropRendered
}

func overlayLine(base, overlay string, x, totalWidth int) string {
	left := ansi.Cut(base, 0, x)
	rightStart := x + lipgloss.Width(overlay)
	right := ""
	if rightStart < totalWidth {
		right = ansi.Cut(base, rightStart, totalWidth)
	}
	return left + overlay + right
}

func (m *Model) previewOverlay(content string) string {
	popup := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color(ColorSubtle)).
		Padding(1, 2).
		Width(m.overlayWidth()).
		Render(content)

	if m.width <= 0 || m.height <= 0 {
		return popup
	}

	backdrop := m.overlayBackdrop()
	popupWidth := lipgloss.Width(popup)
	popupHeight := lipgloss.Height(popup)
	x := max(0, (m.width-popupWidth)/2)
	y := max(0, (m.height-popupHeight)/2)

	baseLines := strings.Split(lipgloss.Place(m.width, m.height, lipgloss.Left, lipgloss.Top, backdrop), "\n")
	popupLines := strings.Split(popup, "\n")
	scrimLine := strings.Repeat(" ", m.width)
	for i := 0; i < popupHeight && y+i < len(baseLines) && i < len(popupLines); i++ {
		baseLines[y+i] = overlayLine(scrimLine, popupLines[i], x, m.width)
	}
	return strings.Join(baseLines, "\n")
}

func (m *Model) overlayWidth() int {
	if m.width <= 0 {
		return OverlayFallbackWidth
	}

	baseWidth := m.width
	if len(m.backStack) > 0 {
		last := m.backStack[len(m.backStack)-1]
		if last.width > 0 {
			baseWidth = last.width
		}
	}

	width := int(float64(baseWidth) * OverlayExpandedRatio)
	width = max(width, OverlayMinWidth)

	if width > m.width-4 {
		width = max(OverlayAbsoluteMinWidth, m.width-4)
	}
	return width
}

// overlayContentWidth returns the usable text width inside the popup produced
// by previewOverlay. Width(overlayWidth()) in lipgloss v1.x sets the total
// outer width; border (2) + horizontal padding (4) = 6 consumed.
func (m *Model) overlayContentWidth() int {
	return m.overlayWidth() - 6
}
