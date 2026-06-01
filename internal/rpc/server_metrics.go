// Metrics, activity-feed and history RPC handlers, split out of server.go.
package rpc

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"patentmine/internal/observability"
	"patentmine/internal/proto"
)

func (s *Server) engineLogger() *slog.Logger {
	if l := s.engine.Logger(); l != nil {
		return l
	}
	return slog.Default()
}

func (s *Server) metricsGet(context.Context, json.RawMessage) (any, error) {
	merged := s.engine.MetricsSnapshot()
	s.clientMetrics.Range(func(_, v any) bool {
		snap, ok := v.(proto.MetricsSnapshot)
		if !ok {
			return true
		}
		for k, tm := range snap.Timings {
			if existing, found := merged.Timings[k]; found {
				merged.Timings[k] = mergeTimingMetric(existing, tm)
			} else {
				merged.Timings[k] = tm
			}
		}
		for k, v := range snap.Counters {
			merged.Counters[k] += v
		}
		for k, v := range snap.Gauges {
			merged.Gauges[k] += v
		}
		return true
	})
	return proto.MetricsResult{Metrics: merged}, nil
}

func (s *Server) metricsPush(_ context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.MetricsPushParams](raw)
	if err != nil {
		return nil, err
	}
	key := p.Component
	if key == "" {
		key = "client"
	}
	s.clientMetrics.Store(key, p.Snapshot)
	return proto.Empty{}, nil
}

func (s *Server) activityRaw(_ context.Context, raw json.RawMessage) (any, error) {
	if s.activityLogsDir == "" {
		return nil, errors.New("rpc: activity logging is not configured")
	}
	p, err := decodeParams[proto.ActivityRawParams](raw)
	if err != nil {
		return nil, err
	}
	limit := p.Limit
	if limit <= 0 {
		limit = 100
	}
	records, err := observability.ReadActivityRecords(s.activityLogsDir, observability.ActivityQuery{
		Limit:     limit,
		Component: p.Component,
		Action:    p.Action,
		Entity:    p.Entity,
		Since:     p.Since,
	})
	if err != nil {
		return nil, err
	}
	return proto.ActivityRawResult{Records: records, Limit: limit, Returned: len(records)}, nil
}

func (s *Server) historyFeed(_ context.Context, raw json.RawMessage) (any, error) {
	if s.activityLogsDir == "" {
		return nil, errors.New("rpc: activity logging is not configured")
	}
	p, err := decodeParams[proto.HistoryFeedParams](raw)
	if err != nil {
		return nil, err
	}
	return observability.ReadHistoryFeed(s.activityLogsDir, observability.HistoryQuery{
		RawLimit:  p.RawLimit,
		Component: p.Component,
		Since:     p.Since,
	})
}
