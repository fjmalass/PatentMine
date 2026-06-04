// Package remind delivers deadline reminders through a pluggable Notifier so the
// engine can surface upcoming office-action responses and patent maintenance
// fees without knowing how they are sent. The default is a no-op (the in-app
// deadline surface is always available); an SMTP email notifier is wired in when
// configured. This is "the plug" — adding a channel (Slack, calendar) means
// adding a Notifier, not touching the engine.
package remind

import (
	"context"
	"fmt"
	"net/smtp"
	"sort"
	"strings"
	"time"
)

// Reminder is one due deadline to notify about.
type Reminder struct {
	Subject      string // stable dedupe key (deadline id, or "oa:<id>")
	Kind         string // human label of the deadline kind
	Title        string
	DueDate      time.Time
	DaysUntil    int
	PatentNumber string
	Project      string
}

// Notifier delivers a batch of reminders over one channel.
type Notifier interface {
	// Send delivers the reminders. It is called only when there is at least one.
	Send(ctx context.Context, rs []Reminder) error
	// Channel names the delivery channel, used as the reminder-log dedupe key.
	Channel() string
	// Enabled reports whether the notifier will actually deliver (a disabled or
	// unconfigured notifier is skipped so nothing is marked sent).
	Enabled() bool
}

// NoopNotifier is the default: it delivers nothing. The in-app deadline surface
// is the always-available channel; email is opt-in.
type NoopNotifier struct{}

func (NoopNotifier) Send(context.Context, []Reminder) error { return nil }
func (NoopNotifier) Channel() string                        { return "noop" }
func (NoopNotifier) Enabled() bool                           { return false }

// EmailConfig configures the SMTP email notifier.
type EmailConfig struct {
	Enabled  bool
	Host     string
	Port     int
	Username string
	Password string
	From     string
	To       string
}

// NewNotifier returns an SMTP email notifier when email is enabled and a host is
// configured, otherwise the no-op notifier. This is the single selection point
// the daemon calls.
func NewNotifier(cfg EmailConfig) Notifier {
	if cfg.Enabled && strings.TrimSpace(cfg.Host) != "" && strings.TrimSpace(cfg.To) != "" {
		return &EmailNotifier{cfg: cfg}
	}
	return NoopNotifier{}
}

// EmailNotifier sends reminders as a single digest email over SMTP using the
// standard library (no external SDK, keeping the single-binary build).
type EmailNotifier struct {
	cfg EmailConfig
}

func (n *EmailNotifier) Channel() string { return "email" }
func (n *EmailNotifier) Enabled() bool   { return true }

func (n *EmailNotifier) Send(ctx context.Context, rs []Reminder) error {
	if len(rs) == 0 {
		return nil
	}
	from := n.cfg.From
	if from == "" {
		from = n.cfg.Username
	}
	body := buildDigest(from, n.cfg.To, rs)
	addr := fmt.Sprintf("%s:%d", n.cfg.Host, n.cfg.Port)

	var auth smtp.Auth
	if n.cfg.Username != "" {
		auth = smtp.PlainAuth("", n.cfg.Username, n.cfg.Password, n.cfg.Host)
	}
	// net/smtp has no context hook; honor cancellation before dialing.
	if err := ctx.Err(); err != nil {
		return err
	}
	recipients := splitRecipients(n.cfg.To)
	if err := smtp.SendMail(addr, auth, from, recipients, []byte(body)); err != nil {
		return fmt.Errorf("remind: send email: %w", err)
	}
	return nil
}

func splitRecipients(to string) []string {
	parts := strings.FieldsFunc(to, func(r rune) bool { return r == ',' || r == ';' || r == ' ' })
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// buildDigest renders the RFC 5322 message for a batch of reminders, soonest due
// first.
func buildDigest(from, to string, rs []Reminder) string {
	sorted := append([]Reminder(nil), rs...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].DueDate.Before(sorted[j].DueDate) })

	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", from)
	fmt.Fprintf(&b, "To: %s\r\n", to)
	fmt.Fprintf(&b, "Subject: PatentMine: %d upcoming deadline(s)\r\n", len(sorted))
	b.WriteString("Content-Type: text/plain; charset=utf-8\r\n\r\n")
	b.WriteString("Upcoming patent deadlines:\r\n\r\n")
	for _, r := range sorted {
		when := r.DueDate.Format("2006-01-02")
		switch {
		case r.DaysUntil < 0:
			fmt.Fprintf(&b, "  • OVERDUE (%dd) — %s — due %s [%s]\r\n", -r.DaysUntil, r.Title, when, r.Kind)
		case r.DaysUntil == 0:
			fmt.Fprintf(&b, "  • DUE TODAY — %s — %s [%s]\r\n", r.Title, when, r.Kind)
		default:
			fmt.Fprintf(&b, "  • in %dd — %s — due %s [%s]\r\n", r.DaysUntil, r.Title, when, r.Kind)
		}
	}
	b.WriteString("\r\n— PatentMine docketing assistant (verify all dates; this is not legal advice)\r\n")
	return b.String()
}

// DefaultThresholds are the days-before-due at which a reminder fires (2 months,
// 15 days, 7 days), plus 0 for an overdue notice.
var DefaultThresholds = []int{60, 15, 7, 0}
