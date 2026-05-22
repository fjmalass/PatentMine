package tui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"patentmine/internal/config"
	"patentmine/internal/domain"
	"patentmine/internal/importer"
	"patentmine/internal/storage"
)

func (m *Model) importPatent(number string) (domain.PatentBundle, error) {
	if m.importCfg.ImportSource == config.ImportSourceUSPTO {
		if m.importCfg.USPTO.APIKey == "" {
			return domain.PatentBundle{}, fmt.Errorf("USPTO import source configured but no API key set (see .env, --uspto-api-key, or config.toml)")
		}
		bundle, err := importer.ImportUSPTO(number, m.importCfg.USPTO.APIKey, m.logger)
		if err == nil {
			bundle.Patent.ImportSource = ImportSourceUSPTO
		}
		return bundle, err
	}
	rawURL, err := importer.GooglePatentsURL(number)
	if err != nil {
		return domain.PatentBundle{}, err
	}
	bundle, err := importer.ImportGooglePatents(rawURL, m.logger)
	if err == nil {
		bundle.Patent.ImportSource = ImportSourceGoogle
	}
	return bundle, err
}

func (m *Model) importByNumber(number, verb string) (tea.Model, tea.Cmd) {
	if verb != importActionRefreshed {
		m.backStack = append(m.backStack, m.snapshot())
	}
	ctx, cancel := context.WithCancel(m.ctx)
	m.loading = true
	m.loadingMsg = fmt.Sprintf("%s%s %s via USPTO ODP...", strings.ToUpper(verb[:1]), verb[1:], number)
	m.cancel = cancel

	repo := m.repo
	projectID := m.ProjectID
	logger := m.logger
	apiKey := m.importCfg.USPTO.APIKey
	currentStatus := m.current.ReviewState

	return m, tea.Batch(
		m.spinner.Tick,
		func() tea.Msg {
			bundle, err := importer.ImportUSPTO(number, apiKey, logger)
			if err != nil {
				return refreshResultMsg{err: fmt.Errorf("import failed: %w", err)}
			}
			bundle.Patent.ImportSource = ImportSourceUSPTO
			if verb == importActionRefreshed {
				bundle.Patent.ReviewState = currentStatus
			} else {
				bundle.Patent.ReviewState = domain.ReviewStateStored
			}
			if err := repo.UpsertPatentBundle(ctx, projectID, bundle); err != nil {
				return refreshResultMsg{err: fmt.Errorf("storage failed: %w", err)}
			}
			p, err := repo.GetPatent(ctx, projectID, bundle.Patent.Number)
			if err != nil {
				return refreshResultMsg{err: err}
			}
			return refreshResultMsg{
				patent:  p,
				mode:    viewDetail,
				message: fmt.Sprintf("%s %s via USPTO ODP", verb, bundle.Patent.Number),
				action:  ActivityPatentImport,
				source:  ImportSourceUSPTO,
			}
		},
	)
}

func (m *Model) enrichClassificationDescriptionsCommand(number string) tea.Cmd {
	return func() tea.Msg {
		classifications, err := m.repo.ListClassifications(m.ctx, m.ProjectID, domain.PatentNumber(number))
		if err != nil || len(classifications) == 0 {
			return nil
		}

		missing := false
		for _, cls := range classifications {
			if cls.Description == "" {
				missing = true
				break
			}
		}
		if !missing {
			return nil
		}

		rawURL, err := importer.GooglePatentsURL(number)
		if err != nil {
			return nil
		}
		bundle, err := importer.ImportGooglePatents(rawURL, m.logger)
		if err != nil {
			return nil
		}

		for _, scraped := range bundle.Classifications {
			if scraped.Description != "" {
				_ = m.repo.UpdateClassificationDescription(m.ctx, m.ProjectID, scraped.System, scraped.Code, scraped.Description)
			}
		}
		return classificationEnrichedMsg{Number: number}
	}
}

