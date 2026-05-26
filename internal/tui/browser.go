package tui

import (
	"context"
	"net/url"
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
	apiKey := strings.TrimSpace(a.usptoAPIKey)
	project := domain.ProjectID("")
	if a.activeProject != nil {
		project = a.activeProject.ID
	}
	return func() tea.Msg {
		opened := 0
		for _, number := range numbers {
			target := patentBrowserURL(number)
			if client != nil {
				ctx, cancel := context.WithTimeout(context.Background(), browserOpenTimeout)
				var res proto.PatentResult
				err := client.Call(ctx, proto.MethodPatentGet, proto.PatentGetParams{Number: number, Project: project}, &res)
				cancel()
				if err == nil {
					target = patentBrowserURL(res.Patent.Number)
					if strings.TrimSpace(res.Patent.SourceURL) != "" {
						target = res.Patent.SourceURL
					}
				}
			}
			target = withUSPTOAPIKey(target, apiKey)
			if err := openURL(target); err != nil {
				return pane.StatusMsg{Key: text.StatusBrowserOpenFailed, Args: []any{err.Error()}, Error: true}
			}
			opened++
		}
		return pane.StatusMsg{Key: text.StatusBrowserOpened, Args: []any{opened}}
	}
}

// withUSPTOAPIKey appends ?api_key=... to URLs whose host is api.uspto.gov so
// the user can open the underlying ODP endpoint directly in a browser. Returns
// the URL unchanged when there is no key, the URL is not a USPTO ODP URL, or
// it already carries an api_key parameter.
func withUSPTOAPIKey(raw, apiKey string) string {
	if apiKey == "" {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	if !strings.EqualFold(u.Host, "api.uspto.gov") {
		return raw
	}
	q := u.Query()
	if q.Get("api_key") != "" {
		return raw
	}
	q.Set("api_key", apiKey)
	u.RawQuery = q.Encode()
	return u.String()
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
