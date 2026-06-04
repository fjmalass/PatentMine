// Drafting subsystem RPC handlers: provisional / non-provisional applications,
// office-action responses, and office-action import.
package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"patentmine/internal/ai"
	"patentmine/internal/domain"
	"patentmine/internal/engine"
	"patentmine/internal/proto"
)

func (s *Server) draftCreate(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.DraftCreateParams](raw)
	if err != nil {
		return nil, err
	}
	d, err := s.engine.CreateDraft(ctx, p.Project, p.Kind, p.Title)
	if err != nil {
		return nil, err
	}
	return proto.DraftResult{Draft: d}, nil
}

func (s *Server) draftGet(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.DraftIDParams](raw)
	if err != nil {
		return nil, err
	}
	d, err := s.engine.Draft(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	return proto.DraftResult{Draft: d}, nil
}

func (s *Server) draftList(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.DraftListParams](raw)
	if err != nil {
		return nil, err
	}
	drafts, err := s.engine.ListDrafts(ctx, p.Project)
	if err != nil {
		return nil, err
	}
	return proto.DraftListResult{Drafts: drafts}, nil
}

func (s *Server) draftSave(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.DraftSaveParams](raw)
	if err != nil {
		return nil, err
	}
	d, err := s.engine.SaveDraft(ctx, p.Draft)
	if err != nil {
		return nil, err
	}
	return proto.DraftResult{Draft: d}, nil
}

func (s *Server) draftDelete(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.DraftIDParams](raw)
	if err != nil {
		return nil, err
	}
	if err := s.engine.DeleteDraft(ctx, p.ID); err != nil {
		return nil, err
	}
	return proto.Empty{}, nil
}

func (s *Server) draftExportDocx(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.DraftIDParams](raw)
	if err != nil {
		return nil, err
	}
	path, err := s.engine.ExportDraftDocx(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	return proto.DraftExportResult{Path: path}, nil
}

func (s *Server) draftSectionAI(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.DraftSectionAIParams](raw)
	if err != nil {
		return nil, err
	}
	refs := make([]ai.PriorArtRef, 0, len(p.PriorArt))
	for _, r := range p.PriorArt {
		refs = append(refs, ai.PriorArtRef{
			Number: r.Number, Title: r.Title, Assignee: r.Assignee,
			Abstract: r.Abstract, Relevant: r.Relevant,
		})
	}
	res, err := s.engine.DraftSection(ctx, engine.DraftAIInput{
		Draft:          p.Draft,
		SectionOrdinal: p.SectionOrdinal,
		Instruction:    p.Instruction,
		Notes:          p.Notes,
		Pinned:         p.Pinned,
		PriorArt:       refs,
	})
	if err != nil {
		return nil, err
	}
	return proto.DraftSectionAIResult{
		Text:     res.Text,
		Problems: res.Problems,
		Provider: res.Provider,
		Model:    res.Model,
	}, nil
}

func (s *Server) officeActionImport(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.OfficeActionImportParams](raw)
	if err != nil {
		return nil, err
	}
	mailDate, err := parseFlexDate(p.MailDate)
	if err != nil {
		return nil, fmt.Errorf("%w: mail_date: %v", ErrBadParams, err)
	}
	oa, err := s.engine.ImportOfficeAction(ctx, engine.ImportOfficeActionInput{
		Project:           p.Project,
		ApplicationNumber: p.ApplicationNumber,
		MailDate:          mailDate,
		Type:              p.Type,
		Examiner:          p.Examiner,
		ArtUnit:           p.ArtUnit,
		SourcePath:        p.SourcePath,
		ExtractedText:     p.ExtractedText,
		Source:            p.Source,
	})
	if err != nil {
		return nil, err
	}
	return proto.OfficeActionResult{OfficeAction: oa}, nil
}

func (s *Server) officeActionList(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.OfficeActionListParams](raw)
	if err != nil {
		return nil, err
	}
	list, err := s.engine.ListOfficeActions(ctx, p.Project)
	if err != nil {
		return nil, err
	}
	return proto.OfficeActionListResult{OfficeActions: list}, nil
}

func (s *Server) officeActionGet(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.OfficeActionIDParams](raw)
	if err != nil {
		return nil, err
	}
	oa, err := s.engine.OfficeAction(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	return proto.OfficeActionResult{OfficeAction: oa}, nil
}

// parseFlexDate accepts either an RFC3339 timestamp or a YYYY-MM-DD date, and
// returns the zero time for an empty string.
func parseFlexDate(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	return time.Parse(domain.DateLayout, s)
}
