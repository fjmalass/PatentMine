package changes

import (
	"context"

	"patentmine/internal/domain"
	"patentmine/internal/storage"
)

// citationReview sets the review state of one or more citation edges. When
// PatentUpdates is non-nil the target patents' own review state is updated in
// the same change — used when reviewing a citation from the detail popup so
// the list column stays in sync.
//
// Fields are exported so a change can be serialized into a recording; the
// Prior* fields are run-state and excluded from the recording.
type citationReview struct {
	ProjectID     string
	Updates       []storage.CitationStateUpdate
	PatentUpdates []storage.PatentStateUpdate
	Prior         []storage.CitationStateUpdate `json:"-"`
	PriorPatents  []storage.PatentStateUpdate   `json:"-"`
}

// SetCitationReviewState builds a change that moves every edge to next. When
// alsoPatent is set, each edge's target patent is moved to next as well.
func SetCitationReviewState(projectID string, edges []domain.CitationEdge, next string, alsoPatent bool) Change {
	updates := make([]storage.CitationStateUpdate, len(edges))
	for i, e := range edges {
		updates[i] = storage.CitationStateUpdate{Edge: e, ReviewState: next}
	}
	var pu []storage.PatentStateUpdate
	if alsoPatent {
		pu = make([]storage.PatentStateUpdate, len(edges))
		for i, e := range edges {
			pu[i] = storage.PatentStateUpdate{Number: e.TargetPatent, ReviewState: next}
		}
	}
	return &citationReview{ProjectID: projectID, Updates: updates, PatentUpdates: pu}
}

func (c *citationReview) run(ctx context.Context, repo storage.Repository) error {
	prior, err := repo.UpdateCitationReviewStates(ctx, c.ProjectID, c.Updates)
	if err != nil {
		return err
	}
	c.Prior = prior
	if len(c.PatentUpdates) > 0 {
		priorPatents, err := repo.UpdatePatentReviewStates(ctx, c.ProjectID, c.PatentUpdates)
		if err != nil {
			return err
		}
		c.PriorPatents = priorPatents
	}
	return nil
}

func (c *citationReview) reverse() Change {
	if len(c.Prior) == 0 {
		return nil
	}
	return &citationReview{
		ProjectID:     c.ProjectID,
		Updates:       c.Prior,
		PatentUpdates: c.PriorPatents,
	}
}

func (c *citationReview) affects() Scope { return ScopeList | ScopeDetail }
func (c *citationReview) label() string  { return "review " + plural(len(c.Updates), "citation") }
func (c *citationReview) kind() string   { return "citationReview" }

// patentReview sets the review state of one or more patents.
type patentReview struct {
	ProjectID string
	Updates   []storage.PatentStateUpdate
	Prior     []storage.PatentStateUpdate `json:"-"`
}

// SetPatentReviewState builds a change that moves every patent to next.
func SetPatentReviewState(projectID string, numbers []string, next string) Change {
	updates := make([]storage.PatentStateUpdate, len(numbers))
	for i, n := range numbers {
		updates[i] = storage.PatentStateUpdate{Number: n, ReviewState: next}
	}
	return &patentReview{ProjectID: projectID, Updates: updates}
}

func (c *patentReview) run(ctx context.Context, repo storage.Repository) error {
	prior, err := repo.UpdatePatentReviewStates(ctx, c.ProjectID, c.Updates)
	if err != nil {
		return err
	}
	c.Prior = prior
	return nil
}

func (c *patentReview) reverse() Change {
	if len(c.Prior) == 0 {
		return nil
	}
	return &patentReview{ProjectID: c.ProjectID, Updates: c.Prior}
}

func (c *patentReview) affects() Scope { return ScopeList | ScopeDetail | ScopeFamily }
func (c *patentReview) label() string  { return "review " + plural(len(c.Updates), "patent") }
func (c *patentReview) kind() string   { return "patentReview" }