func (m *Model) importGooglePatent(rawURL, verb string) (tea.Model, tea.Cmd) {
	if verb != importActionRefreshed {
		m.backStack = append(m.backStack, m.snapshot())
	}

	ctx, cancel := context.WithCancel(m.ctx)
	m.loading = true
	m.loadingMsg = fmt.Sprintf("%s%s from %s...", strings.ToUpper(verb[:1]), verb[1:], rawURL)
	m.cancel = cancel

	repo := m.repo
	projectID := m.ProjectID
	logger := m.logger
	currentStatus := m.current.ReviewState

	return m, tea.Batch(
		m.spinner.Tick,
		func() tea.Msg {
			bundle, err := importer.ImportGooglePatents(rawURL, logger)
			if err != nil {
				return refreshResultMsg{err: fmt.Errorf("import failed: %w", err)}
			}
			bundle.Patent.ImportSource = ImportSourceGoogle
			if verb == importActionRefreshed {
				bundle.Patent.ReviewState = currentStatus
			} else {
				bundle.Patent.ReviewState = domain.ReviewStateStored
			}
			if err := repo.UpsertPatentBundle(ctx, projectID, bundle); err != nil {
				return refreshResultMsg{err: fmt.Errorf("storage failed: %w", err)}
			}
			p, err := repo.GetPatent(ctx, projectID, bundle.Patent.Number)
			if err != nil {
				return refreshResultMsg{err: err}
			}

			msg := fmt.Sprintf("%s %s from %s", verb, bundle.Patent.Number, rawURL)
			if n := len(bundle.FamilyEdges); n > 0 {
				msg += fmt.Sprintf(" · %d family edge(s) — :refresh family to import", n)
			}
			logger.Info("google patent imported", "url", rawURL, "patent", bundle.Patent.Number)
			return refreshResultMsg{
				patent:  p,
				mode:    viewDetail,
				message: msg,
				action:  activityPatentImport,
				source:  ImportSourceGoogle,
			}
		},
	)
}

func (m *Model) refreshCommand(args []string) (tea.Model, tea.Cmd) {
	target := refreshTargetAll
	withDetails := false

	refreshUsage := fmt.Sprintf("usage: :%s [%s|%s] [%s]", commandRefresh, refreshTargetCitations, refreshTargetCitedBy, refreshArgDetails)
	switch len(args) {
	case 0:
	case 1:
		target = strings.ToLower(args[0])
	case 2:
		target = strings.ToLower(args[0])
		if strings.ToLower(args[1]) != refreshArgDetails {
			m.err = refreshUsage
			return m, nil
		}
		withDetails = true
	default:
		m.err = refreshUsage
		return m, nil
	}

	if target == "family" || target == commandFamily {
		return m.pullFamilyCommand()
	}
	if target != refreshTargetAll && target != refreshTargetCitations && target != domain.RelationCites && target != refreshTargetCitedBy && target != domain.RelationCitedBy {
		m.err = refreshUsage
		return m, nil
	}
	if m.current.Number == EmptyFilter {
		m.err = "open a patent before refreshing citations"
		return m, nil
	}

	importSource := m.importCfg.ImportSource
	apiKey := m.importCfg.USPTO.APIKey

	if importSource == config.ImportSourceUSPTO && apiKey == "" {
		m.err = "USPTO ODP source configured but no API key set (use .env, --uspto-api-key, or config.toml)"
		return m, nil
	}

	var rawURL string
	if importSource != config.ImportSourceUSPTO {
		rawURL = m.current.SourceGoogleURL
		if rawURL == "" {
			var err error
			rawURL, err = importer.GooglePatentsURL(string(m.current.Number))
			if err != nil {
				m.err = err.Error()
				return m, nil
			}
		}
	}

	sourceLabel := string(importSource)
	ctx, cancel := context.WithCancel(m.ctx)
	m.loading = true
	m.loadingMsg = fmt.Sprintf("Refreshing %s...", m.current.Number)
	m.cancel = cancel

	repo := m.repo
	projectID := m.ProjectID
	currentNumber := m.current.Number
	text := m.text
	currentMode := m.mode
	logger := m.logger
	currentStatus := m.current.ReviewState

	return m, tea.Batch(
		m.spinner.Tick,
		func() tea.Msg {
			beforeCites, _ := repo.ListCitations(ctx, projectID, currentNumber, domain.RelationCites, storage.ListCitationsOptions{})
			beforeCitedBy, _ := repo.ListCitations(ctx, projectID, currentNumber, domain.RelationCitedBy, storage.ListCitationsOptions{})

			var bundle domain.PatentBundle
			var err error
			if importSource == config.ImportSourceUSPTO && apiKey != "" {
				bundle, err = importer.ImportUSPTO(string(currentNumber), apiKey, logger)
			} else {
				bundle, err = importer.ImportGooglePatents(rawURL, logger)
			}
			if err != nil {
				return refreshResultMsg{err: fmt.Errorf("import failed: %w", err)}
			}

			bundle.Patent.ImportSource = sourceLabel
			bundle.Patent.ReviewState = currentStatus
			if err := repo.UpsertPatentBundle(ctx, projectID, bundle); err != nil {
				return refreshResultMsg{err: fmt.Errorf("storage failed: %w", err)}
			}

			p, err := repo.GetPatent(ctx, projectID, currentNumber)
			if err != nil {
				return refreshResultMsg{err: err}
			}

			afterCites, _ := repo.ListCitations(ctx, projectID, currentNumber, domain.RelationCites, storage.ListCitationsOptions{})
			afterCitedBy, _ := repo.ListCitations(ctx, projectID, currentNumber, domain.RelationCitedBy, storage.ListCitationsOptions{})

			familySuffix := ""
			if n := len(bundle.FamilyEdges); n > 0 {
				familySuffix = fmt.Sprintf(" · %d family edge(s) — :refresh family to import", n)
			}

			msg := refreshResultMsg{
				patent:      p,
				mode:        currentMode,
				withDetails: withDetails,
				action:      ActivityPatentRefresh,
				source:      sourceLabel,
			}

			switch target {
			case refreshTargetCitedBy, domain.RelationCitedBy:
				msg.mode = viewCitedBy
				msg.citationLocalIdx = 0
				msg.message = fmt.Sprintf(text.T(TextMessageRefreshCitedBy), len(afterCitedBy), len(beforeCitedBy)) + familySuffix
			case refreshTargetCitations, domain.RelationCites:
				msg.mode = viewCites
				msg.citationLocalIdx = 0
				msg.message = fmt.Sprintf(text.T(TextMessageRefreshCitations), len(afterCites), len(beforeCites)) + familySuffix
			default:
				msg.citationLocalIdx = 0
				msg.message = fmt.Sprintf(text.T(TextMessageRefreshAll), len(afterCites), len(beforeCites), len(afterCitedBy), len(beforeCitedBy)) + familySuffix
			}

			logger.Info("citations refreshed", "url", rawURL, "patent", currentNumber, domain.RelationCites, len(afterCites), domain.RelationCitedBy, len(afterCitedBy))
			return msg
		},
	)
}

