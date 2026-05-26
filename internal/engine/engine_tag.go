package engine

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"patentmine/internal/domain"
	"patentmine/internal/observability"
	"patentmine/internal/store"
)

// PatentTags returns the tags a patent carries within a project. It is empty
// when no project is named: tags, like membership, are project-scoped.
func (e *Engine) PatentTags(ctx context.Context, project domain.ProjectID, patent domain.PatentNumber) (tags []domain.Tag, err error) {
	defer e.observeDuration("engine.patent_tags", time.Now(), &err)
	if project == "" {
		return nil, nil
	}
	record, err := e.recordNumber(ctx, patent)
	if err != nil {
		return nil, err
	}
	return e.repo.PatentTags(ctx, project, record)
}

// CreateTaxonomyTag registers a new tag in the project's taxonomy.
// The name must match lowercase snake_case constraints.
func (e *Engine) CreateTaxonomyTag(ctx context.Context, project domain.ProjectID, name string) (tag domain.Tag, err error) {
	defer e.observeDuration("engine.create_taxonomy_tag", time.Now(), &err)
	name = strings.TrimSpace(name)
	if project == "" {
		return domain.Tag{}, errors.New("engine: tag needs a project")
	}
	if err := domain.ValidateTagName(name); err != nil {
		return domain.Tag{}, err
	}
	tag, err = e.repo.CreateTag(ctx, project, name)
	if err != nil {
		e.log(ctx, slog.LevelError, "create taxonomy tag failed", slog.String("project_id", string(project)), slog.String("name", name), slog.String("error", err.Error()))
		return domain.Tag{}, err
	}
	e.log(ctx, slog.LevelInfo, "taxonomy tag created", slog.String("project_id", string(project)), slog.String("tag", name))
	e.recordActivity(ctx, observability.Record{
		Action:   observability.ActionTagCreate,
		Entity:   "tag",
		EntityID: string(project) + "/" + name,
		Status:   "committed",
		After:    tag,
	})
	e.announceChange()
	return tag, nil
}

// DeleteTaxonomyTag removes a tag from the project's taxonomy.
func (e *Engine) DeleteTaxonomyTag(ctx context.Context, project domain.ProjectID, name string) (err error) {
	defer e.observeDuration("engine.delete_taxonomy_tag", time.Now(), &err)
	name = strings.TrimSpace(name)
	if project == "" {
		return errors.New("engine: tag needs a project")
	}
	if name == "" {
		return errors.New("engine: tag name must not be empty")
	}

	targetTag, err := e.repo.TagByName(ctx, project, name)
	if errors.Is(err, store.ErrNotFound) {
		return fmt.Errorf("engine: tag %q not found in project's taxonomy", name)
	}
	if err != nil {
		return err
	}

	if err := e.repo.DeleteTag(ctx, project, targetTag.Name); err != nil {
		e.log(ctx, slog.LevelError, "delete taxonomy tag failed", slog.String("project_id", string(project)), slog.String("name", targetTag.Name), slog.String("error", err.Error()))
		return err
	}
	e.log(ctx, slog.LevelInfo, "taxonomy tag deleted", slog.String("project_id", string(project)), slog.String("tag", targetTag.Name))
	e.recordActivity(ctx, observability.Record{
		Action:   observability.ActionTagDelete,
		Entity:   "tag",
		EntityID: string(project) + "/" + targetTag.Name,
		Status:   "committed",
		Before:   targetTag,
	})
	e.announceChange()
	return nil
}

// ListTaxonomyTags lists all tags in the project's taxonomy.
func (e *Engine) ListTaxonomyTags(ctx context.Context, project domain.ProjectID) (tags []domain.Tag, err error) {
	defer e.observeDuration("engine.list_taxonomy_tags", time.Now(), &err)
	if project == "" {
		return nil, errors.New("engine: list taxonomy tags needs a project")
	}
	return e.repo.ProjectTags(ctx, project)
}

