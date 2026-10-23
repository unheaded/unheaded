// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

// verify (every stage, each reported) and diff (quarterly review).

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"unheaded/pkg/gungnir"
)

func keyIDOf(pc *packCmd) string { return gungnir.KeyID(pc.pub) }

func cmdVerify(args []string, stdout, stderr io.Writer) int {
	fs := newFlags("verify", stderr)
	pubkey := fs.String("pubkey", defaultPubkey, "ML-DSA-65 build public key")
	iso := fs.String("iso", "", "optional: also check this ISO against the signed hash")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "verify: expected one positional arg (evidence-pack tarball)")
		return exitUsage
	}
	var isoSum []byte
	var isoSize int64
	if *iso != "" {
		var err error
		if isoSum, isoSize, err = hashFile(*iso); err != nil {
			fmt.Fprintf(stderr, "verify: %v\n", err)
			return exitUsage
		}
	}
	pc, rc := loadPackAndKey("verify", *pubkey, fs.Arg(0), stderr)
	if pc == nil {
		return rc
	}

	type stage struct {
		name string
		run  func() []error
	}
	stages := []stage{
		{"validate", func() []error {
			if _, err := ValidateManifest(pc.pack.File(fileManifest)); err != nil {
				return []error{err}
			}
			return nil
		}},
		{"signature", func() []error { return checkSignatures(pc) }},
		{"gates", func() []error { return checkGates(pc.pack, time.Now()) }},
		{"custody", func() []error {
			m, err := lenientManifest(pc.pack)
			if err != nil {
				return []error{err}
			}
			return checkCustody(m.ChainOfCustody, pc.pub, false)
		}},
	}
	if *iso != "" {
		stages = append(stages, stage{"iso", func() []error { return checkISO(pc, *iso, isoSum, isoSize) }})
	}

	failed := 0
	for _, s := range stages {
		errs := s.run()
		if len(errs) == 0 {
			fmt.Fprintf(stdout, "verify: %s: OK\n", s.name)
			continue
		}
		failed++
		fmt.Fprintf(stdout, "verify: %s: FAIL\n", s.name)
		for _, e := range errs {
			fmt.Fprintf(stderr, "verify: %s: %v\n", s.name, e)
		}
	}
	if *iso == "" {
		fmt.Fprintln(stdout, "verify: iso: SKIPPED (no --iso)")
	}
	if failed > 0 {
		fmt.Fprintf(stderr, "verify: %s: NOT VERIFIED (%d of %d stages failed)\n", pc.path, failed, len(stages))
		return exitFail
	}
	fmt.Fprintf(stdout, "verify: %s: VERIFIED (key %s)\n", pc.path, keyIDOf(pc))
	return exitOK
}

// --- diff ---------------------------------------------------------------------

const exitRewritten = 3

func cmdDiff(args []string, stdout, stderr io.Writer) int {
	fs := newFlags("diff", stderr)
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if fs.NArg() != 2 {
		fmt.Fprintln(stderr, "diff: expected two positional args (old-pack, new-pack)")
		return exitUsage
	}
	var ms [2]*Manifest
	for i, path := range fs.Args() {
		p, _ := loadPack("diff", path, stderr)
		if p == nil {
			return exitUsage
		}
		m, err := ValidateManifest(p.File(fileManifest))
		if err != nil {
			fmt.Fprintf(stderr, "diff: %s: manifest does not validate:\n%v\n", path, err)
			return exitUsage
		}
		ms[i] = m
	}
	lines, rewritten := diffManifests(ms[0], ms[1])
	if len(lines) == 0 {
		fmt.Fprintln(stdout, "diff: no differences")
	}
	for _, l := range lines {
		fmt.Fprintln(stdout, l)
	}
	if rewritten != "" {
		fmt.Fprintf(stderr, "diff: CUSTODY HISTORY REWRITTEN: %s\n", rewritten)
		fmt.Fprintln(stderr, "diff: the old pack's chain_of_custody is not a prefix of the new one; custody is append-only. Do not accept the new pack.")
		return exitRewritten
	}
	return exitOK
}

