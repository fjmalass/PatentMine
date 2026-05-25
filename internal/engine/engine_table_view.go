package engine

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"patentmine/internal/domain"
)

// SaveTableView creates or updates a named saved table view.
func (e *Engine) SaveTableView(ctx context.Context, view domain.SavedTableView) (saved domain.SavedTableView, err error) {
	defer e.observeDuration("engine.save_table_view", time.Now(), &err)
	view.Normalize()
	if err := view.Validate(); err != nil {
		return domain.SavedTableView{}, fmt.Errorf("engine: invalid table view: %w", err)
	}
	saved, err = e.repo.SaveTableView(ctx, view)
	if err != nil {
		e.log(ctx, slog.LevelError, "save table view failed",
			slog.String("owner", view.Owner),
			slog.String("table_type", string(view.TableType)),
			slog.String("name", view.Name),
			slog.String("error", err.Error()))
		return domain.SavedTableView{}, err
	}
	e.announceChange()
	return saved, nil
}

func (e *Engine) TableView(ctx context.Context, owner, id string) (view domain.SavedTableView, err error) {
	defer e.observeDuration("engine.table_view", time.Now(), &err)
	owner = normalizeTableViewOwner(owner)
	if strings.TrimSpace(id) == "" {
		return domain.SavedTableView{}, errors.New("engine: table view id must not be empty")
	}
	return e.repo.TableView(ctx, owner, id)
}

func (e *Engine) ListTableViews(ctx context.Context, owner string, tableType domain.TableType) (views []domain.SavedTableView, err error) {
	defer e.observeDuration("engine.list_table_views", time.Now(), &err)
	owner = normalizeTableViewOwner(owner)
	if tableType != "" && !domain.IsKnownTableType(tableType) {
		return nil, errors.New("engine: unknown table type")
	}
	return e.repo.ListTableViews(ctx, owner, tableType)
}

func (e *Engine) DeleteTableView(ctx context.Context, owner, id string) (err error) {
	defer e.observeDuration("engine.delete_table_view", time.Now(), &err)
	owner = normalizeTableViewOwner(owner)
	if strings.TrimSpace(id) == "" {
		return errors.New("engine: table view id must not be empty")
	}
	if err := e.repo.DeleteTableView(ctx, owner, id); err != nil {
		return err
	}
	e.announceChange()
	return nil
}

func normalizeTableViewOwner(owner string) string {
	owner = strings.TrimSpace(owner)
	if owner == "" {
		return domain.DefaultTableViewOwner
	}
	return owner
}