// TagPatentStrict tags multiple patents within a project, verifying that the tag exists in the taxonomy.
func (e *Engine) TagPatentStrict(ctx context.Context, project domain.ProjectID, patents []domain.PatentNumber, name string) (err error) {
	defer e.observeDuration("engine.tag_patent_strict", time.Now(), &err)
	name = strings.TrimSpace(name)
	if project == "" {
		return errors.New("engine: tag needs a project")
	}
	if name == "" {
		return errors.New("engine: tag name must not be empty")
	}

	matchedTag, err := e.repo.TagByName(ctx, project, name)
	if errors.Is(err, store.ErrNotFound) {
		return fmt.Errorf("engine: tag %q does not exist in the project taxonomy; create it first", name)
	}
	if err != nil {
		return err
	}

	records := make([]domain.PatentNumber, 0, len(patents))
	for _, patent := range patents {
		record, err := e.recordNumber(ctx, patent)
		if err != nil {
			return err
		}
		records = append(records, record)
	}

	// Segregate records that actually need to be tagged from no-ops
	changedRecords := make([]domain.PatentNumber, 0, len(records))
	noopsCount := 0
	for _, record := range records {
		_, err := e.repo.PatentTag(ctx, project, record, name)
		if err == nil {
			// Tag is already present
			noopsCount++
			e.log(ctx, slog.LevelDebug, "patent tag already present", slog.String("project_id", string(project)), slog.String("record", record.String()), slog.String("tag", matchedTag.Name))
		} else {
			changedRecords = append(changedRecords, record)
		}
	}

	if noopsCount > 0 {
		e.incCounter("engine.patent_tag.assign.noop_total", int64(noopsCount))
	}

	if len(changedRecords) == 0 {
		return nil
	}

	assignedAt := time.Now().UTC()
	if err := e.repo.TagPatents(ctx, matchedTag.ID, changedRecords, assignedAt); err != nil {
		e.log(ctx, slog.LevelError, "tag patents strict failed", slog.String("project_id", string(project)), slog.Int("count", len(changedRecords)), slog.String("name", name), slog.String("error", err.Error()))
		return err
	}

	items := make([]domain.MutationItem, 0, len(changedRecords))
	for idx, record := range changedRecords {
		items = append(items, domain.MutationItem{
			Patent:  record,
			Kind:    "tag.assign",
			Ordinal: idx + 1,
			After:   map[string]any{"tag_name": matchedTag.Name, "tag_id": matchedTag.ID, "assigned_at": assignedAt.UTC().Format(time.RFC3339)},
			Inverse: map[string]any{"action": "remove_tag", "tag_name": matchedTag.Name},
		})
	}
	mutationGroupID := e.saveMutationGroup(ctx, project, "patent.tag_assign", changedRecords, map[string]any{"tag_name": matchedTag.Name, "tag_id": matchedTag.ID}, items)

	// Record activities for each changed record
	for _, record := range changedRecords {
		e.log(ctx, slog.LevelInfo, "patent tagged", slog.String("project_id", string(project)), slog.String("record", record.String()), slog.String("tag", matchedTag.Name))
		attrs := map[string]any{
			"requested_number": record.String(),
			"project":          string(project),
		}
		if mutationGroupID != "" {
			attrs["mutation_group_id"] = mutationGroupID
		}
		e.recordActivity(ctx, observability.Record{
			Action:   observability.ActionPatentTagAssign,
			Entity:   "patent_tag",
			EntityID: string(project) + "/" + record.String() + "/" + matchedTag.Name,
			Status:   "committed",
			After:    map[string]any{"tag_name": matchedTag.Name, "tag_id": matchedTag.ID, "assigned_at": assignedAt.UTC().Format(time.RFC3339)},
			Attributes: attrs,
		})
	}

	e.announceChange()
	e.incCounter("engine.patent_tag.assign.changed_total", int64(len(changedRecords)))
	return nil
}

