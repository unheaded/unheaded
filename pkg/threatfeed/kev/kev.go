// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

// Package kev parses, validates, diffs and stores the CISA Known Exploited
// Vulnerabilities catalog (ADR-055). The feed is untrusted input: it arrives
// over the network from a CDN, so every field the Kingdom relies on is
// checked before a snapshot replaces the last good one.
package kev

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
)

// DefaultURL is CISA's published JSON feed.
const DefaultURL = "https://www.cisa.gov/sites/default/files/feeds/known_exploited_vulnerabilities.json"

// MaxCatalogBytes caps what Parse and the fetcher accept. The catalog was
// 1.4 MB with 1583 entries in April 2026; 32 MiB leaves an order of magnitude
// of growth and still refuses a CDN error page or a hostile stream.
const MaxCatalogBytes = 32 << 20

// maxEntries bounds the vulnerability list independently of byte size.
const maxEntries = 200000

var cveIDPattern = regexp.MustCompile(`^CVE-[0-9]{4}-[0-9]{4,7}$`)

// Vulnerability is one catalog entry. Only cveID is required; the rest is
// carried through as published.
type Vulnerability struct {
	CVEID                      string   `json:"cveID"`
	VendorProject              string   `json:"vendorProject"`
	Product                    string   `json:"product"`
	VulnerabilityName          string   `json:"vulnerabilityName"`
	DateAdded                  string   `json:"dateAdded"`
	ShortDescription           string   `json:"shortDescription"`
	RequiredAction             string   `json:"requiredAction"`
	DueDate                    string   `json:"dueDate"`
	KnownRansomwareCampaignUse string   `json:"knownRansomwareCampaignUse"`
	Notes                      string   `json:"notes"`
	CWEs                       []string `json:"cwes,omitempty"`
}

// Catalog is the whole feed.
type Catalog struct {
	Title           string          `json:"title"`
	CatalogVersion  string          `json:"catalogVersion"`
	DateReleased    string          `json:"dateReleased"`
	Count           int             `json:"count"`
	Vulnerabilities []Vulnerability `json:"vulnerabilities"`
}

// ErrInvalid wraps every validation failure, so callers can tell a bad feed
// (keep the last good snapshot, count a failure) from an I/O error.
var ErrInvalid = errors.New("invalid KEV catalog")

// Parse decodes and validates a catalog. It rejects: anything over
// MaxCatalogBytes, unknown trailing data, a missing catalogVersion, a count
// that disagrees with the list, an empty list, a malformed or duplicate
// cveID.
func Parse(data []byte) (*Catalog, error) {
	if len(data) > MaxCatalogBytes {
		return nil, fmt.Errorf("%w: %d bytes exceeds %d", ErrInvalid, len(data), MaxCatalogBytes)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	var c Catalog
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if dec.More() {
		return nil, fmt.Errorf("%w: trailing data after the catalog object", ErrInvalid)
	}
	if c.CatalogVersion == "" {
		return nil, fmt.Errorf("%w: catalogVersion is empty", ErrInvalid)
	}
	n := len(c.Vulnerabilities)
	if n == 0 {
		return nil, fmt.Errorf("%w: no vulnerabilities", ErrInvalid)
	}
	if n > maxEntries {
		return nil, fmt.Errorf("%w: %d entries exceeds %d", ErrInvalid, n, maxEntries)
	}
	if c.Count != n {
		return nil, fmt.Errorf("%w: count %d but %d entries", ErrInvalid, c.Count, n)
	}
	seen := make(map[string]struct{}, n)
	for i, v := range c.Vulnerabilities {
		if !cveIDPattern.MatchString(v.CVEID) {
			return nil, fmt.Errorf("%w: entry %d: malformed cveID %q", ErrInvalid, i, truncate(v.CVEID, 40))
		}
		if _, dup := seen[v.CVEID]; dup {
			return nil, fmt.Errorf("%w: entry %d: duplicate cveID %s", ErrInvalid, i, v.CVEID)
		}
		seen[v.CVEID] = struct{}{}
	}
	return &c, nil
}

// Delta is what changed between two catalogs, by cveID.
type Delta struct {
	Added   []Vulnerability `json:"added"`
	Removed []string        `json:"removed"`
}

// Empty reports whether nothing was added or removed.
func (d Delta) Empty() bool { return len(d.Added) == 0 && len(d.Removed) == 0 }

// Diff returns the entries in cur that are not in prev, and the cveIDs in
// prev that are no longer in cur, each sorted by cveID. A nil prev is a
// bootstrap and yields an empty delta: the first pull is a baseline, not
// 1500 new exploited CVEs.
func Diff(prev, cur *Catalog) Delta {
	var d Delta
	if prev == nil || cur == nil {
		return d
	}
	old := make(map[string]struct{}, len(prev.Vulnerabilities))
	for _, v := range prev.Vulnerabilities {
		old[v.CVEID] = struct{}{}
	}
	now := make(map[string]struct{}, len(cur.Vulnerabilities))
	for _, v := range cur.Vulnerabilities {
		now[v.CVEID] = struct{}{}
		if _, ok := old[v.CVEID]; !ok {
			d.Added = append(d.Added, v)
		}
	}
	for id := range old {
		if _, ok := now[id]; !ok {
			d.Removed = append(d.Removed, id)
		}
	}
	sort.Slice(d.Added, func(i, j int) bool { return d.Added[i].CVEID < d.Added[j].CVEID })
	sort.Strings(d.Removed)
	return d
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
