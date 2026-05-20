package tui

import (
	"context"
	"os/exec"
	"runtime"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/domain"
	"patentmine/internal/proto"
	"patentmine/internal/text"
	"patentmine/internal/tui/pane"
)

// browserOpenTimeout bounds one optional patent lookup before opening a URL.
const browserOpenTimeout = 15 * time.Second

func (a *App) openPatentsInBrowser(numbers []domain.PatentNumber) tea.Cmd {
	client := a.client
	openURL := a.openURL
	project := domain.ProjectID("")
	if a.activeProject != nil {
		project = a.activeProject.ID
	}
	return func() tea.Msg {
		opened := 0
		for _, number := range numbers {
			url := patentBrowserURL(number)
			if client != nil {
				ctx, cancel := context.WithTimeout(context.Background(), browserOpenTimeout)
				var res proto.PatentResult
				err := client.Call(ctx, proto.MethodPatentGet, proto.PatentGetParams{Number: number, Project: project}, &res)
				cancel()
				if err == nil {
					url = patentBrowserURL(res.Patent.Number)
					if strings.TrimSpace(res.Patent.SourceURL) != "" {
						url = res.Patent.SourceURL
					}
				}
			}
			if err := openURL(url); err != nil {
				return pane.StatusMsg{Key: text.StatusBrowserOpenFailed, Args: []any{err.Error()}, Error: true}
			}
			opened++
		}
		return pane.StatusMsg{Key: text.StatusBrowserOpened, Args: []any{opened}}
	}
}

func patentBrowserURL(number domain.PatentNumber) string {
	return "https://patents.google.com/patent/" + number.String()
}

func openExternalURL(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}
