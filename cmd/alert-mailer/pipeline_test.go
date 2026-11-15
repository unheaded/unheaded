// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"unheaded/pkg/metrics"
)

// pipelineTest is a send-mode mailer that expects a pipeline receipt every
// 20 minutes (ADR-116 R4, ADR-147 stage 1). Its clock starts at t0.
func pipelineTest(t *testing.T, dir string, max int, c *clock) (*Mailer, *fakeSender) {
	t.Helper()
	f := &fakeSender{}
	m, err := NewMailer(Settings{Mode: "send", MaxPerDay: max, Reminder: 12 * time.Hour, DigestH: 9, Loc: chicago,
		PipelineSilentAfter: 20 * time.Minute}, f, dir, c.now)
	if err != nil {
		t.Fatal(err)
	}
	return m, f
}

var t0 = time.Date(2026, 11, 20, 10, 0, 0, 0, chicago) // after the 09:00 digest

// tickTo runs Tick once a minute from the clock's time up to end.
func tickTo(t *testing.T, m *Mailer, c *clock, end time.Time) {
	t.Helper()
	for ; !c.t.After(end); c.t = c.t.Add(time.Minute) {
		if err := m.Tick(); err != nil {
			t.Fatal(err)
		}
	}
}

func subjects(f *fakeSender) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, s := range f.sent {
		out = append(out, s.Subject)
	}
	return out
}

func TestPipelineSilentMailsOnce(t *testing.T) {
	c := &clock{t0}
	m, f := pipelineTest(t, t.TempDir(), 6, c)
	c.t = t0.Add(time.Minute)
	m.PipelineSeen()
	var at time.Time
	for ; !c.t.After(t0.Add(60 * time.Minute)); c.t = c.t.Add(time.Minute) {
		n := len(subjects(f))
		if err := m.Tick(); err != nil {
			t.Fatal(err)
		}
		if len(subjects(f)) > n && at.IsZero() {
			at = c.t
		}
	}
	got := subjects(f)
	if len(got) != 1 || !strings.Contains(got[0], "alert pipeline silent") {
		t.Fatalf("want one silent mail, got %q", got)
	}
	if want := t0.Add(22 * time.Minute); !at.Equal(want) {
		t.Fatalf("silent mail at %s, want the first tick after t0+21m (%s)", at.Format("15:04"), want.Format("15:04"))
	}
	if !strings.Contains(f.sent[0].Body, "runbooks/observe/alert-pipeline-silent.yaml") {
		t.Fatalf("no runbook in the silent mail:\n%s", f.sent[0].Body)
	}
}

func TestPipelineBackMailsOnce(t *testing.T) {
	c := &clock{t0}
	m, f := pipelineTest(t, t.TempDir(), 6, c)
	c.t = t0.Add(time.Minute)
	m.PipelineSeen()
	tickTo(t, m, c, t0.Add(60*time.Minute))
	c.t = t0.Add(61 * time.Minute)
	m.PipelineSeen()
	c.t = c.t.Add(time.Minute)
	m.PipelineSeen()
	tickTo(t, m, c, t0.Add(75*time.Minute))
	got := subjects(f)
	if len(got) != 2 || !strings.Contains(got[1], "alert pipeline back") {
		t.Fatalf("want silent then back, got %q", got)
	}
}

func TestPipelineGraceAfterStart(t *testing.T) {
	c := &clock{t0}
	m, f := pipelineTest(t, t.TempDir(), 6, c)
	tickTo(t, m, c, t0.Add(20*time.Minute))
	if got := subjects(f); len(got) != 0 {
		t.Fatalf("mailed inside the grace after start: %q", got)
	}
	tickTo(t, m, c, t0.Add(21*time.Minute))
	if got := subjects(f); len(got) != 1 || !strings.Contains(got[0], "alert pipeline silent") {
		t.Fatalf("want the silent mail at the first tick past start+20m, got %q", got)
	}
}

func TestPipelineOffWhenZero(t *testing.T) {
	c := &clock{t0}
	f := &fakeSender{}
	m, err := NewMailer(Settings{Mode: "send", MaxPerDay: 6, Reminder: 12 * time.Hour, DigestH: 9, Loc: chicago}, f, t.TempDir(), c.now)
	if err != nil {
		t.Fatal(err)
	}
	tickTo(t, m, c, t0.Add(2*time.Hour))
	if got := subjects(f); len(got) != 0 {
		t.Fatalf("pipeline off, but mailed %q", got)
	}
}

func TestPipelineStatePersists(t *testing.T) {
	dir := t.TempDir()
	c := &clock{t0}
	m, f := pipelineTest(t, dir, 6, c)
	m.PipelineSeen()
	tickTo(t, m, c, t0.Add(30*time.Minute))
	if len(subjects(f)) != 1 {
		t.Fatalf("want the silent mail before the restart, got %q", subjects(f))
	}
	m2, f2 := pipelineTest(t, dir, 6, c) // restart while still silent
	tickTo(t, m2, c, c.t.Add(time.Hour))
	if got := subjects(f2); len(got) != 0 {
		t.Fatalf("a restart while silent mailed again: %q", got)
	}
	m2.PipelineSeen()
	if got := subjects(f2); len(got) != 1 || !strings.Contains(got[0], "alert pipeline back") {
		t.Fatalf("after the restart the back mail is still owed, got %q", got)
	}
}

func TestPipelineMailRespectsCap(t *testing.T) {
	c := &clock{t0}
	m, f := pipelineTest(t, t.TempDir(), 1, c)
	if err := page(m, alert("firing", "slo-a", "fast")); err != nil { // the day's one mail
		t.Fatal(err)
	}
	tickTo(t, m, c, t0.Add(30*time.Minute))
	if got := subjects(f); len(got) != 1 {
		t.Fatalf("the silent mail went past the cap: %q", got)
	}
	var outcome string
	for _, e := range m.Status(true).Outbox {
		if strings.Contains(e.Subject, "alert pipeline silent") {
			outcome = e.Outcome
		}
	}
	if outcome != "capped" {
		t.Fatalf("silent mail outcome %q, want capped", outcome)
	}
	if s := m.Status(false); s.HeldToday != 1 {
		t.Fatalf("held %d, want 1", s.HeldToday)
	}
}

func TestPipelineHandler(t *testing.T) {
	c := &clock{t0}
	m, _ := pipelineTest(t, t.TempDir(), 6, c)
	srv := httptest.NewServer(handler(m, metrics.NewRegistry()))
	defer srv.Close()
	c.t = t0.Add(5 * time.Minute)
	body := `{"version":"4","status":"firing","alerts":[{"status":"firing","labels":{"alertname":"AlertPipelineHeartbeat"}}]}`
	resp, err := http.Post(srv.URL+"/v1/alerts/pipeline", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		t.Fatalf("POST: %d", resp.StatusCode)
	}
	if got := m.PipelineLast(); !got.Equal(c.t) {
		t.Fatalf("receipt time %s, want %s", got, c.t)
	}
	resp, err = http.Get(srv.URL + "/v1/alerts/pipeline")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("GET: %d, want 405", resp.StatusCode)
	}
	big := fmt.Sprintf(`{"version":"4","alerts":[],"pad":%q}`, strings.Repeat("x", maxWebhookBytes))
	resp, err = http.Post(srv.URL+"/v1/alerts/pipeline", "application/json", strings.NewReader(big))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized body: %d, want 413", resp.StatusCode)
	}
}
