// Crawl, source-mode, import, relations and family-graph RPC handlers.
package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"patentmine/internal/domain"
	"patentmine/internal/engine"
	"patentmine/internal/proto"
	"patentmine/internal/store"
	"time"
)

func (s *Server) crawlFamily(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.CrawlFamilyParams](raw)
	if err != nil {
		return nil, err
	}
	id, err := s.engine.StartProjectFamilyCrawl(ctx, p.Project, p.Root, p.Depth, p.Profile, p.Force)
	if err != nil {
		return nil, err
	}
	return proto.CrawlStartResult{JobID: string(id)}, nil
}

func (s *Server) crawlConfig(_ context.Context, _ json.RawMessage) (any, error) {
	return s.engine.CrawlConfig(), nil
}

func (s *Server) sourceModeGet(_ context.Context, _ json.RawMessage) (any, error) {
	return s.engine.SourceMode(), nil
}

func (s *Server) sourceModeSet(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.SourceModeParams](raw)
	if err != nil {
		return nil, err
	}
	return s.engine.SetSourceMode(ctx, p.Mode)
}

func (s *Server) importFile(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.ImportFileParams](raw)
	if err != nil {
		return nil, err
	}
	if err := s.engine.ImportFile(ctx, p.Path); err != nil {
		return nil, err
	}
	return proto.Empty{}, nil
}

// addedExport renders the plain-text list of a project's manually-added
// patents. With OutputPath set it writes the file; otherwise it returns the
// list body in Content (the form the REST route uses).
func (s *Server) crawlCancel(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.CrawlCancelParams](raw)
	if err != nil {
		return nil, err
	}
	if err := s.engine.CancelCrawl(ctx, engine.JobID(p.JobID)); err != nil {
		return nil, err
	}
	return proto.Empty{}, nil
}

func (s *Server) relations(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.RelationsParams](raw)
	if err != nil {
		return nil, err
	}
	if !domain.PatentTableAllowsSort(p.Project, p.SortColumn) {
		s.Logger().Warn("unsupported patent relations sort",
			slog.String("project", string(p.Project)),
			slog.String("sort_column", string(p.SortColumn)),
			slog.String("kind", string(p.Kind)))
		if metrics := s.engineMetrics(); metrics != nil {
			metrics.IncCounter("patent_relations.sort_unsupported_total", 1)
		}
		return nil, fmt.Errorf("%w: unsupported patent relations sort %q", ErrBadParams, p.SortColumn)
	}
	var tagSortStart time.Time
	if p.SortColumn == domain.SortByTags {
		tagSortStart = time.Now()
		if metrics := s.engineMetrics(); metrics != nil {
			metrics.IncCounter("patent_relations.sort_tags_total", 1)
		}
		defer func() {
			d := time.Since(tagSortStart)
			if metrics := s.engineMetrics(); metrics != nil {
				metrics.ObserveDuration("rpc.method.patent.relations.sort_tags", d, err != nil)
				metrics.SetGauge("rpc.method.patent.relations.sort_tags.limit", int64(p.Limit))
				metrics.SetGauge("rpc.method.patent.relations.sort_tags.offset", int64(p.Offset))
			}
			if d >= slowTagSortRPC {
				s.Logger().Warn("slow patent relations tag sort",
					slog.String("project", string(p.Project)),
					slog.String("kind", string(p.Kind)),
					slog.Int("limit", p.Limit),
					slog.Int("offset", p.Offset),
					slog.Int64("duration_ms", d.Milliseconds()),
					slog.Bool("failed", err != nil))
			}
		}()
	}
	q := store.PatentQuery{
		Relation:       p.Number,
		RelationKind:   p.Kind,
		Project:        p.Project,
		Filter:         p.Filter,
		ReviewState:    p.ReviewState,
		IDSStatus:      p.IDSStatus,
		Search:         p.Search,
		Classification: p.Classification,
		Inventor:       p.Inventor,
		Limit:          p.Limit,
		Offset:         p.Offset,
		SortColumn:     p.SortColumn,
		SortAscending:  p.SortAscending,
	}
	patents, total, err := s.engine.Relations(ctx, q)
	if err != nil {
		return nil, err
	}
	return proto.RelationsResult{Patents: patents, Total: total}, nil
}

func (s *Server) familyGraph(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.FamilyGraphParams](raw)
	if err != nil {
		return nil, err
	}
	return s.engine.FamilyGraph(ctx, p)
}
