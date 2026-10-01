// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"context"
	"fmt"
	"os"

	"unheaded/pkg/compliance/crosswalk"
	"unheaded/pkg/compliance/crosswalk/collect"
	"unheaded/pkg/compliance/well"
	"unheaded/pkg/database"
)

// collectJournals gathers host-journal evidence (ADR-099 phase 5): what
// each host's profile wants comes from The Well when it answers, else the
// repo YAML (said on stderr). When WELL_OPS_USER/WELL_OPS_PASSWORD name a
// writer, each journal read is also ingested into The Well, which refuses
// a rewritten or truncated one; that refusal is printed, loudly, and the
// evidence already fails on a broken chain.
func collectJournals(ctx context.Context, repoDir string, sources []crosswalk.Source) ([]crosswalk.Record, error) {
	found := false
	for _, s := range sources {
		found = found || s.Kind == crosswalk.KindHostJournal
	}
	if !found {
		return nil, nil
	}
	store, why, err := well.Open(ctx, repoDir, well.ConfigFromEnv())
	if err != nil {
		return nil, err
	}
	if why != "" {
		fmt.Fprintln(os.Stderr, "host-journal: profiles from the repo YAML, not The Well:", why)
	}
	ps, err := store.Profiles(ctx)
	if err != nil {
		return nil, err
	}
	bs, err := store.Bindings(ctx)
	if err != nil {
		return nil, err
	}
	h := &collect.HostJournal{Wanted: map[string][]string{}, SHA256: map[string]string{}}
	for _, b := range bs {
		h.Wanted[b.Host] = ps.Resolve(b.Profile)
	}
	for _, f := range store.Model().Fixes {
		if h.SHA256[f.ID], err = f.SHA256(); err != nil {
			return nil, err
		}
	}
	recs, err := h.Collect(ctx, sources)
	if err != nil {
		return nil, err
	}
	if os.Getenv("WELL_OPS_PASSWORD") != "" && len(h.Journals) > 0 {
		db, err := database.Connect(ctx, well.OpsConfigFromEnv())
		if err != nil {
			fmt.Fprintln(os.Stderr, "host-journal: not ingested into The Well:", err)
			return recs, nil
		}
		defer db.Close()
		ops := well.NewOpsStore(db)
		for host, entries := range h.Journals {
			n, err := ops.IngestJournal(ctx, host, entries)
			if err != nil {
				fmt.Fprintf(os.Stderr, "host-journal: %s NOT ingested: %v\n", host, err)
				continue
			}
			fmt.Fprintf(os.Stderr, "host-journal: %s: %d new entries ingested into The Well\n", host, n)
		}
	}
	return recs, nil
}
