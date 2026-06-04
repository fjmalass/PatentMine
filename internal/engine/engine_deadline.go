package engine

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"patentmine/internal/domain"
	"patentmine/internal/observability"
	"patentmine/internal/remind"
	"patentmine/internal/store"
)

// ListDeadlines returns every pending deadline across the database — the stored
// ones (maintenance fees, custom) plus the office-action response deadlines
// synthesized from open office actions — soonest due first. This single merged
// view backs the :deadlines surface and the reminder engine, so OA responses and
// renewal fees are tracked the same way.
func (e *Engine) ListDeadlines(ctx context.Context) ([]domain.Deadline, error) {
	stored, err := e.repo.ListPendingDeadlines(ctx)
	if err != nil {
		return nil, err
	}
	oas, err := e.repo.ListOpenOfficeActions(ctx)
	if err != nil {
		return nil, err
	}
	out := append([]domain.Deadline(nil), stored...)
	for _, oa := range oas {
		out = append(out, officeActionDeadline(oa))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].DueDate.Before(out[j].DueDate) })
	return out, nil
}

// officeActionDeadline synthesizes a deadline from an open office action. Its ID
// is "oa:<id>" — a stable key for reminder dedupe and display, not a deadline
// table row (the office_action row remains the source of truth).
func officeActionDeadline(oa domain.OfficeAction) domain.Deadline {
	title := "Response to office action"
	if !oa.MailDate.IsZero() {
		title += " mailed " + oa.MailDate.Format(domain.DateLayout)
	}
	return domain.Deadline{
		ID:             "oa:" + oa.ID,
		Kind:           domain.DeadlineOAResponse,
		Project:        oa.Project,
		OfficeActionID: oa.ID,
		Title:          title,
		DueDate:        oa.ResponseDue,
		Status:         domain.DeadlinePending,
	}
}

// TrackRenewals derives the U.S. maintenance-fee schedule for a granted patent
// from its grant date and records the deadlines, replacing any it had before so
// the schedule never double-tracks. Returns the deadlines created.
func (e *Engine) TrackRenewals(ctx context.Context, patentNumber string, entitySize string) (out []domain.Deadline, err error) {
	defer e.observeDuration("engine.track_renewals", time.Now(), &err)
	if entitySize != "" {
		switch entitySize {
		case "large", "small", "micro":
			// valid
		default:
			return nil, fmt.Errorf("engine: invalid entity size %q; must be large, small, or micro", entitySize)
		}
	}
	num, err := domain.ParsePatentNumber(patentNumber)
	if err != nil {
		return nil, fmt.Errorf("engine: track renewals: %w", err)
	}
	patent, err := e.repo.Patent(ctx, num)
	if err != nil {
		return nil, fmt.Errorf("engine: track renewals: %w", err)
	}
	if patent.GrantDate.IsZero() {
		return nil, errors.New("engine: patent has no grant date; maintenance fees apply only to granted patents")
	}
	canonical := num.String()
	deadlines := domain.USMaintenanceDeadlines(canonical, patent.GrantDate)
	if len(deadlines) == 0 {
		return nil, errors.New("engine: no maintenance deadlines derived")
	}
	if err := e.repo.DeleteDeadlinesForPatent(ctx, canonical, domain.DeadlineMaintenanceFee); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	for i := range deadlines {
		deadlines[i].ID = fmt.Sprintf("dl-%d-%d", now.UnixNano(), i)
		if err := e.repo.SaveDeadline(ctx, deadlines[i]); err != nil {
			return nil, err
		}
	}

	var createdAt time.Time
	existing, err := e.repo.PatentRenewal(ctx, patent.Number)
	if err == nil {
		if entitySize == "" {
			entitySize = existing.EntitySize
		}
		createdAt = existing.CreatedAt
	} else if errors.Is(err, store.ErrNotFound) {
		if entitySize == "" {
			entitySize = "large"
		}
		createdAt = now
	} else {
		return nil, err
	}

	ren := domain.PatentRenewal{
		PatentNumber: patent.Number,
		EntitySize:   entitySize,
		IsTracked:    true,
		CreatedAt:    createdAt,
		UpdatedAt:    now,
	}
	if err := e.repo.SavePatentRenewal(ctx, ren); err != nil {
		return nil, err
	}

	e.recordActivity(ctx, observability.Record{
		Action: observability.ActionRenewalsTrack, Entity: "patent", EntityID: canonical, Status: "committed",
		Attributes: map[string]any{"deadlines": len(deadlines)},
	})
	e.announceChange()
	return deadlines, nil
}

