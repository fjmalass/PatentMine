package domain

import "strings"

// Assignee "pull type" — the kind of observation that produced an
// AssigneeHistoryEntry. These are the values stored in
// assignee_history.pull_type.
const (
	// PullTypeAtGrant is the assignee as of the patent grant (record.assignee).
	PullTypeAtGrant = "at_grant"
	// PullTypeAssignment is a party on a recorded USPTO assignment (a transfer).
	PullTypeAssignment = "assignment"
	// PullTypeAssigneeSearch is reserved for rows discovered via assignee-name
	// search rather than a per-patent fetch.
	PullTypeAssigneeSearch = "assignee_search"
)

// AssigneeHistoryEntry is one row of a record's ownership timeline. Rows are
// derived (delete-then-insert) from the raw source tables and keyed off the
// stable surrogate record id. IsLatest marks every party of the most-recent
// ownership event, so a jointly-owned patent has more than one latest row.
type AssigneeHistoryEntry struct {
	RecordID       string       `json:"record_id"`
	RecordNumber   PatentNumber `json:"record_number"`
	PullType       string       `json:"pull_type"`
	Ordinal        int          `json:"ordinal"`
	AssigneeName   string       `json:"assignee_name"`
	AssigneeNorm   string       `json:"assignee_norm"`
	EffectiveDate  string       `json:"effective_date,omitempty"`
	PulledAt       string       `json:"pulled_at,omitempty"`
	ReelFrame      string       `json:"reel_frame,omitempty"`
	ConveyanceText string       `json:"conveyance_text,omitempty"`
	IsLatest       bool         `json:"is_latest"`
}

// AssigneeGroup aggregates one normalized assignee across a set of patents for a
// project rollup. Norm is the dedup key; Display is the most-frequent raw name
// seen for that key.
type AssigneeGroup struct {
	Norm      string `json:"norm"`
	Display   string `json:"display"`
	Patents   int    `json:"patents"`
	FirstDate string `json:"first_date,omitempty"`
	LastDate  string `json:"last_date,omitempty"`
	// Current is true when this assignee is a current owner of at least one live
	// (non-expired) patent in the rollup scope.
	Current bool `json:"current"`
}

// ProjectAssigneeRollup answers "who owns this project's patents": CurrentOwners
// is the deduplicated current owners over live patents; AllAssignees is every
// assignee ever seen (deduplicated). Expired patents are reported separately and
// excluded from CurrentOwners; NotFetched counts members with no assignee history
// yet.
type ProjectAssigneeRollup struct {
	CurrentOwners  []AssigneeGroup `json:"current_owners"`
	AllAssignees   []AssigneeGroup `json:"all_assignees"`
	LivePatents    int             `json:"live_patents"`
	ExpiredPatents int             `json:"expired_patents"`
	NotFetched     int             `json:"not_fetched"`
}

// assigneeOrgSuffixes are trailing organizational-form tokens folded away when
// normalizing an assignee name so different textual forms of the same entity
// collapse to one dedup key (e.g. "Acme Corp" and "Acme Corporation, Inc.").
var assigneeOrgSuffixes = map[string]bool{
	"CORP": true, "CORPORATION": true, "INC": true, "INCORPORATED": true,
	"LLC": true, "LLP": true, "LP": true, "LTD": true, "LIMITED": true,
	"CO": true, "COMPANY": true, "PLC": true, "GMBH": true, "AG": true,
	"SA": true, "NV": true, "BV": true, "KK": true, "AB": true, "OY": true,
	"PTE": true, "PTY": true, "SRL": true, "SPA": true,
}

// NormalizeAssignee is the single dedup key for "unique assignee" rollups. It
// uppercases, drops a leading "THE ", strips punctuation to spaces, collapses
// whitespace, and folds trailing organizational-form suffixes. It is deliberately
// conservative: it merges obvious textual variants but does not attempt entity
// resolution across genuinely different names (that is the deferred alias layer).
func NormalizeAssignee(name string) string {
	s := strings.ToUpper(strings.TrimSpace(name))
	if s == "" {
		return ""
	}
	s = strings.TrimPrefix(s, "THE ")

	// Replace any non-alphanumeric rune with a space.
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte(' ')
		}
	}
	tokens := strings.Fields(b.String())

	// Drop trailing org-form suffixes (there may be several stacked).
	for len(tokens) > 1 && assigneeOrgSuffixes[tokens[len(tokens)-1]] {
		tokens = tokens[:len(tokens)-1]
	}
	return strings.Join(tokens, " ")
}
