// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// compliance-evidence collects evidence for every source the compliance
// catalog references (GitHub Actions job conclusions, commit signatures,
// gate scripts), merges it into the evidence file, and prints each
// framework's figures with their denominators (ADR-097).
//
//	go run ./cmd/compliance-evidence                # from the repo root
//	go run ./cmd/compliance-evidence -skip-gates    # GitHub + git only
//	go run ./cmd/compliance-evidence findings       # the findings register (ADR-098)
//	go run ./cmd/compliance-evidence findings -adr docs/adr/ADR-098-findings-register-and-baseline-modes.md
//	go run ./cmd/compliance-evidence baseline       # regenerate compliance/baseline/baseline.yaml
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
	if len(os.Args) > 1 && os.Args[1] == "findings" {
		if err := runFindings(os.Args[2:], os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, "compliance-evidence findings:", err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "baseline" {
		if err := runBaseline(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "compliance-evidence baseline:", err)
			os.Exit(1)
		}
		return
	}
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
				sources = append(sources, s.Source)
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

// runBaseline writes compliance/baseline/baseline.yaml from the catalog: the
// host-configuration checks the host agent runs (ADR-098).
func runBaseline(args []string) error {
	fl := flag.NewFlagSet("baseline", flag.ContinueOnError)
	repoDir := fl.String("repo-dir", ".", "repository root")
	if err := fl.Parse(args); err != nil {
		return err
	}
	cat, err := crosswalk.LoadFS(os.DirFS(*repoDir), "compliance/catalog/frameworks", "compliance/catalog/controls.yaml")
	if err != nil {
		return err
	}
	path := filepath.Join(*repoDir, "compliance", "baseline", "baseline.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil { // #nosec G301 -- repo directory
		return err
	}
	b := collect.BaselineFromCatalog(cat)
	if err := os.WriteFile(path, b.Marshal(), 0o644); err != nil { // #nosec G306 -- repo file, shipped world-readable
		return err
	}
	fmt.Printf("%d host checks written to %s\n", len(b.Sources), path)
	return nil
}
