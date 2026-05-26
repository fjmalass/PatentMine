package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"patentmine/internal/domain"
)

// SaveMutationGroup appends a mutation-group header and its ordered patent-level items.
func (r *Repo) SaveMutationGroup(ctx context.Context, group domain.MutationGroup, items []domain.MutationItem) (err error) {
	defer r.observeDuration("save_mutation_group", time.Now(), &err)
	if group.ID == "" {
		return fmt.Errorf("store/sqlite: mutation group id must not be empty")
	}
	if group.Action == "" {
		return fmt.Errorf("store/sqlite: mutation group action must not be empty")
	}
	createdAt := group.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	selectionJSON, err := json.Marshal(group.SelectionSnapshot)
	if err != nil {
		return fmt.Errorf("store/sqlite: encode mutation selection snapshot: %w", err)
	}
	attrsJSON, err := json.Marshal(group.Attributes)
	if err != nil {
		return fmt.Errorf("store/sqlite: encode mutation attrs: %w", err)
	}

	tx, err := r.writer.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store/sqlite: save mutation group tx: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if _, err = tx.ExecContext(ctx, `INSERT INTO mutation_group (id, project_id, action, created_at, selection_snapshot_json, attrs_json) VALUES (?,?,?,?,?,?)`,
		group.ID, string(group.Project), group.Action, encodeTime(createdAt), string(selectionJSON), string(attrsJSON)); err != nil {
		return fmt.Errorf("store/sqlite: insert mutation group: %w", err)
	}

	if err := insertMutationItems(ctx, tx, group.ID, items); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store/sqlite: save mutation group commit: %w", err)
	}
	return nil
}

func insertMutationItems(ctx context.Context, tx *sql.Tx, groupID string, items []domain.MutationItem) error {
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO mutation_item (group_id, ordinal, patent_number, kind, before_json, after_json, inverse_json) VALUES (?,?,?,?,?,?,?)`)
	if err != nil {
		return fmt.Errorf("store/sqlite: prepare mutation items: %w", err)
	}
	defer stmt.Close()

	for idx, item := range items {
		if item.Patent.IsZero() {
			continue
		}
		ordinal := item.Ordinal
		if ordinal <= 0 {
			ordinal = idx + 1
		}
		beforeJSON, err := json.Marshal(item.Before)
		if err != nil {
			return fmt.Errorf("store/sqlite: encode mutation before: %w", err)
		}
		afterJSON, err := json.Marshal(item.After)
		if err != nil {
			return fmt.Errorf("store/sqlite: encode mutation after: %w", err)
		}
		inverseJSON, err := json.Marshal(item.Inverse)
		if err != nil {
			return fmt.Errorf("store/sqlite: encode mutation inverse: %w", err)
		}
		if _, err := stmt.ExecContext(ctx, groupID, ordinal, item.Patent.Normalized(), item.Kind, string(beforeJSON), string(afterJSON), string(inverseJSON)); err != nil {
			return fmt.Errorf("store/sqlite: insert mutation item: %w", err)
		}
	}
	return nil
}
