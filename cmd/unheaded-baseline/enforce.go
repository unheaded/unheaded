// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

// Enforcing mode (ADR-098 phase 4, ADR-099 phase 4; S-WELL-SPRING Step 276).
//
// The code path exists; every host's mode file says audit, and the flip is
// Stevie's (E-E), after the soak (docs/compliance/enforcing-soak.md).
// Enforcing is in effect only when all of these hold, checked every run:
//
//   - the mode file says exactly mode=enforcing;
//   - the host's fix model loaded (a sealed snapshot: only the fixes of the
//     host's profile, ADR-099 "hosts act only on sealed snapshots");
//   - the soak: for 7 days of audit runs no deviation mapped to a fix
//     (would_enforce stayed 0), measured by this agent in soak.json;
//   - nothing in the state directory was changed behind the agent's back
//     (report.json is what it last wrote; soak.json agrees with it).
//
// Otherwise the run is audit and, when enforcing was asked for, a CRITICAL
// line says why. In effect, each deviation whose check a fix `closes` is
// fixed through the executor (pkg/compliance/executor: backed up, undoable,
// journaled in /var/lib/unheaded/remediate). A deviation no fix closes
// stays audit. A fix applied twice in the last 24 h is not applied a third
// time: the flapping breaker drops it to audit and raises a critical
// event, because something keeps undoing it and a loop of root changes is
// worse than the deviation.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"unheaded/pkg/compliance/executor"
	"unheaded/pkg/compliance/remediation"
	"unheaded/pkg/compliance/snapshot"
	"unheaded/pkg/gungnir"
)

const (
	soakPeriod     = 7 * 24 * time.Hour
	breakerWindow  = 24 * time.Hour
	breakerApplies = 2 // applies inside the window after which the next one trips
	soakFile       = "soak.json"
	reportFile     = "report.json"
)

// enforcer applies one fix and answers how often it was applied: the
// executor, journaled; a fake in tests.
type enforcer interface {
	Apply(ctx context.Context, fix remediation.Fix) (string, error)
	AppliesSince(fixID string, since time.Time) (int, error)
}

// soakState is the agent's memory between runs, in its state directory.
type soakState struct {
	// CleanSince is when the current run of audit runs with nothing to
	// enforce began; nil while something would be enforced.
	CleanSince        *time.Time `json:"clean_since,omitempty"`
	WouldEnforceTotal int64      `json:"would_enforce_total"`
	EnforcedTotal     int64      `json:"enforced_total"`
	// LastReportSHA256 is the report.json this agent last wrote.
	LastReportSHA256 string `json:"last_report_sha256,omitempty"`
}

// critical records a critical event: a CRITICAL log line (journald) and an
// entry in the report, which is published on compliance.deviation.<host>.
func (r *report) critical(logw io.Writer, format string, a ...any) {
	msg := fmt.Sprintf(format, a...)
	r.CriticalEvents = append(r.CriticalEvents, msg)
	logf(logw, "CRITICAL host=%s %s", r.Host, msg)
}

// loadSoak reads soak.json and checks it, and report.json, against what
// this agent last wrote. Any disagreement is a critical event and restarts
// the soak: state the agent cannot vouch for never counts toward enforcing.
func loadSoak(stateDir string, rep *report, logw io.Writer) soakState {
	var s soakState
	raw, err := readBounded(filepath.Join(stateDir, soakFile))
	if errors.Is(err, fs.ErrNotExist) {
		if _, rerr := os.Lstat(filepath.Join(stateDir, reportFile)); rerr == nil {
			rep.critical(logw, "event=state-tamper detail=%q", "report.json exists but soak.json is gone; soak restarted")
		}
		return s
	}
	if err == nil {
		err = json.Unmarshal(raw, &s)
	}
	if err != nil {
		rep.critical(logw, "event=state-tamper detail=%q", "soak.json unreadable: "+err.Error()+"; soak restarted")
		return soakState{}
	}
	prev, rerr := readBounded(filepath.Join(stateDir, reportFile))
	switch {
	case errors.Is(rerr, fs.ErrNotExist):
		if s.LastReportSHA256 != "" {
			rep.critical(logw, "event=state-tamper detail=%q", "report.json was removed; soak restarted")
			s.CleanSince = nil
		}
		return s
	case rerr != nil:
		rep.critical(logw, "event=state-tamper detail=%q", "report.json unreadable: "+rerr.Error()+"; soak restarted")
		s.CleanSince = nil
		return s
	}
	sum := sha256.Sum256(prev)
	if hex.EncodeToString(sum[:]) != s.LastReportSHA256 {
		rep.critical(logw, "event=state-tamper detail=%q", "report.json was changed outside the agent (forged or replayed); soak restarted")
		s.CleanSince = nil
		return s
	}
	var last struct {
		SoakCleanSince *time.Time `json:"soak_clean_since"`
	}
	if err := json.Unmarshal(prev, &last); err != nil || !sameTime(last.SoakCleanSince, s.CleanSince) {
		rep.critical(logw, "event=state-tamper detail=%q", "soak.json does not agree with the last report; soak restarted")
		s.CleanSince = nil
	}
	return s
}

func sameTime(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Equal(*b)
}

