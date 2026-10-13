// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package collect

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"unheaded/pkg/compliance/crosswalk"
)

// HostAgent collects host-agent evidence (ADR-098 phase 2c): it reads each
// host's unheaded-baseline report over SSH and checks it against the
// central pull. Root on a host can always defeat an agent on that host;
// what this guarantees is that it cannot do so undetected, because the
// agent's push and this independent pull must agree.
type HostAgent struct {
	// Central holds the central collectors' records from the same run.
	Central []crosswalk.Record
	// BaselineSHA256 is the sha256 of the repo's compliance/baseline/baseline.yaml.
	BaselineSHA256 string
	// MaxAge is how old a report may be; the agent's timer runs hourly.
	MaxAge time.Duration

	Hostname string
	Now      func() time.Time
	Run      func(ctx context.Context, host, script string) (string, error)
}

// AgentReportPath is where unheaded-baseline writes its report.
const AgentReportPath = "/var/lib/unheaded/baseline/report.json"

const agentScript = `sudo -n sh -c 'if [ -e ` + AgentReportPath + ` ]; then head -c 4194304 ` + AgentReportPath + `; else echo @none; fi; echo @ok'`

// agentReport is the part of unheaded-baseline's report.json this reads.
type agentReport struct {
	Host           string    `json:"host"`
	ModeConfig     string    `json:"mode_config"`
	ModeEffective  string    `json:"mode_effective"`
	ConfigError    string    `json:"config_error"`
	ObservedAt     time.Time `json:"observed_at"`
	BaselineSHA256 string    `json:"baseline_sha256"`
	Checks         []struct {
		Key     string `json:"key"`
		Verdict string `json:"verdict"`
	} `json:"checks"`
}

// JudgeAgentReport decides a host's host-agent evidence. central holds that
// host's central records; only checks both sides evaluated are compared.
func JudgeAgentReport(out, host string, central []crosswalk.Record, baselineSHA string, now time.Time, maxAge time.Duration) (bool, string) {
	body, ok := strings.CutSuffix(strings.TrimRight(out, "\n"), "@ok")
	if !ok {
		return false, "agent report not read (sudo or the read failed)"
	}
	if strings.TrimSpace(body) == "@none" {
		return false, "no agent report: unheaded-baseline has not run on this host"
	}
	var r agentReport
	if err := json.Unmarshal([]byte(body), &r); err != nil {
		return false, "agent report unreadable: " + err.Error()
	}
	if r.Host != host {
		return false, fmt.Sprintf("agent report is for host %q", r.Host)
	}
	if age := now.Sub(r.ObservedAt); age > maxAge || age < -5*time.Minute {
		return false, fmt.Sprintf("agent silent: last report %s (%s ago)", r.ObservedAt.UTC().Format(time.RFC3339), age.Round(time.Minute))
	}
	if r.BaselineSHA256 != baselineSHA {
		return false, fmt.Sprintf("agent ran baseline %.12s, the repo's is %.12s", r.BaselineSHA256, baselineSHA)
	}
	if r.ConfigError != "" || r.ModeConfig != r.ModeEffective || r.ModeEffective != "audit" {
		return false, fmt.Sprintf("agent mode config=%s effective=%s %s", r.ModeConfig, r.ModeEffective, r.ConfigError)
	}
	latest := map[string]crosswalk.Record{}
	for _, c := range central {
		k := c.Source.Key()
		if prev, ok := latest[k]; !ok || c.ObservedAt.After(prev.ObservedAt) {
			latest[k] = c
		}
	}
	var differ []string
	compared := 0
	for _, c := range r.Checks {
		if c.Verdict != "pass" && c.Verdict != "fail" {
			continue
		}
		cr, ok := latest[c.Key]
		if !ok || (cr.Verdict != crosswalk.VerdictPass && cr.Verdict != crosswalk.VerdictFail) {
			continue
		}
		compared++
		if string(cr.Verdict) != c.Verdict {
			differ = append(differ, fmt.Sprintf("%s (agent %s, central %s)", c.Key, c.Verdict, cr.Verdict))
		}
	}
	if len(differ) > 0 {
		sort.Strings(differ)
		return false, "agent and central pull disagree: " + firstN(differ, 3)
	}
	if compared == 0 {
		return false, "no check evaluated by both the agent and the central pull"
	}
	return true, fmt.Sprintf("report %s old, repo baseline, audit mode; %d checks agree with the central pull",
		now.Sub(r.ObservedAt).Round(time.Minute), compared)
}

// Collect returns one record per host-agent source that could be read.
func (h *HostAgent) Collect(ctx context.Context, sources []crosswalk.Source) ([]crosswalk.Record, error) {
	now := time.Now
	if h.Now != nil {
		now = h.Now
	}
	local := h.Hostname
	if local == "" {
		local, _ = os.Hostname()
	}
	run := h.Run
	if run == nil {
		run = func(ctx context.Context, host, script string) (string, error) {
			return RunScript(ctx, host, script, host == local)
		}
	}
	maxAge := h.MaxAge
	if maxAge <= 0 {
		maxAge = 3 * time.Hour
	}
	var recs []crosswalk.Record
	for _, s := range sources {
		if s.Kind != crosswalk.KindHostAgent {
			continue
		}
		host, _, _ := strings.Cut(s.Ref, ":")
		out, err := run(ctx, host, agentScript)
		if err != nil {
			// Unreachable: NOT_ASSESSED, not a pass, and the run goes on.
			fmt.Fprintln(os.Stderr, "host-agent:", err)
			continue
		}
		var central []crosswalk.Record
		for _, c := range h.Central {
			if crosswalk.HostOf(c.Source) == host {
				central = append(central, c)
			}
		}
		pass, detail := JudgeAgentReport(out, host, central, h.BaselineSHA256, now().UTC(), maxAge)
		v := crosswalk.VerdictFail
		if pass {
			v = crosswalk.VerdictPass
		}
		recs = append(recs, crosswalk.Record{Source: s, Verdict: v, ObservedAt: now().UTC(), Detail: detail})
	}
	return recs, nil
}
