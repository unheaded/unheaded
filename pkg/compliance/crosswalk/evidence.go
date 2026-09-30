// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package crosswalk

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
)

// MaxEvidenceBytes bounds an evidence file read.
const MaxEvidenceBytes = 16 << 20

// ErrInvalidEvidence wraps every evidence file rejection.
var ErrInvalidEvidence = errors.New("invalid evidence file")

type evidenceFile struct {
	Version int      `json:"version"`
	Records []Record `json:"records"`
}

// ReadEvidence reads an evidence file written by WriteEvidence.
func ReadEvidence(r io.Reader) ([]Record, error) {
	b, err := io.ReadAll(io.LimitReader(r, MaxEvidenceBytes+1))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidEvidence, err)
	}
	if len(b) > MaxEvidenceBytes {
		return nil, fmt.Errorf("%w: larger than %d bytes", ErrInvalidEvidence, MaxEvidenceBytes)
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	var f evidenceFile
	if err := dec.Decode(&f); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidEvidence, err)
	}
	if f.Version != 1 {
		return nil, fmt.Errorf("%w: version %d", ErrInvalidEvidence, f.Version)
	}
	return f.Records, nil
}

// WriteEvidence writes records as a version-1 evidence file.
func WriteEvidence(w io.Writer, recs []Record) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(evidenceFile{Version: 1, Records: recs})
}

// MergeRecords combines old and new records, drops exact duplicates (same
// source, verdict, time and commit), and keeps the keep newest per source,
// newest first within a source.
func MergeRecords(old, fresh []Record, keep int) []Record {
	type key struct {
		src, verdict, commit string
		at                   int64
	}
	seen := make(map[key]bool)
	bySource := make(map[string][]Record)
	var order []string
	for _, r := range append(append([]Record(nil), fresh...), old...) {
		k := key{r.Source.Key(), string(r.Verdict), r.Commit, r.ObservedAt.UnixNano()}
		if seen[k] {
			continue
		}
		seen[k] = true
		sk := r.Source.Key()
		if _, ok := bySource[sk]; !ok {
			order = append(order, sk)
		}
		bySource[sk] = append(bySource[sk], r)
	}
	sort.Strings(order)
	var out []Record
	for _, sk := range order {
		rs := bySource[sk]
		sort.SliceStable(rs, func(i, j int) bool { return rs[i].ObservedAt.After(rs[j].ObservedAt) })
		if keep > 0 && len(rs) > keep {
			rs = rs[:keep]
		}
		out = append(out, rs...)
	}
	return out
}
