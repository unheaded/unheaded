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
	"time"

	"unheaded/pkg/compliance/snapshot"
	"unheaded/pkg/compliance/well"
	"unheaded/pkg/gungnir"
)

// `compliance-evidence snapshot -host H -key SEED -out FILE` seals one
// host's binding and the fixes its profile turns on (ADR-099 amended,
// COMPLIANCE-WELL step 105), for unheaded-remediate -snapshot on that host.
// It reads The Well; with The Well down it refuses unless -from-yaml says
// to build from the repo files, and the snapshot records which.
func runSnapshot(args []string, stdout io.Writer) error {
	fl := flag.NewFlagSet("snapshot", flag.ContinueOnError)
	repoDir := fl.String("repo-dir", ".", "repository root")
	host := fl.String("host", "", "host the snapshot is for")
	keyPath := fl.String("key", "", "ML-DSA-65 seed file (owner-only)")
	out := fl.String("out", "", "file to write")
	ttl := fl.Duration("ttl", 7*24*time.Hour, "how long the host trusts it (at most 31 days)")
	commit := fl.String("model-commit", "", "git commit of the model, recorded in the snapshot")
	fromYAML := fl.Bool("from-yaml", false, "build from the repo YAML when The Well is unavailable (recorded in the snapshot)")
	if err := fl.Parse(args); err != nil {
		return err
	}
	if *host == "" || *keyPath == "" || *out == "" {
		return errors.New("usage: compliance-evidence snapshot -host H -key SEED -out FILE [-ttl 168h] [-from-yaml]")
	}
	priv, err := gungnir.LoadSeedFile(*keyPath)
	if err != nil {
		return err
	}
	signer, err := gungnir.NewSignerWithID(priv)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	store, why, err := well.Open(ctx, *repoDir, well.ConfigFromEnv())
	if err != nil {
		return err
	}
	if store.Source() != well.SourceWell && !*fromYAML {
		return fmt.Errorf("The Well is unavailable (%s); refusing to seal the repo YAML without -from-yaml", why)
	}
	snap, err := snapshot.Build(ctx, store, *host, *commit, time.Now())
	if err != nil {
		return err
	}
	raw, err := snapshot.Seal(snap, signer, *ttl)
	if err != nil {
		return err
	}
	if err := os.WriteFile(*out, raw, 0o644); err != nil { // #nosec G306 -- a signed, public snapshot; integrity comes from the seal
		return err
	}
	fmt.Fprintf(stdout, "%s: profile %s, %d fixes, from %s, valid %s; written to %s\n",
		*host, snap.Profile, len(snap.Fixes), snap.Source, *ttl, *out)
	return nil
}
