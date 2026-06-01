// Patent-note RPC handlers, split out of server.go.
package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"patentmine/internal/domain"
	"patentmine/internal/proto"
	"patentmine/internal/store"
	"strings"
	"time"
)

func (s *Server) patentNoteGet(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.PatentNoteParams](raw)
	if err != nil {
		return nil, err
	}
	note, ok, err := s.engine.PatentNoteOf(ctx, p.Project, p.Patent)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, store.ErrNotFound
	}
	return proto.PatentNoteResult{Note: note}, nil
}

func (s *Server) patentNoteSave(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.PatentNoteSaveParams](raw)
	if err != nil {
		return nil, err
	}
	note, err := s.engine.SavePatentNote(ctx, p.Note)
	if err != nil {
		return nil, err
	}
	return proto.PatentNoteResult{Note: note}, nil
}

func (s *Server) patentNoteDelete(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.PatentNoteParams](raw)
	if err != nil {
		return nil, err
	}
	if err := s.engine.DeletePatentNote(ctx, p.Project, p.Patent); err != nil {
		return nil, err
	}
	return proto.Empty{}, nil
}

func (s *Server) patentNoteList(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.PatentNoteListParams](raw)
	if err != nil {
		return nil, err
	}
	sortByDate := p.SortBy != proto.NoteSortByPatent
	notes, err := s.engine.ListPatentNotes(ctx, p.Project, sortByDate)
	if err != nil {
		return nil, err
	}
	return proto.PatentNoteListResult{Notes: notes}, nil
}

func (s *Server) patentNoteExport(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.PatentNoteExportParams](raw)
	if err != nil {
		return nil, err
	}
	t0 := time.Now()
	sortByDate := p.SortBy != proto.NoteSortByPatent
	notes, err := s.engine.ListPatentNotes(ctx, p.Project, sortByDate)
	if err != nil {
		return nil, fmt.Errorf("list notes: %w", err)
	}

	md := buildNotesMarkdown(notes, string(p.Project), sortByDate)
	bytes := len(md)

	if p.OutputPath != "" {
		if err := os.WriteFile(p.OutputPath, []byte(md), 0o644); err != nil {
			return nil, fmt.Errorf("write export file: %w", err)
		}
		s.engineLogger().Info("notes exported",
			slog.Int("count", len(notes)),
			slog.Int("bytes", bytes),
			slog.String("path", p.OutputPath),
			slog.Int64("duration_ms", time.Since(t0).Milliseconds()))
		return proto.PatentNoteExportResult{Path: p.OutputPath, Count: len(notes), Bytes: bytes}, nil
	}
	s.engineLogger().Info("notes export rendered",
		slog.Int("count", len(notes)),
		slog.Int("bytes", bytes),
		slog.Int64("duration_ms", time.Since(t0).Milliseconds()))
	return proto.PatentNoteExportResult{Count: len(notes), Bytes: bytes, Content: md}, nil
}

// buildNotesMarkdown generates the markdown document for a set of patent notes.
func buildNotesMarkdown(notes []domain.PatentNote, projectName string, sortByDate bool) string {
	var b strings.Builder
	b.WriteString("# PatentMine Notes")
	if projectName != "" {
		b.WriteString(" — " + projectName)
	}
	b.WriteString("\n\n")
	sortLabel := "date (most recent first)"
	if !sortByDate {
		sortLabel = "patent number"
	}
	fmt.Fprintf(&b, "Exported: %s  ·  Sorted by: %s\n\n",
		time.Now().Format(domain.DateTimeLayout), sortLabel)
	b.WriteString(strings.Repeat("─", 72) + "\n\n")
	for _, note := range notes {
		fmt.Fprintf(&b, "## %s\n\n_Updated: %s_\n\n",
			note.Patent.String(), note.UpdatedAt.Format(domain.DateTimeLayout))
		b.WriteString(note.Markdown)
		b.WriteString("\n\n" + strings.Repeat("─", 72) + "\n\n")
	}
	return b.String()
}

// engineLogger returns the engine's logger if available, or the default.
