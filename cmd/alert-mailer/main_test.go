// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const basePolicy = `mailer:
  mode: send
  max_per_day: 6
  max_per_hour: 3
  digest_at: "09:00"
  timezone: America/Chicago
%RUN%
page:
  repeat_interval: 12h
`

func writePolicy(t *testing.T, runDays string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "policy.yaml")
	if err := os.WriteFile(p, []byte(strings.Replace(basePolicy, "%RUN%", runDays, 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// ADR-147 decision 2 (SS2): mailer.run_days raises the caps on listed dates.
func TestLoadSettingsRunDays(t *testing.T) {
	for _, tc := range []struct {
		name, block, wantErr string
		dates                int
	}{
		{"absent", "", "", 0},
		{"loaded", "  run_days:\n    max_per_day: 24\n    max_per_hour: 6\n    dates: [\"2026-12-01\"]", "", 1},
		{"bad date", "  run_days:\n    max_per_day: 24\n    max_per_hour: 6\n    dates: [\"2026-13-01\"]", "mailer.run_days.dates", 0},
		{"below normal", "  run_days:\n    max_per_day: 3\n    max_per_hour: 3\n    dates: []", "at least mailer.max_per_day", 0},
		{"over 50", "  run_days:\n    max_per_day: 51\n    max_per_hour: 6\n    dates: []", "1..50", 0},
		{"hour over day", "  run_days:\n    max_per_day: 24\n    max_per_hour: 30\n    dates: []", "1..run_days.max_per_day", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, err := loadSettings(writePolicy(t, tc.block))
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err %v, want one naming %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(s.RunDates) != tc.dates {
				t.Fatalf("RunDates %v, want %d", s.RunDates, tc.dates)
			}
			if tc.dates == 0 && (s.RunDayMaxPerDay != s.MaxPerDay || s.RunDayMaxPerHour != s.MaxPerHour) {
				t.Fatalf("no block: run caps %d/%d, want the normal %d/%d", s.RunDayMaxPerDay, s.RunDayMaxPerHour, s.MaxPerDay, s.MaxPerHour)
			}
			if tc.dates == 1 && (!s.RunDates["2026-12-01"] || s.RunDayMaxPerDay != 24 || s.RunDayMaxPerHour != 6) {
				t.Fatalf("loaded wrong: %+v", s)
			}
		})
	}
}
