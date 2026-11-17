// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis. All rights reserved.

package main

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// newDigestTest is newTest with mailer.delivery: digest and the given
// immediate list.
func newDigestTest(t *testing.T, max int, immediate ...string) (*Mailer, *fakeSender, *clock) {
	t.Helper()
	c := &clock{time.Date(2026, 10, 20, 2, 0, 0, 0, chicago)}
	f := &fakeSender{}
	now := map[string]bool{}
	for _, n := range immediate {
		now[n] = true
	}
	m, err := NewMailer(Settings{Mode: "send", MaxPerDay: max, Reminder: 12 * time.Hour, DigestH: 9, Loc: chicago,
		Delivery: "digest", Immediate: now, PipelineSilentAfter: 20 * time.Minute}, f, t.TempDir(), c.now)
	if err != nil {
		t.Fatal(err)
	}
	return m, f, c
}

// promoted is a promoted cause alert as Alertmanager hands it to /v1/page.
func promoted(status, name string) Alert {
	return Alert{Status: status, Fingerprint: "fp-" + name,
		Labels:      map[string]string{"alertname": name, "urgency": "page"},
		Annotations: map[string]string{"summary": name + " on west"},
		StartsAt:    time.Date(2026, 10, 20, 1, 55, 0, 0, chicago)}
}

