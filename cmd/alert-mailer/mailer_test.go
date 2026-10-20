// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"errors"
	"fmt"
	"net"
	"net/textproto"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeSender struct {
	mu   sync.Mutex
	sent []Message
	err  error
}

func (f *fakeSender) Send(m Message) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	f.sent = append(f.sent, m)
	return nil
}

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

var chicago, _ = time.LoadLocation("America/Chicago")

func newTest(t *testing.T, mode string, max int) (*Mailer, *fakeSender, *clock, string) {
	t.Helper()
	dir := t.TempDir()
	c := &clock{time.Date(2026, 10, 20, 2, 0, 0, 0, chicago)}
	f := &fakeSender{}
	m, err := NewMailer(Settings{Mode: mode, MaxPerDay: max, Reminder: 12 * time.Hour, DigestH: 9, Loc: chicago}, f, dir, c.now)
	if err != nil {
		t.Fatal(err)
	}
	return m, f, c, dir
}

func alert(status, slo, tier string) Alert {
	return Alert{Status: status, Fingerprint: slo + tier,
		Labels:      map[string]string{"alertname": "ErrorBudgetBurn", "slo": slo, "tier": tier, "urgency": "page"},
		Annotations: map[string]string{"summary": "SLO " + slo + ": down", "description": "burning 25x"},
		StartsAt:    time.Date(2026, 10, 20, 1, 55, 0, 0, chicago)}
}

func page(m *Mailer, as ...Alert) error { return m.Page(Webhook{Version: "4", Alerts: as}) }

// The cap is hard: past max_per_day nothing goes out, the last mail says so,
// and the next digest counts what was held back.
func TestCapIsHardAndReported(t *testing.T) {
	m, f, c, _ := newTest(t, "send", 3)
	for i := 0; i < 10; i++ { // ten different SLOs starting to burn: each is news
		if err := page(m, alert("firing", fmt.Sprintf("slo-%d", i), "fast")); err != nil {
			t.Fatal(err)
		}
	}
	if len(f.sent) != 3 {
		t.Fatalf("%d mails sent, cap is 3", len(f.sent))
	}
	if !strings.Contains(f.sent[2].Body, "last one") || strings.Contains(f.sent[1].Body, "last one") {
		t.Fatalf("only the third mail should say it is the last:\n%s", f.sent[2].Body)
	}
	if s := m.Status(false); s.HeldToday != 7 {
		t.Fatalf("held = %d, want 7", s.HeldToday)
	}
	c.t = time.Date(2026, 10, 21, 9, 0, 0, 0, chicago) // next day: cap reset, digest due
	if err := m.Tick(); err != nil {
		t.Fatal(err)
	}
	if len(f.sent) != 4 || !strings.Contains(f.sent[3].Subject, "7 held back") {
		t.Fatalf("digest should report 7 held back; sent %d, last %q", len(f.sent), f.sent[len(f.sent)-1].Subject)
	}
}

// dry-run decides exactly as send would, cap included, and sends nothing.
func TestDryRunSendsNothing(t *testing.T) {
	m, f, _, _ := newTest(t, "dry-run", 2)
	for i := 0; i < 3; i++ {
		_ = page(m, alert("firing", fmt.Sprintf("slo-%d", i), "fast"))
	}
	if len(f.sent) != 0 {
		t.Fatalf("dry-run sent %d mails", len(f.sent))
	}
	var outcomes []string
	for _, e := range m.Status(true).Outbox {
		outcomes = append(outcomes, e.Outcome)
	}
	if got := strings.Join(outcomes, ","); got != "dry-run,dry-run,capped" {
		t.Fatalf("outbox outcomes %s", got)
	}
}

