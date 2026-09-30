// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package crosswalk

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

// The findings register (ADR-098). The count is generated from evidence; the
// register only adds triage. A finding closes when its evidence passes,
// never because its entry was edited.

// ErrInvalidRegister wraps every register rejection.
var ErrInvalidRegister = errors.New("invalid findings register")

// Severity of a finding. SeverityUntriaged is assigned, never written.
type Severity string

const (
	SeverityCritical  Severity = "critical"
	SeverityHigh      Severity = "high"
	SeverityMedium    Severity = "medium"
	SeverityLow       Severity = "low"
	SeverityUntriaged Severity = "untriaged"
)

// slaDays: days from opened to due, per severity (ADR-098 §1).
var slaDays = map[Severity]int{SeverityCritical: 7, SeverityHigh: 30, SeverityMedium: 90, SeverityLow: 180}

// sortRank puts untriaged first: it needs a decision before anything else.
var sortRank = map[Severity]int{SeverityUntriaged: 0, SeverityCritical: 1, SeverityHigh: 2, SeverityMedium: 3, SeverityLow: 4}

// Classes follow ADR-098 §2, where each has a fixed enforcing action.
var findingClasses = map[string]bool{
	"sysctl": true, "sshd": true, "firewall": true, "service": true,
	"kmod": true, "fileperm": true, "mount": true, "package": true,
	"app": true, "container": true, "ci": true, "backup": true, "process": true,
}

var lockoutRisks = map[string]bool{"none": true, "low": true, "high": true}

// maxAcceptance caps a risk acceptance: it must be re-decided, not left.
const maxAcceptance = 90 * 24 * time.Hour

var manualIDRe = regexp.MustCompile(`^FND-[0-9]{3}$`)

// Acceptance is a decision to live with a finding for a while. It never
// removes the finding from the count.
type Acceptance struct {
	Reason       string `yaml:"reason" json:"reason"`
	Compensating string `yaml:"compensating" json:"compensating"`
	AcceptedOn   string `yaml:"accepted_on" json:"accepted_on"`
	Until        string `yaml:"until" json:"until"`
	until        time.Time
}

// Resolution closes a hand-listed finding, which has no evidence source to
// pass. It names the evidence, and the signed commit that adds it is the record.
type Resolution struct {
	On       string `yaml:"on" json:"on"`
	Evidence string `yaml:"evidence" json:"evidence"`
}

// RegisterEntry is the triage for one finding: Source for one the tools
// see, ID/Title/Verify for one they cannot yet.
type RegisterEntry struct {
	Source   string      `yaml:"source,omitempty" json:"source,omitempty"`
	ID       string      `yaml:"id,omitempty" json:"id,omitempty"`
	Title    string      `yaml:"title,omitempty" json:"title,omitempty"`
	Verify   string      `yaml:"verify,omitempty" json:"verify,omitempty"`
	Host     string      `yaml:"host,omitempty" json:"host,omitempty"`
	Severity Severity    `yaml:"severity" json:"severity"`
	Class    string      `yaml:"class" json:"class"`
	Opened   string      `yaml:"opened" json:"opened"`
	Exposure string      `yaml:"exposure,omitempty" json:"exposure,omitempty"`
	Lockout  string      `yaml:"lockout" json:"lockout"`
	Step     int         `yaml:"step" json:"step"`
	Plan     string      `yaml:"plan,omitempty" json:"plan,omitempty"`
	Decision string      `yaml:"decision,omitempty" json:"decision,omitempty"`
	Accepted *Acceptance `yaml:"accepted,omitempty" json:"accepted,omitempty"`
	Resolved *Resolution `yaml:"resolved,omitempty" json:"resolved,omitempty"`
	opened   time.Time
}

// Key is the finding's stable ID: the source key or FND-NNN.
func (e *RegisterEntry) Key() string {
	if e.Source != "" {
		return e.Source
	}
	return e.ID
}

// Register is the loaded, validated triage file.
type Register struct {
	Entries  []*RegisterEntry
	bySource map[string]*RegisterEntry
}

func invalidRegister(format string, args ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{ErrInvalidRegister}, args...)...)
}

func parseDate(field, s string) (time.Time, error) {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("%s %q is not YYYY-MM-DD", field, s)
	}
	return t, nil
}

