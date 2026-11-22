// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

// Snooze (Stevie, 2026-11-22): an SLO or a promotion may carry
// snooze_until (MM-DD-YYYY) and snooze_reason. Absent: no snooze.
func TestValidateSnooze(t *testing.T) {
	defer func(f func() time.Time) { today = f }(today)
	today = func() time.Time { return time.Date(2026, 11, 22, 16, 0, 0, 0, time.UTC) }
	for _, tc := range []struct {
		until, reason string
		ok            bool
	}{
		{"", "", true},
		{"12-01-2026", "waiting on the disk swap", true},
		{"02-20-2027", "90 days out", true},
		{"02-21-2027", "91 days out", false},
		{"11-01-2026", "a past date is inactive, not wrong", true},
		{"12-01-2026", "", false},  // a snooze says why
		{"", "no date", false},     // a reason with nothing to explain
		{"2026-12-01", "x", false}, // MM-DD-YYYY only
		{"13-01-2026", "x", false},
		{"02-30-2027", "x", false},
		{"12-1-2026", "x", false},
		{"12/01/2026", "x", false},
	} {
		for _, item := range []string{"slo", "promote"} {
			p, err := loadPolicy(filepath.Join("..", "..", policyPath))
			if err != nil {
				t.Fatal(err)
			}
			if item == "slo" {
				p.SLOs[0].SnoozeUntil, p.SLOs[0].SnoozeReason = tc.until, tc.reason
			} else {
				if len(p.Promote) == 0 {
					t.Fatal("the repository's policy has no promotions")
				}
				p.Promote[0].SnoozeUntil, p.Promote[0].SnoozeReason = tc.until, tc.reason
			}
			var hit bool
			for _, e := range p.validate() {
				hit = hit || strings.Contains(e.Error(), "snooze")
			}
			if hit == tc.ok {
				t.Errorf("%s snooze %q reason %q: refused=%v, want %v", item, tc.until, tc.reason, hit, !tc.ok)
			}
		}
	}
}

// The ranges cover every moment up to the end of the date in the zone; the
// year before is whole, since a snooze is at most 90 days out.
func TestSnoozeRanges(t *testing.T) {
	loc := "America/Chicago"
	for until, want := range map[string][]amTimeRange{
		"12-01-2026": {
			{Location: loc, Years: []string{"2025"}},
			{Location: loc, Years: []string{"2026"}, Months: []string{"1:11"}},
			{Location: loc, Years: []string{"2026"}, Months: []string{"12"}, DaysOfMonth: []string{"1"}},
		},
		"01-15-2027": {
			{Location: loc, Years: []string{"2026"}},
			{Location: loc, Years: []string{"2027"}, Months: []string{"1"}, DaysOfMonth: []string{"1:15"}},
		},
		"03-31-2027": {
			{Location: loc, Years: []string{"2026"}},
			{Location: loc, Years: []string{"2027"}, Months: []string{"1:2"}},
			{Location: loc, Years: []string{"2027"}, Months: []string{"3"}, DaysOfMonth: []string{"1:31"}},
		},
	} {
		d, err := time.Parse(snoozeLayout, until)
		if err != nil {
			t.Fatal(err)
		}
		if got := snoozeRanges(d, loc); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: %+v\nwant %+v", until, got, want)
		}
	}
}

func snoozedNames(f amFile) []string {
	var n []string
	for _, ti := range f.TimeIntervals {
		n = append(n, ti.Name)
	}
	return n
}

// A snoozed promotion's route is muted by its own interval.
func TestSnoozeMutesPromotion(t *testing.T) {
	f := renderAM(t, func(p *Policy) {
		p.Promote[0].SnoozeUntil, p.Promote[0].SnoozeReason = "12-01-2026", "test"
	})
	p, _ := loadPolicy(filepath.Join("..", "..", policyPath))
	name := "snooze-promote-" + p.Promote[0].Alert
	if !slices.Contains(snoozedNames(f), name) {
		t.Fatalf("time_intervals %v: no %s", snoozedNames(f), name)
	}
	found := false
	for _, r := range f.Route.Routes {
		if slices.Contains(r.Matchers, `alertname="`+p.Promote[0].Alert+`"`) {
			found = true
			if !reflect.DeepEqual(r.MuteTimeIntervals, []string{name}) {
				t.Errorf("route %+v: mute %v, want [%s]", r.Matchers, r.MuteTimeIntervals, name)
			}
		} else if len(r.MuteTimeIntervals) > 0 {
			t.Errorf("route %+v muted, only the snoozed one may be", r.Matchers)
		}
	}
	if !found {
		t.Fatal("no route for the snoozed promotion")
	}
}

// A snoozed SLO gets its own page and ticket routes, muted, ahead of the
// shared ones; the other SLOs still reach the shared routes.
func TestSnoozeMutesSLO(t *testing.T) {
	f := renderAM(t, func(p *Policy) {
		p.SLOs[0].SnoozeUntil, p.SLOs[0].SnoozeReason = "12-01-2026", "test"
	})
	p, _ := loadPolicy(filepath.Join("..", "..", policyPath))
	slo := p.SLOs[0].Name
	name := "snooze-slo-" + slo
	if !slices.Contains(snoozedNames(f), name) {
		t.Fatalf("time_intervals %v: no %s", snoozedNames(f), name)
	}
	var muted, sharedAt, firstMuted int = 0, -1, -1
	for i, r := range f.Route.Routes {
		if slices.Contains(r.Matchers, `slo="`+slo+`"`) {
			muted++
			if firstMuted < 0 {
				firstMuted = i
			}
			if !reflect.DeepEqual(r.MuteTimeIntervals, []string{name}) || (r.Receiver != "page" && r.Receiver != "ticket") {
				t.Errorf("route %d %+v: receiver %s mute %v", i, r.Matchers, r.Receiver, r.MuteTimeIntervals)
			}
		}
		if reflect.DeepEqual(r.Matchers, []string{`urgency="page"`}) && sharedAt < 0 {
			sharedAt = i
		}
	}
	if muted != 2 || firstMuted > sharedAt {
		t.Fatalf("%d muted routes for %s (first at %d), shared page route at %d: want 2, ahead of it", muted, slo, firstMuted, sharedAt)
	}
}

// No snooze: no intervals and no muted route, so the file is as before.
func TestNoSnoozeNoIntervals(t *testing.T) {
	f := renderAM(t, func(*Policy) {})
	if len(f.TimeIntervals) != 0 {
		t.Fatalf("time_intervals %v with no snooze", snoozedNames(f))
	}
}
