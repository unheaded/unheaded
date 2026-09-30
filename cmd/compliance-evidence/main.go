// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// compliance-evidence collects evidence for every source the compliance
// catalog references (GitHub Actions job conclusions, commit signatures,
// gate scripts), merges it into the evidence file, and prints each
// framework's figures with their denominators (ADR-097).
//
//	go run ./cmd/compliance-evidence                # from the repo root
//	go run ./cmd/compliance-evidence -skip-gates    # GitHub + git only
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"unheaded/pkg/compliance/crosswalk"
	"unheaded/pkg/compliance/crosswalk/collect"
)

func main() {
	repoDir := flag.String("repo-dir", ".", "repository root (catalog, git, scripts/)")
	out := flag.String("out", "var/compliance/evidence.json", "evidence file to merge into")
	ghRepo := flag.String("github-repo", "unheaded/unheaded", "owner/name for GitHub Actions evidence")
	branch := flag.String("branch", "develop", "branch whose CI runs are evidence")
	keep := flag.Int("keep", 50, "records kept per source")
	skipGates := flag.Bool("skip-gates", false, "do not run gate scripts")
	flag.Parse()

	if err := run(*repoDir, *out, *ghRepo, *branch, *keep, *skipGates); err != nil {
		fmt.Fprintln(os.Stderr, "compliance-evidence:", err)
		os.Exit(1)
	}
}

func run(repoDir, out, ghRepo, branch string, keep int, skipGates bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	cat, err := crosswalk.LoadFS(os.DirFS(repoDir), "compliance/catalog/frameworks", "compliance/catalog/controls.yaml")
	if err != nil {
		return err
	}
	var sources []crosswalk.Source
	seen := map[string]bool{}
	for _, c := range cat.Controls {
		for _, s := range c.Evidence {
			if !seen[s.Key()] {
				seen[s.Key()] = true
				sources = append(sources, s)
			}
		}
	}

	var fresh []crosswalk.Record
	gh := &collect.GitHubJobs{Repo: ghRepo, Branch: branch, Token: os.Getenv("GITHUB_TOKEN")}
	recs, err := gh.Collect(ctx, sources)
	if err != nil {
		fmt.Fprintln(os.Stderr, "github evidence unavailable:", err) // the controls stay NOT_ASSESSED
	}
	fresh = append(fresh, recs...)

	sig := &collect.GitSignatures{RepoDir: repoDir, Depth: 50}
	if recs, err = sig.Collect(ctx, sources); err != nil {
		return err
	}
	fresh = append(fresh, recs...)

	att := &collect.Attestations{RepoDir: repoDir}
	if recs, err = att.Collect(ctx, sources); err != nil {
		return err
	}
	fresh = append(fresh, recs...)

	sshd := &collect.HostSSHD{}
	if recs, err = sshd.Collect(ctx, sources); err != nil {
		return err
	}
	fresh = append(fresh, recs...)

	probes := &collect.HostProbes{}
	if recs, err = probes.Collect(ctx, sources); err != nil {
		return err
	}
	fresh = append(fresh, recs...)

	host := &collect.HostSysctl{}
	if recs, err = host.Collect(ctx, sources); err != nil {
		return err
	}
	fresh = append(fresh, recs...)

	if !skipGates {
		gates := &collect.GateScripts{RepoDir: repoDir, Timeout: 10 * time.Minute}
		if recs, err = gates.Collect(ctx, sources); err != nil {
			return err
		}
		fresh = append(fresh, recs...)
	}

	path := out
	if !filepath.IsAbs(path) {
		path = filepath.Join(repoDir, path)
	}
	var old []crosswalk.Record
	if f, err := os.Open(path); err == nil {
		old, err = crosswalk.ReadEvidence(f)
		f.Close()
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	merged := crosswalk.MergeRecords(old, fresh, keep)
	if err := writeAtomic(path, merged); err != nil {
		return err
	}
	fmt.Printf("%d new records, %d kept, written to %s\n\n", len(fresh), len(merged), path)

	s := crosswalk.Summarize(cat, merged, time.Now())
	for _, c := range cat.Controls {
		fmt.Printf("  %-12s %-13s %s\n", c.ID, s.Controls[c.ID], c.Title)
	}
	fmt.Println()
	for _, fw := range s.Frameworks {
		fmt.Printf("  %-18s %4d evidenced  %3d failing  %3d incomplete  %4d unmapped  of %4d\n", fw.ID,
			fw.Counts[crosswalk.ReqEvidenced], fw.Counts[crosswalk.ReqFailing],
			fw.Counts[crosswalk.ReqIncomplete], fw.Counts[crosswalk.ReqUnmapped], fw.Total)
	}
	return nil
}

func writeAtomic(path string, recs []crosswalk.Record) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil { // #nosec G301 -- evidence is not secret; the dashboard reads it
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".evidence-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := crosswalk.WriteEvidence(tmp, recs); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
