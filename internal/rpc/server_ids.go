// IDS export/preview and entry RPC handlers, split out of server.go.
package rpc

import (
	"context"
	"encoding/json"
	"patentmine/internal/engine"
	"patentmine/internal/proto"
	"patentmine/internal/store"
)

func (s *Server) idsPDFPreview(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.IDSPDFExportParams](raw)
	if err != nil {
		return nil, err
	}
	prev, err := s.engine.PreviewIDSPDF(ctx, p.Project, engine.IDSPDFOptions{
		CumulativeCount: p.CumulativeCount,
	})
	if err != nil {
		return nil, err
	}
	return proto.IDSPDFPreviewResult{
		BaseDir:         prev.BaseDir,
		USCount:         prev.USCount,
		ForeignCount:    prev.ForeignCount,
		Sheets:          prev.Sheets,
		FeeTier:         prev.FeeTier,
		CumulativeCount: prev.CumulativeCount,
		ExistingDirs:    prev.ExistingDirs,
		MissingFields:   prev.MissingFields,
	}, nil
}

func (s *Server) idsEntryGet(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.IDSEntryParams](raw)
	if err != nil {
		return nil, err
	}
	entry, ok, err := s.engine.IDSEntryOf(ctx, p.Project, p.Patent)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, store.ErrNotFound
	}
	return proto.IDSEntryResult{Entry: entry}, nil
}

func (s *Server) idsEntrySave(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.IDSEntrySaveParams](raw)
	if err != nil {
		return nil, err
	}
	entry, err := s.engine.SaveIDSEntry(ctx, p.Entry)
	if err != nil {
		return nil, err
	}
	return proto.IDSEntryResult{Entry: entry}, nil
}

func (s *Server) idsEntryBulkSetStatus(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.IDSEntryBulkSetStatusParams](raw)
	if err != nil {
		return nil, err
	}
	entries, err := s.engine.BulkSetIDSStatus(ctx, p.Project, p.Patents, p.Status, p.DefaultInFull)
	if err != nil {
		return nil, err
	}
	return proto.IDSEntriesResult{Entries: entries}, nil
}

func (s *Server) idsEntryDelete(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.IDSEntryParams](raw)
	if err != nil {
		return nil, err
	}
	if err := s.engine.DeleteIDSEntry(ctx, p.Project, p.Patent); err != nil {
		return nil, err
	}
	return proto.Empty{}, nil
}
