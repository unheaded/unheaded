// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

// Package osv parses Open Source Vulnerability records and decides whether a
// component version falls inside an affected range (DP-721 stage 1a). Records
// are untrusted input, so each one is validated before use. This package reads
// local files only; there is no network code here.
package osv

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// MaxRecordBytes caps one record.
const MaxRecordBytes = 4 << 20

// maxRecords bounds a directory of records.
const maxRecords = 500000

// Ecosystem names as OSV spells them.
const (
	EcoGo     = "Go"
	EcoCrates = "crates.io"
	EcoPyPI   = "PyPI"
)

// ErrInvalid wraps every validation failure.
var ErrInvalid = errors.New("invalid OSV record")

var idPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

// Event is one point of a range; exactly one field is set.
type Event struct {
	Introduced   string `json:"introduced,omitempty"`
	Fixed        string `json:"fixed,omitempty"`
	LastAffected string `json:"last_affected,omitempty"`
}

// Range is an ordered set of events.
type Range struct {
	Type   string  `json:"type"`
	Events []Event `json:"events"`
}

// Package names what a record affects.
type Package struct {
	Name      string `json:"name"`
	Ecosystem string `json:"ecosystem"`
}

// Affected is one package's affected versions.
type Affected struct {
	Package  Package  `json:"package"`
	Ranges   []Range  `json:"ranges,omitempty"`
	Versions []string `json:"versions,omitempty"`
}

// Vuln is one OSV record.
type Vuln struct {
	ID        string     `json:"id"`
	Aliases   []string   `json:"aliases,omitempty"`
	Summary   string     `json:"summary,omitempty"`
	Withdrawn string     `json:"withdrawn,omitempty"`
	Affected  []Affected `json:"affected"`
}

// Parse decodes and validates one record.
func Parse(data []byte) (*Vuln, error) {
	if len(data) > MaxRecordBytes {
		return nil, fmt.Errorf("%w: %d bytes exceeds %d", ErrInvalid, len(data), MaxRecordBytes)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	var v Vuln
	if err := dec.Decode(&v); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if dec.More() {
		return nil, fmt.Errorf("%w: trailing data after the record", ErrInvalid)
	}
	if !idPattern.MatchString(v.ID) {
		return nil, fmt.Errorf("%w: bad id %q", ErrInvalid, v.ID)
	}
	if len(v.Affected) == 0 {
		return nil, fmt.Errorf("%w: %s has no affected packages", ErrInvalid, v.ID)
	}
	for _, a := range v.Affected {
		if a.Package.Name == "" || a.Package.Ecosystem == "" {
			return nil, fmt.Errorf("%w: %s has an affected entry without a package", ErrInvalid, v.ID)
		}
		for _, r := range a.Ranges {
			for _, e := range r.Events {
				n := 0
				for _, s := range []string{e.Introduced, e.Fixed, e.LastAffected} {
					if s != "" {
						n++
					}
				}
				if n != 1 {
					return nil, fmt.Errorf("%w: %s has an event with %d fields set", ErrInvalid, v.ID, n)
				}
			}
		}
	}
	return &v, nil
}

// LoadDir parses every *.json in dir, sorted by name. A duplicate id is an
// error, and so is an empty directory: a scan with no data must not read clean.
func LoadDir(dir string) ([]Vuln, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("%w: no *.json records in %s", ErrInvalid, dir)
	}
	if len(paths) > maxRecords {
		return nil, fmt.Errorf("%w: %d records exceeds %d", ErrInvalid, len(paths), maxRecords)
	}
	sort.Strings(paths)
	seen := make(map[string]bool, len(paths))
	out := make([]Vuln, 0, len(paths))
	for _, p := range paths {
		b, err := os.ReadFile(p) // #nosec G304 -- operator-supplied record directory
		if err != nil {
			return nil, err
		}
		v, err := Parse(b)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", filepath.Base(p), err)
		}
		if seen[v.ID] {
			return nil, fmt.Errorf("%w: duplicate id %s", ErrInvalid, v.ID)
		}
		seen[v.ID] = true
		out = append(out, *v)
	}
	return out, nil
}

// NormalizeName folds a package name the way its ecosystem compares names.
func NormalizeName(eco, name string) string {
	if eco != EcoPyPI {
		return name
	}
	return strings.ToLower(regexp.MustCompile(`[-_.]+`).ReplaceAllString(name, "-"))
}

// Matches reports whether eco/name at version is affected by this entry.
func (a Affected) Matches(eco, name, version string) (bool, error) {
	if a.Package.Ecosystem != eco || NormalizeName(eco, a.Package.Name) != NormalizeName(eco, name) {
		return false, nil
	}
	want := strings.TrimPrefix(version, "v")
	for _, v := range a.Versions {
		if strings.TrimPrefix(v, "v") == want {
			return true, nil
		}
	}
	for _, r := range a.Ranges {
		if r.Type != "SEMVER" && r.Type != "ECOSYSTEM" {
			continue
		}
		hit, err := inRange(eco, version, r.Events)
		if err != nil {
			return false, err
		}
		if hit {
			return true, nil
		}
	}
	return false, nil
}

func eventVersion(e Event) string {
	switch {
	case e.Introduced != "":
		return e.Introduced
	case e.Fixed != "":
		return e.Fixed
	}
	return e.LastAffected
}

// inRange walks the events in version order: at or past an introduced event
// the version is affected, at or past a fixed event it is not, and past a
// last_affected event it is not. "0" sorts first.
func inRange(eco, version string, events []Event) (bool, error) {
	sorted := append([]Event(nil), events...)
	var sortErr error
	sort.SliceStable(sorted, func(i, j int) bool {
		a, b := eventVersion(sorted[i]), eventVersion(sorted[j])
		if a == "0" || b == "0" {
			return a == "0" && b != "0"
		}
		c, err := Compare(eco, a, b)
		if err != nil {
			sortErr = err
		}
		return c < 0
	})
	if sortErr != nil {
		return false, sortErr
	}
	affected := false
	for _, e := range sorted {
		ev := eventVersion(e)
		if e.Introduced == "0" {
			affected = true
			continue
		}
		c, err := Compare(eco, version, ev)
		if err != nil {
			return false, err
		}
		switch {
		case e.Introduced != "" && c >= 0:
			affected = true
		case e.Fixed != "" && c >= 0:
			affected = false
		case e.LastAffected != "" && c > 0:
			affected = false
		}
	}
	return affected, nil
}

// FirstFixed returns the lowest fixed version in the entry, or "".
func (a Affected) FirstFixed(eco string) string {
	best := ""
	for _, r := range a.Ranges {
		for _, e := range r.Events {
			if e.Fixed == "" {
				continue
			}
			if best == "" {
				best = e.Fixed
				continue
			}
			if c, err := Compare(eco, e.Fixed, best); err == nil && c < 0 {
				best = e.Fixed
			}
		}
	}
	return best
}
