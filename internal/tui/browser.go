package tui

import (
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"patentmine/internal/domain"
	"patentmine/internal/importer"
)

type browserOpenedMsg struct {
	URL string
}

type browserOpenFailedMsg struct {
	URL string
	Err error
}

func openBrowserCommand(rawURL string) tea.Cmd {
	return func() tea.Msg {
		if err := openBrowserURL(rawURL); err != nil {
			return browserOpenFailedMsg{URL: rawURL, Err: err}
		}
		return browserOpenedMsg{URL: rawURL}
	}
}

func openBrowserURL(rawURL string) error {
	for _, command := range browserCommands(rawURL) {
		if err := exec.Command(command.name, command.args...).Start(); err == nil {
			return nil
		}
	}
	return fmt.Errorf("no browser launcher found for %s", rawURL)
}

type browserCommand struct {
	name string
	args []string
}

func (m *Model) openBrowser(args []string) (tea.Model, tea.Cmd) {
	rawURL, err := m.browserURL(args)
	if err != nil {
		m.err = err.Error()
		return m, nil
	}
	m.message = EmptyMessage
	m.err = EmptyError
	m.logger.Info("open browser", "url", rawURL)
	return m, openBrowserCommand(rawURL)
}

func (m *Model) browserURL(args []string) (string, error) {
	if len(args) > 1 {
		return "", errors.New(m.text.T(TextMessageBrowserUsage))
	}
	if len(args) == 1 {
		return m.patentBrowserURL(args[0])
	}
	switch {
	case m.isCitationView():
		edge, ok, err := m.selectedCitationEdge()
		if err != nil {
			return "", err
		}
		if !ok {
			return "", errors.New(m.text.T(TextMessageBrowserNoPatent))
		}
		return m.patentBrowserURL(string(edge.TargetPatent))
	case m.mode == viewReview:
		edge, ok, err := m.selectedReviewCitationEdge()
		if err != nil {
			return "", err
		}
		if !ok {
			return "", errors.New(m.text.T(TextMessageBrowserNoPatent))
		}
		return m.patentBrowserURL(string(edge.TargetPatent))
	case m.mode == viewPopupPatentDetail:
		return m.patentURL(m.current)
	case m.mode == viewList && len(m.patents) > 0:
		return m.patentURL(m.patents[clamp(m.patentSelected, 0, len(m.patents)-1)])
	default:
		return m.patentURL(m.current)
	}
}

func (m *Model) patentURL(p domain.Patent) (string, error) {
	if strings.TrimSpace(p.SourceGoogleURL) != "" {
		return p.SourceGoogleURL, nil
	}
	return m.patentBrowserURL(string(p.Number))
}

func (m *Model) patentBrowserURL(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New(m.text.T(TextMessageBrowserEmpty))
	}
	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		return value, nil
	}
	return importer.GooglePatentsURL(value)
}

func (m *Model) openPatent(number string) (tea.Model, tea.Cmd) {
	m.backStack = append(m.backStack, m.snapshot())
	p, err := m.repo.GetPatent(m.ctx, m.ProjectID, domain.PatentNumber(number))
	if err != nil {
		m.backStack = m.backStack[:len(m.backStack)-1]
		m.err = err.Error()
		m.logger.Error("open patent failed", "patent", number, "error", err)
		return m, nil
	}
	m.current = p
	m.populateDetailCache()
	m.setMode(viewDetail)
	m.message = "opened " + string(p.Number)
	return m, m.enrichClassificationDescriptionsCommand(number)
}

func browserCommands(rawURL string) []browserCommand {
	switch runtime.GOOS {
	case "darwin":
		return []browserCommand{{name: "open", args: []string{rawURL}}}
	case "windows":
		return []browserCommand{{name: "rundll32", args: []string{"url.dll,FileProtocolHandler", rawURL}}}
	default:
		return []browserCommand{
			{name: "xdg-open", args: []string{rawURL}},
			{name: "wslview", args: []string{rawURL}},
			{name: "rundll32.exe", args: []string{"url.dll,FileProtocolHandler", rawURL}},
			{name: "sensible-browser", args: []string{rawURL}},
		}
	}
}
