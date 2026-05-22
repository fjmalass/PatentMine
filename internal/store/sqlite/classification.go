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

func (r *Repo) SaveClassificationDefinition(ctx context.Context, c domain.Classification) (err error) {
	defer r.observeDuration("save_classification_definition", time.Now(), &err)
	if c.System == "" || c.Code == "" {
		return errors.New("store/sqlite: system and code are required for classification definition")
	}

	_, err = r.writer.ExecContext(ctx,
		`INSERT INTO classification_definition (system, code, section, class, subclass, main_group, subgroup, description)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(system, code) DO UPDATE SET
			section = excluded.section,
			class = excluded.class,
			subclass = excluded.subclass,
			main_group = excluded.main_group,
			subgroup = excluded.subgroup,
			description = excluded.description`,
		c.System, c.Code, c.Section, c.Class, c.Subclass, c.MainGroup, c.Subgroup, c.Description)
	if err != nil {
		return fmt.Errorf("store/sqlite: save classification definition: %w", err)
	}
	return nil
}

func (r *Repo) ClassificationDefinition(ctx context.Context, system, code string) (c domain.Classification, err error) {
	defer r.observeDuration("classification_definition", time.Now(), &err)
	row := r.reader.QueryRowContext(ctx,
		`SELECT system, code, section, class, subclass, main_group, subgroup, description
		 FROM classification_definition WHERE LOWER(system) = LOWER(?) AND LOWER(code) = LOWER(?)`,
		system, code)

	err = row.Scan(&c.System, &c.Code, &c.Section, &c.Class, &c.Subclass, &c.MainGroup, &c.Subgroup, &c.Description)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Classification{}, store.ErrNotFound
		}
		return domain.Classification{}, fmt.Errorf("store/sqlite: load classification definition: %w", err)
	}
	return c, nil
}

func (r *Repo) ListClassificationDefinitions(ctx context.Context) (out []domain.Classification, err error) {
	defer r.observeDuration("list_classification_definitions", time.Now(), &err)
	rows, err := r.reader.QueryContext(ctx,
		`SELECT system, code, section, class, subclass, main_group, subgroup, description
		 FROM classification_definition ORDER BY system, code`)
	if err != nil {
		return nil, fmt.Errorf("store/sqlite: list classification definitions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var c domain.Classification
		if err := rows.Scan(&c.System, &c.Code, &c.Section, &c.Class, &c.Subclass, &c.MainGroup, &c.Subgroup, &c.Description); err != nil {
			return nil, fmt.Errorf("store/sqlite: scan classification definition: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store/sqlite: list classification definitions: %w", err)
	}
	return out, nil
}

func (r *Repo) DeleteClassificationDefinition(ctx context.Context, system, code string) (err error) {
	defer r.observeDuration("delete_classification_definition", time.Now(), &err)
	_, err = r.writer.ExecContext(ctx,
		`DELETE FROM classification_definition WHERE LOWER(system) = LOWER(?) AND LOWER(code) = LOWER(?)`,
		system, code)
	if err != nil {
		return fmt.Errorf("store/sqlite: delete classification definition: %w", err)
	}
	return nil
}
