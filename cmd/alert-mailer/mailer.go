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

	"github.com/rs/zerolog/log"
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
	Mode       string // dry-run or send
	MaxPerDay  int
	MaxPerHour int           // 0: no hourly cap
	Reminder   time.Duration // re-mail a page still burning after this long
	DigestH    int
	DigestM    int
	Loc        *time.Location
	// Run days (ADR-147 decision 2, mailer.run_days): on a listed date
	// (YYYY-MM-DD in Loc) these caps replace MaxPerDay and MaxPerHour.
	RunDayMaxPerDay  int
	RunDayMaxPerHour int
	RunDates         map[string]bool
	// PipelineSilentAfter (pipeline.silent_after): with no pipeline receipt
	// from Alertmanager for this long, mail once that the alert path is
	// silent (ADR-116 R4, ADR-147 stage 1). 0 turns it off.
	PipelineSilentAfter time.Duration
}

// OutboxEntry records what was mailed, or would have been, or was held back.
type OutboxEntry struct {
	Time    time.Time `json:"time"`
	Urgency string    `json:"urgency"`
	Outcome string    `json:"outcome"` // sent, dry-run, capped, capped-hour, failed, unchanged (not mailed: nothing new)
	Subject string    `json:"subject"`
	Body    string    `json:"body"`
}

const outboxKeep = 50

// pipelineOverCapPerDay bounds the alert-pipeline mails (silent, back) that
// may go past a spent cap in one day: two outages, each mailed and answered.
const pipelineOverCapPerDay = 4

// state survives restarts (state.json in the state dir): a restart must not
// reset the daily cap or forget the open tickets.
type state struct {
	Day             string           `json:"day"`               // YYYY-MM-DD in Settings.Loc
	Sent            int              `json:"sent"`              // mails counted against today's cap
	Hour            string           `json:"hour"`              // YYYY-MM-DDTHH in Settings.Loc
	HourSent        int              `json:"hour_sent"`         // mails counted against this hour's cap
	Held            int              `json:"held"`              // mails the cap held back today
	HeldSinceDigest int              `json:"held_since_digest"` // reported, then reset, by the digest
	LastDigest      string           `json:"last_digest"`       // day the digest last ran
	Tickets         map[string]Alert `json:"tickets"`           // open tickets by fingerprint
	Resolved        []string         `json:"resolved"`          // tickets resolved since the last digest
	Outbox          []OutboxEntry    `json:"outbox"`
	PageFiring      []string         `json:"page_firing"`       // SLOs the last page mail reported burning
	PageMailedAt    time.Time        `json:"page_mailed_at"`    // when
	PipelineLast    time.Time        `json:"pipeline_last"`     // last pipeline receipt from Alertmanager
	PipelineAlerted bool             `json:"pipeline_alerted"`  // the silent mail went out; the back mail is owed
	PipelineOverCap int              `json:"pipeline_over_cap"` // pipeline mails sent past a spent cap today
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
	started   time.Time // the pipeline's grace runs from here after a start
}

// NewMailer loads state from dir (an empty state when there is none).
func NewMailer(cfg Settings, send Sender, dir string, now func() time.Time) (*Mailer, error) {
	if cfg.Mode == "send" && send == nil {
		return nil, errors.New("mode send needs an SMTP sender")
	}
	m := &Mailer{cfg: cfg, send: send, now: now, statePath: filepath.Join(dir, "state.json"), count: func(string, string) {}, started: now()}
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
		m.st.Day, m.st.Sent, m.st.Held, m.st.PipelineOverCap = d, 0, 0, 0
	}
}

// rollHour resets the hourly count when the hour in Settings.Loc changes.
func (m *Mailer) rollHour() {
	if h := m.now().In(m.cfg.Loc).Format("2006-01-02T15"); h != m.st.Hour {
		m.st.Hour, m.st.HourSent = h, 0
	}
}

// caps returns today's daily and hourly caps: the run-day caps on a listed
// run day, the normal ones otherwise. A run-day cap never lowers a cap.
func (m *Mailer) caps() (day, hour int) {
	day, hour = m.cfg.MaxPerDay, m.cfg.MaxPerHour
	if m.cfg.RunDates[m.today()] {
		day, hour = max(day, m.cfg.RunDayMaxPerDay), max(hour, m.cfg.RunDayMaxPerHour)
	}
	return day, hour
}

// held reports whether a cap kept the mail back.
func held(o string) bool { return o == "capped" || o == "capped-hour" }

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
	log.Info().Str("urgency", msg.Urgency).Str("outcome", outcome).Str("subject", msg.Subject).Msg("alert-mail")
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
	return m.deliverCapped(msg, false)
}

// deliverPipeline is deliver for the alert-pipeline mails. Nothing retries
// them (Alertmanager is the thing that is down), and a cap spent by the
// incident's own pages would lose the one mail that says no alert can reach
// you (review 2026-11-15, REV-1). They go past a spent cap, still counted,
// at most pipelineOverCapPerDay a day. Called with m.mu held.
func (m *Mailer) deliverPipeline(msg Message) (string, error) {
	m.rollDay()
	return m.deliverCapped(msg, m.st.PipelineOverCap < pipelineOverCapPerDay)
}

