// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis. All rights reserved.

package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"unheaded/pkg/health"
	"unheaded/pkg/metrics"
)

type sent struct {
	topic string
	body  []byte
}

func newTestRemediator(t *testing.T) (*remediator, *[]sent) {
	t.Helper()
	var out []sent
	r := newRemediator("west", func(topic string, v interface{}) {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, sent{topic, b})
	}, metrics.NewRegistry())
	r.now = func() time.Time { return time.UnixMilli(1_761_600_000_000) }
	return r, &out
}

func verdict(svc string, failing, reporters int) health.Verdict {
	rate := float64(failing) / float64(reporters)
	return health.Verdict{
		Service: svc, Reporters: reporters, Failing: failing, FailureRate: rate,
		Severity: health.SeverityFor(rate), Remediate: rate >= health.ConsensusThreshold,
	}
}

func TestRemediatePublishesOnCriticalEntry(t *testing.T) {
	r, out := newTestRemediator(t)
	r.onChange(verdict("timeguru", 1, 4)) // WARN: nothing
	if len(*out) != 0 {
		t.Fatalf("WARN published %v", *out)
	}
	r.onChange(verdict("timeguru", 5, 8)) // 62.5 %: CRITICAL entry
	if len(*out) != 1 {
		t.Fatalf("CRITICAL entry published %d messages, want 1", len(*out))
	}
	got := (*out)[0]
	if got.topic != topicRemediate || topicRemediate != "system.health.remediate" {
		t.Fatalf("topic %q", got.topic)
	}
	var s remediateSuggestion
	dec := json.NewDecoder(bytes.NewReader(got.body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&s); err != nil {
		t.Fatal(err)
	}
	want := remediateSuggestion{
		Service: "timeguru", Host: "west", Band: health.SeverityCritical, Pct: 62.5,
		Suggested: "systemctl restart unheaded-timeguru.service", At: 1_761_600_000_000,
	}
	if s != want {
		t.Fatalf("suggestion = %+v, want %+v", s, want)
	}
	for _, k := range []string{`"service"`, `"host"`, `"band"`, `"pct"`, `"suggested"`, `"at"`} {
		if !bytes.Contains(got.body, []byte(k)) {
			t.Errorf("payload lacks %s: %s", k, got.body)
		}
	}
}

func TestRemediateOncePerBandEntry(t *testing.T) {
	r, out := newTestRemediator(t)
	r.onChange(verdict("sophia", 3, 4)) // 75 % CRITICAL
	r.onChange(verdict("sophia", 3, 4)) // same band again: no repeat
	r.onChange(verdict("sophia", 2, 3)) // 66.7 % still CRITICAL: no repeat
	if len(*out) != 1 {
		t.Fatalf("published %d times inside one CRITICAL band, want 1", len(*out))
	}
	r.onChange(verdict("sophia", 4, 4)) // PANIC: a new band entry
	if len(*out) != 2 {
		t.Fatalf("PANIC entry: %d messages, want 2", len(*out))
	}
	r.onChange(verdict("sophia", 0, 4)) // recovered
	r.onChange(verdict("sophia", 3, 4)) // CRITICAL again: a fresh entry
	if len(*out) != 3 {
		t.Fatalf("re-entry after recovery: %d messages, want 3", len(*out))
	}
	// Another service's bands are its own.
	r.onChange(verdict("monad", 3, 4))
	if len(*out) != 4 {
		t.Fatalf("second service: %d messages, want 4", len(*out))
	}
}

func TestRemediateCountsSuggestions(t *testing.T) {
	reg := metrics.NewRegistry()
	r := newRemediator("east", func(string, interface{}) {}, reg)
	r.onChange(verdict("captain", 3, 4))
	r.onChange(verdict("captain", 4, 4))
	r.onChange(verdict("kanban", 3, 4))
	var b strings.Builder
	if err := reg.Gather(&b); err != nil {
		t.Fatal(err)
	}
	text := b.String()
	for _, line := range []string{
		`akira_remediate_suggested_total{service="captain"} 2`,
		`akira_remediate_suggested_total{service="kanban"} 1`,
	} {
		if !strings.Contains(text, line) {
			t.Errorf("metrics lack %q:\n%s", line, text)
		}
	}
}

func TestRemediateSuggestionUnitNameIsSafe(t *testing.T) {
	// The service name comes off the ballot, which other reporters fill
	// through Wotan. A name that is not a plain unit name gets no command.
	r, out := newTestRemediator(t)
	r.onChange(verdict("foo; rm -rf /", 4, 4))
	if len(*out) != 1 {
		t.Fatalf("published %d", len(*out))
	}
	var s remediateSuggestion
	if err := json.Unmarshal((*out)[0].body, &s); err != nil {
		t.Fatal(err)
	}
	if s.Suggested != "" {
		t.Fatalf("unsafe service name got a command: %q", s.Suggested)
	}
}
