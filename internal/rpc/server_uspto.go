package rpc

// USPTO ODP, source-reconciliation, and assignee/assignment RPC handlers.
// Split out of server.go to keep that file focused on the dispatch core and the
// general patent/project handlers.

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"

	"patentmine/internal/domain"
	"patentmine/internal/proto"
	"patentmine/internal/uspto"
)

func (s *Server) usptoFetchXML(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.USPTOFetchXMLParams](raw)
	if err != nil {
		return nil, err
	}
	return s.engine.FetchUSPTOXML(ctx, p.Number, p.Kind)
}

func (s *Server) usptoGrantBody(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.USPTOGrantBodyParams](raw)
	if err != nil {
		return nil, err
	}
	body, sourcePath, kind, present, err := s.engine.USPTOGrantBody(ctx, p.Number, p.Kind)
	if err != nil {
		return nil, err
	}
	return proto.USPTOGrantBodyResult{Present: present, Body: body, SourcePath: sourcePath, Kind: kind}, nil
}

func (s *Server) fullTextSearch(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.FullTextSearchParams](raw)
	if err != nil {
		return nil, err
	}
	return s.engine.FullTextSearch(ctx, p.Numbers, p.Query)
}

func (s *Server) sourceResolveDiffs(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.SourceResolveDiffsParams](raw)
	if err != nil {
		return nil, err
	}
	if err := s.engine.ResolveSourceDiffs(ctx, p.Number, p.Diffs); err != nil {
		return nil, err
	}
	if metrics := s.engineMetrics(); metrics != nil {
		metrics.IncCounter("rpc.source.resolve_diffs.success_total", 1)
	}
	return proto.SourceResolveDiffsResult{
		Resolved: len(p.Diffs),
		Message:  "reconciled",
	}, nil
}

func (s *Server) sourceDiffsList(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.SourceDiffsListParams](raw)
	if err != nil {
		return nil, err
	}
	diffs, err := s.engine.ListSourceDiffs(ctx, p.Number)
	if err != nil {
		return nil, err
	}
	return proto.SourceDiffsListResult{Diffs: diffs}, nil
}

func (s *Server) sourceBibsList(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.SourceBibsListParams](raw)
	if err != nil {
		return nil, err
	}
	bibs, err := s.engine.SourceBibs(ctx, p.Number)
	if err != nil {
		return nil, err
	}
	return proto.SourceBibsListResult{Bibs: bibs}, nil
}

func (s *Server) usptoLookup(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.USPTOLookupParams](raw)
	if err != nil {
		return nil, err
	}
	rawJSON, err := s.engine.USPTOLookup(ctx, p.Number)
	if err != nil {
		return nil, err
	}
	return proto.USPTOLookupResult{RawJSON: rawJSON}, nil
}

func (s *Server) usptoFetchAssignments(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.USPTOFetchAssignmentsParams](raw)
	if err != nil {
		return nil, err
	}
	return s.engine.FetchUSPTOAssignments(ctx, p.Number)
}

func (s *Server) usptoFetchAssignmentsBatch(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.USPTOFetchAssignmentsBatchParams](raw)
	if err != nil {
		return nil, err
	}
	// A project drives a curated-members batch; an explicit list drives a
	// list/file batch. Project takes precedence when both are supplied.
	if p.Project != "" {
		return s.engine.FetchUSPTOAssignmentsForProject(ctx, p.Project, p.IncludeExpired)
	}
	return s.engine.FetchUSPTOAssignmentsBatch(ctx, p.Numbers, p.IncludeExpired)
}

func (s *Server) assigneeHistory(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.AssigneeHistoryParams](raw)
	if err != nil {
		return nil, err
	}
	return s.engine.AssigneeHistory(ctx, p.Number)
}

func (s *Server) projectAssignees(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.ProjectAssigneesParams](raw)
	if err != nil {
		return nil, err
	}
	return s.engine.ProjectAssignees(ctx, p.Project)
}

