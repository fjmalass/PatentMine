package observability

import "time"

const defaultHistoryRawLimit = 500

// HistoryQuery selects the raw activity window to transform into the
// user-facing history feed. RawLimit is intentionally pre-grouping input size:
// the returned history row count may be lower after noisy repeats collapse.
type HistoryQuery struct {
	RawLimit  int
	Component string
	Since     time.Time
}

// HistoryFeed is the derived /history response plus accounting that makes the
// raw-vs-grouped behavior explicit for clients and metrics.
type HistoryFeed struct {
	Records    []Record `json:"records"`
	RawLimit   int      `json:"raw_limit"`
	RawScanned int      `json:"raw_scanned"`
	Returned   int      `json:"returned"`
	Suppressed int      `json:"suppressed"`
}

// ReadHistoryFeed returns the grouped, replay-oriented activity feed. It is
// derived from raw activity records but preserves every mutation record while
// collapsing consecutive noisy rows such as repeated focus observations.
func ReadHistoryFeed(logsDir string, query HistoryQuery) (HistoryFeed, error) {
	query = normalizeHistoryQuery(query)
	raw, err := ReadActivityRecords(logsDir, ActivityQuery{Limit: query.RawLimit, Component: query.Component, Since: query.Since})
	feed := HistoryFeed{RawLimit: query.RawLimit, RawScanned: len(raw)}
	if err != nil {
		return feed, err
	}
	feed.Records, feed.Suppressed = groupHistoryRecords(raw)
	feed.Returned = len(feed.Records)
	return feed, nil
}

func normalizeHistoryQuery(query HistoryQuery) HistoryQuery {
	if query.RawLimit <= 0 {
		query.RawLimit = defaultHistoryRawLimit
	}
	return query
}

func groupHistoryRecords(raw []Record) (records []Record, suppressed int) {
	var previousKey string
	for _, rec := range raw {
		if !historyFeedIncludes(rec) {
			suppressed++
			continue
		}
		key := HistoryFeedGroupKey(rec)
		if key == previousKey {
			suppressed++
			continue
		}
		records = append(records, rec)
		previousKey = key
	}
	return records, suppressed
}

func historyFeedIncludes(rec Record) bool {
	if rec.Action == ActionUIFocus && rec.Entity == "patent" {
		scope, _ := rec.Attributes["scope"].(string)
		return historyFocusScope(scope)
	}
	return IsHistoryFeedAction(rec.Action)
}

func historyFocusScope(scope string) bool {
	switch scope {
	case "detail", "citations", "family", "ids", "fulltext":
		return true
	default:
		return false
	}
}

// IsHistoryFeedAction reports whether an action belongs in the derived history
// feed. Low-signal/background records remain available from /activity/raw.
func IsHistoryFeedAction(action string) bool {
	switch action {
	case ActionCrawlStart,
		ActionSourceModeSet,
		ActionFilterApply,
		ActionProjectSwitch,
		ActionMembershipAdd,
		ActionMembershipSetState,
		ActionPatentTagAssign,
		ActionPatentTagRemove,
		ActionIDSEntrySave,
		ActionIDSEntryDelete,
		ActionIDSEntryBulkSetStatus,
		ActionUIFocus,
		ActionNotesExport,
		ActionNotesSave,
		ActionNotesDelete:
		return true
	default:
		return false
	}
}

// HistoryFeedGroupKey is the key used to collapse consecutive rows in the
// derived history feed. Mutations get unique keys; noisy/navigation records get
// coarse keys so repeated adjacent entries collapse.
func HistoryFeedGroupKey(rec Record) string {
	if historyFeedPreservesEveryRecord(rec) {
		return historyFeedUniqueKey(rec)
	}
	return historyFeedCollapseKey(rec)
}

func historyFeedPreservesEveryRecord(rec Record) bool {
	switch rec.Action {
	case ActionCrawlStart,
		ActionSourceModeSet,
		ActionMembershipAdd,
		ActionMembershipSetState,
		ActionPatentTagAssign,
		ActionPatentTagRemove,
		ActionIDSEntrySave,
		ActionIDSEntryDelete,
		ActionIDSEntryBulkSetStatus,
		ActionNotesExport,
		ActionNotesSave,
		ActionNotesDelete:
		return true
	default:
		return false
	}
}

func historyFeedUniqueKey(rec Record) string {
	key := historyFeedBaseKey(rec)
	if rec.ID != "" {
		return key + ":id=" + rec.ID
	}
	return key + ":time=" + rec.Timestamp.Format(time.RFC3339Nano)
}

func historyFeedCollapseKey(rec Record) string {
	return historyFeedBaseKey(rec)
}

func historyFeedBaseKey(rec Record) string {
	return rec.Action + ":" + rec.Entity + ":" + rec.EntityID
}
