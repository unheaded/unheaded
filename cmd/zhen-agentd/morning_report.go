// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// The morning report (S-TWO-BRAINS Step 243): one Markdown file per night
// in $STATE/zhen-reports/<date>.md (0600, outside the repo), and a
// counts-only line on Wotan zhen.agent.report.

// TopicAgentReport carries one summary per night.
const TopicAgentReport = "zhen.agent.report"

func writeMorningReport(dir string, rep nightReport, w *worker) (string, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	if err := os.Chmod(dir, 0o700); err != nil { // #nosec G302 -- a directory: 0700 is owner-only (x is needed to enter it)
		return "", err
	}
	name := rep.Start.Format("2006-01-02")
	path := filepath.Join(dir, name+".md")
	if _, err := os.Lstat(path); err == nil {
		path = filepath.Join(dir, name+"-"+rep.Start.Format("150405")+".md")
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# Zhen night %s\n\n", name)
	fmt.Fprintf(&b, "- Window %s; ran %s to %s\n", rep.Window, rep.Start.Format("15:04"), rep.End.Format("15:04"))
	fmt.Fprintf(&b, "- Stopped: **%s**", rep.Stop)
	if rep.StopDetail != "" {
		fmt.Fprintf(&b, " (%s)", oneLine(rep.StopDetail, 300))
	}
	fmt.Fprintf(&b, "\n- Cards: tried %d, succeeded %d, failed %d, cancelled %d; deferred %d\n",
		rep.Tried, rep.Succeeded, rep.Failed, rep.Cancelled, rep.Deferred)
	if len(rep.Deferrals) > 0 {
		fmt.Fprintf(&b, "- Deferrals: %s\n", countsLine(rep.Deferrals))
	}
	fmt.Fprintf(&b, "- Claude: %s of the night's %s cap; escalations: %s\n", rep.SpentNano.USD(), rep.CapNano.USD(), countsOrNone(rep.Escalations))
	fmt.Fprintf(&b, "- Local model: local attempts %d (token counts per request are in unheaded_ops.llm_requests)\n", rep.LocalTries)
	if len(rep.Errors) > 0 {
		fmt.Fprintf(&b, "- Errors: %d (last: %s)\n", len(rep.Errors), oneLine(rep.Errors[len(rep.Errors)-1], 300))
	}

	b.WriteString("\n## Runs\n\n| Card | Branch | Outcome | Route | Check | Claude | Note |\n|---|---|---|---|---|---|---|\n")
	if len(rep.Results) == 0 {
		b.WriteString("| (none) | | | | | | |\n")
	}
	for _, r := range rep.Results {
		outcome := r.Outcome
		if r.Reason != "" {
			outcome += ": " + oneLine(r.Reason, 120)
		}
		fmt.Fprintf(&b, "| %s | `%s` | %s | %s | %s | %s | %s |\n", cell(r.Card), r.Branch, cell(outcome),
			cell(r.Route.RouteReason), cell(r.Route.Check), r.Route.Cost.USD(), cell(r.ReviewNote))
	}

	b.WriteString("\n## Review\n\n```bash\nzhen-claude review list\n")
	for _, r := range rep.Results {
		if r.Outcome != outcomeSucceeded || r.Run == "" {
			continue
		}
		fmt.Fprintf(&b, "zhen-claude review show %s      # %s\n", r.Run, r.Branch)
		base := "develop"
		if w != nil {
			base = w.cfg.Base
		}
		fmt.Fprintf(&b, "git log --stat %s..%s && git merge --no-ff %s\n", base, r.Branch, r.Branch)
		fmt.Fprintf(&b, "zhen-claude review accept %s   # after the merge; or: review reject %s -reason '...'\n", r.Run, r.Run)
	}
	b.WriteString("```\n")
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) // #nosec G304 -- operator's report dir, name from the date
	if err != nil {
		return "", err
	}
	_, werr := f.WriteString(b.String())
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	return path, werr
}

func cell(s string) string { return strings.ReplaceAll(oneLine(s, 200), "|", "/") }

func countsLine(m map[string]int) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s: %d", k, m[k]))
	}
	return strings.Join(parts, ", ")
}

func countsOrNone(m map[string]int) string {
	if len(m) == 0 {
		return "none"
	}
	return countsLine(m)
}

// jsonPublisher is the part of a Wotan auditor the summary needs.
type jsonPublisher interface{ PublishJSON(topic string, v any) }

func publishReport(aud *agentdAudit, rep nightReport) {
	if aud == nil || aud.wotan == nil {
		return
	}
	publishReportTo(aud.wotan, rep)
}

// publishReportTo sends the counts-only summary (no card, branch or path).
func publishReportTo(p jsonPublisher, rep nightReport) {
	if p == nil {
		return
	}
	p.PublishJSON(TopicAgentReport, map[string]any{
		"ts": time.Now().UTC().Format(time.RFC3339Nano), "source": "zhen-agentd", "window": rep.Window,
		"stop": string(rep.Stop), "tried": rep.Tried, "succeeded": rep.Succeeded, "failed": rep.Failed,
		"cancelled": rep.Cancelled, "deferred": rep.Deferred, "deferrals": rep.Deferrals, "escalations": rep.Escalations,
		"local_attempts": rep.LocalTries, "spent_nano_usd": int64(rep.SpentNano), "cap_nano_usd": int64(rep.CapNano),
		"minutes": int(rep.End.Sub(rep.Start).Minutes()),
	})
}
