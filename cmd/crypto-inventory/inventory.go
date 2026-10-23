// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// scanAll runs every scanner over the tree.
func scanAll(t *Tree) ([]Asset, error) {
	var all []Asset
	for _, s := range []struct {
		name string
		fn   func(*Tree) ([]Asset, error)
	}{
		{"go", scanGo},
		{"rust", scanRust},
		{"config", scanConfig},
		{"hosts", scanHosts},
	} {
		assets, err := s.fn(t)
		if err != nil {
			return nil, fmt.Errorf("%s scanner: %w", s.name, err)
		}
		all = append(all, assets...)
	}
	return all, nil
}

// generate scans the tree and renders the CBOM bytes.
func generate(t *Tree) ([]byte, []Asset, error) {
	assets, err := scanAll(t)
	if err != nil {
		return nil, nil, err
	}
	b, err := encodeBOM(buildBOM(assets))
	if err != nil {
		return nil, nil, err
	}
	return b, assets, nil
}

// errStale means the checked-in CBOM does not match a regeneration.
var errStale = errors.New("crypto inventory is stale")

// compareCBOM explains how have (checked in) differs from want
// (regenerated). nil when byte-identical.
func compareCBOM(want, have []byte) error {
	if bytes.Equal(want, have) {
		return nil
	}
	var w, h BOM
	if err := json.Unmarshal(want, &w); err != nil {
		return fmt.Errorf("regenerated cbom does not parse: %w", err)
	}
	if err := json.Unmarshal(have, &h); err != nil {
		return fmt.Errorf("%w: checked-in cbom does not parse (%v)", errStale, err)
	}
	wm, hm := map[string]string{}, map[string]string{}
	for _, c := range w.Components {
		b, _ := json.Marshal(c)
		wm[c.BOMRef] = string(b)
	}
	for _, c := range h.Components {
		b, _ := json.Marshal(c)
		hm[c.BOMRef] = string(b)
	}
	var lines []string
	for ref, wc := range wm {
		hc, ok := hm[ref]
		switch {
		case !ok:
			lines = append(lines, "+ "+ref)
		case hc != wc:
			lines = append(lines, "~ "+ref)
		}
	}
	for ref := range hm {
		if _, ok := wm[ref]; !ok {
			lines = append(lines, "- "+ref)
		}
	}
	sort.Slice(lines, func(i, j int) bool { return lines[i][2:] < lines[j][2:] })
	if len(lines) == 0 {
		lines = append(lines, "  (components identical; header or formatting differs)")
	}
	return fmt.Errorf("%w (+ in the tree, not the file; - in the file, not the tree; ~ changed):\n%s",
		errStale, strings.Join(lines, "\n"))
}

// summarize counts components by pq-status and owner.
func summarize(assets []Asset) string {
	type key struct{ status, owner string }
	counts := map[key]int{}
	total := map[string]int{}
	for _, a := range assets {
		counts[key{a.PQStatus, a.Owner}]++
		total[a.PQStatus]++
	}
	statuses := make([]string, 0, len(total))
	for s := range total {
		statuses = append(statuses, s)
	}
	sort.Strings(statuses)
	var b strings.Builder
	fmt.Fprintf(&b, "%-22s %6s %12s %12s\n", "pq-status", "total", "first-party", "third-party")
	for _, s := range statuses {
		fmt.Fprintf(&b, "%-22s %6d %12d %12d\n", s, total[s], counts[key{s, ownerFirstParty}], counts[key{s, ownerThirdParty}])
	}
	fmt.Fprintf(&b, "%-22s %6d\n", "all", len(assets))
	return b.String()
}