// Stevie, 2026-11-17: one digest mail a day and nothing else. Under
// delivery: digest a page is not mailed: it is recorded, and it leads the
// day's one digest.
func TestDigestDeliveryMailsOnceADay(t *testing.T) {
	m, f, c := newDigestTest(t, 6) // a cap of 6: the delivery, not the cap, keeps it to one
	if err := m.PipelineSeen(); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ { // ten pages through the night, repeated as Alertmanager repeats them
		for r := 0; r < 3; r++ {
			if err := page(m, promoted("firing", fmt.Sprintf("ContainerStopped%d", i))); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := m.Ticket(Webhook{Alerts: []Alert{alert("firing", "kev-feed", "slow")}}); err != nil {
		t.Fatal(err)
	}
	if err := m.Tick(); err != nil { // 02:00, before the digest
		t.Fatal(err)
	}
	if len(f.sent) != 0 {
		t.Fatalf("%d mails before the digest hour; delivery: digest mails nothing but the digest", len(f.sent))
	}
	queued := 0
	for _, e := range m.Status(true).Outbox {
		if e.Urgency == "page" && e.Outcome == "digest" {
			queued++
		}
	}
	if queued != 10 {
		t.Fatalf("outbox shows %d pages kept for the digest, want 10 (one each, not one per repeat)", queued)
	}

	c.t = time.Date(2026, 10, 20, 9, 0, 0, 0, chicago)
	_ = m.PipelineSeen()
	_ = m.Tick()
	_ = m.Tick()
	if len(f.sent) != 1 {
		t.Fatalf("%d mails at the digest hour, want 1", len(f.sent))
	}
	d := f.sent[0]
	if d.Subject != "[digest] 11 open (10 urgent)" {
		t.Fatalf("digest subject %q", d.Subject)
	}
	urgent, rest := strings.Index(d.Body, "Urgent"), strings.Index(d.Body, "Open (worth a look this week)")
	if urgent < 0 || rest < 0 || urgent > rest || strings.Index(d.Body, "ContainerStopped3") > rest || strings.Index(d.Body, "kev-feed") < rest {
		t.Fatalf("pages must lead the digest, tickets follow:\n%s", d.Body)
	}

	// The rest of the day: more pages, a resolution. No second mail.
	c.t = time.Date(2026, 10, 20, 15, 0, 0, 0, chicago)
	_ = m.PipelineSeen()
	_ = page(m, promoted("firing", "MemoryKill"))
	_ = page(m, promoted("resolved", "ContainerStopped3"))
	_ = m.Tick()
	if len(f.sent) != 1 {
		t.Fatalf("%d mails in one day, want 1", len(f.sent))
	}

	// Next day's digest carries both.
	c.t = time.Date(2026, 10, 21, 9, 0, 0, 0, chicago)
	_ = m.PipelineSeen()
	_ = m.Tick()
	if len(f.sent) != 2 {
		t.Fatalf("%d mails after the second digest hour, want 2", len(f.sent))
	}
	d = f.sent[1]
	if d.Subject != "[digest] 11 open (10 urgent), 1 resolved" || !strings.Contains(d.Body, "MemoryKill") ||
		!strings.Contains(d.Body, "- ContainerStopped3") {
		t.Fatalf("second digest %q\n%s", d.Subject, d.Body)
	}
}

// An alert named in mailer.immediate is mailed at once whatever delivery
// says (an intrusion, an indicator of compromise); its neighbours in the
// same notification still wait for the digest.
func TestDigestDeliveryImmediateListMailsAtOnce(t *testing.T) {
	m, f, c := newDigestTest(t, 3, "IntrusionDetected")
	if err := page(m, promoted("firing", "IntrusionDetected"), promoted("firing", "ContainerStopped")); err != nil {
		t.Fatal(err)
	}
	if len(f.sent) != 1 || f.sent[0].Subject != "[page] IntrusionDetected: firing" {
		t.Fatalf("want one mail, [page] IntrusionDetected: firing; got %d: %+v", len(f.sent), f.sent)
	}
	if strings.Contains(f.sent[0].Body, "ContainerStopped") {
		t.Fatalf("the page mail carries an alert that waits for the digest:\n%s", f.sent[0].Body)
	}
	c.t = time.Date(2026, 10, 20, 9, 0, 0, 0, chicago)
	_ = m.PipelineSeen()
	_ = m.Tick()
	if len(f.sent) != 2 || f.sent[1].Subject != "[digest] 1 open (1 urgent)" || strings.Contains(f.sent[1].Body, "IntrusionDetected") {
		t.Fatalf("digest should hold ContainerStopped only; sent %d, last %q", len(f.sent), f.sent[len(f.sent)-1].Subject)
	}
	// The intrusion clears: mailed at once too.
	_ = page(m, promoted("resolved", "IntrusionDetected"))
	if len(f.sent) != 3 || f.sent[2].Subject != "[resolved] IntrusionDetected" {
		t.Fatalf("resolution of an immediate alert: sent %d, last %q", len(f.sent), f.sent[len(f.sent)-1].Subject)
	}
}

// Under delivery: digest a silent alert path is not a mail of its own: it
// is the first line of the digest, which goes out even with nothing else
// open, and its return is one resolved line.
func TestDigestDeliveryPipelineSilentIsADigestLine(t *testing.T) {
	m, f, c := newDigestTest(t, 1)
	c.t = c.t.Add(30 * time.Minute) // 02:30: no receipt for 30 minutes
	if err := m.Tick(); err != nil {
		t.Fatal(err)
	}
	if len(f.sent) != 0 {
		t.Fatalf("pipeline silent mailed on its own under delivery: digest (%q)", f.sent[0].Subject)
	}
	c.t = time.Date(2026, 10, 20, 9, 0, 0, 0, chicago)
	_ = m.Tick()
	if len(f.sent) != 1 || f.sent[0].Subject != "[digest] 0 open, alert pipeline silent" ||
		!strings.Contains(f.sent[0].Body, "no alert can reach you") {
		t.Fatalf("digest must say the alert path is silent; sent %d: %+v", len(f.sent), f.sent)
	}
	// Receipts return in the afternoon: no mail; tomorrow's digest says so once.
	c.t = time.Date(2026, 10, 20, 14, 0, 0, 0, chicago)
	if err := m.PipelineSeen(); err != nil {
		t.Fatal(err)
	}
	if len(f.sent) != 1 {
		t.Fatalf("pipeline back mailed on its own under delivery: digest")
	}
	c.t = time.Date(2026, 10, 21, 9, 0, 0, 0, chicago)
	_ = m.PipelineSeen()
	_ = m.Tick()
	if len(f.sent) != 2 || f.sent[1].Subject != "[digest] 0 open, 1 resolved" || !strings.Contains(f.sent[1].Body, "alert pipeline silent") {
		t.Fatalf("second digest should report the pipeline back; sent %d, last %q", len(f.sent), f.sent[len(f.sent)-1].Subject)
	}
	c.t = time.Date(2026, 10, 22, 9, 0, 0, 0, chicago)
	_ = m.PipelineSeen()
	_ = m.Tick()
	if len(f.sent) != 2 {
		t.Fatalf("a quiet day sent a digest: %q", f.sent[len(f.sent)-1].Subject)
	}
}

// /v1/status says which delivery is in force.
func TestStatusShowsDelivery(t *testing.T) {
	m, _, _ := newDigestTest(t, 1)
	if got := m.Status(false).Delivery; got != "digest" {
		t.Fatalf("status delivery %q, want digest", got)
	}
	im, _, _, _ := newTest(t, "send", 3)
	if got := im.Status(false).Delivery; got != "immediate" {
		t.Fatalf("status delivery %q, want immediate when unset", got)
	}
}