// UntagPatentStrict removes a tag assignment from multiple patents.
// If len(patents) == 1, it should fail if the patent lacks the tag.
// If len(patents) > 1, it should succeed as long as *at least one* patent had the tag assigned, and fail with a clear error only if *none* did.
func (e *Engine) UntagPatentStrict(ctx context.Context, project domain.ProjectID, patents []domain.PatentNumber, name string) (err error) {
	defer e.observeDuration("engine.untag_patent_strict", time.Now(), &err)
	name = strings.TrimSpace(name)
	if project == "" {
		return errors.New("engine: tag needs a project")
	}

	matchedTag, err := e.repo.TagByName(ctx, project, name)
	if errors.Is(err, store.ErrNotFound) {
		return fmt.Errorf("engine: tag %q does not exist in taxonomy", name)
	}
	if err != nil {
		return err
	}

	records := make([]domain.PatentNumber, 0, len(patents))
	for _, patent := range patents {
		record, err := e.recordNumber(ctx, patent)
		if err != nil {
			return err
		}
		records = append(records, record)
	}

	// Look up the tag for each patent before removing, to record in the Before journal for review/replay
	beforeStates := make(map[domain.PatentNumber]domain.Tag)
	hasAny := false
	for _, record := range records {
		t, err := e.repo.PatentTag(ctx, project, record, name)
		if err == nil {
			beforeStates[record] = t
			hasAny = true
		}
	}

	if len(records) == 1 {
		if !hasAny {
			return fmt.Errorf("engine: patent %s has no tag %q", records[0], name)
		}
	} else if len(records) > 1 {
		if !hasAny {
			return fmt.Errorf("engine: none of the %d selected patents have tag %q", len(records), name)
		}
	}

	// Segregate records that actually need to be untagged from no-ops
	changedRecords := make([]domain.PatentNumber, 0, len(records))
	noopsCount := 0
	for _, record := range records {
		if _, ok := beforeStates[record]; ok {
			changedRecords = append(changedRecords, record)
		} else {
			noopsCount++
			e.log(ctx, slog.LevelDebug, "patent tag already absent", slog.String("project_id", string(project)), slog.String("record", record.String()), slog.String("tag", matchedTag.Name))
		}
	}

	if noopsCount > 0 {
		e.incCounter("engine.patent_tag.remove.noop_total", int64(noopsCount))
	}

	if len(changedRecords) == 0 {
		return nil
	}

	if err := e.repo.UntagPatents(ctx, matchedTag.ID, changedRecords); err != nil {
		e.log(ctx, slog.LevelError, "untag patents strict failed", slog.String("project_id", string(project)), slog.Int("count", len(changedRecords)), slog.String("name", name), slog.String("error", err.Error()))
		return err
	}

	items := make([]domain.MutationItem, 0, len(changedRecords))
	for idx, record := range changedRecords {
		before := map[string]any{"tag_name": matchedTag.Name, "tag_id": matchedTag.ID}
		if t, ok := beforeStates[record]; ok {
			before["assigned_at"] = t.AssignedAt.UTC().Format(time.RFC3339)
		}
		items = append(items, domain.MutationItem{
			Patent:  record,
			Kind:    "tag.remove",
			Ordinal: idx + 1,
			Before:  before,
			Inverse: map[string]any{"action": "add_tag", "tag_name": matchedTag.Name},
		})
	}
	mutationGroupID := e.saveMutationGroup(ctx, project, "patent.tag_remove", changedRecords, map[string]any{"tag_name": matchedTag.Name, "tag_id": matchedTag.ID}, items)

	// Record activities for each changed record
	for _, record := range changedRecords {
		e.log(ctx, slog.LevelInfo, "patent untagged", slog.String("project_id", string(project)), slog.String("record", record.String()), slog.String("tag", matchedTag.Name))
		attrs := map[string]any{
			"requested_number": record.String(),
			"project":          string(project),
		}
		if mutationGroupID != "" {
			attrs["mutation_group_id"] = mutationGroupID
		}
		rec := observability.Record{
			Action:   observability.ActionPatentTagRemove,
			Entity:   "patent_tag",
			EntityID: string(project) + "/" + record.String() + "/" + matchedTag.Name,
			Status:   "committed",
			Attributes: attrs,
		}
		if t, ok := beforeStates[record]; ok {
			rec.Before = map[string]any{
				"tag_name":    t.Name,
				"tag_id":      t.ID,
				"assigned_at": t.AssignedAt.UTC().Format(time.RFC3339),
			}
		}
		e.recordActivity(ctx, rec)
	}

	e.announceChange()
	e.incCounter("engine.patent_tag.remove.changed_total", int64(len(changedRecords)))
	return nil
}

// TagPatent tags multiple patents within a project, creating the named tag when the
// project does not have it yet. When a single patent is targeted, it is passed in a slice of length 1.
func (e *Engine) TagPatent(ctx context.Context, project domain.ProjectID, patents []domain.PatentNumber, name string) (err error) {
	_, _ = e.CreateTaxonomyTag(ctx, project, name)
	return e.TagPatentStrict(ctx, project, patents, name)
}

// UntagPatent removes a tag from multiple patents within a project. When a single patent is targeted,
// it is passed in a slice of length 1.
func (e *Engine) UntagPatent(ctx context.Context, project domain.ProjectID, patents []domain.PatentNumber, name string) (err error) {
	return e.UntagPatentStrict(ctx, project, patents, name)
}