// LoadRegister parses the register and checks it against cat: every source
// entry must name a source the catalog still declares, so deleting a check
// also deletes its triage in the same (visible) commit.
func LoadRegister(b []byte, cat *Catalog) (*Register, error) {
	if cat == nil {
		return nil, invalidRegister("no catalog")
	}
	if len(b) > MaxCatalogBytes {
		return nil, invalidRegister("larger than %d bytes", MaxCatalogBytes)
	}
	var doc struct {
		Findings []*RegisterEntry `yaml:"findings"`
	}
	if err := decodeStrict(b, &doc); err != nil {
		return nil, invalidRegister("%v", err)
	}
	declared := make(map[string]bool)
	for _, c := range cat.Controls {
		for _, s := range c.Evidence {
			declared[s.Key()] = true
		}
	}
	r := &Register{bySource: make(map[string]*RegisterEntry)}
	seen := make(map[string]bool)
	for i, e := range doc.Findings {
		if e == nil {
			return nil, invalidRegister("entry %d is empty", i)
		}
		if err := validateEntry(e, declared); err != nil {
			return nil, invalidRegister("entry %d (%s): %v", i, e.Key(), err)
		}
		if seen[e.Key()] {
			return nil, invalidRegister("%s listed twice", e.Key())
		}
		seen[e.Key()] = true
		if e.Source != "" {
			r.bySource[e.Source] = e
		}
		r.Entries = append(r.Entries, e)
	}
	return r, nil
}

func validateEntry(e *RegisterEntry, declared map[string]bool) error {
	switch {
	case e.Source != "" && e.ID != "":
		return errors.New("has both source and id")
	case e.Source != "":
		if !declared[e.Source] {
			return errors.New("source is not declared by any catalog control")
		}
		if e.Resolved != nil {
			return errors.New("a finding with an evidence source closes when that evidence passes, not by a resolved entry")
		}
	case e.ID != "":
		if !manualIDRe.MatchString(e.ID) {
			return errors.New("id must be FND-NNN")
		}
		if e.Title == "" || e.Verify == "" {
			return errors.New("a hand-listed finding needs a title and how it will be verified")
		}
	default:
		return errors.New("needs a source or an id")
	}
	if _, ok := slaDays[e.Severity]; !ok {
		return fmt.Errorf("severity %q is not one of critical, high, medium, low", e.Severity)
	}
	if !findingClasses[e.Class] {
		return fmt.Errorf("class %q is unknown", e.Class)
	}
	if !lockoutRisks[e.Lockout] {
		return fmt.Errorf("lockout %q is not none, low or high", e.Lockout)
	}
	if e.Step < 1 {
		return errors.New("step (remediation order, ADR-098 §4) must be >= 1")
	}
	var err error
	if e.opened, err = parseDate("opened", e.Opened); err != nil {
		return err
	}
	if a := e.Accepted; a != nil {
		if a.Reason == "" || a.Compensating == "" {
			return errors.New("an acceptance needs a reason and a compensating control")
		}
		on, err := parseDate("accepted_on", a.AcceptedOn)
		if err != nil {
			return err
		}
		if a.until, err = parseDate("until", a.Until); err != nil {
			return err
		}
		if !a.until.After(on) || a.until.Sub(on) > maxAcceptance {
			return errors.New("acceptance must end after it starts and last at most 90 days")
		}
	}
	if res := e.Resolved; res != nil {
		if res.Evidence == "" {
			return errors.New("resolved needs the evidence that closed it")
		}
		if _, err := parseDate("resolved on", res.On); err != nil {
			return err
		}
	}
	return nil
}

// Finding is one open finding in a report.
type Finding struct {
	Key               string         `json:"key"`
	Controls          []string       `json:"controls,omitempty"`
	Host              string         `json:"host,omitempty"`
	Detail            string         `json:"detail,omitempty"`
	ObservedAt        time.Time      `json:"observed_at,omitzero"`
	Severity          Severity       `json:"severity"`
	Entry             *RegisterEntry `json:"entry,omitempty"`
	Due               time.Time      `json:"due,omitzero"`
	Overdue           bool           `json:"overdue"`
	Accepted          bool           `json:"accepted"`
	AcceptanceExpired bool           `json:"acceptance_expired"`
}

// FindingsReport is the register as of At.
type FindingsReport struct {
	At         time.Time        `json:"at"`
	Open       []Finding        `json:"open"`
	Counts     map[Severity]int `json:"counts"`
	Accepted   int              `json:"accepted"`
	Overdue    int              `json:"overdue"`
	Resolved   []string         `json:"resolved"`   // entries to remove (source passes) or closed by evidence
	Unobserved []string         `json:"unobserved"` // declared sources with no record: NOT_ASSESSED, not a pass
	Stale      []string         `json:"stale"`      // latest pass older than the strictest window using it
	// ZeroClaimable holds only with nothing open, unobserved or stale.
	ZeroClaimable bool   `json:"zero_claimable"`
	ZeroReason    string `json:"zero_reason"`
}