func (m *Model) refreshVisibleCitationDetails() (tea.Model, tea.Cmd) {
	edges, err := m.visibleCitationEdges()
	if err != nil {
		m.err = err.Error()
		return m, nil
	}
	if len(edges) == 0 {
		m.err = "open citations, cited-by, or a review queue before refreshing details"
		return m, nil
	}

	ctx, cancel := context.WithCancel(m.ctx)
	m.loading = true
	m.loadingMsg = fmt.Sprintf("Refreshing details for %d citations...", len(edges))
	m.cancel = cancel

	repo := m.repo
	projectID := m.ProjectID
	logger := m.logger
	importSource := m.importCfg.ImportSource
	apiKey := m.importCfg.USPTO.APIKey

	if importSource == config.ImportSourceUSPTO && apiKey == "" {
		m.err = "USPTO ODP source configured but no API key set (use .env, --uspto-api-key, or config.toml)"
		return m, nil
	}

	return m, tea.Batch(
		m.spinner.Tick,
		func() tea.Msg {
			updatedCount := 0
			skippedCount := 0
			for _, edge := range edges {
				select {
				case <-ctx.Done():
					return refreshDetailsResultMsg{err: ctx.Err()}
				default:
				}

				if edge.TargetTitle != "" && len(edge.TargetInventors) > 0 && edge.TargetExpirationDate != "" {
					skippedCount++
					continue
				}

				_, err := repo.GetPatent(ctx, projectID, edge.TargetPatent)
				exists := err == nil

				var bundle domain.PatentBundle
				if importSource == config.ImportSourceUSPTO {
					bundle, err = importer.ImportUSPTO(string(edge.TargetPatent), apiKey, logger)
				} else {
					var rawURL string
					rawURL, err = importer.GooglePatentsURL(string(edge.TargetPatent))
					if err == nil {
						bundle, err = importer.ImportGooglePatents(rawURL, logger)
					}
				}
				if err != nil {
					logger.Error("citation details import failed", "patent", edge.TargetPatent, "error", err)
					skippedCount++
					continue
				}

				if importSource == config.ImportSourceUSPTO && apiKey != "" {
					bundle.Patent.ImportSource = ImportSourceUSPTO
				} else {
					bundle.Patent.ImportSource = ImportSourceGoogle
				}
				bundle.Patent.ReviewState = domain.ReviewStateCached
				if err := repo.UpsertPatentBundle(ctx, projectID, bundle); err != nil {
					logger.Error("citation details storage failed", "patent", edge.TargetPatent, "error", err)
					return refreshDetailsResultMsg{err: err}
				}

				status := nextCitationDetailReviewState(edge.ReviewState, exists)
				if err := repo.UpdateCitationReviewState(ctx, projectID, edge, status); err != nil {
					return refreshDetailsResultMsg{err: err}
				}
				updatedCount++
			}
			return refreshDetailsResultMsg{
				message: fmt.Sprintf("citation details refreshed: %d updated, %d skipped", updatedCount, skippedCount),
			}
		},
	)
}

