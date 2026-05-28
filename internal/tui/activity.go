package tui

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/command"
	"patentmine/internal/observability"
	"patentmine/internal/tui/pane"
)

const replayHistoryCap = 10000

type focusDwellMsg struct {
	key     string
	started time.Time
}

func (a *App) observeFocus() tea.Cmd {
	if a.activity == nil || len(a.overlays) > 0 || len(a.panes) == 0 {
		return nil
	}
	focus, ok := a.currentActivityFocus()
	if !ok || focus.Entity == "" || focus.EntityID == "" {
		return nil
	}
	key := focus.Entity + ":" + focus.EntityID + ":" + fmt.Sprint(focus.Attributes["scope"]) + ":" + fmt.Sprint(focus.Attributes["line"])
	if key == a.focusKey {
		return nil
	}
	a.focusKey = key
	a.focusStarted = time.Now()
	a.focusRecorded = false
	started := a.focusStarted
	return tea.Tick(a.activityMin, func(time.Time) tea.Msg {
		return focusDwellMsg{key: key, started: started}
	})
}

func (a *App) handleFocusDwell(m focusDwellMsg) tea.Cmd {
	if a.activity == nil || a.focusRecorded || m.key != a.focusKey || !m.started.Equal(a.focusStarted) {
		return nil
	}
	focus, ok := a.currentActivityFocus()
	if !ok || focus.Entity == "" || focus.EntityID == "" {
		return nil
	}
	duration := time.Since(a.focusStarted)
	if duration < a.activityMin {
		return nil
	}
	a.focusRecorded = true
	attrs := cloneAttributes(focus.Attributes)
	attrs["label"] = focus.Label
	attrs["duration_ms"] = duration.Milliseconds()
	return a.recordActivity(observability.Record{
		Action:     observability.ActionUIFocus,
		Entity:     focus.Entity,
		EntityID:   focus.EntityID,
		Status:     "observed",
		Attributes: attrs,
	})
}

func (a *App) currentActivityFocus() (pane.ActivityFocus, bool) {
	if provider, ok := a.focusedPane().(pane.ActivityFocusProvider); ok {
		return provider.ActivityFocus()
	}
	number, ok := a.focusedPane().Selection()
	if !ok || number.Serial == "" {
		return pane.ActivityFocus{}, false
	}
	return pane.ActivityFocus{
		Entity:     "patent",
		EntityID:   number.String(),
		Label:      a.focusedPane().Title(),
		Attributes: map[string]any{"scope": string(a.focusedPane().Scope())},
	}, true
}

func (a *App) recordTypedActivity(id command.ID, args []string) tea.Cmd {
	if a.activity == nil {
		return nil
	}
	action := "tui.command"
	entity := "command"
	entityID := string(id)
	attrs := map[string]any{"scope": string(a.scope()), "args": args}
	if id == command.Filter {
		action = "filter.apply"
		entity = "filter"
		entityID = strings.Join(args, " ")
		attrs["filter"] = entityID
	}
	return a.recordActivity(observability.Record{
		Action:     action,
		Entity:     entity,
		EntityID:   entityID,
		Status:     "requested",
		Attributes: attrs,
	})
}

func (a *App) recordActivity(rec observability.Record) tea.Cmd {
	activity := a.activity
	logger := a.log()
	return func() tea.Msg {
		if a.activeProject != nil {
			if rec.Attributes == nil {
				rec.Attributes = map[string]any{}
			}
			rec.Attributes["project"] = string(a.activeProject.ID)
			rec.Attributes["project_name"] = a.activeProject.Name
		}
		if err := activity.Record(context.Background(), rec); err != nil {
			logger.Warn("activity record failed", slog.String("action", rec.Action), slog.String("error", err.Error()))
		}
		return nil
	}
}

func (a *App) recordReplayHistory(rec observability.Record) tea.Cmd {
	logsDir := a.activityDir
	logger := a.log()
	return func() tea.Msg {
		if logsDir == "" {
			return nil
		}
		attrs := cloneAttributes(rec.Attributes)
		attrs["source"] = "tui.activity_overlay"
		entry := observability.ReplayHistoryEntry{
			ActivityID:        rec.ID,
			ActivityTimestamp: rec.Timestamp,
			Action:            rec.Action,
			Entity:            rec.Entity,
			EntityID:          rec.EntityID,
			Status:            rec.Status,
			Attributes:        attrs,
		}
		if err := observability.AppendReplayHistory(logsDir, entry, replayHistoryCap); err != nil {
			logger.Warn("replay history record failed", slog.String("activity_id", rec.ID), slog.String("error", err.Error()))
		}
		return nil
	}
}

func cloneAttributes(in map[string]any) map[string]any {
	out := make(map[string]any, len(in)+2)
	for k, v := range in {
		out[k] = v
	}
	return out
}
