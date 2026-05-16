package changes

import (
	"context"
	"sort"

	"patentmine/internal/domain"
	"patentmine/internal/storage"
)

// addNote appends a research note to a patent.
type addNote struct {
	ProjectID string
	Number    string
	Body      string
	NoteID    int64 `json:"-"` // set by run
}

func AddNote(projectID, number, body string) Change {
	return &addNote{ProjectID: projectID, Number: number, Body: body}
}

func (c *addNote) run(ctx context.Context, repo storage.Repository) error {
	n, err := repo.AddNote(ctx, c.ProjectID, c.Number, c.Body)
	if err != nil {
		return err
	}
	c.NoteID = n.ID
	return nil
}

// reverse is nil: the repository has no note-delete path yet.
func (c *addNote) reverse() Change { return nil }
func (c *addNote) affects() Scope  { return ScopeDetail }
func (c *addNote) label() string   { return "add note" }
func (c *addNote) kind() string    { return "addNote" }

// tagState is the desired membership of one tag.
type tagState struct {
	TagID int64
	Apply bool
}

// applyTags brings a set of patents to a desired tag membership.
type applyTags struct {
	ProjectID string
	Patents   []string
	Desired   []tagState
}

// ApplyTags builds a change that, for every patent, applies each tag whose
// desired value is true and removes each tag whose desired value is false.
func ApplyTags(projectID string, patents []string, desired map[int64]bool) Change {
	ds := make([]tagState, 0, len(desired))
	for id, on := range desired {
		ds = append(ds, tagState{TagID: id, Apply: on})
	}
	sort.Slice(ds, func(i, j int) bool { return ds[i].TagID < ds[j].TagID })
	return &applyTags{ProjectID: projectID, Patents: patents, Desired: ds}
}

func (c *applyTags) run(ctx context.Context, repo storage.Repository) error {
	for _, num := range c.Patents {
		for _, t := range c.Desired {
			var err error
			if t.Apply {
				err = repo.ApplyTagToPatent(ctx, num, t.TagID)
			} else {
				err = repo.RemoveTagFromPatent(ctx, num, t.TagID)
			}
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// reverse is nil: prior per-patent tag membership is not captured.
func (c *applyTags) reverse() Change { return nil }
func (c *applyTags) affects() Scope  { return ScopeDetail | ScopeTags }
func (c *applyTags) label() string   { return "tag " + plural(len(c.Patents), "patent") }
func (c *applyTags) kind() string    { return "applyTags" }

// setPatentDate edits one of a patent's lifecycle dates.
type setPatentDate struct {
	Number   string
	DateType string
	Value    string
}

func SetPatentDate(number, dateType, value string) Change {
	return &setPatentDate{Number: number, DateType: dateType, Value: value}
}

func (c *setPatentDate) run(ctx context.Context, repo storage.Repository) error {
	return repo.UpdatePatentDate(ctx, c.Number, c.DateType, c.Value)
}

// reverse is nil: the prior date value is not captured.
func (c *setPatentDate) reverse() Change { return nil }
func (c *setPatentDate) affects() Scope  { return ScopeDetail | ScopeList }
func (c *setPatentDate) label() string   { return "edit date" }
func (c *setPatentDate) kind() string    { return "setPatentDate" }

// editIDS replaces an IDS entry with an edited copy.
type editIDS struct {
	Before domain.IDSEntry
	After  domain.IDSEntry
}

func EditIDS(before, after domain.IDSEntry) Change {
	return &editIDS{Before: before, After: after}
}

func (c *editIDS) run(ctx context.Context, repo storage.Repository) error {
	return repo.UpdateIDSEntry(ctx, c.After)
}

func (c *editIDS) reverse() Change { return &editIDS{Before: c.After, After: c.Before} }
func (c *editIDS) affects() Scope  { return ScopeDetail }
func (c *editIDS) label() string   { return "edit IDS entry" }
func (c *editIDS) kind() string    { return "editIDS" }

// deletePatents soft-deletes patents (review state → ignored) and removes
// their IDS entries, all atomically per stage.
type deletePatents struct {
	ProjectID string
	Numbers   []string
}

func DeletePatents(projectID string, numbers []string) Change {
	return &deletePatents{ProjectID: projectID, Numbers: numbers}
}

func (c *deletePatents) run(ctx context.Context, repo storage.Repository) error {
	if _, err := repo.DeleteIDSEntriesForPatents(ctx, c.ProjectID, c.Numbers); err != nil {
		return err
	}
	updates := make([]storage.PatentStateUpdate, len(c.Numbers))
	for i, n := range c.Numbers {
		updates[i] = storage.PatentStateUpdate{Number: n, ReviewState: domain.ReviewStateIgnored}
	}
	_, err := repo.UpdatePatentReviewStates(ctx, c.ProjectID, updates)
	return err
}

// reverse is nil: deleting IDS entries is not recoverable.
func (c *deletePatents) reverse() Change { return nil }
func (c *deletePatents) affects() Scope  { return ScopeList }
func (c *deletePatents) label() string   { return "delete " + plural(len(c.Numbers), "patent") }
func (c *deletePatents) kind() string    { return "deletePatents" }