// UntrackRenewals stops tracking a patent's renewals by deleting its maintenance deadlines and setting is_tracked to false.
func (e *Engine) UntrackRenewals(ctx context.Context, patentNumber string) (err error) {
	defer e.observeDuration("engine.untrack_renewals", time.Now(), &err)
	num, err := domain.ParsePatentNumber(patentNumber)
	if err != nil {
		return fmt.Errorf("engine: untrack renewals: %w", err)
	}
	canonical := num.String()
	if err := e.repo.DeleteDeadlinesForPatent(ctx, canonical, domain.DeadlineMaintenanceFee); err != nil {
		return err
	}

	now := time.Now().UTC()
	ren, err := e.repo.PatentRenewal(ctx, num)
	if errors.Is(err, store.ErrNotFound) {
		ren = domain.PatentRenewal{
			PatentNumber: num,
			EntitySize:   "large",
			CreatedAt:    now,
		}
	} else if err != nil {
		return err
	}

	ren.IsTracked = false
	ren.UpdatedAt = now
	if err := e.repo.SavePatentRenewal(ctx, ren); err != nil {
		return err
	}

	e.recordActivity(ctx, observability.Record{
		Action: observability.ActionRenewalsUntrack, Entity: "patent", EntityID: canonical, Status: "committed",
	})
	e.announceChange()
	return nil
}

// DeadlinesForPatent returns a patent's tracked deadlines.
func (e *Engine) DeadlinesForPatent(ctx context.Context, patentNumber string) ([]domain.Deadline, error) {
	return e.repo.ListDeadlinesForPatent(ctx, patentNumber)
}

// SetDeadlineStatus marks a deadline done/dismissed/pending (a synthesized
// "oa:<id>" deadline cannot be set this way — close the office action instead).
func (e *Engine) SetDeadlineStatus(ctx context.Context, id string, status domain.DeadlineStatus) (err error) {
	defer e.observeDuration("engine.set_deadline_status", time.Now(), &err)
	if id == "" {
		return errors.New("engine: deadline id required")
	}
	if err := e.repo.SetDeadlineStatus(ctx, id, status); err != nil {
		return err
	}
	e.recordActivity(ctx, observability.Record{
		Action: observability.ActionDeadlineStatus, Entity: "deadline", EntityID: id, Status: "committed",
		Attributes: map[string]any{"status": string(status)},
	})
	e.announceChange()
	return nil
}

// StartReminderLoop runs SendDueReminders on a ticker (and once shortly after
// startup) so reminders fire as deadlines cross the thresholds without anyone
// opening the app. It returns immediately; the loop stops when ctx is cancelled.
// A no-op notifier makes each tick cheap, but callers typically start the loop
// only when a real channel (email) is configured.
func (e *Engine) StartReminderLoop(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 24 * time.Hour
	}
	go func() {
		// Delay the first run so daemon startup is not blocked on SMTP.
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Minute):
		}
		for {
			if _, err := e.SendDueReminders(ctx); err != nil {
				e.log(ctx, slog.LevelWarn, "deadline reminder run failed", slog.String("error", err.Error()))
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(interval):
			}
		}
	}()
}