func (s *Server) usptoExpirationCalculate(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.USPTOExpirationCalculateParams](raw)
	if err != nil {
		return nil, err
	}

	app, err := s.engine.ComputeAndStoreUSPTOExpiration(ctx, p.Number, p.Refresh)
	if err != nil {
		return nil, err
	}

	if p.ProjectID != "" {
		if _, _, _, addErr := s.engine.AddToProjectFromSource(ctx, domain.ProjectID(p.ProjectID), p.Number, domain.SourceUSPTO, ""); addErr != nil {
			s.Logger().Warn("failed to auto-add patent to project during expiration calculation",
				slog.String("project_id", p.ProjectID),
				slog.String("patent", p.Number.String()),
				slog.String("error", addErr.Error()),
			)
		}
	}

	var googleExpStr, grantDateStr string
	var title, inventors string
	patentRec, err := s.engine.Patent(ctx, p.Number)
	if err == nil {
		title = patentRec.Title
		var invs []string
		for _, inv := range patentRec.Inventors {
			invs = append(invs, string(inv))
		}
		inventors = strings.Join(invs, ", ")

		if !patentRec.GrantDate.IsZero() {
			grantDateStr = patentRec.GrantDate.Format(domain.DateLayout)
		}
	}
	// The "Google estimate" comparison value comes from Google's own source_bib
	// row, not the record projection — the projection now holds the authoritative
	// USPTO computed date after ComputeAndStoreUSPTOExpiration runs above.
	if bibs, bibErr := s.engine.SourceBibs(ctx, p.Number); bibErr == nil {
		for _, bib := range bibs {
			if bib.Source == domain.SourceGoogle && !bib.ExpirationDate.IsZero() {
				googleExpStr = bib.ExpirationDate.Format(domain.DateLayout)
				break
			}
		}
	}
	if title == "" {
		title = app.InventionTitle
	}
	if inventors == "" {
		inventors = app.FirstInventorName
	}
	// Fall back to ApplicationStatusDate for the grant date when the patent
	// record does not store one but the USPTO application status indicates
	// a granted patent (e.g. "Patented Case").
	if grantDateStr == "" {
		grantDateStr = uspto.GrantDateFromStatus(app.ApplicationStatusText, app.ApplicationStatusDate)
	}

	res := proto.USPTOExpirationCalculateResult{
		ApplicationNumber:        app.ApplicationNumber,
		PatentNumber:             p.Number.String(),
		Title:                    title,
		Inventors:                inventors,
		FilingDate:               app.FilingDate,
		GrantDate:                grantDateStr,
		EarliestTermFilingDate:   app.EarliestTermFilingDate,
		PatentTermAdjustmentDays: app.PatentTermAdjustmentDays,
		PatentTermExtensionDays:  app.PatentTermExtension,
		TerminalDisclaimerDate:   app.TerminalDisclaimerDate,
		ComputedExpirationDate:   app.ComputedExpirationDate,
		GoogleExpirationDate:     googleExpStr,
		ComputedAt:               app.FetchedAt,
	}

	// Populate earliest-term parent info when the earliest filing date comes from a parent patent.
	if app.EarliestTermAppNum != "" && app.EarliestTermAppNum != app.ApplicationNumber {
		res.EarliestTermAppNum = app.EarliestTermAppNum
		if parentApp, err := s.engine.USPTOApplicationOrFetch(ctx, app.EarliestTermAppNum); err == nil {
			grantNum, grantDate := uspto.EarliestTermSource(parentApp)
			// A cached parent stub may carry the application status but not the
			// granted patent number; fetch fresh by application number to backfill
			// it (and the inventor) before reporting.
			if grantNum == "" {
				if fresh, ferr := s.engine.RefreshUSPTOApplicationByAppNum(ctx, app.EarliestTermAppNum); ferr == nil {
					parentApp = fresh
					grantNum, grantDate = uspto.EarliestTermSource(parentApp)
				}
			}
			res.EarliestTermPatentNumber = grantNum
			res.EarliestTermGrantDate = grantDate
			// Even when the granted patent number is unknown, the parent's grant
			// date can still be derived from its application status.
			if res.EarliestTermGrantDate == "" {
				res.EarliestTermGrantDate = uspto.GrantDateFromStatus(parentApp.ApplicationStatusText, parentApp.ApplicationStatusDate)
			}
			res.EarliestTermTitle = parentApp.InventionTitle
			res.EarliestTermInventors = parentApp.FirstInventorName
		}
	}

	return res, nil
}

func (s *Server) usptoAssignmentList(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.USPTOAssignmentListParams](raw)
	if err != nil {
		return nil, err
	}
	app, err := s.engine.USPTOApplicationFor(ctx, p.Number)
	if err != nil {
		return nil, err
	}
	assignments, err := s.engine.USPTOAssignments(ctx, app.ApplicationNumber)
	if err != nil {
		return nil, err
	}
	return proto.USPTOAssignmentListResult{
		ApplicationNumber: app.ApplicationNumber,
		Assignments:       assignments,
	}, nil
}
