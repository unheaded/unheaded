// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

// verify-gates: the CI gates a pack claims, re-checked from the pack itself.
// The manifest cannot contradict its own attachments: CVE counts are
// recomputed from cve-scan.json, the SBOM package count from sbom.spdx.json,
// CIS/Lynis/doctor values from their JSON, and every overlay patch is hashed.
//
// Waivers: the manifest's cve_scan counts are UNWAIVED findings (the schema
// pins critical and high to 0). A CRITICAL/HIGH finding in cve-scan.json is
// waived only by a cve_scan.waivers entry that has not expired (expires_date
// is inclusive, compared with the verification date) and whose CVE ID also
// appears in a chain_of_custody action (the README's "log in chain of
// custody" rule).

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	fileCVEScan = "cve-scan.json"
	fileSPDX    = "sbom.spdx.json"
	fileCIS     = "cis-benchmark.json"
	fileDoctor  = "upc-doctor.json"
	dirPatches  = "overlay-patches/"
)

// lenientManifest decodes the manifest without schema rules, so gate failures
// can be reported by value even when the manifest also fails validation.
func lenientManifest(p *Pack) (*Manifest, error) {
	b := p.File(fileManifest)
	if !p.Has(fileManifest) {
		return nil, fmt.Errorf("%s missing from the pack", fileManifest)
	}
	if len(b) > maxManifestBytes {
		return nil, fmt.Errorf("%s is larger than %d bytes", fileManifest, maxManifestBytes)
	}
	var m Manifest
	if err := yaml.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("%s does not decode: %v", fileManifest, err)
	}
	return &m, nil
}

func decodeJSON(p *Pack, name string, v any) error {
	if !p.Has(name) {
		return fmt.Errorf("%s missing from the pack", name)
	}
	dec := json.NewDecoder(bytes.NewReader(p.File(name)))
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("%s: %v", name, err)
	}
	if _, err := dec.Token(); err != io.EOF {
		return fmt.Errorf("%s: trailing data", name)
	}
	return nil
}

type trivyReport struct {
	Results []struct {
		Vulnerabilities []struct {
			VulnerabilityID string `json:"VulnerabilityID"`
			Severity        string `json:"Severity"`
		} `json:"Vulnerabilities"`
	} `json:"Results"`
}

func checkGates(p *Pack, now time.Time) []error {
	var errs []error
	fail := func(format string, args ...any) { errs = append(errs, fmt.Errorf(format, args...)) }

	if _, err := ValidateManifest(p.File(fileManifest)); err != nil {
		fail("%s does not validate against schema v1:\n%v", fileManifest, err)
	}
	m, err := lenientManifest(p)
	if err != nil {
		return append(errs, err)
	}

	// Thresholds (OS-FORK-DISCIPLINE.md Pillar 4, README invariants).
	if m.CVEScan.Critical != 0 {
		fail("cve_scan.critical = %d, gate requires 0", m.CVEScan.Critical)
	}
	if m.CVEScan.High != 0 {
		fail("cve_scan.high = %d, gate requires 0 (waive via cve_scan.waivers + chain_of_custody)", m.CVEScan.High)
	}
	if m.CISBenchmark.ScorePercent < 95 {
		fail("cis_benchmark.score_percent = %v, gate requires >= 95", m.CISBenchmark.ScorePercent)
	}
	if m.CISBenchmark.LynisHardeningScore < 90 {
		fail("cis_benchmark.lynis_hardening_score = %d, gate requires >= 90", m.CISBenchmark.LynisHardeningScore)
	}
	if m.UPCDoctor.ExitCode != 0 {
		fail("upc_doctor.exit_code = %d, gate requires 0", m.UPCDoctor.ExitCode)
	}
	if m.Overlay.PatchCount > 50 {
		fail("overlay.patch_count = %d, gate requires <= 50", m.Overlay.PatchCount)
	}
	if m.Overlay.TotalLOCDelta > 5000 {
		fail("overlay.total_loc_delta = %d, gate requires <= 5000", m.Overlay.TotalLOCDelta)
	}

	errs = append(errs, checkCVEScan(p, m, now)...)

	var spdx struct {
		Packages *[]json.RawMessage `json:"packages"`
	}
	if err := decodeJSON(p, fileSPDX, &spdx); err != nil {
		fail("%v", err)
	} else if spdx.Packages == nil {
		fail("%s has no packages array", fileSPDX)
	} else if len(*spdx.Packages) != m.SBOM.PackageCount {
		fail("sbom.package_count = %d, %s lists %d packages", m.SBOM.PackageCount, fileSPDX, len(*spdx.Packages))
	}

	var cis struct {
		ScorePercent        *float64 `json:"score_percent"`
		LynisHardeningScore *int     `json:"lynis_hardening_score"`
	}
	if err := decodeJSON(p, fileCIS, &cis); err != nil {
		fail("%v", err)
	} else {
		if cis.ScorePercent == nil {
			fail("%s has no score_percent", fileCIS)
		} else if *cis.ScorePercent != m.CISBenchmark.ScorePercent {
			fail("cis_benchmark.score_percent = %v, %s says %v", m.CISBenchmark.ScorePercent, fileCIS, *cis.ScorePercent)
		}
		if cis.LynisHardeningScore != nil && *cis.LynisHardeningScore != m.CISBenchmark.LynisHardeningScore {
			fail("cis_benchmark.lynis_hardening_score = %d, %s says %d", m.CISBenchmark.LynisHardeningScore, fileCIS, *cis.LynisHardeningScore)
		}
	}

	var doctor struct {
		ExitCode *int `json:"exit_code"`
	}
	if err := decodeJSON(p, fileDoctor, &doctor); err != nil {
		fail("%v", err)
	} else if doctor.ExitCode == nil {
		fail("%s has no exit_code", fileDoctor)
	} else if *doctor.ExitCode != m.UPCDoctor.ExitCode {
		fail("upc_doctor.exit_code = %d, %s says %d", m.UPCDoctor.ExitCode, fileDoctor, *doctor.ExitCode)
	}

	errs = append(errs, checkPatches(p, m)...)
	return errs
}