// hostOf names the host a host-* source checks; "local" is the collector's
// own host, "" means the source is not host-scoped.
func hostOf(s Source) string {
	switch s.Kind {
	case KindHostSysctl, KindHostSSHD, KindHostProbe:
		if h, _, ok := strings.Cut(s.Ref, ":"); ok {
			return h
		}
		return "local"
	}
	return ""
}

// Findings joins catalog, evidence and register. reg may be nil (every
// finding untriaged). Per source the latest record at or before now counts,
// as in ControlStatus; records for sources the catalog no longer declares
// are ignored.
func Findings(cat *Catalog, recs []Record, reg *Register, now time.Time) *FindingsReport {
	var order []Source
	controls := make(map[string][]string)
	window := make(map[string]time.Duration)
	for _, c := range cat.Controls {
		w := time.Duration(c.FreshnessDays) * 24 * time.Hour
		for _, s := range c.Evidence {
			k := s.Key()
			if _, ok := controls[k]; !ok {
				order = append(order, s)
				window[k] = w
			}
			controls[k] = append(controls[k], c.ID)
			window[k] = min(window[k], w)
		}
	}
	latest := make(map[string]Record)
	for _, r := range recs {
		k := r.Source.Key()
		if _, ok := controls[k]; !ok || r.ObservedAt.After(now) {
			continue
		}
		if r.Verdict != VerdictPass && r.Verdict != VerdictFail {
			continue
		}
		if cur, ok := latest[k]; !ok || r.ObservedAt.After(cur.ObservedAt) {
			latest[k] = r
		}
	}

	rep := &FindingsReport{At: now, Counts: make(map[Severity]int), Resolved: []string{}, Unobserved: []string{}, Stale: []string{}}
	open := make(map[string]bool)
	for _, s := range order {
		k := s.Key()
		r, ok := latest[k]
		switch {
		case !ok:
			rep.Unobserved = append(rep.Unobserved, k)
		case r.Verdict == VerdictFail:
			ctls := append([]string(nil), controls[k]...)
			sort.Strings(ctls)
			f := Finding{Key: k, Controls: ctls, Host: hostOf(s), Detail: r.Detail, ObservedAt: r.ObservedAt}
			var e *RegisterEntry
			if reg != nil {
				e = reg.bySource[k]
			}
			rep.Open = append(rep.Open, triage(f, e, now))
			open[k] = true
		case now.Sub(r.ObservedAt) > window[k]:
			rep.Stale = append(rep.Stale, k)
		}
	}
	if reg != nil {
		for _, e := range reg.Entries {
			switch {
			case e.Source != "":
				if !open[e.Source] {
					if _, observed := latest[e.Source]; observed {
						rep.Resolved = append(rep.Resolved, e.Source)
					}
				}
			case e.Resolved != nil:
				rep.Resolved = append(rep.Resolved, e.ID)
			default:
				rep.Open = append(rep.Open, triage(Finding{Key: e.ID, Host: e.Host, Detail: e.Title}, e, now))
			}
		}
	}
	sort.SliceStable(rep.Open, func(i, j int) bool {
		a, b := rep.Open[i], rep.Open[j]
		if sortRank[a.Severity] != sortRank[b.Severity] {
			return sortRank[a.Severity] < sortRank[b.Severity]
		}
		if as, bs := step(a), step(b); as != bs {
			return as < bs
		}
		return a.Key < b.Key
	})
	for _, f := range rep.Open {
		rep.Counts[f.Severity]++
		if f.Accepted {
			rep.Accepted++
		}
		if f.Overdue {
			rep.Overdue++
		}
	}
	var why []string
	if n := len(rep.Open); n > 0 {
		why = append(why, fmt.Sprintf("%d open", n))
	}
	if n := len(rep.Unobserved); n > 0 {
		why = append(why, fmt.Sprintf("%d never observed", n))
	}
	if n := len(rep.Stale); n > 0 {
		why = append(why, fmt.Sprintf("%d stale", n))
	}
	rep.ZeroClaimable = len(why) == 0
	rep.ZeroReason = strings.Join(why, ", ")
	return rep
}

func step(f Finding) int {
	if f.Entry == nil {
		return 0
	}
	return f.Entry.Step
}

func triage(f Finding, e *RegisterEntry, now time.Time) Finding {
	if e == nil {
		f.Severity = SeverityUntriaged
		return f
	}
	f.Entry, f.Severity = e, e.Severity
	f.Due = e.opened.AddDate(0, 0, slaDays[e.Severity])
	f.Overdue = now.After(f.Due)
	if e.Accepted != nil {
		f.Accepted = true
		// An acceptance holds through its last day.
		f.AcceptanceExpired = !now.Before(e.Accepted.until.AddDate(0, 0, 1))
	}
	if f.Host == "" {
		f.Host = e.Host
	}
	return f
}
