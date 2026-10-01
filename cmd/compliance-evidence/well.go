// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"unheaded/pkg/compliance/well"
	"unheaded/pkg/database"
)

// `compliance-evidence well sync [-dry-run]` (ADR-099 amended, COMPLIANCE-
// WELL step 46): mirror the repo's fixes, controls and frameworks into The
// Well and seed profiles, bindings and findings it does not have yet. It
// writes, so it needs The Well and fails loudly without it (no fallback):
//
//	WELL_CONFIG_USER=config_admin WELL_CONFIG_PASSWORD=... \
//	WELL_OPS_USER=ops_writer WELL_OPS_PASSWORD=... \
//	go run ./cmd/compliance-evidence well sync -dry-run

func runWell(args []string, stdout io.Writer) error {
	if len(args) == 0 || args[0] != "sync" {
		return errors.New("usage: compliance-evidence well sync [-dry-run] [-repo-dir .]")
	}
	fl := flag.NewFlagSet("well sync", flag.ContinueOnError)
	repoDir := fl.String("repo-dir", ".", "repository root")
	dry := fl.Bool("dry-run", false, "report what would change; write nothing")
	if err := fl.Parse(args[1:]); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cfg, err := database.Connect(ctx, well.ConfigFromEnv())
	if err != nil {
		return fmt.Errorf("unheaded_config (needs WELL_CONFIG_USER=config_admin and WELL_CONFIG_PASSWORD): %w", err)
	}
	defer cfg.Close()
	ops, err := database.Connect(ctx, well.OpsConfigFromEnv())
	if err != nil {
		return fmt.Errorf("unheaded_ops (needs WELL_OPS_USER=ops_writer and WELL_OPS_PASSWORD): %w", err)
	}
	defer ops.Close()
	rep, err := well.Sync(ctx, *repoDir, cfg, ops, operator(), *dry)
	if err != nil {
		return err
	}
	_, err = io.WriteString(stdout, renderSync(rep, *dry))
	return err
}

// operator is who runs the command: the sudo caller if any, else the user.
func operator() string {
	for _, k := range []string{"SUDO_USER", "USER"} {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}
	return "unknown"
}

func renderSync(r *well.SyncReport, dry bool) string {
	var b strings.Builder
	verb := "synced"
	if dry {
		verb = "dry run (nothing written)"
	}
	fmt.Fprintf(&b, "well sync: %s\n", verb)
	tables := map[string]bool{}
	for _, m := range []map[string]int{r.Created, r.Updated, r.Same} {
		for t := range m {
			tables[t] = true
		}
	}
	names := make([]string, 0, len(tables))
	for t := range tables {
		names = append(names, t)
	}
	sort.Strings(names)
	for _, t := range names {
		fmt.Fprintf(&b, "  %-28s %4d new %4d updated %4d no change\n", t, r.Created[t], r.Updated[t], r.Same[t])
	}
	for _, d := range r.Diverged {
		fmt.Fprintf(&b, "  diverged (The Well differs from the YAML; left as is): %s\n", d)
	}
	for _, o := range r.Orphaned {
		fmt.Fprintf(&b, "  orphaned (in The Well, gone from the repo; not deleted): %s\n", o)
	}
	return b.String()
}
