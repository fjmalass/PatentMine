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

// IDSEntryOf returns the curated IDS entry for a project/patent pair.
func (e *Engine) IDSEntryOf(ctx context.Context, project domain.ProjectID, patent domain.PatentNumber) (entry domain.IDSEntry, ok bool, err error) {
	defer e.observeDuration("engine.ids_entry_of", time.Now(), &err)
	if project == "" {
		return domain.IDSEntry{}, false, nil
	}
	record, err := e.recordNumber(ctx, patent)
	if err != nil {
		return domain.IDSEntry{}, false, err
	}
	entry, err = e.repo.IDSEntry(ctx, project, record)
	if errors.Is(err, store.ErrNotFound) {
		return domain.IDSEntry{}, false, nil
	}
	if err != nil {
		return domain.IDSEntry{}, false, err
	}
	return entry, true, nil
}

// SaveIDSEntry inserts or updates one curated IDS entry.
func (e *Engine) SaveIDSEntry(ctx context.Context, entry domain.IDSEntry) (saved domain.IDSEntry, err error) {
	defer e.observeDuration("engine.save_ids_entry", time.Now(), &err)
	if entry.Project == "" {
		return domain.IDSEntry{}, errors.New("engine: ids entry needs a project")
	}
	record, err := e.recordNumber(ctx, entry.Patent)
	if err != nil {
		return domain.IDSEntry{}, err
	}
	entry.Patent = record
	if !entry.Status.Valid() {
		entry.Status = domain.IDSEntryPending
	}
	saved, err = e.repo.SaveIDSEntry(ctx, entry)
	if err != nil {
		return domain.IDSEntry{}, err
	}
	e.announceChange()
	return saved, nil
}

// DeleteIDSEntry removes one curated IDS entry.
func (e *Engine) DeleteIDSEntry(ctx context.Context, project domain.ProjectID, patent domain.PatentNumber) (err error) {
	defer e.observeDuration("engine.delete_ids_entry", time.Now(), &err)
	record, err := e.recordNumber(ctx, patent)
	if err != nil {
		return err
	}
	if err := e.repo.DeleteIDSEntry(ctx, project, record); err != nil {
		return err
	}
	e.announceChange()
	return nil
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
		Action:   "tag.create",
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
		Action:   "tag.delete",
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

// AssignPatentTag tags a patent within a project, verifying that the tag exists in the taxonomy.
func (e *Engine) AssignPatentTag(ctx context.Context, project domain.ProjectID, patent domain.PatentNumber, name string) (err error) {
	defer e.observeDuration("engine.assign_patent_tag", time.Now(), &err)
	name = strings.TrimSpace(name)
	if project == "" {
		return errors.New("engine: tag needs a project")
	}
	if name == "" {
		return errors.New("engine: tag name must not be empty")
	}
	record, err := e.recordNumber(ctx, patent)
	if err != nil {
		return err
	}

	matchedTag, err := e.repo.TagByName(ctx, project, name)
	if errors.Is(err, store.ErrNotFound) {
		return fmt.Errorf("engine: tag %q does not exist in the project taxonomy; create it first", name)
	}
	if err != nil {
		return err
	}

	assignedAt := time.Now().UTC()
	changed, err := e.repo.TagPatent(ctx, matchedTag.ID, record, assignedAt)
	if err != nil {
		e.log(ctx, slog.LevelError, "assign patent tag failed", slog.String("record", record.String()), slog.String("name", name), slog.String("error", err.Error()))
		return err
	}
	if !changed {
		e.incCounter("engine.patent_tag.assign.noop_total", 1)
		e.log(ctx, slog.LevelDebug, "patent tag already assigned", slog.String("project_id", string(project)), slog.String("record", record.String()), slog.String("tag", matchedTag.Name))
		return nil
	}

	e.log(ctx, slog.LevelInfo, "patent tagged", slog.String("project_id", string(project)), slog.String("record", record.String()), slog.String("tag", matchedTag.Name))
	e.recordActivity(ctx, observability.Record{
		Action:   "patent.tag_assign",
		Entity:   "patent_tag",
		EntityID: string(project) + "/" + record.String() + "/" + matchedTag.Name,
		Status:   "committed",
		After:    map[string]any{"tag_name": matchedTag.Name, "tag_id": matchedTag.ID, "assigned_at": assignedAt.UTC().Format(time.RFC3339)},
		Metadata: map[string]any{"requested_number": patent.String()},
	})
	e.announceChange()
	e.incCounter("engine.patent_tag.assign.changed_total", 1)
	return nil
}

// RemovePatentTag removes a tag assignment from a patent.
func (e *Engine) RemovePatentTag(ctx context.Context, project domain.ProjectID, patent domain.PatentNumber, name string) (err error) {
	defer e.observeDuration("engine.remove_patent_tag", time.Now(), &err)
	name = strings.TrimSpace(name)
	if project == "" {
		return errors.New("engine: tag needs a project")
	}
	record, err := e.recordNumber(ctx, patent)
	if err != nil {
		return err
	}
	t, err := e.repo.PatentTag(ctx, project, record, name)
	if errors.Is(err, store.ErrNotFound) {
		return fmt.Errorf("engine: patent %s has no tag %q", record, name)
	}
	if err != nil {
		return err
	}
	changed, err := e.repo.UntagPatent(ctx, t.ID, record)
	if err != nil {
		e.log(ctx, slog.LevelError, "untag patent failed", slog.String("record", record.String()), slog.String("name", name), slog.String("error", err.Error()))
		return err
	}
	if !changed {
		e.incCounter("engine.patent_tag.remove.noop_total", 1)
		e.log(ctx, slog.LevelDebug, "patent tag already absent", slog.String("project_id", string(project)), slog.String("record", record.String()), slog.String("tag", t.Name))
		return nil
	}
	e.log(ctx, slog.LevelInfo, "patent untagged", slog.String("project_id", string(project)), slog.String("record", record.String()), slog.String("tag", t.Name))
	e.recordActivity(ctx, observability.Record{
		Action:   "patent.tag_remove",
		Entity:   "patent_tag",
		EntityID: string(project) + "/" + record.String() + "/" + t.Name,
		Status:   "committed",
		Before:   map[string]any{"tag_name": t.Name, "tag_id": t.ID, "assigned_at": t.AssignedAt.UTC().Format(time.RFC3339)},
	})
	e.announceChange()
	e.incCounter("engine.patent_tag.remove.changed_total", 1)
	return nil
}

// AssignTag tags a patent within a project, creating the named tag when the
// project does not have it yet. The patent may be given by any of its document
// numbers; it is resolved to the record before the tag is assigned.
func (e *Engine) AssignTag(ctx context.Context, project domain.ProjectID, patent domain.PatentNumber, name string) (err error) {
	_, _ = e.CreateTaxonomyTag(ctx, project, name)
	return e.AssignPatentTag(ctx, project, patent, name)
}

// RemoveTag removes a tag from a patent within a project. It reports an error
// when the patent does not carry a tag of that name.
func (e *Engine) RemoveTag(ctx context.Context, project domain.ProjectID, patent domain.PatentNumber, name string) (err error) {
	return e.RemovePatentTag(ctx, project, patent, name)
}
