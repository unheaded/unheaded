// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package collect

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"unheaded/pkg/compliance/crosswalk"
	"unheaded/pkg/compliance/remediation"
)

// HostJournal collects host-journal evidence (ADR-099): it reads each
// host's remediation journal over SSH and judges it against the fixes the
// host's profile turns on. The journal is read inside sudo with "@ok"
// printed from the same privileged command, so a sudo or ssh failure is a
// collection error, never a pass.
type HostJournal struct {
	// Wanted maps a host to the fixes its bound profile turns on (a host
	// with no binding is absent, which fails: nothing says what it should
	// have applied), and
	// SHA256 a fix to its current content hash (remediation.Fix.SHA256).
	Wanted map[string][]string
	SHA256 map[string]string

	Hostname string
	Now      func() time.Time
	Run      func(ctx context.Context, host, script string) (string, error)

	// Journals holds each host's parsed entries from the last Collect, for
	// the caller to ingest into The Well.
	Journals map[string][]remediation.JournalEntry
}

// JournalPath is where unheaded-remediate keeps the journal.
const JournalPath = "/var/lib/unheaded/remediate/journal.jsonl"

const journalScript = `sudo -n sh -c 'if [ -e ` + JournalPath + ` ]; then cat ` + JournalPath + `; else echo @none; fi; echo @ok'`

// JudgeJournal decides a host's journal evidence: the chain must verify
// from genesis, and each wanted fix's latest apply-or-undo must be a
// successful apply of its current content.
func JudgeJournal(out string, wanted []string, sums map[string]string) (bool, string, []remediation.JournalEntry) {
	body, ok := strings.CutSuffix(strings.TrimRight(out, "\n"), "@ok")
	if !ok {
		return false, "journal not read (sudo or the read failed)", nil
	}
	if strings.TrimSpace(body) == "@none" {
		if len(wanted) == 0 {
			return true, "no journal, and the profile turns no fix on", nil
		}
		return false, "no journal: no fix has been applied by unheaded-remediate (" + strings.Join(wanted, ", ") + " wanted)", nil
	}
	entries, err := remediation.ReadJournal(strings.NewReader(body))
	if err != nil {
		return false, "journal unreadable: " + err.Error(), nil
	}
	if err := remediation.VerifyChain(entries, remediation.GenesisHash); err != nil {
		return false, "journal chain broken (tampering or corruption; never repaired): " + err.Error(), entries
	}
	latest := map[string]remediation.JournalEntry{}
	for _, e := range entries {
		if e.Action == "plan" || (e.Action == "apply" && e.Outcome != "success") {
			continue // a refused or restored apply changed nothing
		}
		latest[e.FixID] = e
	}
	var missing, stale []string
	for _, id := range wanted {
		e, ok := latest[id]
		switch {
		case !ok || e.Action != "apply":
			missing = append(missing, id)
		case e.FixSHA256 != sums[id]:
			stale = append(stale, id)
		}
	}
	sort.Strings(missing)
	sort.Strings(stale)
	var problems []string
	if len(missing) > 0 {
		problems = append(problems, "not applied: "+strings.Join(missing, ", "))
	}
	if len(stale) > 0 {
		problems = append(problems, "changed since applied (re-apply): "+strings.Join(stale, ", "))
	}
	if len(problems) > 0 {
		return false, strings.Join(problems, "; "), entries
	}
	return true, fmt.Sprintf("%d entries, chain verifies; all %d wanted fixes applied at their current content", len(entries), len(wanted)), entries
}

// Collect returns one record per host-journal source that could be read.
func (h *HostJournal) Collect(ctx context.Context, sources []crosswalk.Source) ([]crosswalk.Record, error) {
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
	h.Journals = map[string][]remediation.JournalEntry{}
	var recs []crosswalk.Record
	var errs []error
	for _, s := range sources {
		if s.Kind != crosswalk.KindHostJournal {
			continue
		}
		host, _, _ := strings.Cut(s.Ref, ":")
		wanted, bound := h.Wanted[host]
		if !bound {
			// No binding read as "the profile turns no fix on", a pass: an
			// empty bindings table passed every host.
			recs = append(recs, crosswalk.Record{Source: s, Verdict: crosswalk.VerdictFail, ObservedAt: now().UTC(),
				Detail: "no profile is bound to " + host + ": what it should have applied is unknown"})
			continue
		}
		out, err := run(ctx, host, journalScript)
		if err != nil {
			errs = append(errs, err)
			continue // NOT_ASSESSED, not a pass
		}
		pass, detail, entries := JudgeJournal(out, wanted, h.SHA256)
		if entries != nil {
			h.Journals[host] = entries
		}
		v := crosswalk.VerdictFail
		if pass {
			v = crosswalk.VerdictPass
		}
		recs = append(recs, crosswalk.Record{Source: s, Verdict: v, ObservedAt: now().UTC(), Detail: detail})
	}
	// An unreachable host is reported and left NOT_ASSESSED; it does not
	// abort the run and blank the other hosts' evidence.
	for _, err := range errs {
		fmt.Fprintln(os.Stderr, "host-journal:", err)
	}
	return recs, nil
}
