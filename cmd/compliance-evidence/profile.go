// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"unheaded/pkg/compliance/remediation"
	"unheaded/pkg/compliance/well"
	"unheaded/pkg/database"
)

// Switching controls (ADR-099 §2, COMPLIANCE-WELL steps 97-98):
//
//	compliance-evidence profile set -host east -profile cis-l1-server -reason "why" [-yes]
//	compliance-evidence profile export [-repo-dir .]
//
// set changes desired state only: nothing is applied to the host until an
// operator runs unheaded-remediate apply (or, later, the host is enforcing
// under ADR-098's gate). Without -yes it shows what would change and stops.
// Both need The Well (WELL_CONFIG_USER=config_admin and WELL_CONFIG_PASSWORD
// for set); there is no YAML fallback for a write.

func runProfile(args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: compliance-evidence profile set|export ...")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	switch args[0] {
	case "set":
		return profileSet(ctx, args[1:], stdout)
	case "export":
		return profileExport(ctx, args[1:], stdout)
	}
	return fmt.Errorf("unknown profile command %q (set, export)", args[0])
}

func openWell(ctx context.Context, repoDir string) (well.Store, func(), error) {
	m, err := well.LoadModel(repoDir)
	if err != nil {
		return nil, nil, err
	}
	db, err := database.Connect(ctx, well.ConfigFromEnv())
	if err != nil {
		return nil, nil, fmt.Errorf("The Well (unheaded_config; set WELL_CONFIG_USER and WELL_CONFIG_PASSWORD): %w", err)
	}
	return well.NewPGStore(db, m), func() { _ = db.Close() }, nil
}

func profileSet(ctx context.Context, args []string, stdout io.Writer) error {
	fl := flag.NewFlagSet("profile set", flag.ContinueOnError)
	repoDir := fl.String("repo-dir", ".", "repository root")
	host := fl.String("host", "", "host to bind")
	profile := fl.String("profile", "", "profile name (none, lab, cis-l1-server, all, ...)")
	reason := fl.String("reason", "", "why (recorded with who and when)")
	yes := fl.Bool("yes", false, "make the change; without it, only show it")
	if err := fl.Parse(args); err != nil {
		return err
	}
	s, done, err := openWell(ctx, *repoDir)
	if err != nil {
		return err
	}
	defer done()
	ps, err := s.Profiles(ctx)
	if err != nil {
		return err
	}
	c := well.Change{Host: *host, Profile: *profile, Mode: "audit", Actor: operator(), Reason: *reason}
	if err := c.Validate(ps); err != nil {
		return err
	}
	bs, err := s.Bindings(ctx)
	if err != nil {
		return err
	}
	current := ""
	for _, b := range bs {
		if b.Host == *host {
			current = b.Profile
		}
	}
	fmt.Fprint(stdout, describeSwitch(ps, *host, current, *profile))
	if !*yes {
		fmt.Fprintln(stdout, "not changed: add -yes to make this change")
		return nil
	}
	if err := s.SetBinding(ctx, c); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "%s bound to %s (recorded: %s, %q). Nothing was applied to the host: run unheaded-remediate plan/apply there.\n",
		*host, *profile, c.Actor, c.Reason)
	return nil
}

// describeSwitch lists the fixes a switch turns on and off.
func describeSwitch(ps *remediation.ProfileSet, host, from, to string) string {
	was := map[string]bool{}
	for _, id := range ps.Resolve(from) {
		was[id] = true
	}
	var on, off []string
	now := map[string]bool{}
	for _, id := range ps.Resolve(to) {
		now[id] = true
		if !was[id] {
			on = append(on, id)
		}
	}
	for _, id := range ps.Resolve(from) {
		if !now[id] {
			off = append(off, id)
		}
	}
	if from == "" {
		from = "(unbound)"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s: %s -> %s\n", host, from, to)
	fmt.Fprintf(&b, "  turns on:  %s\n", orNone(on))
	fmt.Fprintf(&b, "  turns off: %s (switching off does not undo an applied fix; unheaded-remediate undo does)\n", orNone(off))
	return b.String()
}

func orNone(s []string) string {
	if len(s) == 0 {
		return "nothing"
	}
	return strings.Join(s, ", ")
}

func profileExport(ctx context.Context, args []string, stdout io.Writer) error {
	fl := flag.NewFlagSet("profile export", flag.ContinueOnError)
	repoDir := fl.String("repo-dir", ".", "repository root")
	if err := fl.Parse(args); err != nil {
		return err
	}
	s, done, err := openWell(ctx, *repoDir)
	if err != nil {
		return err
	}
	defer done()
	changed, err := well.Export(ctx, s, *repoDir)
	if err != nil {
		return err
	}
	if len(changed) == 0 {
		fmt.Fprintln(stdout, "fallback YAML already matches The Well")
	}
	for _, p := range changed {
		fmt.Fprintln(stdout, "wrote", p)
	}
	return nil
}
