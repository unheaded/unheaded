// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

// Snooze (Stevie, 2026-11-22): any SLO or promotion in the policy may carry
//
//	snooze_until: "12-01-2026"      # MM-DD-YYYY, through the end of that day
//	snooze_reason: why it is quiet  # required with a date
//
// and its alerts are muted in Alertmanager until then: they still fire and
// show in Prometheus and in Alertmanager (as muted), but reach no receiver,
// so nothing of them is mailed or digested. The day ends in mailer.timezone.
// Absent: no snooze. A past date is inactive and may stay. At most
// maxSnooze ahead, so a forgotten line ends by itself within a quarter.
//
// The rendered interval depends only on the date, never on today, so
// --check gives the same answer every day.

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	snoozeLayout = "01-02-2006" // MM-DD-YYYY
	maxSnooze    = 90           // days ahead of today
)

var snoozeRE = regexp.MustCompile(`^(0[1-9]|1[0-2])-(0[1-9]|[12][0-9]|3[01])-[0-9]{4}$`)

// today is the validation clock; tests replace it.
var today = time.Now

type amTimeInterval struct {
	Name          string        `yaml:"name"`
	TimeIntervals []amTimeRange `yaml:"time_intervals"`
}

// amTimeRange is one Alertmanager time interval: its fields AND, the
// list's entries OR.
type amTimeRange struct {
	Location    string   `yaml:"location,omitempty"`
	Years       []string `yaml:"years,omitempty"`
	Months      []string `yaml:"months,omitempty"`
	DaysOfMonth []string `yaml:"days_of_month,omitempty"`
}

// validateSnooze reports a bad date, a date past maxSnooze, and a date or a
// reason without the other.
func (p *Policy) validateSnooze(where string, s Snooze, bad func(string, ...any)) {
	if s.SnoozeUntil == "" {
		if strings.TrimSpace(s.SnoozeReason) != "" {
			bad("%s: snooze_reason without snooze_until", where)
		}
		return
	}
	d, err := time.Parse(snoozeLayout, s.SnoozeUntil)
	if !snoozeRE.MatchString(s.SnoozeUntil) || err != nil {
		bad("%s: snooze_until %q: want a date MM-DD-YYYY", where, s.SnoozeUntil)
		return
	}
	if strings.TrimSpace(s.SnoozeReason) == "" {
		bad("%s: snooze_until %s needs a snooze_reason", where, s.SnoozeUntil)
	}
	loc, err := time.LoadLocation(p.Mailer.Timezone)
	if err != nil {
		return // mailer.timezone is reported on its own
	}
	t := today().In(loc)
	last := time.Date(t.Year(), t.Month(), t.Day()+maxSnooze, 0, 0, 0, 0, time.UTC)
	if d.After(last) {
		bad("%s: snooze_until %s: at most %d days ahead (%s)", where, s.SnoozeUntil, maxSnooze, last.Format(snoozeLayout))
	}
}

// snoozeRoute returns r muted by the item's interval and that interval, or r
// unchanged and none when the item has no snooze.
func snoozeRoute(r amRoute, item string, s Snooze, zone string) (amRoute, []amTimeInterval) {
	if s.SnoozeUntil == "" {
		return r, nil
	}
	d, err := time.Parse(snoozeLayout, s.SnoozeUntil)
	if err != nil {
		return r, nil // validate refuses it first
	}
	name := "snooze-" + item
	r.MuteTimeIntervals = []string{name}
	return r, []amTimeInterval{{Name: name, TimeIntervals: snoozeRanges(d, zone)}}
}

// snoozeRanges covers every moment up to the end of day d in zone: the whole
// year before (a snooze is at most 90 days out, so it starts no earlier), the
// months of d's year before d's month, and d's month up to d.
func snoozeRanges(d time.Time, zone string) []amTimeRange {
	y := strconv.Itoa(d.Year())
	out := []amTimeRange{{Location: zone, Years: []string{strconv.Itoa(d.Year() - 1)}}}
	if m := int(d.Month()); m > 1 {
		out = append(out, amTimeRange{Location: zone, Years: []string{y}, Months: []string{span(1, m-1)}})
	}
	return append(out, amTimeRange{Location: zone, Years: []string{y}, Months: []string{strconv.Itoa(int(d.Month()))}, DaysOfMonth: []string{span(1, d.Day())}})
}

func span(a, b int) string {
	if a == b {
		return strconv.Itoa(a)
	}
	return fmt.Sprintf("%d:%d", a, b)
}
