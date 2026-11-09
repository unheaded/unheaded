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
	"strings"
	"time"

	"unheaded/pkg/compliance/crosswalk"
)

// The findings register (ADR-098): `compliance-evidence findings` joins the
// catalog, the evidence file and compliance/findings/register.yaml. It
// collects nothing; run the collector first for fresh evidence. The report is
// collected data, so it goes to stdout and optionally a file on the host
// (var/ or outside the repository), never into a tracked doc.

const registerPath = "compliance/findings/register.yaml"

func runFindings(args []string, stdout io.Writer) error {
	fl := flag.NewFlagSet("findings", flag.ContinueOnError)
	repoDir := fl.String("repo-dir", ".", "repository root")
	evidence := fl.String("evidence", "var/compliance/evidence.json", "evidence file")
	outFlag := fl.String("out", "", "also write the report here: under var/ or outside the repository")
	if err := fl.Parse(args); err != nil {
		return err
	}
	if fl.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q", fl.Arg(0))
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

	out := renderFindings(crosswalk.Findings(cat, recs, reg, time.Now().UTC()))
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
