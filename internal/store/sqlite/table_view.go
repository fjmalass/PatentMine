package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"patentmine/internal/domain"
	"patentmine/internal/store"
)

// SaveTableView inserts or updates a saved table view. If the view is marked as
// default, any prior default for the same owner/table/scope is cleared.
func (r *Repo) SaveTableView(ctx context.Context, view domain.SavedTableView) (saved domain.SavedTableView, err error) {
	defer r.observeDuration("save_table_view", time.Now(), &err)
	view.Normalize()
	if err := view.Validate(); err != nil {
		return domain.SavedTableView{}, err
	}
	if view.ID == "" {
		view.ID = fmt.Sprintf("view-%d", time.Now().UnixNano())
	}
	now := time.Now().UTC()
	view.UpdatedAt = now

	tx, err := r.writer.BeginTx(ctx, nil)
	if err != nil {
		return domain.SavedTableView{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var createdAtText string
	selectErr := tx.QueryRowContext(ctx,
		`SELECT created_at FROM saved_table_view WHERE id = ? AND owner = ?`,
		view.ID, view.Owner).Scan(&createdAtText)
	if selectErr != nil && !errors.Is(selectErr, sql.ErrNoRows) {
		return domain.SavedTableView{}, selectErr
	}
	if errors.Is(selectErr, sql.ErrNoRows) {
		view.CreatedAt = now
	} else {
		createdAt, err := decodeTime(createdAtText)
		if err != nil {
			return domain.SavedTableView{}, err
		}
		view.CreatedAt = createdAt
	}

	if view.IsDefault {
		if _, err := tx.ExecContext(ctx,
			`UPDATE saved_table_view SET is_default = 0, updated_at = ?
			 WHERE owner = ? AND table_type = ? AND scope = ?`,
			encodeTime(now), view.Owner, view.TableType, view.Scope); err != nil {
			return domain.SavedTableView{}, err
		}
	}

	if errors.Is(selectErr, sql.ErrNoRows) {
		_, err = tx.ExecContext(ctx, `INSERT INTO saved_table_view
			(id, owner, table_type, name, scope, is_default, view_json, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			view.ID, view.Owner, view.TableType, view.Name, view.Scope, boolInt(view.IsDefault), string(view.ViewJSON), encodeTime(view.CreatedAt), encodeTime(view.UpdatedAt))
	} else {
		_, err = tx.ExecContext(ctx, `UPDATE saved_table_view
			SET table_type = ?, name = ?, scope = ?, is_default = ?, view_json = ?, updated_at = ?
			WHERE id = ? AND owner = ?`,
			view.TableType, view.Name, view.Scope, boolInt(view.IsDefault), string(view.ViewJSON), encodeTime(view.UpdatedAt), view.ID, view.Owner)
	}
	if err != nil {
		return domain.SavedTableView{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.SavedTableView{}, err
	}
	return view, nil
}

func (r *Repo) TableView(ctx context.Context, owner, id string) (view domain.SavedTableView, err error) {
	defer r.observeDuration("table_view", time.Now(), &err)
	if owner == "" {
		owner = domain.DefaultTableViewOwner
	}
	return scanTableView(r.reader.QueryRowContext(ctx, `SELECT id, owner, table_type, name, scope, is_default, view_json, created_at, updated_at
		FROM saved_table_view WHERE owner = ? AND id = ?`, owner, id))
}

func (r *Repo) ListTableViews(ctx context.Context, owner string, tableType domain.TableType) (views []domain.SavedTableView, err error) {
	defer r.observeDuration("list_table_views", time.Now(), &err)
	if owner == "" {
		owner = domain.DefaultTableViewOwner
	}
	query := `SELECT id, owner, table_type, name, scope, is_default, view_json, created_at, updated_at
		FROM saved_table_view WHERE owner = ?`
	args := []any{owner}
	if tableType != "" {
		query += ` AND table_type = ?`
		args = append(args, tableType)
	}
	query += ` ORDER BY is_default DESC, updated_at DESC, name COLLATE NOCASE ASC`

	rows, err := r.reader.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		view, err := scanTableView(rows)
		if err != nil {
			return nil, err
		}
		views = append(views, view)
	}
	return views, rows.Err()
}

func (r *Repo) DeleteTableView(ctx context.Context, owner, id string) (err error) {
	defer r.observeDuration("delete_table_view", time.Now(), &err)
	if owner == "" {
		owner = domain.DefaultTableViewOwner
	}
	res, err := r.writer.ExecContext(ctx, `DELETE FROM saved_table_view WHERE owner = ? AND id = ?`, owner, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
}

func scanTableView(scanner rowScanner) (domain.SavedTableView, error) {
	var view domain.SavedTableView
	var tableType, scope, viewJSON, createdAt, updatedAt string
	var isDefault int
	if err := scanner.Scan(&view.ID, &view.Owner, &tableType, &view.Name, &scope, &isDefault, &viewJSON, &createdAt, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.SavedTableView{}, store.ErrNotFound
		}
		return domain.SavedTableView{}, err
	}
	created, err := decodeTime(createdAt)
	if err != nil {
		return domain.SavedTableView{}, err
	}
	updated, err := decodeTime(updatedAt)
	if err != nil {
		return domain.SavedTableView{}, err
	}
	view.TableType = domain.TableType(tableType)
	view.Scope = domain.TableViewScope(scope)
	view.IsDefault = isDefault != 0
	view.ViewJSON = []byte(viewJSON)
	view.CreatedAt = created
	view.UpdatedAt = updated
	return view, nil
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
