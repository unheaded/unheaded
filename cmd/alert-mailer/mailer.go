// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Alert is one alert in an Alertmanager webhook (payload version 4).
type Alert struct {
	Status      string            `json:"status"`
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
	StartsAt    time.Time         `json:"startsAt"`
	EndsAt      time.Time         `json:"endsAt"`
	Fingerprint string            `json:"fingerprint"`
}

// Webhook is Alertmanager's webhook body; only the fields used here.
type Webhook struct {
	Version string  `json:"version"`
	Status  string  `json:"status"`
	Alerts  []Alert `json:"alerts"`
}

// Message is one mail.
type Message struct {
	Urgency string // page, ticket (the digest)
	Subject string
	Body    string
}

// Sender delivers one message. nil in dry-run.
type Sender interface {
	Send(Message) error
}

// Settings is the mailer: block of observability/alerting/policy.yaml.
type Settings struct {
	Mode      string // dry-run or send
	MaxPerDay int
	Reminder  time.Duration // re-mail a page still burning after this long
	DigestH   int
	DigestM   int
	Loc       *time.Location
}

// OutboxEntry records what was mailed, or would have been, or was held back.
type OutboxEntry struct {
	Time    time.Time `json:"time"`
	Urgency string    `json:"urgency"`
	Outcome string    `json:"outcome"` // sent, dry-run, capped, failed, unchanged (not mailed: nothing new)
	Subject string    `json:"subject"`
	Body    string    `json:"body"`
}

const outboxKeep = 50

// state survives restarts (state.json in the state dir): a restart must not
// reset the daily cap or forget the open tickets.
type state struct {
	Day             string           `json:"day"`               // YYYY-MM-DD in Settings.Loc
	Sent            int              `json:"sent"`              // mails counted against today's cap
	Held            int              `json:"held"`              // mails the cap held back today
	HeldSinceDigest int              `json:"held_since_digest"` // reported, then reset, by the digest
	LastDigest      string           `json:"last_digest"`       // day the digest last ran
	Tickets         map[string]Alert `json:"tickets"`           // open tickets by fingerprint
	Resolved        []string         `json:"resolved"`          // tickets resolved since the last digest
	Outbox          []OutboxEntry    `json:"outbox"`
	PageFiring      []string         `json:"page_firing"`    // SLOs the last page mail reported burning
	PageMailedAt    time.Time        `json:"page_mailed_at"` // when
}

// Mailer turns Alertmanager webhooks into at most Settings.MaxPerDay mails a
// day: pages at once, tickets in one digest at Settings.DigestH:DigestM.
type Mailer struct {
	mu        sync.Mutex
	cfg       Settings
	send      Sender
	now       func() time.Time
	statePath string
	st        state
	count     func(urgency, outcome string)
}

// NewMailer loads state from dir (an empty state when there is none).
func NewMailer(cfg Settings, send Sender, dir string, now func() time.Time) (*Mailer, error) {
	if cfg.Mode == "send" && send == nil {
		return nil, errors.New("mode send needs an SMTP sender")
	}
	m := &Mailer{cfg: cfg, send: send, now: now, statePath: filepath.Join(dir, "state.json"), count: func(string, string) {}}
	data, err := os.ReadFile(m.statePath) // #nosec G304 -- our own state file
	switch {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return nil, err
	default:
		if err := json.Unmarshal(data, &m.st); err != nil {
			return nil, fmt.Errorf("%s: %w", m.statePath, err)
		}
	}
	if m.st.Tickets == nil {
		m.st.Tickets = map[string]Alert{}
	}
	return m, nil
}

func (m *Mailer) today() string { return m.now().In(m.cfg.Loc).Format("2006-01-02") }

// rollDay resets the cap at local midnight.
func (m *Mailer) rollDay() {
	if d := m.today(); d != m.st.Day {
		m.st.Day, m.st.Sent, m.st.Held = d, 0, 0
	}
}

func (m *Mailer) save() error {
	data, err := json.MarshalIndent(m.st, "", "  ")
	if err != nil {
		return err
	}
	tmp := m.statePath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, m.statePath)
}

func (m *Mailer) record(msg Message, outcome string) {
	m.st.Outbox = append(m.st.Outbox, OutboxEntry{Time: m.now().UTC(), Urgency: msg.Urgency, Outcome: outcome, Subject: msg.Subject, Body: msg.Body})
	if n := len(m.st.Outbox); n > outboxKeep {
		m.st.Outbox = m.st.Outbox[n-outboxKeep:]
	}
	m.count(msg.Urgency, outcome)
}

