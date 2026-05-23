package engine

import (
	"context"
	"log/slog"
	"strconv"
	"time"

	"patentmine/internal/domain"
)

func (e *Engine) saveMutationGroup(ctx context.Context, project domain.ProjectID, action string, patents []domain.PatentNumber, metadata map[string]any, items []domain.MutationItem) string {
	groupID := strconv.FormatInt(time.Now().UTC().UnixNano(), 10)
	if err := e.repo.SaveMutationGroup(ctx, domain.MutationGroup{
		ID:                groupID,
		Project:           project,
		Action:            action,
		CreatedAt:         time.Now().UTC(),
		SelectionSnapshot: append([]domain.PatentNumber(nil), patents...),
		Metadata:          metadata,
	}, items); err != nil {
		e.log(ctx, slog.LevelWarn, "save mutation group failed",
			slog.String("action", action),
			slog.String("project_id", string(project)),
			slog.Int("count", len(patents)),
			slog.String("error", err.Error()))
		return ""
	}
	return groupID
}