func (m *Model) refreshSelectedCitationDetail() (tea.Model, tea.Cmd) {
	edges, err := m.currentCitationEdges()
	if err != nil {
		m.err = err.Error()
		return m, nil
	}
	if len(edges) == 0 {
		m.err = "no citations visible"
		return m, nil
	}
	sel := clamp(m.citationLocalIdx, 0, len(edges)-1)
	edge := edges[sel]

	ctx, cancel := context.WithCancel(m.ctx)
	m.loading = true
	m.loadingMsg = fmt.Sprintf("Refreshing %s...", edge.TargetPatent)
	m.cancel = cancel

	repo := m.repo
	projectID := m.ProjectID
	logger := m.logger
	importSource := m.importCfg.ImportSource
	apiKey := m.importCfg.USPTO.APIKey
	targetPatent := edge.TargetPatent

	if importSource == config.ImportSourceUSPTO && apiKey == "" {
		m.err = "USPTO ODP source configured but no API key set (use .env, --uspto-api-key, or config.toml)"
		return m, nil
	}

	return m, tea.Batch(
		m.spinner.Tick,
		func() tea.Msg {
			var bundle domain.PatentBundle
			var err error
			if importSource == config.ImportSourceUSPTO {
				bundle, err = importer.ImportUSPTO(string(targetPatent), apiKey, logger)
			} else {
				var rawURL string
				rawURL, err = importer.GooglePatentsURL(string(targetPatent))
				if err == nil {
					bundle, err = importer.ImportGooglePatents(rawURL, logger)
				}
			}
			if err != nil {
				return refreshDetailsResultMsg{err: err}
			}
			bundle.Patent.ImportSource = string(importSource)
			bundle.Patent.ReviewState = domain.ReviewStateCached
			_, existsErr := repo.GetPatent(ctx, projectID, edge.TargetPatent)
			exists := existsErr == nil
			if err := repo.UpsertPatentBundle(ctx, projectID, bundle); err != nil {
				logger.Error("citation refresh failed", "patent", edge.TargetPatent, "error", err)
				return refreshDetailsResultMsg{err: err}
			}
			status := nextCitationDetailReviewState(edge.ReviewState, exists)
			if err := repo.UpdateCitationReviewState(ctx, projectID, edge, status); err != nil {
				return refreshDetailsResultMsg{err: err}
			}
			return refreshDetailsResultMsg{message: fmt.Sprintf("refreshed %s", edge.TargetPatent)}
		},
	)
}

func (m *Model) importCitationDetailsCommand(edge domain.CitationEdge) tea.Cmd {
	repo := m.repo
	projectID := m.ProjectID
	target := edge.TargetPatent
	importSource := m.importCfg.ImportSource

	return func() tea.Msg {
		bundle, err := m.importPatent(string(target))
		if err != nil {
			return refreshDetailsResultMsg{err: fmt.Errorf("bulk import failed for %s: %w", target, err)}
		}

		bundle.Patent.ReviewState = domain.ReviewStateStored
		bundle.Patent.ImportSource = string(importSource)
		if err := repo.UpsertPatentBundle(context.Background(), projectID, bundle); err != nil {
			return refreshDetailsResultMsg{err: fmt.Errorf("bulk save failed for %s: %w", target, err)}
		}

		if err := repo.UpdateCitationReviewState(context.Background(), projectID, edge, domain.ReviewStateStored); err != nil {
			return refreshDetailsResultMsg{err: fmt.Errorf("bulk status update failed for %s: %w", target, err)}
		}

		return refreshDetailsResultMsg{message: fmt.Sprintf("imported %s", target)}
	}
}

func nextCitationDetailReviewState(current string, exists bool) string {
	if exists {
		return current
	}
	return domain.ReviewStateCached
}