func diffManifests(a, b *Manifest) ([]string, string) {
	var out []string
	str := func(field, x, y string) {
		if x != y {
			out = append(out, fmt.Sprintf("%s: %s -> %s", field, x, y))
		}
	}
	num := func(field string, x, y int) {
		if x != y {
			out = append(out, fmt.Sprintf("%s: %d -> %d (%+d)", field, x, y, y-x))
		}
	}
	str("image.iso_sha256", a.Image.IsoSha256, b.Image.IsoSha256)
	str("image.build_date_utc", a.Image.BuildDateUTC, b.Image.BuildDateUTC)
	str("anchor.release_codename", a.Anchor.ReleaseCodename, b.Anchor.ReleaseCodename)
	str("anchor.release_version", a.Anchor.ReleaseVersion, b.Anchor.ReleaseVersion)
	str("anchor.release_date", a.Anchor.ReleaseDate, b.Anchor.ReleaseDate)
	str("anchor.debian_commit", a.Anchor.DebianCommit, b.Anchor.DebianCommit)

	pa, pb := map[string]string{}, map[string]string{}
	for _, p := range a.Overlay.Patches {
		pa[p.Filename] = p.Sha256
	}
	for _, p := range b.Overlay.Patches {
		pb[p.Filename] = p.Sha256
	}
	names := map[string]bool{}
	for n := range pa {
		names[n] = true
	}
	for n := range pb {
		names[n] = true
	}
	for _, n := range sortedKeys(names) {
		sa, inA := pa[n]
		sb, inB := pb[n]
		switch {
		case !inA:
			out = append(out, "overlay.patches: + "+n)
		case !inB:
			out = append(out, "overlay.patches: - "+n)
		case sa != sb:
			out = append(out, fmt.Sprintf("overlay.patches: ~ %s sha256 %s -> %s", n, sa, sb))
		}
	}
	num("overlay.patch_count", a.Overlay.PatchCount, b.Overlay.PatchCount)
	num("overlay.total_loc_delta", a.Overlay.TotalLOCDelta, b.Overlay.TotalLOCDelta)

	num("sbom.package_count", a.SBOM.PackageCount, b.SBOM.PackageCount)
	lic := map[string]bool{}
	for k := range a.SBOM.LicenseSummary {
		lic[k] = true
	}
	for k := range b.SBOM.LicenseSummary {
		lic[k] = true
	}
	for _, k := range sortedKeys(lic) {
		num("sbom.license_summary."+k, a.SBOM.LicenseSummary[k], b.SBOM.LicenseSummary[k])
	}

	num("cve_scan.critical", a.CVEScan.Critical, b.CVEScan.Critical)
	num("cve_scan.high", a.CVEScan.High, b.CVEScan.High)
	num("cve_scan.medium", a.CVEScan.Medium, b.CVEScan.Medium)
	num("cve_scan.low", a.CVEScan.Low, b.CVEScan.Low)
	wa, wb := map[string]bool{}, map[string]bool{}
	for _, w := range a.CVEScan.Waivers {
		wa[w.CVEID] = true
	}
	for _, w := range b.CVEScan.Waivers {
		wb[w.CVEID] = true
	}
	for _, id := range sortedKeys(wb) {
		if !wa[id] {
			out = append(out, "cve_scan.waivers: + "+id)
		}
	}
	for _, id := range sortedKeys(wa) {
		if !wb[id] {
			out = append(out, "cve_scan.waivers: - "+id)
		}
	}
	if a.CISBenchmark.ScorePercent != b.CISBenchmark.ScorePercent {
		out = append(out, fmt.Sprintf("cis_benchmark.score_percent: %v -> %v", a.CISBenchmark.ScorePercent, b.CISBenchmark.ScorePercent))
	}
	num("cis_benchmark.lynis_hardening_score", a.CISBenchmark.LynisHardeningScore, b.CISBenchmark.LynisHardeningScore)

	// Custody is append-only: the old chain must be a prefix of the new one.
	ca, cb := a.ChainOfCustody, b.ChainOfCustody
	rewritten := ""
	if len(cb) < len(ca) {
		rewritten = fmt.Sprintf("old has %d entries, new has %d", len(ca), len(cb))
	} else {
		for i := range ca {
			if ca[i] != cb[i] {
				rewritten = fmt.Sprintf("entry %d differs (old %q by %q, new %q by %q)", i, ca[i].Action, ca[i].Actor, cb[i].Action, cb[i].Actor)
				break
			}
		}
	}
	if rewritten == "" && len(cb) > len(ca) {
		out = append(out, fmt.Sprintf("chain_of_custody: +%d entries appended", len(cb)-len(ca)))
		for _, e := range cb[len(ca):] {
			signed := "unsigned"
			if e.Signature != "" {
				signed = "signed"
			}
			out = append(out, fmt.Sprintf("  + %s %s: %s (%s)", e.TimestampUTC, e.Actor, strings.TrimSpace(e.Action), signed))
		}
	}
	return out, rewritten
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