// reminderMark records that a reminder for one deadline should be logged at a
// threshold once delivered.
type reminderMark struct {
	subject   string
	threshold int
}

// dueReminders computes which reminders should fire now: for each pending
// deadline, the smallest not-yet-sent threshold T (ascending) with daysUntil ≤ T.
// This sends exactly one reminder per deadline per stage as the date approaches.
func (e *Engine) dueReminders(ctx context.Context, channel string) ([]remind.Reminder, []reminderMark, error) {
	deadlines, err := e.ListDeadlines(ctx)
	if err != nil {
		return nil, nil, err
	}
	thresholds := append([]int(nil), e.reminderDays...)
	sort.Ints(thresholds)

	var batch []remind.Reminder
	var marks []reminderMark
	for _, d := range deadlines {
		days := d.DaysUntilDue()
		// The current stage is the smallest threshold the deadline has reached.
		// Once reminded at that stage we go quiet until it reaches a smaller one;
		// we never backfill a larger (earlier) threshold.
		for _, t := range thresholds {
			if days > t {
				continue
			}
			sent, err := e.repo.ReminderSent(ctx, d.ID, t, channel)
			if err != nil {
				return nil, nil, err
			}
			if !sent {
				var entitySize string
				if d.Kind == domain.DeadlineMaintenanceFee || d.Kind == domain.DeadlineAnnuity {
					if num, err := domain.ParsePatentNumber(d.PatentNumber); err == nil {
						if ren, err := e.repo.PatentRenewal(ctx, num); err == nil {
							entitySize = ren.EntitySize
						}
					}
				}
				batch = append(batch, remind.Reminder{
					Subject:      d.ID,
					Kind:         d.Kind.Label(),
					Title:        d.Title,
					DueDate:      d.DueDate,
					DaysUntil:    days,
					PatentNumber: d.PatentNumber,
					Project:      string(d.Project),
					EntitySize:   entitySize,
				})
				marks = append(marks, reminderMark{subject: d.ID, threshold: t})
			}
			break // only the current stage matters
		}
	}
	return batch, marks, nil
}

// DueReminders returns the reminders that would fire now, for a dry-run preview
// (no delivery, no dedupe write).
func (e *Engine) DueReminders(ctx context.Context) ([]remind.Reminder, error) {
	batch, _, err := e.dueReminders(ctx, e.notifier.Channel())
	return batch, err
}

// SendDueReminders delivers the due reminders through the configured notifier and
// records each as sent so it is not repeated. It is a no-op (returns 0) when no
// real channel is configured — the in-app deadline surface stands alone. Safe to
// call on a schedule (a daily tick) or on demand.
func (e *Engine) SendDueReminders(ctx context.Context) (sent int, err error) {
	defer e.observeDuration("engine.send_due_reminders", time.Now(), &err)
	if e.notifier == nil || !e.notifier.Enabled() {
		return 0, nil
	}
	channel := e.notifier.Channel()
	batch, marks, err := e.dueReminders(ctx, channel)
	if err != nil {
		return 0, err
	}
	if len(batch) == 0 {
		return 0, nil
	}
	if err := e.notifier.Send(ctx, batch); err != nil {
		return 0, fmt.Errorf("engine: send reminders: %w", err)
	}
	for _, m := range marks {
		if err := e.repo.MarkReminderSent(ctx, m.subject, m.threshold, channel); err != nil {
			e.log(ctx, slog.LevelWarn, "mark reminder sent failed", slog.String("subject", m.subject), slog.String("error", err.Error()))
		}
	}
	e.recordActivity(ctx, observability.Record{
		Action: observability.ActionDeadlineRemind, Entity: "deadline", EntityID: channel, Status: "committed",
		Attributes: map[string]any{"channel": channel, "count": len(batch)},
	})
	e.log(ctx, slog.LevelInfo, "sent deadline reminders", slog.String("channel", channel), slog.Int("count", len(batch)))
	return len(batch), nil
}
