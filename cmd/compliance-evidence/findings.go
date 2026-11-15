// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"unheaded/pkg/compliance/crosswalk"
)

// The findings register (ADR-098): `compliance-evidence findings` joins the
// catalog, the evidence file and compliance/findings/register.yaml. It
// collects nothing; run the collector first for fresh evidence. The report is
// collected data, so it goes to stdout and optionally a file on the host
// (var/ or outside the repository), never into a tracked doc.
// `findings -enforce-ready <host>` prints, from the same open list, only what
// stands between that host and enforcing mode.

const registerPath = "compliance/findings/register.yaml"

func runFindings(args []string, stdout io.Writer) error {
	fl := flag.NewFlagSet("findings", flag.ContinueOnError)
	repoDir := fl.String("repo-dir", ".", "repository root")
	evidence := fl.String("evidence", "var/compliance/evidence.json", "evidence file")
	outFlag := fl.String("out", "", "also write the report here: under var/ or outside the repository")
	readyHost := fl.String("enforce-ready", "", "instead of the report, list this `host`'s open findings in the classes enforcing acts on, then \"ready: yes\" or \"ready: no (N)\" (ADR-098 condition 3)")
	if err := fl.Parse(args); err != nil {
		return err
	}
	if fl.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q", fl.Arg(0))
	}
	readyAsked := false
	fl.Visit(func(f *flag.Flag) { readyAsked = readyAsked || f.Name == "enforce-ready" })
	if readyAsked {
		hosts, err := knownHosts(*repoDir)
		if err != nil {
			return err
		}
		if !slices.Contains(hosts, *readyHost) {
			return fmt.Errorf("-enforce-ready: no host %q in %s (known: %s)", *readyHost, hostsDir, strings.Join(hosts, ", "))
		}
	}
	var outPath string
	if *outFlag != "" {
		p, err := hostOutPath(*repoDir, *outFlag)
		if err != nil {
			return err
		}
		outPath = p
	}
	cat, err := crosswalk.LoadFS(os.DirFS(*repoDir), "compliance/catalog/frameworks", "compliance/catalog/controls.yaml")
	if err != nil {
		return err
	}
	raw, err := os.ReadFile(filepath.Join(*repoDir, registerPath))
	if err != nil {
		return err
	}
	reg, err := crosswalk.LoadRegister(raw, cat)
	if err != nil {
		return err
	}
	path := *evidence
	if !filepath.IsAbs(path) {
		path = filepath.Join(*repoDir, path)
	}
	var recs []crosswalk.Record
	if f, err := os.Open(path); err == nil { // #nosec G304 -- operator-chosen evidence file from a flag of this local CLI
		recs, err = crosswalk.ReadEvidence(f)
		err = errors.Join(err, f.Close())
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}

	rep := crosswalk.Findings(cat, recs, reg, time.Now().UTC())
	out := renderFindings(rep)
	if readyAsked {
		out = renderEnforceReady(rep, *readyHost)
	}
	if _, err := io.WriteString(stdout, out); err != nil {
		return err
	}
	if outPath == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o750); err != nil {
		return err
	}
	return writeFileAtomic(outPath, []byte(out))
}

// hostsDir holds one binding file per host (ADR-098 mode, ADR-099 profile).
const hostsDir = "compliance/hosts"

// knownHosts lists the hosts that have a binding file. The name asked for
// is compared with this list and never joined into a path.
func knownHosts(repoDir string) ([]string, error) {
	ents, err := os.ReadDir(filepath.Join(repoDir, hostsDir))
	if err != nil {
		return nil, err
	}
	var hosts []string
	for _, e := range ents {
		if name, ok := strings.CutSuffix(e.Name(), ".yaml"); ok && !e.IsDir() && name != "" {
			hosts = append(hosts, name)
		}
	}
	sort.Strings(hosts)
	return hosts, nil
}

