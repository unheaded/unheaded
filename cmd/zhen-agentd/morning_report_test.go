// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"unheaded/pkg/claudebridge"
)

type capturePub struct {
	topic string
	msg   []byte
}

func (c *capturePub) PublishJSON(topic string, v any) {
	c.topic = topic
	c.msg, _ = json.Marshal(v)
}

func sampleNight() nightReport {
	start := time.Date(2026, 10, 24, 1, 0, 0, 0, time.Local)
	return nightReport{
		Window: "01:00-07:00", Start: start, End: start.Add(95 * time.Minute), Stop: stopStopFile, StopDetail: "/s/zhen-agentd.stop",
		Tried: 3, Succeeded: 1, Failed: 1, Cancelled: 1, Deferred: 2, Deferrals: map[string]int{deferGPU: 2},
		Escalations: map[string]int{"escalated_check_fail": 1}, LocalTries: 4, SpentNano: 16_656_000, CapNano: 3_000_000_000,
		Results: []cardResult{
			{Outcome: outcomeSucceeded, Card: "card-a", Run: "0123456789abcdef", Branch: "zhen/card-a",
				Route:      claudebridge.RouteOutcome{Route: "local_first", RouteReason: "escalated_check_fail", Check: "pass", Escalated: true, Cost: 16_656_000},
				ReviewNote: "/s/zhen-reviews/card-a.md"},
			{Outcome: outcomeFailed, Card: "card-b", Run: "fedcba9876543210", Branch: "zhen/card-b", Reason: "check fail",
				Route: claudebridge.RouteOutcome{Route: "local_only", RouteReason: "local_only", Check: "fail"}},
			{Outcome: outcomeCancelled, Card: "card-c", Run: "00000000000000aa", Branch: "zhen/card-c", Reason: "stopped mid-card; nothing committed"},
		},
	}
}

func TestMorningReport_Content(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "reports")
	path, err := writeMorningReport(dir, sampleNight(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) != "2026-10-24.md" {
		t.Errorf("path = %s", path)
	}
	b, _ := os.ReadFile(path)
	s := string(b)
	for _, want := range []string{
		"stop_file", "01:00-07:00", "tried 3", "succeeded 1", "failed 1", "cancelled 1",
		"gpu_lease: 2", "escalated_check_fail: 1", "$0.016656", "$3.000000", "local attempts 4",
		"zhen/card-a", "zhen-claude review show 0123456789abcdef", "zhen-claude review accept 0123456789abcdef", "/s/zhen-reviews/card-a.md",
		"zhen/card-b", "check fail",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("report lacks %q:\n%s", want, s)
		}
	}
	for p, mode := range map[string]os.FileMode{dir: 0o700, path: 0o600} {
		if fi, err := os.Stat(p); err != nil || fi.Mode().Perm() != mode {
			t.Errorf("%s: %v %v", p, fi, err)
		}
	}
	// A second night on the same date does not overwrite the first.
	p2, err := writeMorningReport(dir, sampleNight(), nil)
	if err != nil || p2 == path {
		t.Errorf("second report %s (%v)", p2, err)
	}
}

func TestMorningReport_WotanCountsOnly(t *testing.T) {
	pub := &capturePub{}
	publishReportTo(pub, sampleNight())
	if pub.topic != "zhen.agent.report" {
		t.Errorf("topic %q", pub.topic)
	}
	if len(pub.msg) > claudebridge.MaxWotanPayloadBytes {
		t.Errorf("payload %d bytes", len(pub.msg))
	}
	s := string(pub.msg)
	for _, want := range []string{`"stop":"stop_file"`, `"tried":3`, `"succeeded":1`, `"spent_nano_usd":16656000`} {
		if !strings.Contains(s, want) {
			t.Errorf("summary lacks %s: %s", want, s)
		}
	}
	for _, leak := range []string{"card-a", "zhen/", "check fail", "/s/"} {
		if strings.Contains(s, leak) {
			t.Errorf("summary carries %q (counts only): %s", leak, s)
		}
	}
}
