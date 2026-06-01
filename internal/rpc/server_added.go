// Added-list import/export RPC handlers, split out of server.go.
package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"patentmine/internal/domain"
	"patentmine/internal/patentlist"
	"patentmine/internal/proto"
	"time"
)

func (s *Server) addedExport(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.AddedExportParams](raw)
	if err != nil {
		return nil, err
	}
	numbers, err := s.engine.ManualMemberships(ctx, p.Project)
	if err != nil {
		return nil, fmt.Errorf("list manual memberships: %w", err)
	}
	body := patentlist.Format(numbers, string(p.Project), time.Now().Format(domain.DateLayout))
	if p.OutputPath != "" {
		if err := os.WriteFile(p.OutputPath, []byte(body), 0o644); err != nil {
			return nil, fmt.Errorf("write export file: %w", err)
		}
		s.engineLogger().Info("added list exported",
			slog.String("project", string(p.Project)),
			slog.Int("count", len(numbers)),
			slog.String("path", p.OutputPath))
		return proto.AddedExportResult{Path: p.OutputPath, Count: len(numbers)}, nil
	}
	return proto.AddedExportResult{Count: len(numbers), Content: body}, nil
}

// addedImport parses a plain-text patent list (from Content, or the file at
// Path) and adds every number to the project with manual provenance.
func (s *Server) addedImport(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.AddedImportParams](raw)
	if err != nil {
		return nil, err
	}
	content := p.Content
	if content == "" {
		if p.Path == "" {
			return nil, fmt.Errorf("%w: added import requires content or path", ErrBadParams)
		}
		body, readErr := os.ReadFile(p.Path)
		if readErr != nil {
			return nil, fmt.Errorf("read patent list %s: %w", p.Path, readErr)
		}
		content = string(body)
	}
	numbers, err := patentlist.Parse(content)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBadParams, err)
	}
	added, failures, err := s.engine.ImportPatentList(ctx, p.Project, numbers)
	if err != nil {
		return nil, err
	}
	s.engineLogger().Info("added list imported",
		slog.String("project", string(p.Project)),
		slog.Int("total", len(numbers)),
		slog.Int("added", added),
		slog.Int("failed", len(failures)))
	return proto.AddedImportResult{
		Total:    len(numbers),
		Added:    added,
		Failed:   len(failures),
		Failures: failures,
	}, nil
}