// renderEnforceReady answers ADR-098 condition 3 for one host from the same
// open list the report prints: could this host be flipped to enforcing?
// It lists the host's open findings in the classes enforcing acts on
// (crosswalk.IsEnforceableClass). Three kinds of row count that a stricter
// reading might leave out, each because leaving it out could answer "yes"
// with a finding open:
//
//   - an accepted row: an acceptance never removes a finding from the count;
//   - an untriaged row on the host: it has no class yet, so it cannot be
//     shown to be outside the table;
//   - a row in an enforceable class that names no host: it may be this one.
//
// The last line is "ready: yes" or "ready: no (N)". Not ready is an answer,
// not an error.
func renderEnforceReady(rep *crosswalk.FindingsReport, host string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "As of %s: open findings on %s in the classes enforcing acts on (ADR-098 condition 3).\n\n",
		rep.At.UTC().Format(time.RFC3339), host)
	counts := map[string]int{}
	var order []string
	n := 0
	for _, f := range rep.Open {
		class, sev, plan := "untriaged", "UNTRIAGED", "add an entry to "+registerPath
		switch e := f.Entry; {
		case e == nil:
			if f.Host != host {
				continue
			}
		case !crosswalk.IsEnforceableClass(e.Class) || (f.Host != host && f.Host != ""):
			continue
		default:
			class, sev, plan = e.Class, string(f.Severity), e.Plan
			if f.Accepted {
				plan = "accepted until " + e.Accepted.Until + " (still counted); " + plan
			}
			if f.Host == "" {
				plan = "no host named, so it counts for every host; " + plan
			}
		}
		if n == 0 {
			b.WriteString("| Class | Severity | Finding | Observed | Plan |\n|---|---|---|---|---|\n")
		}
		n++
		if counts[class] == 0 {
			order = append(order, class)
		}
		counts[class]++
		fmt.Fprintf(&b, "| %s | %s | `%s` | %s | %s |\n", cell(class), sev, cell(f.Key), cell(f.Detail), cell(plan))
	}
	if n == 0 {
		b.WriteString("None.\n\nready: yes\n")
		return b.String()
	}
	sort.Strings(order)
	parts := make([]string, 0, len(order))
	for _, c := range order {
		parts = append(parts, fmt.Sprintf("%s %d", c, counts[c]))
	}
	fmt.Fprintf(&b, "\nBy class: %s.\n\nready: no (%d)\n", strings.Join(parts, ", "), n)
	return b.String()
}

func cell(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.ReplaceAll(s, "|", `\|`)
}

func renderFindings(rep *crosswalk.FindingsReport) string {
	var b strings.Builder
	claim := "no"
	if rep.ZeroClaimable {
		claim = "yes"
	}
	fmt.Fprintf(&b, "As of %s: **%d open**", rep.At.UTC().Format(time.RFC3339), len(rep.Open))
	var parts []string
	for _, s := range []crosswalk.Severity{crosswalk.SeverityUntriaged, crosswalk.SeverityCritical,
		crosswalk.SeverityHigh, crosswalk.SeverityMedium, crosswalk.SeverityLow} {
		if n := rep.Counts[s]; n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, s))
		}
	}
	if len(parts) > 0 {
		fmt.Fprintf(&b, " (%s)", strings.Join(parts, ", "))
	}
	fmt.Fprintf(&b, ", %d accepted, %d overdue; zero claimable: **%s**", rep.Accepted, rep.Overdue, claim)
	if !rep.ZeroClaimable {
		fmt.Fprintf(&b, " (%s)", rep.ZeroReason)
	}
	b.WriteString(".\n\n")

	if len(rep.Open) > 0 {
		b.WriteString("| Severity | Finding | Host | Class | Step | Lockout | Due | Observed | Plan |\n")
		b.WriteString("|---|---|---|---|---|---|---|---|---|\n")
		for _, f := range rep.Open {
			sev, class, step, lock, due, plan := "UNTRIAGED", "", "", "", "", "add an entry to "+registerPath
			if e := f.Entry; e != nil {
				sev, class, step, lock, plan = string(f.Severity), e.Class, fmt.Sprint(e.Step), e.Lockout, e.Plan
				due = f.Due.Format(time.DateOnly)
				if f.Overdue {
					due += " **overdue**"
				}
				if f.Accepted {
					plan = "accepted until " + e.Accepted.Until + "; " + plan
					if f.AcceptanceExpired {
						plan = "**acceptance expired** " + plan
					}
				}
				if e.Decision != "" {
					plan += " **decision:** " + e.Decision
				}
			}
			fmt.Fprintf(&b, "| %s | `%s` | %s | %s | %s | %s | %s | %s | %s |\n",
				sev, cell(f.Key), cell(f.Host), cell(class), step, cell(lock), due, cell(f.Detail), cell(plan))
		}
		b.WriteString("\n")
	}
	list := func(title string, keys []string) {
		if len(keys) == 0 {
			return
		}
		b.WriteString(title + "\n\n")
		for _, k := range keys {
			fmt.Fprintf(&b, "- `%s`\n", cell(k))
		}
		b.WriteString("\n")
	}
	for _, k := range rep.Resolved {
		fmt.Fprintf(&b, "- remove from the register: `%s` (its evidence passes, or it was closed with evidence)\n", cell(k))
	}
	if len(rep.Resolved) > 0 {
		b.WriteString("\n")
	}
	list("Never observed (NOT_ASSESSED, not a pass):", rep.Unobserved)
	list("Stale (latest pass older than its freshness window):", rep.Stale)
	return b.String()
}