func checkCVEScan(p *Pack, m *Manifest, now time.Time) []error {
	var rep trivyReport
	if err := decodeJSON(p, fileCVEScan, &rep); err != nil {
		return []error{err}
	}
	var errs []error
	today := now.UTC().Format("2006-01-02")
	logged := func(id string) bool {
		for _, e := range m.ChainOfCustody {
			if strings.Contains(e.Action, id) {
				return true
			}
		}
		return false
	}
	// waivedBy reports whether id is waived, or why not.
	waivedBy := func(id string) (bool, string) {
		for _, w := range m.CVEScan.Waivers {
			if w.CVEID != id {
				continue
			}
			if w.ExpiresDate < today {
				return false, fmt.Sprintf("waiver expired %s", w.ExpiresDate)
			}
			if !logged(id) {
				return false, "waiver is not logged in chain_of_custody"
			}
			return true, ""
		}
		return false, "no waiver"
	}
	counts := map[string]int{}
	for _, r := range rep.Results {
		for _, v := range r.Vulnerabilities {
			sev := strings.ToUpper(v.Severity)
			if sev == "CRITICAL" || sev == "HIGH" {
				ok, why := waivedBy(v.VulnerabilityID)
				if ok {
					continue
				}
				errs = append(errs, fmt.Errorf("%s: %s %s is not waived (%s)", fileCVEScan, sev, v.VulnerabilityID, why))
			}
			counts[sev]++
		}
	}
	for _, c := range []struct {
		sev      string
		manifest int
	}{{"CRITICAL", m.CVEScan.Critical}, {"HIGH", m.CVEScan.High}, {"MEDIUM", m.CVEScan.Medium}, {"LOW", m.CVEScan.Low}} {
		if counts[c.sev] != c.manifest {
			errs = append(errs, fmt.Errorf("cve_scan.%s = %d, %s has %d unwaived", strings.ToLower(c.sev), c.manifest, fileCVEScan, counts[c.sev]))
		}
	}
	return errs
}

func checkPatches(p *Pack, m *Manifest) []error {
	var errs []error
	if len(m.Overlay.Patches) != m.Overlay.PatchCount {
		errs = append(errs, fmt.Errorf("overlay.patch_count = %d, overlay.patches lists %d", m.Overlay.PatchCount, len(m.Overlay.Patches)))
	}
	listed := map[string]bool{}
	for _, pt := range m.Overlay.Patches {
		listed[pt.Filename] = true
		name := dirPatches + pt.Filename
		if !p.Has(name) {
			errs = append(errs, fmt.Errorf("overlay patch %s is listed but missing from %s", pt.Filename, dirPatches))
			continue
		}
		h := sha256.Sum256(p.File(name))
		if got := hex.EncodeToString(h[:]); got != pt.Sha256 {
			errs = append(errs, fmt.Errorf("overlay patch %s: sha256 %s, manifest says %s", pt.Filename, got, pt.Sha256))
		}
	}
	var extra []string
	for _, n := range p.Names() {
		if strings.HasPrefix(n, dirPatches) && !listed[strings.TrimPrefix(n, dirPatches)] {
			extra = append(extra, n)
		}
	}
	sort.Strings(extra)
	for _, n := range extra {
		errs = append(errs, fmt.Errorf("%s is in the pack but not in overlay.patches", n))
	}
	return errs
}

func cmdVerifyGates(args []string, stdout, stderr io.Writer) int {
	fs := newFlags("verify-gates", stderr)
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "verify-gates: expected one positional arg (evidence-pack tarball)")
		return exitUsage
	}
	p, rc := loadPack("verify-gates", fs.Arg(0), stderr)
	if p == nil {
		return rc
	}
	return report(stdout, stderr, "verify-gates", "CVE 0/0 (waivers logged), CIS >= 95, Lynis >= 90, doctor 0, patches <= 50, LOC <= 5000, attachments agree", checkGates(p, time.Now()))
}

func cmdVerifyCustody(args []string, stdout, stderr io.Writer) int {
	fs := newFlags("verify-custody", stderr)
	pubkey := fs.String("pubkey", defaultPubkey, "ML-DSA-65 build public key")
	allowUnsigned := fs.Bool("allow-unsigned-entries", false, "inspect a legacy pack: list unsigned entries (still exits 1)")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "verify-custody: expected one positional arg (evidence-pack tarball)")
		return exitUsage
	}
	pc, rc := loadPackAndKey("verify-custody", *pubkey, fs.Arg(0), stderr)
	if pc == nil {
		return rc
	}
	m, err := lenientManifest(pc.pack)
	if err != nil {
		return report(stdout, stderr, "verify-custody", "", []error{err})
	}
	return report(stdout, stderr, "verify-custody",
		fmt.Sprintf("%d entries, monotonic, hash-chained, signed by key %s", len(m.ChainOfCustody), keyIDOf(pc)),
		checkCustody(m.ChainOfCustody, pc.pub, *allowUnsigned))
}