// fixesByKey maps each evidence-source key on host to the fix that closes
// it ("*" in a closes key stands for every host).
func fixesByKey(m *remediation.Model, host string) map[string]remediation.Fix {
	out := map[string]remediation.Fix{}
	for _, f := range m.Fixes {
		for _, key := range f.Closes {
			kind, rest, ok := strings.Cut(key, ":")
			h, ref, ok2 := strings.Cut(rest, ":")
			if !ok || !ok2 || (h != "*" && h != host) {
				continue
			}
			k := kind + ":" + host + ":" + ref
			if _, taken := out[k]; !taken {
				out[k] = f
			}
		}
	}
	return out
}

// loadSnapshotModel opens the host's sealed snapshot (trust-checked like the
// baseline) and returns the fixes of its profile.
func loadSnapshotModel(snapPath, pubPath, host string, trustedUID int) (*remediation.Model, error) {
	if snapPath == "" {
		return nil, errors.New("no fix snapshot configured (-snapshot)")
	}
	for _, p := range []string{snapPath, pubPath} {
		if err := checkTrust(p, trustedUID); err != nil {
			return nil, err
		}
	}
	pub, err := gungnir.LoadPublicKeyFile(pubPath)
	if err != nil {
		return nil, err
	}
	raw, err := readBounded(snapPath)
	if err != nil {
		return nil, err
	}
	snap, err := snapshot.Open(raw, pub, host)
	if err != nil {
		return nil, err
	}
	return snap.Model(), nil
}

// newExecutorEnforcer is the real enforcer: the executor, as root, with its
// journal in state. The unit runs the agent unprivileged today, so an apply
// is refused ("apply needs root") until the enforcing flip gives it root.
func newExecutorEnforcer(state string) func(context.Context, string, io.Writer) (enforcer, error) {
	return func(ctx context.Context, host string, logw io.Writer) (enforcer, error) {
		return executor.New(ctx, executor.Options{State: state, Host: host, Operator: "unheaded-baseline (enforcing)",
			Cmd: executor.CommandsFromEnv(), Out: logw})
	}
}

// decideMode returns enforcing only when every condition at the top of this
// file holds, else audit and the reason.
func decideMode(rep *report, s soakState, modelErr error, now time.Time) (string, string) {
	switch {
	case rep.ModeConfig != modeEnforcing || rep.ConfigError != "":
		return modeAudit, ""
	case modelErr != nil:
		return modeAudit, "no fix model: " + modelErr.Error()
	case len(rep.CriticalEvents) > 0:
		return modeAudit, "the state directory was tampered with this run"
	case s.CleanSince == nil:
		return modeAudit, "the 7-day soak has not started (something would be enforced, or the soak was restarted)"
	case now.Sub(*s.CleanSince) < soakPeriod:
		return modeAudit, fmt.Sprintf("the 7-day soak is at %s", now.Sub(*s.CleanSince).Truncate(time.Hour))
	}
	return modeEnforcing, ""
}

// enforce applies, once per fix, the fixes that close this run's deviations.
// It returns an error (the run fails, loudly) only when enforcing cannot
// start: no journal directory, or no executor.
func enforce(ctx context.Context, o options, rep *report, logw io.Writer, now time.Time) error {
	fi, err := os.Stat(o.remediateState)
	if err != nil || !fi.IsDir() {
		rep.critical(logw, "event=enforce-refused detail=%q", "no remediation journal directory "+o.remediateState)
		return fmt.Errorf("enforcing needs the remediation journal directory %s (creating it would start a new journal and hide a deleted one): %v", o.remediateState, err)
	}
	mk := o.newEnforcer
	if mk == nil {
		mk = newExecutorEnforcer(o.remediateState)
	}
	enf, err := mk(ctx, rep.Host, logw)
	if err != nil {
		rep.critical(logw, "event=enforce-refused detail=%q", err.Error())
		return err
	}
	done := map[string]string{} // fix id -> outcome, so a fix runs once per run
	for i := range rep.Checks {
		c := &rep.Checks[i]
		if c.Verdict != "fail" || c.fix == nil {
			continue
		}
		id := c.Fix
		if outcome, ok := done[id]; ok {
			c.Enforced = outcome
			continue
		}
		n, err := enf.AppliesSince(id, now.Add(-breakerWindow))
		switch {
		case err != nil:
			done[id] = "failed"
			rep.critical(logw, "event=enforce-failed fix=%s detail=%q", id, "journal: "+err.Error())
		case n >= breakerApplies:
			done[id] = "breaker open"
			rep.BreakerOpen = append(rep.BreakerOpen, id)
			rep.critical(logw, "event=breaker-open fix=%s detail=%q", id,
				fmt.Sprintf("applied %d times in 24 h and deviating again; audit-only for this fix until the window passes", n))
		default:
			dir, err := enf.Apply(ctx, *c.fix)
			if err != nil {
				done[id] = "failed"
				rep.critical(logw, "event=enforce-failed fix=%s detail=%q", id, err.Error())
				break
			}
			done[id] = "applied " + dir
			rep.Enforced++
			logf(logw, "ENFORCED host=%s key=%q fix=%s backup=%s", rep.Host, c.Key, id, dir)
		}
		c.Enforced = done[id]
	}
	for _, c := range rep.Checks {
		if c.Verdict == "fail" && !strings.HasPrefix(c.Enforced, "applied ") {
			rep.CriticalDeviations++
		}
	}
	sort.Strings(rep.BreakerOpen)
	return nil
}
