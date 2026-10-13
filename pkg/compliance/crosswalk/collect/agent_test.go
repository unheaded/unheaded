// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package collect

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"unheaded/pkg/compliance/crosswalk"
)

func agentOut(t *testing.T, mutate func(m map[string]any)) string {
	t.Helper()
	m := map[string]any{
		"version": 1, "host": "east", "mode_config": "audit", "mode_effective": "audit",
		"observed_at": "2026-10-13T11:30:00Z", "baseline_sha256": "abc123",
		"checks": []map[string]string{
			{"key": "host-probe:east:su-restricted", "verdict": "fail"},
			{"key": "host-sysctl:east:fs.suid_dumpable=0", "verdict": "pass"},
			{"key": "host-probe:east:aide-timer", "verdict": "unevaluated"},
		},
	}
	if mutate != nil {
		mutate(m)
	}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return string(b) + "\n@ok\n"
}

func centralRecs(suid crosswalk.Verdict) []crosswalk.Record {
	at := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	return []crosswalk.Record{
		{Source: crosswalk.Source{Kind: crosswalk.KindHostProbe, Ref: "east:su-restricted"}, Verdict: crosswalk.VerdictFail, ObservedAt: at},
		{Source: crosswalk.Source{Kind: crosswalk.KindHostSysctl, Ref: "east:fs.suid_dumpable=0"}, Verdict: crosswalk.VerdictPass, ObservedAt: at.Add(-time.Hour)},
		{Source: crosswalk.Source{Kind: crosswalk.KindHostSysctl, Ref: "east:fs.suid_dumpable=0"}, Verdict: suid, ObservedAt: at}, // latest wins
	}
}

func TestJudgeAgentReport(t *testing.T) {
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name, out string
		central   []crosswalk.Record
		pass      bool
		detail    string
	}{
		{"agree", agentOut(t, nil), centralRecs(crosswalk.VerdictPass), true, "2 checks agree"},
		{"disagree", agentOut(t, nil), centralRecs(crosswalk.VerdictFail), false, "host-sysctl:east:fs.suid_dumpable=0 (agent pass, central fail)"},
		{"no report", "@none\n@ok\n", centralRecs(crosswalk.VerdictPass), false, "has not run"},
		{"not read", "@none\n", centralRecs(crosswalk.VerdictPass), false, "not read"},
		{"garbage", "{not json\n@ok\n", centralRecs(crosswalk.VerdictPass), false, "unreadable"},
		{"other host", agentOut(t, func(m map[string]any) { m["host"] = "west" }), centralRecs(crosswalk.VerdictPass), false, `for host "west"`},
		{"stale", agentOut(t, func(m map[string]any) { m["observed_at"] = "2026-10-13T08:00:00Z" }), centralRecs(crosswalk.VerdictPass), false, "agent silent"},
		{"future", agentOut(t, func(m map[string]any) { m["observed_at"] = "2026-10-13T13:00:00Z" }), centralRecs(crosswalk.VerdictPass), false, "agent silent"},
		{"other baseline", agentOut(t, func(m map[string]any) { m["baseline_sha256"] = "def456" }), centralRecs(crosswalk.VerdictPass), false, "ran baseline def456"},
		{"enforcing asked", agentOut(t, func(m map[string]any) { m["mode_config"] = "enforcing" }), centralRecs(crosswalk.VerdictPass), false, "config=enforcing effective=audit"},
		{"nothing compared", agentOut(t, nil), nil, false, "no check evaluated by both"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pass, detail := JudgeAgentReport(tc.out, "east", tc.central, "abc123", now, 3*time.Hour)
			if pass != tc.pass || !strings.Contains(detail, tc.detail) {
				t.Fatalf("= %v %q, want %v containing %q", pass, detail, tc.pass, tc.detail)
			}
		})
	}
}

func TestHostAgentCollect(t *testing.T) {
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	west := centralRecs(crosswalk.VerdictPass)[0]
	west.Source.Ref = "west:su-restricted"
	west.Verdict = crosswalk.VerdictPass // west's record must not be compared with east's report
	h := &HostAgent{
		Central: append(centralRecs(crosswalk.VerdictPass), west), BaselineSHA256: "abc123",
		Hostname: "west", Now: func() time.Time { return now },
		Run: func(_ context.Context, host, script string) (string, error) {
			if !strings.Contains(script, AgentReportPath) || !strings.HasPrefix(script, "sudo -n ") {
				t.Errorf("script %q", script)
			}
			if host == "gone" {
				return "", errors.New("unreachable")
			}
			return agentOut(t, nil), nil
		},
	}
	src := func(r string) crosswalk.Source { return crosswalk.Source{Kind: crosswalk.KindHostAgent, Ref: r} }
	recs, err := h.Collect(context.Background(), []crosswalk.Source{
		src("east:baseline-report"), src("gone:baseline-report"),
		{Kind: crosswalk.KindHostProbe, Ref: "east:su-restricted"}, // other kinds ignored
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 1 || recs[0].Source.Ref != "east:baseline-report" || recs[0].Verdict != crosswalk.VerdictPass {
		t.Fatalf("records %+v (unreachable must leave no record, never a pass)", recs)
	}
}

func TestHostAgentRefValidated(t *testing.T) {
	ctl := func(ref string) string {
		return `controls:
  - id: UH-X-01
    title: t
    statement: s
    rationale: r
    freshness_days: 1
    evidence:
      - {kind: host-agent, ref: "` + ref + `"}
    mappings:
      fw: [A]
`
	}
	fw := []byte("frameworks:\n  - {id: fw, name: F, version: \"1\", source: s, requirements: [{id: A}]}\n")
	if _, err := crosswalk.Load(fw, []byte(ctl("east:baseline-report"))); err != nil {
		t.Fatalf("valid ref rejected: %v", err)
	}
	for _, bad := range []string{"east:report", "baseline-report", "east;rm:baseline-report", "East:baseline-report"} {
		if _, err := crosswalk.Load(fw, []byte(ctl(bad))); err == nil {
			t.Errorf("ref %q accepted", bad)
		}
	}
}
