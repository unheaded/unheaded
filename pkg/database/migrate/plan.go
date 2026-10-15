// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package migrate

import (
	"fmt"
	"sort"
	"time"
)

// Record is one ledger row.
type Record struct {
	DB        string
	File      string
	SHA256    string
	Mode      string // initdb | apply | baseline
	AppliedAt time.Time
	AppliedBy string
}

// Ledger is every database's records, by database then file name.
type Ledger map[string]map[string]Record

// State of one manifest entry against the ledger.
type State string

const (
	Pending  State = "pending"
	Applied  State = "applied"
	Mismatch State = "mismatch" // recorded with another sha256: the file was edited after it ran
)

// Step is a manifest entry with its state.
type Step struct {
	File
	State  State
	Record *Record
}

// Plan is the manifest checked against the ledger.
type Plan struct {
	Steps []Step
	// Orphans are ledger rows no manifest entry explains.
	Orphans []Record
	// Gaps are pending entries that come before an applied entry in the
	// same database: someone ran a later migration first.
	Gaps []Step
}

// Pending returns the steps still to run, in manifest order.
func (p Plan) Pending() []Step {
	var out []Step
	for _, s := range p.Steps {
		if s.State == Pending {
			out = append(out, s)
		}
	}
	return out
}

// Mismatches returns the applied files whose content changed since.
func (p Plan) Mismatches() []Step {
	var out []Step
	for _, s := range p.Steps {
		if s.State == Mismatch {
			out = append(out, s)
		}
	}
	return out
}

// Drift is nil when the ledger and the manifest agree on everything that
// has run: no edited files, no orphans, no gaps. Apply refuses otherwise.
func (p Plan) Drift() error {
	if m := p.Mismatches(); len(m) > 0 {
		return fmt.Errorf("%s on %s changed after it was applied (sha256 %.12s, ledger %.12s): write a new migration instead",
			m[0].File.File, m[0].DB, m[0].SHA256, m[0].Record.SHA256)
	}
	if len(p.Orphans) > 0 {
		return fmt.Errorf("ledger of %s records %s, which the manifest does not name", p.Orphans[0].DB, p.Orphans[0].File)
	}
	if len(p.Gaps) > 0 {
		return fmt.Errorf("%s on %s is pending but a later migration on %s is applied", p.Gaps[0].File.File, p.Gaps[0].DB, p.Gaps[0].DB)
	}
	return nil
}

// Compute checks the set against the ledger.
func Compute(set Set, led Ledger) Plan {
	var p Plan
	named := map[[2]string]bool{}
	lastApplied := map[string]int{} // db -> index of its last applied step
	for i, f := range set.Files {
		named[[2]string{f.DB, f.File}] = true
		st := Step{File: f, State: Pending}
		if r, ok := led[f.DB][f.File]; ok {
			st.Record = &r
			st.State = Applied
			if r.SHA256 != f.SHA256 {
				st.State = Mismatch
			}
			lastApplied[f.DB] = i
		}
		p.Steps = append(p.Steps, st)
	}
	for i, st := range p.Steps {
		if last, ok := lastApplied[st.DB]; ok && st.State == Pending && i < last {
			p.Gaps = append(p.Gaps, st)
		}
	}
	for db, rows := range led {
		for file, r := range rows {
			if !named[[2]string{db, file}] {
				p.Orphans = append(p.Orphans, r)
			}
		}
	}
	sort.Slice(p.Orphans, func(i, j int) bool {
		if p.Orphans[i].DB != p.Orphans[j].DB {
			return p.Orphans[i].DB < p.Orphans[j].DB
		}
		return p.Orphans[i].File < p.Orphans[j].File
	})
	return p
}

// Through returns the plan cut after the last manifest entry for file, so
// baseline can record exactly what an older Well already has (say,
// through 012) and leave the rest for apply.
func (p Plan) Through(file string) (Plan, error) {
	last := -1
	for i, s := range p.Steps {
		if s.File.File == file {
			last = i
		}
	}
	if last < 0 {
		return p, fmt.Errorf("%s is not in the manifest", file)
	}
	q := p
	q.Steps = p.Steps[:last+1]
	return q, nil
}

// ForDB returns the plan limited to one database, so a Well whose
// databases are in different states (one built by hand, one empty) can be
// baselined or migrated one database at a time. Drift is still judged on
// the whole plan by the caller first.
func (p Plan) ForDB(db string) (Plan, error) {
	q := Plan{}
	for _, s := range p.Steps {
		if s.DB == db {
			q.Steps = append(q.Steps, s)
		}
	}
	if len(q.Steps) == 0 {
		return p, fmt.Errorf("the manifest applies nothing to %s", db)
	}
	return q, nil
}
