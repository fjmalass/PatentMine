package tui

import (
	"context"
	"log/slog"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/domain"
	"patentmine/internal/observability"
	"patentmine/internal/proto"
	"patentmine/internal/text"
	"patentmine/internal/tui/pane"
)

// browserOpenTimeout bounds one optional patent lookup before opening a URL.
const browserOpenTimeout = 15 * time.Second

type browseTarget string

const (
	browseTargetDefault    browseTarget = "default"
	browseTargetUSPTOSmart browseTarget = "uspto"
	browseTargetUSPTOGrant browseTarget = "uspto_grant"
	browseTargetUSPTOPGPub browseTarget = "uspto_pgpub"
	browseTargetGoogle     browseTarget = "google"
)

type browseResolution struct {
	URL      string
	Provider string
	Kind     string
}

func (a *App) openPatentsInBrowser(numbers []domain.PatentNumber, target browseTarget) tea.Cmd {
	client := a.client
	openURL := a.openURL
	apiKey := strings.TrimSpace(a.usptoAPIKey)
	metrics := a.metrics
	activity := a.activity
	logger := a.log()
	project := domain.ProjectID("")
	projectName := ""
	if a.activeProject != nil {
		project = a.activeProject.ID
		projectName = a.activeProject.Name
	}
	return func() tea.Msg {
		start := time.Now()
		opened := 0
		for _, number := range numbers {
			requested := number
			resolution := browseResolution{URL: patentBrowserURL(number), Provider: "google", Kind: "fallback"}
			var result *proto.PatentResult
			if client != nil {
				ctx, cancel := context.WithTimeout(context.Background(), browserOpenTimeout)
				var res proto.PatentResult
				err := client.Call(ctx, proto.MethodPatentGet, proto.PatentGetParams{Number: number, Project: project}, &res)
				cancel()
				if err == nil {
					result = &res
				} else if target != browseTargetGoogle && target != browseTargetDefault {
					recordBrowseTelemetry(metrics, activity, logger, requested, target, browseResolution{}, project, projectName, time.Since(start), err.Error(), false)
					return pane.StatusMsg{Key: text.StatusUsage, Args: []any{err.Error()}, Error: true}
				}
			}
			var errText string
			resolution, errText = resolveBrowseURL(number, result, target)
			if errText != "" {
				recordBrowseTelemetry(metrics, activity, logger, requested, target, resolution, project, projectName, time.Since(start), errText, false)
				return pane.StatusMsg{Key: text.StatusUsage, Args: []any{errText}, Error: true}
			}
			openTarget := withUSPTOAPIKey(resolution.URL, apiKey)
			if err := openURL(openTarget); err != nil {
				recordBrowseTelemetry(metrics, activity, logger, requested, target, resolution, project, projectName, time.Since(start), err.Error(), false)
				return pane.StatusMsg{Key: text.StatusBrowserOpenFailed, Args: []any{err.Error()}, Error: true}
			}
			recordBrowseTelemetry(metrics, activity, logger, requested, target, resolution, project, projectName, time.Since(start), "", true)
			opened++
		}
		return pane.StatusMsg{Key: text.StatusBrowserOpened, Args: []any{opened}}
	}
}

func resolveBrowseURL(number domain.PatentNumber, result *proto.PatentResult, target browseTarget) (browseResolution, string) {
	patent := domain.Patent{Number: number, DisplayNumber: number}
	var app *domain.USPTOApplication
	if result != nil {
		patent = result.Patent
		app = result.USPTOApplication
	}
	display := patent.NumberToShow()
	if display.IsZero() {
		display = number
	}

	sourceURL := strings.TrimSpace(patent.SourceURL)
	switch target {
	case browseTargetGoogle:
		return browseResolution{URL: patentBrowserURL(display), Provider: "google", Kind: "forced"}, ""
	case browseTargetUSPTOSmart:
		if app != nil && strings.TrimSpace(app.PatentGrantXMLURL) != "" {
			return browseResolution{URL: app.PatentGrantXMLURL, Provider: "uspto", Kind: "grant"}, ""
		}
		if app != nil && strings.TrimSpace(app.PGPubXMLURL) != "" {
			return browseResolution{URL: app.PGPubXMLURL, Provider: "uspto", Kind: "pgpub"}, ""
		}
		return browseResolution{Provider: "uspto", Kind: "smart"}, "no USPTO grant or publication XML URL for " + display.Normalized()
	case browseTargetUSPTOGrant:
		if app != nil && strings.TrimSpace(app.PatentGrantXMLURL) != "" {
			return browseResolution{URL: app.PatentGrantXMLURL, Provider: "uspto", Kind: "grant"}, ""
		}
		return browseResolution{Provider: "uspto", Kind: "grant"}, "no USPTO grant XML URL for " + display.Normalized()
	case browseTargetUSPTOPGPub:
		if app != nil && strings.TrimSpace(app.PGPubXMLURL) != "" {
			return browseResolution{URL: app.PGPubXMLURL, Provider: "uspto", Kind: "pgpub"}, ""
		}
		return browseResolution{Provider: "uspto", Kind: "pgpub"}, "no USPTO publication XML URL for " + display.Normalized()
	default:
		if sourceURL != "" {
			return browseResolution{URL: sourceURL, Provider: string(patent.Source), Kind: "source"}, ""
		}
		return browseResolution{URL: patentBrowserURL(display), Provider: "google", Kind: "fallback"}, ""
	}
}

func recordBrowseTelemetry(metrics *observability.Metrics, activity *observability.Recorder, logger *slog.Logger, number domain.PatentNumber, target browseTarget, resolution browseResolution, project domain.ProjectID, projectName string, elapsed time.Duration, errText string, ok bool) {
	status := "ok"
	if !ok {
		status = "error"
	}
	if metrics != nil {
		metrics.IncCounter("tui.browse.requested", 1)
		metrics.IncCounter("tui.browse.target."+string(target)+".requested", 1)
		metrics.IncCounter("tui.browse."+status, 1)
		metrics.IncCounter("tui.browse.target."+string(target)+"."+status, 1)
		if resolution.Provider != "" {
			metrics.IncCounter("tui.browse.provider."+resolution.Provider+"."+status, 1)
		}
		if resolution.Kind != "" {
			metrics.IncCounter("tui.browse.kind."+resolution.Kind+"."+status, 1)
		}
		metrics.ObserveDuration("tui.browse.duration", elapsed, !ok)
	}
	attrs := map[string]any{
		"target":      string(target),
		"provider":    resolution.Provider,
		"kind":        resolution.Kind,
		"url":         redactBrowseURL(resolution.URL),
		"duration_ms": elapsed.Milliseconds(),
	}
	if project != "" {
		attrs["project"] = string(project)
		attrs["project_name"] = projectName
	}
	if errText != "" {
		attrs["error"] = errText
	}
	if logger != nil {
		logger.Info("tui browse", slog.String("patent", number.Normalized()), slog.String("target", string(target)), slog.String("provider", resolution.Provider), slog.String("kind", resolution.Kind), slog.String("status", status), slog.Int64("duration_ms", elapsed.Milliseconds()))
	}
	if activity != nil {
		_ = activity.Record(context.Background(), observability.Record{
			Action:     observability.ActionUIBrowse,
			Entity:     "patent",
			EntityID:   number.Normalized(),
			Status:     status,
			Attributes: attrs,
		})
	}
}

func redactBrowseURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	q := u.Query()
	if q.Get("api_key") != "" {
		q.Set("api_key", "REDACTED")
		u.RawQuery = q.Encode()
	}
	return u.String()
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