// Tickets are never mailed one by one: one digest a day, only if there is
// something to say, and resolved ones appear once.
func TestTicketsOnlyInTheDailyDigest(t *testing.T) {
	m, f, c, _ := newTest(t, "send", 6)
	a := alert("firing", "overlay", "fast")
	b := alert("firing", "kev-feed", "slow")
	for i := 0; i < 5; i++ {
		if err := m.Ticket(Webhook{Alerts: []Alert{a, b}}); err != nil {
			t.Fatal(err)
		}
	}
	_ = m.Tick() // 02:00, before the digest
	if len(f.sent) != 0 {
		t.Fatalf("tickets mailed before the digest: %d", len(f.sent))
	}
	a.Status = "resolved"
	_ = m.Ticket(Webhook{Alerts: []Alert{a}})
	c.t = time.Date(2026, 10, 20, 9, 1, 0, 0, chicago)
	_ = m.Tick()
	_ = m.Tick()
	if len(f.sent) != 1 {
		t.Fatalf("%d digests, want 1", len(f.sent))
	}
	d := f.sent[0]
	if d.Subject != "[digest] 1 open, 1 resolved" || !strings.Contains(d.Body, "kev-feed") || !strings.Contains(d.Body, "overlay (fast tier)") {
		t.Fatalf("digest: %q\n%s", d.Subject, d.Body)
	}
	// Next day: kev-feed still open, nothing resolved: one line, no repeat of overlay.
	c.t = time.Date(2026, 10, 21, 9, 0, 0, 0, chicago)
	_ = m.Tick()
	if len(f.sent) != 2 || f.sent[1].Subject != "[digest] 1 open" {
		t.Fatalf("second digest %q", f.sent[len(f.sent)-1].Subject)
	}
	// All clear: no mail at all.
	b.Status = "resolved"
	_ = m.Ticket(Webhook{Alerts: []Alert{b}})
	c.t = time.Date(2026, 10, 22, 9, 0, 0, 0, chicago)
	_ = m.Tick() // reports the resolution
	c.t = time.Date(2026, 10, 23, 9, 0, 0, 0, chicago)
	_ = m.Tick() // nothing to say
	if len(f.sent) != 3 {
		t.Fatalf("%d mails; a day with nothing open must send nothing", len(f.sent))
	}
}

// A relay failure is returned (Alertmanager retries) and not counted.
func TestSendFailureNotCounted(t *testing.T) {
	m, f, _, _ := newTest(t, "send", 2)
	f.err = errors.New("421 try later")
	if err := page(m, alert("firing", "wotan", "fast")); err == nil {
		t.Fatal("send failure swallowed: Alertmanager would never retry")
	}
	f.err = nil
	_ = page(m, alert("firing", "wotan", "fast"))
	_ = page(m, alert("firing", "well", "fast"))
	if len(f.sent) != 2 {
		t.Fatalf("failed attempt used a slot: %d sent", len(f.sent))
	}
}

// The cap and the open tickets survive a restart.
func TestStateSurvivesRestart(t *testing.T) {
	m, f, c, dir := newTest(t, "send", 2)
	_ = page(m, alert("firing", "wotan", "fast"))
	_ = page(m, alert("firing", "well", "fast"))
	_ = m.Ticket(Webhook{Alerts: []Alert{alert("firing", "overlay", "fast")}})
	m2, err := NewMailer(m.cfg, f, dir, c.now)
	if err != nil {
		t.Fatal(err)
	}
	_ = page(m2, alert("firing", "dashboard", "fast"))
	if len(f.sent) != 2 {
		t.Fatalf("restart reset the cap: %d sent", len(f.sent))
	}
	if n := m2.Status(false).OpenTickets; n != 1 {
		t.Fatalf("restart lost tickets: %d open", n)
	}
}