// deliver applies the cap, then sends (or, in dry-run, only records), and
// returns the outcome. A send error is returned, uncounted, so Alertmanager
// retries the webhook. Called with m.mu held.
func (m *Mailer) deliver(msg Message) (string, error) {
	m.rollDay()
	if m.st.Sent >= m.cfg.MaxPerDay {
		m.st.Held++
		m.st.HeldSinceDigest++
		m.record(msg, "capped")
		return "capped", m.save()
	}
	n := m.st.Sent + 1
	msg.Body += fmt.Sprintf("\n--\nMail %d of at most %d today (%s).", n, m.cfg.MaxPerDay, m.cfg.Mode)
	if n == m.cfg.MaxPerDay {
		msg.Body += " This is the last one: anything else today is held back and counted in the next digest."
	}
	outcome := "dry-run"
	if m.cfg.Mode == "send" {
		if err := m.send.Send(msg); err != nil {
			m.record(msg, "failed")
			_ = m.save()
			return "failed", err
		}
		outcome = "sent"
	}
	m.st.Sent = n
	m.record(msg, outcome)
	return outcome, m.save()
}

// Page mails an Alertmanager page notification only when it says something
// new: an SLO starts burning, or everything has stopped. Tier changes and
// partial recoveries are recorded as "unchanged", not mailed; a page still
// burning after Settings.Reminder is mailed again. One incident: two mails.
func (m *Mailer) Page(w Webhook) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	firing, resolved := split(w.Alerts)
	if len(firing)+len(resolved) == 0 {
		return nil
	}
	now := m.now()
	names := names(firing)
	// An SLO page says its budget is burning; a promoted cause alert (no slo
	// label, e.g. HeartbeatMissing) only that it fires.
	what := ": firing"
	for _, a := range firing {
		if a.Labels["slo"] != "" {
			what = ": error budget burning"
		}
	}
	subject := "[page] " + strings.Join(names, ", ") + what
	newSLO := false
	for _, n := range names {
		newSLO = newSLO || !contains(m.st.PageFiring, n)
	}
	switch {
	case len(firing) == 0 && len(m.st.PageFiring) == 0:
		return nil // a resolution nobody was told about
	case len(firing) == 0:
		subject = "[resolved] " + strings.Join(m.st.PageFiring, ", ")
	case newSLO:
	case now.Sub(m.st.PageMailedAt) >= m.cfg.Reminder:
		subject = "[still burning] " + strings.Join(names, ", ")
	default:
		m.st.PageFiring = names
		m.record(Message{Urgency: "page", Subject: subject}, "unchanged")
		return m.save()
	}
	var b strings.Builder
	if len(firing) > 0 {
		b.WriteString("Burning now:\n\n")
		for _, a := range firing {
			writeAlert(&b, a)
		}
	}
	if len(resolved) > 0 {
		b.WriteString("Resolved:\n\n")
		for _, a := range resolved {
			fmt.Fprintf(&b, "- %s%s, since %s\n", name(a), tierNote(a), a.StartsAt.In(m.cfg.Loc).Format("Jan 2 15:04"))
		}
		b.WriteString("\n")
	}
	b.WriteString(footer)
	outcome, err := m.deliver(Message{Urgency: "page", Subject: subject, Body: b.String()})
	if err == nil && outcome != "capped" {
		m.st.PageFiring, m.st.PageMailedAt = names, now
		err = m.save()
	}
	return err
}

// Ticket keeps the open-ticket list current; nothing is mailed until the digest.
func (m *Mailer) Ticket(w Webhook) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, a := range w.Alerts {
		key := a.Fingerprint
		if key == "" {
			key = a.Labels["slo"] + "/" + a.Labels["tier"] + "/" + a.Labels["alertname"]
		}
		if a.Status == "resolved" {
			if _, open := m.st.Tickets[key]; open {
				delete(m.st.Tickets, key)
				m.st.Resolved = append(m.st.Resolved, fmt.Sprintf("%s (%s tier), open since %s",
					name(a), a.Labels["tier"], a.StartsAt.In(m.cfg.Loc).Format("Jan 2 15:04")))
			}
			continue
		}
		m.st.Tickets[key] = a
	}
	return m.save()
}