// deliverCapped is deliver; pastCap sends even when a cap is spent.
func (m *Mailer) deliverCapped(msg Message, pastCap bool) (string, error) {
	m.rollDay()
	m.rollHour()
	maxDay, maxHour := m.caps()
	overHour := maxHour > 0 && m.st.HourSent >= maxHour
	overDay := m.st.Sent >= maxDay
	if overHour && !pastCap {
		m.st.Held++
		m.st.HeldSinceDigest++
		m.record(msg, "capped-hour")
		return "capped-hour", m.save()
	}
	if overDay && !pastCap {
		m.st.Held++
		m.st.HeldSinceDigest++
		m.record(msg, "capped")
		return "capped", m.save()
	}
	over := overHour || overDay
	n := m.st.Sent + 1
	if over {
		msg.Body += fmt.Sprintf("\n--\nMail %d today, past the cap of %d (%s): the alert path itself is the news.", n, maxDay, m.cfg.Mode)
	} else {
		msg.Body += fmt.Sprintf("\n--\nMail %d of at most %d today (%s).", n, maxDay, m.cfg.Mode)
		if n == maxDay {
			msg.Body += " This is the last one: anything else today is held back and counted in the next digest."
		}
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
	m.st.HourSent++
	if over {
		m.st.PipelineOverCap++
	}
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
	if err == nil && !held(outcome) {
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
				m.st.Resolved = append(m.st.Resolved, fmt.Sprintf("%s%s, open since %s",
					name(a), tierNote(a), a.StartsAt.In(m.cfg.Loc).Format("Jan 2 15:04")))
			}
			continue
		}
		m.st.Tickets[key] = a
	}
	return m.save()
}

// PipelineSeen records a pipeline receipt: Alertmanager handed over its
// AlertPipelineHeartbeat, so Prometheus evaluates, Alertmanager routes and
// the route reaches here. After a silent mail it mails, once, that receipts
// are back.
func (m *Mailer) PipelineSeen() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.now()
	m.st.PipelineLast = now
	if m.st.PipelineAlerted {
		m.st.PipelineAlerted = false
		if _, err := m.deliverPipeline(Message{Urgency: "page", Subject: "[page] alert pipeline back",
			Body: fmt.Sprintf("alert pipeline back: receipts from Alertmanager resumed at %s.\n\n%s",
				now.In(m.cfg.Loc).Format("Jan 2 15:04 MST"), footer)}); err != nil {
			m.st.PipelineAlerted = true // owed again: the next receipt retries
			_ = m.save()
			return err
		}
	}
	return m.save()
}

// PipelineLast is the time of the last pipeline receipt (zero: none yet).
func (m *Mailer) PipelineLast() time.Time {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.st.PipelineLast
}

// pipelineCheck mails once when no receipt has come for
// PipelineSilentAfter, counted from the last receipt or from this start,
// whichever is later. A rule cannot report that its own evaluator is down;
// the mailer can. Called with m.mu held.
func (m *Mailer) pipelineCheck() error {
	if m.cfg.PipelineSilentAfter <= 0 || m.st.PipelineAlerted {
		return nil
	}
	since := m.st.PipelineLast
	if m.started.After(since) {
		since = m.started
	}
	if m.now().Sub(since) <= m.cfg.PipelineSilentAfter {
		return nil
	}
	m.st.PipelineAlerted = true
	last := "never since this start"
	if !m.st.PipelineLast.IsZero() {
		last = m.st.PipelineLast.In(m.cfg.Loc).Format("Jan 2 15:04 MST")
	}
	_, err := m.deliverPipeline(Message{Urgency: "page", Subject: "[page] alert pipeline silent",
		Body: fmt.Sprintf("alert pipeline silent: no heartbeat from Alertmanager since %s; Prometheus, Alertmanager or the route is down.\n"+
			"Until it is back, no alert can reach you.\n  Runbook: runbooks/observe/alert-pipeline-silent.yaml\n\n%s", last, footer)})
	if err != nil {
		m.st.PipelineAlerted = false // the relay refused: the next tick retries
		_ = m.save()
		return err
	}
	return m.save()
}

// Tick checks the alert pipeline, then sends the day's digest once it is
// due. Nothing open, nothing resolved and nothing held back means no digest.
func (m *Mailer) Tick() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.pipelineCheck(); err != nil {
		return err
	}
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
		b.WriteString("Open (worth a look this week):\n\n")
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
		fmt.Fprintf(&b, "%d mail(s) were held back by a cap since the last digest; see /v1/outbox.\n\n", h)
	}
	b.WriteString(footer)
	outcome, err := m.deliver(Message{Urgency: "ticket", Subject: subject, Body: b.String()})
	if err != nil {
		return err
	}
	// A capped digest keeps its news for the next one.
	if !held(outcome) {
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
	maxDay, _ := m.caps()
	s := Status{Mode: m.cfg.Mode, Day: m.st.Day, SentToday: m.st.Sent, MaxPerDay: maxDay, HeldToday: m.st.Held,
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
