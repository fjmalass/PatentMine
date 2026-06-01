// Saved table-view RPC handlers, split out of server.go.
package rpc

import (
	"context"
	"encoding/json"
	"patentmine/internal/proto"
	"time"
)

func (s *Server) tableViewList(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.TableViewListParams](raw)
	if err != nil {
		return nil, err
	}
	views, err := s.engine.ListTableViews(ctx, p.Owner, p.TableType)
	if err != nil {
		return nil, err
	}
	return proto.TableViewListResult{Views: views}, nil
}

func (s *Server) tableViewGet(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.TableViewGetParams](raw)
	if err != nil {
		return nil, err
	}
	view, err := s.engine.TableView(ctx, p.Owner, p.ID)
	if err != nil {
		return nil, err
	}
	return proto.TableViewResult{View: view}, nil
}

func (s *Server) tableViewSave(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.TableViewSaveParams](raw)
	if err != nil {
		return nil, err
	}
	view, err := s.engine.SaveTableView(ctx, p.View)
	if err != nil {
		return nil, err
	}
	return proto.TableViewResult{View: view}, nil
}

func (s *Server) tableViewDelete(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.TableViewGetParams](raw)
	if err != nil {
		return nil, err
	}
	if err := s.engine.DeleteTableView(ctx, p.Owner, p.ID); err != nil {
		return nil, err
	}
	return proto.Empty{}, nil
}

// mergeTimingMetric combines two timing summaries for the same key.
func mergeTimingMetric(a, b proto.TimingMetric) proto.TimingMetric {
	count := a.Count + b.Count
	totalNanos := a.TotalNanos + b.TotalNanos
	errors := a.Errors + b.Errors
	minNanos := a.MinNanos
	if b.MinNanos < minNanos {
		minNanos = b.MinNanos
	}
	maxNanos := a.MaxNanos
	if b.MaxNanos > maxNanos {
		maxNanos = b.MaxNanos
	}
	lastNanos := b.LastNanos
	var avgNanos int64
	if count > 0 {
		avgNanos = totalNanos / count
	}
	return proto.TimingMetric{
		Count:      count,
		Errors:     errors,
		TotalNanos: totalNanos,
		AvgNanos:   avgNanos,
		AvgMillis:  avgNanos / int64(time.Millisecond),
		MinNanos:   minNanos,
		MinMillis:  minNanos / int64(time.Millisecond),
		MaxNanos:   maxNanos,
		MaxMillis:  maxNanos / int64(time.Millisecond),
		LastNanos:  lastNanos,
		LastMillis: lastNanos / int64(time.Millisecond),
	}
}
