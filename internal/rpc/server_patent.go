// Patent read/delete RPC handlers, split out of server.go.
package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"patentmine/internal/domain"
	"patentmine/internal/proto"
	"patentmine/internal/store"
	"time"
)

func (s *Server) patentGet(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.PatentGetParams](raw)
	if err != nil {
		return nil, err
	}
	patent, err := s.engine.Patent(ctx, p.Number)
	if err != nil {
		return nil, err
	}
	result := proto.PatentResult{Patent: patent}
	// State and tags are project-scoped; populate them only when the caller
	// named a project, leaving them empty for a project-independent lookup.
	if p.Project != "" {
		m, ok, err := s.engine.MembershipOf(ctx, p.Project, p.Number)
		if err != nil {
			return nil, err
		}
		if ok {
			result.ReviewState = m.ReviewState
			result.Membership = &m
		}
		tags, err := s.engine.PatentTags(ctx, p.Project, p.Number)
		if err != nil {
			return nil, err
		}
		result.Tags = tags
		entry, ok, err := s.engine.IDSEntryOf(ctx, p.Project, p.Number)
		if err != nil {
			return nil, err
		}
		if ok {
			result.IDSEntry = &entry
		}
		note, ok, err := s.engine.PatentNoteOf(ctx, p.Project, p.Number)
		if err != nil {
			return nil, err
		}
		if ok {
			result.PatentNote = &note
		}
	}
	usptoApp, err := s.engine.USPTOApplication(ctx, p.Number)
	if err == nil {
		result.USPTOApplication = &usptoApp
	}
	return result, nil
}

func (s *Server) patentInventorStats(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.PatentGetParams](raw)
	if err != nil {
		return nil, err
	}
	stats, err := s.engine.PatentInventorStats(ctx, p.Number, p.Project)
	if err != nil {
		return nil, err
	}
	return proto.PatentInventorStatsResult{Stats: stats}, nil
}

func (s *Server) patentAssigneeStats(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.PatentAssigneeStatsParams](raw)
	if err != nil {
		return nil, err
	}
	stats, err := s.engine.PatentAssigneeStats(ctx, p.Project)
	if err != nil {
		return nil, err
	}
	return proto.PatentAssigneeStatsResult{Stats: stats}, nil
}

func (s *Server) patentClassificationStats(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.PatentClassificationStatsParams](raw)
	if err != nil {
		return nil, err
	}
	stats, err := s.engine.PatentClassificationStats(ctx, p.Project)
	if err != nil {
		return nil, err
	}
	return proto.PatentClassificationStatsResult{Stats: stats}, nil
}

func (s *Server) patentList(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.PatentListParams](raw)
	if err != nil {
		return nil, err
	}
	if !domain.PatentTableAllowsSort(p.Project, p.SortColumn) {
		s.Logger().Warn("unsupported patent list sort",
			slog.String("project", string(p.Project)),
			slog.String("sort_column", string(p.SortColumn)))
		if metrics := s.engineMetrics(); metrics != nil {
			metrics.IncCounter("patent_list.sort_unsupported_total", 1)
		}
		return nil, fmt.Errorf("%w: unsupported patent list sort %q", ErrBadParams, p.SortColumn)
	}
	if p.SortColumn != "" {
		if metrics := s.engineMetrics(); metrics != nil {
			metrics.IncCounter("patent_list.sort_request_total", 1)
		}
	}
	var tagSortStart time.Time
	if p.SortColumn == domain.SortByTags {
		tagSortStart = time.Now()
		if metrics := s.engineMetrics(); metrics != nil {
			metrics.IncCounter("patent_list.sort_tags_total", 1)
		}
		defer func() {
			d := time.Since(tagSortStart)
			if metrics := s.engineMetrics(); metrics != nil {
				metrics.ObserveDuration("rpc.method.patent.list.sort_tags", d, err != nil)
				metrics.SetGauge("rpc.method.patent.list.sort_tags.limit", int64(p.Limit))
				metrics.SetGauge("rpc.method.patent.list.sort_tags.offset", int64(p.Offset))
			}
			if d >= slowTagSortRPC {
				s.Logger().Warn("slow patent list tag sort",
					slog.String("project", string(p.Project)),
					slog.Int("limit", p.Limit),
					slog.Int("offset", p.Offset),
					slog.Int64("duration_ms", d.Milliseconds()),
					slog.Bool("failed", err != nil))
			}
		}()
	}
	patents, total, err := s.engine.ListPatents(ctx, store.PatentQuery{
		Numbers:            p.Numbers,
		Project:            p.Project,
		Filter:             p.Filter,
		ReviewState:        p.ReviewState,
		IDSStatus:          p.IDSStatus,
		Search:             p.Search,
		Classification:     p.Classification,
		ClassificationCode: p.ClassificationCode,
		Inventor:           p.Inventor,
		Assignee:           p.Assignee,
		Limit:              p.Limit,
		Offset:             p.Offset,
		SortColumn:         p.SortColumn,
		SortAscending:      p.SortAscending,
	})
	if err != nil {
		return nil, err
	}
	return proto.PatentListResult{Patents: patents, Total: total}, nil
}

func (s *Server) patentTableColumns(_ context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.PatentTableColumnsParams](raw)
	if err != nil {
		return nil, err
	}
	return proto.PatentTableColumnsResult{Columns: domain.PatentTableColumns(p.Project)}, nil
}

func (s *Server) patentDelete(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.PatentDeleteParams](raw)
	if err != nil {
		return nil, err
	}
	if err := s.engine.DeletePatent(ctx, p.Number); err != nil {
		return nil, err
	}
	return proto.Empty{}, nil
}

func (s *Server) patentDeleteBulk(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.PatentDeleteBulkParams](raw)
	if err != nil {
		return nil, err
	}
	if err := s.engine.DeletePatents(ctx, p.Patents); err != nil {
		return nil, err
	}
	return proto.Empty{}, nil
}

func (s *Server) patentClearCache(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.PatentClearCacheParams](raw)
	if err != nil {
		return nil, err
	}
	cleared, bytesSaved, err := s.engine.ClearPatentCache(ctx, p.Patents)
	if err != nil {
		return nil, err
	}
	return proto.PatentClearCacheResult{ClearedCount: cleared, BytesSaved: bytesSaved}, nil
}