// One incident is two mails: the start and the end. Tier changes, partial
// recoveries and Alertmanager's re-sends in between are not news.
func TestOneIncidentTwoMails(t *testing.T) {
	m, f, c, _ := newTest(t, "send", 6)
	step := func(as ...Alert) { c.t = c.t.Add(10 * time.Minute); _ = page(m, as...) }
	step(alert("firing", "wotan", "fast"), alert("firing", "well", "fast"))
	step(alert("firing", "wotan", "fast"), alert("firing", "well", "fast"), alert("firing", "wotan", "medium"))
	step(alert("resolved", "wotan", "fast"), alert("firing", "well", "fast"), alert("firing", "wotan", "medium"))
	step(alert("resolved", "well", "fast"), alert("firing", "wotan", "medium"))
	step(alert("resolved", "wotan", "medium"))
	var got []string
	for _, s := range f.sent {
		got = append(got, s.Subject)
	}
	if want := "[page] well, wotan: error budget burning|[resolved] wotan"; strings.Join(got, "|") != want {
		t.Fatalf("mails:\n  %s\nwant\n  %s", strings.Join(got, "\n  "), want)
	}
	// A new SLO joining mid-incident is news.
	step(alert("firing", "wotan", "fast"))
	step(alert("firing", "wotan", "fast"), alert("firing", "dashboard", "fast"))
	if n := len(f.sent); n != 4 {
		t.Fatalf("%d mails, want 4 (wotan, then dashboard joining)", n)
	}
	// Still burning 12h later: one reminder.
	c.t = c.t.Add(12 * time.Hour)
	_ = page(m, alert("firing", "wotan", "fast"), alert("firing", "dashboard", "fast"))
	if last := f.sent[len(f.sent)-1].Subject; !strings.HasPrefix(last, "[still burning]") {
		t.Fatalf("no reminder: %q", last)
	}
}

func TestSettingsRefusals(t *testing.T) {
	env := func(kv map[string]string) func(string) string { return func(k string) string { return kv[k] } }
	good := map[string]string{"SMTP_SMARTHOST": "smtp.gmail.com:587", "SMTP_USERNAME": "a@bellis.tech",
		"SMTP_PASSWORD_FILE": "/run/secrets/smtp-password", "ALERT_MAIL_FROM": "a@bellis.tech", "ALERT_MAIL_TO": "a@bellis.tech"}
	if _, err := smtpFromEnv(env(good)); err != nil {
		t.Fatalf("good settings refused: %v", err)
	}
	for k, v := range map[string]string{
		"SMTP_STARTTLS":      "false", // a login in plaintext
		"SMTP_SMARTHOST":     "smtp.gmail.com",
		"ALERT_MAIL_TO":      "not-an-address",
		"SMTP_PASSWORD_FILE": "",
	} {
		bad := map[string]string{}
		for gk, gv := range good {
			bad[gk] = gv
		}
		bad[k] = v
		if _, err := smtpFromEnv(env(bad)); err == nil {
			t.Errorf("%s=%q accepted", k, v)
		}
	}
	if _, err := NewMailer(Settings{Mode: "send", MaxPerDay: 1, Loc: chicago}, nil, t.TempDir(), time.Now); err == nil {
		t.Error("mode send without a relay accepted")
	}
}

// Alert text cannot add headers.
func TestFormatStripsHeaderInjection(t *testing.T) {
	raw := string(format("a@bellis.tech", []string{"a@bellis.tech"},
		Message{Urgency: "page", Subject: "x\r\nBcc: victim@example.org", Body: "line\n.\nend"}, time.Unix(0, 0)))
	hdr, _, _ := strings.Cut(raw, "\r\n\r\n")
	if strings.Contains(hdr, "\r\nBcc:") {
		t.Fatalf("header injected:\n%s", hdr)
	}
	for _, h := range []string{"Auto-Submitted: auto-generated", "Message-ID: <", "@bellis.tech>"} {
		if !strings.Contains(hdr, h) {
			t.Errorf("missing %q", h)
		}
	}
}

// End to end against a minimal SMTP server: no STARTTLS offered means no
// send when STARTTLS is required (the default).
func TestSMTPRefusesWithoutStartTLS(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		tp := textproto.NewConn(c)
		_ = tp.PrintfLine("220 test")
		for {
			line, err := tp.ReadLine()
			if err != nil {
				return
			}
			switch {
			case strings.HasPrefix(line, "EHLO"):
				_ = tp.PrintfLine("250 test")
			case strings.HasPrefix(line, "QUIT"):
				_ = tp.PrintfLine("221 bye")
				return
			default:
				_ = tp.PrintfLine("250 ok")
			}
		}
	}()
	s := &smtpSender{addr: ln.Addr().String(), host: "127.0.0.1", from: "a@bellis.tech", to: []string{"a@bellis.tech"}, startTLS: true, timeout: 5 * time.Second}
	if err := s.Send(Message{Subject: "x", Body: "y"}); err == nil || !strings.Contains(err.Error(), "STARTTLS") {
		t.Fatalf("sent without TLS: %v", err)
	}
}
