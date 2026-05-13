package sqlite

import (
	"context"
	"patentmine/internal/domain"
)

func (r *Repository) CreateTag(ctx context.Context, projectID, name, color string) (int64, error) {
	now := nowString()
	res, err := r.db.ExecContext(ctx, `insert into tags (project_id, name, color, created_at) values (?, ?, ?, ?)`,
		projectID, name, color, now)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (r *Repository) ListTagsWithCounts(ctx context.Context, projectID string) ([]domain.TagWithCount, error) {
	query := `
		select t.id, t.project_id, t.name, t.color, t.created_at, count(pt.patent_number) as patent_count
		from tags t
		left join patent_tags pt on t.id = pt.tag_id
		where t.project_id = ?
		group by t.id
		order by t.name asc
	`
	rows, err := r.db.QueryContext(ctx, query, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tags []domain.TagWithCount
	for rows.Next() {
		var t domain.TagWithCount
		var createdAt string
		if err := rows.Scan(&t.ID, &t.ProjectID, &t.Name, &t.Color, &createdAt, &t.PatentCount); err != nil {
			return nil, err
		}
		t.CreatedAt = parseTime(createdAt)
		tags = append(tags, t)
	}
	return tags, rows.Err()
}

func (r *Repository) ListPatentTagsForProject(ctx context.Context, projectID string) (map[string][]domain.Tag, error) {
	query := `
		select pt.patent_number, t.id, t.project_id, t.name, t.color, t.created_at
		from tags t
		join patent_tags pt on t.id = pt.tag_id
		where t.project_id = ?
		order by t.name asc
	`
	rows, err := r.db.QueryContext(ctx, query, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string][]domain.Tag)
	for rows.Next() {
		var patentNum string
		var t domain.Tag
		var createdAt string
		if err := rows.Scan(&patentNum, &t.ID, &t.ProjectID, &t.Name, &t.Color, &createdAt); err != nil {
			return nil, err
		}
		t.CreatedAt = parseTime(createdAt)
		out[patentNum] = append(out[patentNum], t)
	}
	return out, rows.Err()
}

func (r *Repository) DeleteTag(ctx context.Context, tagID int64) error {
	_, err := r.db.ExecContext(ctx, `delete from tags where id = ?`, tagID)
	return err
}

func (r *Repository) RenameTag(ctx context.Context, tagID int64, newName string) error {
	_, err := r.db.ExecContext(ctx, `update tags set name = ? where id = ?`, newName, tagID)
	return err
}

func (r *Repository) UpdateTagColor(ctx context.Context, tagID int64, color string) error {
	_, err := r.db.ExecContext(ctx, `update tags set color = ? where id = ?`, color, tagID)
	return err
}

func (r *Repository) GetTagByName(ctx context.Context, projectID, name string) (domain.Tag, error) {
	var t domain.Tag
	var createdAt string
	err := r.db.QueryRowContext(ctx, `select id, project_id, name, color, created_at from tags where project_id = ? and name = ?`,
		projectID, name).Scan(&t.ID, &t.ProjectID, &t.Name, &t.Color, &createdAt)
	if err != nil {
		return domain.Tag{}, err
	}
	t.CreatedAt = parseTime(createdAt)
	return t, nil
}

func (r *Repository) ApplyTagToPatent(ctx context.Context, patentNumber string, tagID int64) error {
	_, err := r.db.ExecContext(ctx, `insert or ignore into patent_tags (tag_id, patent_number) values (?, ?)`, tagID, patentNumber)
	return err
}

func (r *Repository) RemoveTagFromPatent(ctx context.Context, patentNumber string, tagID int64) error {
	_, err := r.db.ExecContext(ctx, `delete from patent_tags where tag_id = ? and patent_number = ?`, tagID, patentNumber)
	return err
}

func (r *Repository) GetPatentTags(ctx context.Context, patentNumber string) ([]domain.Tag, error) {
	query := `
		select t.id, t.project_id, t.name, t.color, t.created_at
		from tags t
		join patent_tags pt on t.id = pt.tag_id
		where pt.patent_number = ?
		order by t.name asc
	`
	rows, err := r.db.QueryContext(ctx, query, patentNumber)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tags []domain.Tag
	for rows.Next() {
		var t domain.Tag
		var createdAt string
		if err := rows.Scan(&t.ID, &t.ProjectID, &t.Name, &t.Color, &createdAt); err != nil {
			return nil, err
		}
		t.CreatedAt = parseTime(createdAt)
		tags = append(tags, t)
	}
	return tags, rows.Err()
}