// Tick sends the day's digest once it is due. Nothing open, nothing
// resolved and nothing held back means no mail at all.
func (m *Mailer) Tick() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.now().In(m.cfg.Loc)
	due := time.Date(now.Year(), now.Month(), now.Day(), m.cfg.DigestH, m.cfg.DigestM, 0, 0, m.cfg.Loc)
	if now.Before(due) || m.st.LastDigest == m.today() {
		return nil
	}
	if len(m.st.Tickets) == 0 && len(m.st.Resolved) == 0 && m.st.HeldSinceDigest == 0 {
		m.st.LastDigest = m.today()
		return m.save()
	}
	open := make([]Alert, 0, len(m.st.Tickets))
	for _, a := range m.st.Tickets {
		open = append(open, a)
	}
	sort.Slice(open, func(i, j int) bool {
		return name(open[i])+open[i].Labels["tier"] < name(open[j])+open[j].Labels["tier"]
	})
	var b strings.Builder
	subject := fmt.Sprintf("[digest] %d open", len(open))
	if len(open) > 0 {
		b.WriteString("Open (budget burning slowly; worth a look this week):\n\n")
		for _, a := range open {
			writeAlert(&b, a)
		}
	}
	if len(m.st.Resolved) > 0 {
		subject += fmt.Sprintf(", %d resolved", len(m.st.Resolved))
		b.WriteString("Resolved since the last digest:\n\n")
		for _, r := range m.st.Resolved {
			b.WriteString("- " + r + "\n")
		}
		b.WriteString("\n")
	}
	if h := m.st.HeldSinceDigest; h > 0 {
		subject += fmt.Sprintf(", %d held back", h)
		fmt.Fprintf(&b, "%d mail(s) were held back by the daily cap since the last digest; see /v1/outbox.\n\n", h)
	}
	b.WriteString(footer)
	outcome, err := m.deliver(Message{Urgency: "ticket", Subject: subject, Body: b.String()})
	if err != nil {
		return err
	}
	// A capped digest keeps its news for the next one.
	if outcome != "capped" {
		m.st.Resolved = nil
		m.st.HeldSinceDigest = 0
	}
	m.st.LastDigest = m.today()
	return m.save()
}

// Status is a summary for /v1/status.
type Status struct {
	Mode        string        `json:"mode"`
	Day         string        `json:"day"`
	SentToday   int           `json:"sent_today"`
	MaxPerDay   int           `json:"max_per_day"`
	HeldToday   int           `json:"held_today"`
	OpenTickets int           `json:"open_tickets"`
	LastDigest  string        `json:"last_digest"`
	Outbox      []OutboxEntry `json:"outbox,omitempty"`
}

func (m *Mailer) Status(withOutbox bool) Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rollDay()
	s := Status{Mode: m.cfg.Mode, Day: m.st.Day, SentToday: m.st.Sent, MaxPerDay: m.cfg.MaxPerDay, HeldToday: m.st.Held,
		OpenTickets: len(m.st.Tickets), LastDigest: m.st.LastDigest}
	if withOutbox {
		s.Outbox = append([]OutboxEntry(nil), m.st.Outbox...)
	}
	return s
}

const footer = `What is firing: Alertmanager on west, :9093.
Working on it, or a planned outage: scripts/monitoring/maintenance.sh 1h "why".
Tune what notifies: observability/alerting/policy.yaml.
`

func split(as []Alert) (firing, resolved []Alert) {
	for _, a := range as {
		if a.Status == "resolved" {
			resolved = append(resolved, a)
		} else {
			firing = append(firing, a)
		}
	}
	return firing, resolved
}

func name(a Alert) string {
	if s := a.Labels["slo"]; s != "" {
		return s
	}
	return a.Labels["alertname"]
}

// names lists the distinct SLO (or alert) names, sorted.
func names(as []Alert) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, a := range as {
		if n := name(a); !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	sort.Strings(out)
	return out
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// tierNote is " (fast tier)" for an SLO alert and "" for one without a tier.
func tierNote(a Alert) string {
	if t := a.Labels["tier"]; t != "" {
		return " (" + t + " tier)"
	}
	return ""
}

func writeAlert(b *strings.Builder, a Alert) {
	fmt.Fprintf(b, "- %s%s\n", a.Annotations["summary"], tierNote(a))
	if d := a.Annotations["description"]; d != "" {
		fmt.Fprintf(b, "  %s\n", d)
	}
	if r := a.Annotations["runbook_url"]; r != "" {
		fmt.Fprintf(b, "  Runbook: %s\n", r)
	}
	b.WriteString("\n")
}
