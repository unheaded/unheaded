// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// withCustody rewrites the manifest's chain_of_custody section (it is the last
// section of every fixture manifest) after signing: a tamper, not a re-sign.
func withCustody(t *testing.T, edit func([]CustodyEntry) []CustodyEntry) func(map[string][]byte) {
	return func(f map[string][]byte) {
		t.Helper()
		m, err := ValidateManifest(f["manifest.yaml"])
		if err != nil {
			t.Fatal(err)
		}
		s := string(f["manifest.yaml"])
		i := strings.Index(s, "chain_of_custody:\n")
		f["manifest.yaml"] = []byte(s[:i] + renderCustody(edit(append([]CustodyEntry(nil), m.ChainOfCustody...))))
	}
}

func editText(name, old, new string) func(map[string][]byte) {
	return func(f map[string][]byte) {
		f[name] = bytes.Replace(f[name], []byte(old), []byte(new), 1)
	}
}

func threeEntries() []CustodyEntry {
	return []CustodyEntry{
		{TimestampUTC: "2026-10-23T12:20:00Z", Actor: "stevie@builder-1", Action: "reviewed SBOM"},
		{TimestampUTC: "2026-10-23T12:30:00Z", Actor: "stevie@builder-1", Action: "approved for staging"},
	}
}

// --- verify-gates -----------------------------------------------------------

func TestVerifyGates(t *testing.T) {
	fx := buildFixturePack(t, packOpts{})
	if rc, out, errs := runCLI(t, "verify-gates", fx.pack); rc != exitOK {
		t.Fatalf("fixture: rc=%d out=%s err=%s", rc, out, errs)
	}
	fail := map[string]string{
		"recomputed CRITICAL differs": fx.tampered("g1.tar.gz", editText("cve-scan.json", `"Severity":"LOW"`, `"Severity":"CRITICAL"`)),
		"recomputed HIGH differs":     fx.tampered("g2.tar.gz", editText("cve-scan.json", `"Severity":"LOW"`, `"Severity":"HIGH"`)),
		"recomputed MEDIUM differs":   fx.tampered("g3.tar.gz", editText("cve-scan.json", `"Severity":"MEDIUM"`, `"Severity":"LOW"`)),
		"CIS 94":                      fx.tampered("g4.tar.gz", editText("manifest.yaml", "score_percent: 96.5", "score_percent: 94")),
		"Lynis 89":                    fx.tampered("g5.tar.gz", editText("manifest.yaml", "lynis_hardening_score: 91", "lynis_hardening_score: 89")),
		"doctor exit 1":               fx.tampered("g6.tar.gz", editText("manifest.yaml", "  exit_code: 0", "  exit_code: 1")),
		"patch_count 51":              fx.tampered("g7.tar.gz", editText("manifest.yaml", "patch_count: 0", "patch_count: 51")),
		"loc 5001":                    fx.tampered("g8.tar.gz", editText("manifest.yaml", "total_loc_delta: 0", "total_loc_delta: 5001")),
		"manifest critical 1":         fx.tampered("g9.tar.gz", editText("manifest.yaml", "  critical: 0", "  critical: 1")),
		"cis json disagrees":          fx.tampered("g10.tar.gz", editText("cis-benchmark.json", "96.5", "97.5")),
		"lynis json disagrees":        fx.tampered("g11.tar.gz", editText("cis-benchmark.json", "91", "92")),
		"doctor json disagrees":       fx.tampered("g12.tar.gz", editText("upc-doctor.json", "0", "3")),
		"cve-scan missing":            fx.tampered("g13.tar.gz", func(f map[string][]byte) { delete(f, "cve-scan.json") }),
		"spdx missing":                fx.tampered("g14.tar.gz", func(f map[string][]byte) { delete(f, "sbom.spdx.json") }),
		"cve-scan not json":           fx.tampered("g15.tar.gz", func(f map[string][]byte) { f["cve-scan.json"] = []byte("TODO") }),
		"unlisted patch file":         fx.tampered("g16.tar.gz", func(f map[string][]byte) { f["overlay-patches/0009-sneaky.patch"] = []byte("x") }),
	}
	if rc, _, _ := runCLI(t, "verify-gates", buildFixturePack(t, packOpts{sbomPackages: 4}).pack); rc != exitFail {
		t.Errorf("SPDX package count 4 vs manifest 3: rc=%d", rc)
	}
	for name, pack := range fail {
		if rc, _, errs := runCLI(t, "verify-gates", pack); rc != exitFail {
			t.Errorf("%s: rc=%d, want 1 (stderr %q)", name, rc, errs)
		}
	}

	patched := buildFixturePack(t, packOpts{patches: map[string]string{"0001-a.patch": "diff a\n", "0002-b.patch": "diff b\n"}})
	if rc, _, errs := runCLI(t, "verify-gates", patched.pack); rc != exitOK {
		t.Fatalf("pack with patches: rc=%d %s", rc, errs)
	}
	for name, edit := range map[string]func(map[string][]byte){
		"listed patch missing": func(f map[string][]byte) { delete(f, "overlay-patches/0002-b.patch") },
		"patch content changed": func(f map[string][]byte) {
			f["overlay-patches/0001-a.patch"] = []byte("diff A\n")
		},
	} {
		if rc, _, _ := runCLI(t, "verify-gates", patched.tampered(strings.ReplaceAll(name, " ", "-")+".tar.gz", edit)); rc != exitFail {
			t.Errorf("%s: rc=%d, want 1", name, rc)
		}
	}
}

func TestVerifyGates_Waivers(t *testing.T) {
	high := []vuln{{ID: "CVE-2026-12345", Severity: "HIGH"}}
	waiver := Waiver{CVEID: "CVE-2026-12345", Reason: "not reachable", Approver: "Round Table", ExpiresDate: farFuture()}
	logged := CustodyEntry{TimestampUTC: "2026-10-23T12:20:00Z", Actor: "stevie@builder-1", Action: "waiver CVE-2026-12345 approved by Round Table"}

	ok := buildFixturePack(t, packOpts{vulns: high, waivers: []Waiver{waiver}, custody: []CustodyEntry{logged}})
	if rc, _, errs := runCLI(t, "verify-gates", ok.pack); rc != exitOK {
		t.Fatalf("waived HIGH, logged in custody: rc=%d %s", rc, errs)
	}
	noWaiver := buildFixturePack(t, packOpts{vulns: high})
	if rc, _, errs := runCLI(t, "verify-gates", noWaiver.pack); rc != exitFail || !strings.Contains(errs, "CVE-2026-12345") {
		t.Errorf("HIGH without waiver: rc=%d %s", rc, errs)
	}
	notLogged := buildFixturePack(t, packOpts{vulns: high, waivers: []Waiver{waiver}})
	if rc, _, errs := runCLI(t, "verify-gates", notLogged.pack); rc != exitFail || !strings.Contains(errs, "chain_of_custody") {
		t.Errorf("waiver missing from custody: rc=%d %s", rc, errs)
	}
	expired := waiver
	expired.ExpiresDate = time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	exp := buildFixturePack(t, packOpts{vulns: high, waivers: []Waiver{expired}, custody: []CustodyEntry{logged}})
	if rc, _, errs := runCLI(t, "verify-gates", exp.pack); rc != exitFail || !strings.Contains(errs, "expired") {
		t.Errorf("expired waiver: rc=%d %s", rc, errs)
	}
	crit := buildFixturePack(t, packOpts{vulns: []vuln{{ID: "CVE-2026-12345", Severity: "CRITICAL"}}, waivers: []Waiver{waiver}, custody: []CustodyEntry{logged}})
	if rc, _, errs := runCLI(t, "verify-gates", crit.pack); rc != exitOK {
		t.Errorf("waived CRITICAL, logged: rc=%d %s", rc, errs)
	}
}

// --- verify-custody ---------------------------------------------------------

func TestVerifyCustody(t *testing.T) {
	fx := buildFixturePack(t, packOpts{custody: threeEntries(), signCustody: true})
	if rc, out, errs := runCLI(t, "verify-custody", "--pubkey", fx.pubkey, fx.pack); rc != exitOK {
		t.Fatalf("signed chain: rc=%d out=%s err=%s", rc, out, errs)
	}
	otherSeed := filepath.Join(fx.dir, "other.key")
	writeSeed(t, otherSeed, 0o600)
	otherSigner, err := loadSigner(otherSeed)
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]func(map[string][]byte){
		"reordered": withCustody(t, func(e []CustodyEntry) []CustodyEntry {
			e[1], e[2] = e[2], e[1]
			e[1].TimestampUTC, e[2].TimestampUTC = e[2].TimestampUTC, e[1].TimestampUTC // keep timestamps monotonic
			return e
		}),
		"middle removed": withCustody(t, func(e []CustodyEntry) []CustodyEntry { return append(e[:1], e[2:]...) }),
		"first removed":  withCustody(t, func(e []CustodyEntry) []CustodyEntry { return e[1:] }),
		"action edited": withCustody(t, func(e []CustodyEntry) []CustodyEntry {
			e[1].Action = "approved for production"
			return e
		}),
		"unsigned entry": withCustody(t, func(e []CustodyEntry) []CustodyEntry {
			e[2].Signature = ""
			return e
		}),
		"signature not hex": withCustody(t, func(e []CustodyEntry) []CustodyEntry {
			e[0].Signature = "zz"
			return e
		}),
		"signed by another key": withCustody(t, func(e []CustodyEntry) []CustodyEntry {
			sig, err := otherSigner.SignEvidenceDetached(custodyMessages(e)[2], purposeCustody)
			if err != nil {
				t.Fatal(err)
			}
			e[2].Signature = hex.EncodeToString(sig)
			return e
		}),
		"appended unsigned": withCustody(t, func(e []CustodyEntry) []CustodyEntry {
			return append(e, CustodyEntry{TimestampUTC: "2026-10-24T00:00:00Z", Actor: "mallory", Action: "nothing to see"})
		}),
	}
	for name, edit := range cases {
		if rc, _, errs := runCLI(t, "verify-custody", "--pubkey", fx.pubkey, fx.tampered(strings.ReplaceAll(name, " ", "-")+".tar.gz", edit)); rc != exitFail {
			t.Errorf("%s: rc=%d, want 1 (%s)", name, rc, errs)
		}
	}

	// A decreasing timestamp fails even when every entry is validly signed.
	back := buildFixturePack(t, packOpts{signCustody: true, custody: []CustodyEntry{
		{TimestampUTC: "2026-10-23T11:00:00Z", Actor: "stevie@builder-1", Action: "backdated"},
	}})
	if rc, _, errs := runCLI(t, "verify-custody", "--pubkey", back.pubkey, back.pack); rc != exitFail || !strings.Contains(errs, "before") {
		t.Errorf("decreasing timestamp: rc=%d %s", rc, errs)
	}

	unsigned := buildFixturePack(t, packOpts{custody: threeEntries()})
	if rc, _, _ := runCLI(t, "verify-custody", "--pubkey", unsigned.pubkey, unsigned.pack); rc != exitFail {
		t.Errorf("unsigned chain: rc=%d", rc)
	}
	rc, _, errs := runCLI(t, "verify-custody", "--allow-unsigned-entries", "--pubkey", unsigned.pubkey, unsigned.pack)
	if rc != exitFail || strings.Count(errs, "UNSIGNED") != 3 {
		t.Errorf("--allow-unsigned-entries: rc=%d (must still be 1), stderr=%q", rc, errs)
	}
	if rc, _, _ := runCLI(t, "verify-custody", "--pubkey", fx.otherPubkey(), fx.pack); rc != exitFail {
		t.Errorf("other pubkey: rc=%d", rc)
	}
}

// custodyGolden pins the chained construction. Cross-checked with an
// independent Python implementation of the documented layout (struct.pack(">I")
// length prefixes, hashlib.sha256 links).
const custodyGolden = "af5ecaf6ca869de585f2f7e9d62be3edcf934189602fb79511fad258f038b6f8"

// The custody chain message is a fixed, documented construction: pin it.
func TestCustodyMessageGolden(t *testing.T) {
	msgs := custodyMessages([]CustodyEntry{{TimestampUTC: "2026-10-23T12:10:00Z", Actor: "a", Action: "b"}, {TimestampUTC: "2026-10-23T12:11:00Z", Actor: "c", Action: "d"}})
	h0 := sha256.Sum256(msgs[0])
	if !bytes.HasSuffix(msgs[1], h0[:]) || !bytes.HasSuffix(msgs[0], make([]byte, 32)) {
		t.Error("chain link is not SHA-256 of the previous message")
	}
	if !bytes.HasPrefix(msgs[0], append([]byte{0, 0, 0, 20}, []byte("yggdrasil/custody/v1")...)) {
		t.Errorf("domain prefix missing: %x", msgs[0][:24])
	}
	h1 := sha256.Sum256(msgs[1])
	if got := hex.EncodeToString(h1[:]); got != custodyGolden {
		t.Errorf("sha256(custody message 1) = %s, golden %s: the signed construction changed, which breaks every signed pack", got, custodyGolden)
	}
}

// --- verify -----------------------------------------------------------------

func TestVerify_AllStages(t *testing.T) {
	fx := buildFixturePack(t, packOpts{custody: threeEntries(), signCustody: true})
	rc, out, errs := runCLI(t, "verify", "--pubkey", fx.pubkey, "--iso", fx.iso, fx.pack)
	if rc != exitOK {
		t.Fatalf("full verify: rc=%d out=%s err=%s", rc, out, errs)
	}
	for _, stage := range []string{"validate", "signature", "gates", "custody", "iso"} {
		if !strings.Contains(out, stage+": OK") {
			t.Errorf("stage %s not reported OK:\n%s", stage, out)
		}
	}
	if rc, out, _ := runCLI(t, "verify", "--pubkey", fx.pubkey, fx.pack); rc != exitOK || !strings.Contains(out, "iso: SKIPPED") {
		t.Errorf("verify without --iso: rc=%d out=%s", rc, out)
	}
}

// --- diff -------------------------------------------------------------------

func TestDiff(t *testing.T) {
	old := buildFixturePack(t, packOpts{patches: map[string]string{"0001-a.patch": "a\n", "0002-b.patch": "b\n"}})
	same, out, errs := runCLI(t, "diff", old.pack, old.pack)
	if same != exitOK || !strings.Contains(out, "no differences") {
		t.Errorf("identical: rc=%d out=%s err=%s", same, out, errs)
	}
	newer := old.tampered("new.tar.gz", func(f map[string][]byte) {
		m := string(f["manifest.yaml"])
		m = strings.Replace(m, `release_codename: "bookworm"`, `release_codename: "trixie"`, 1)
		m = strings.Replace(m, "package_count: 3", "package_count: 5", 1)
		m = strings.Replace(m, "MIT: 2", "MIT: 4", 1)
		m = strings.Replace(m, "  medium: 2", "  medium: 1", 1)
		m = strings.Replace(m, "score_percent: 96.5", "score_percent: 97", 1)
		m = strings.Replace(m, "lynis_hardening_score: 91", "lynis_hardening_score: 93", 1)
		// patches: drop 0002, change 0001's sha, add 0003
		i := strings.Index(m, "    - filename: \"0002-b.patch\"")
		j := strings.Index(m, "  total_loc_delta:")
		m = m[:i] + "    - filename: \"0003-c.patch\"\n      sha256: \"" + strings.Repeat("c", 64) + "\"\n      subject: \"s\"\n      reason: \"r\"\n      upstream_status: \"submitted\"\n      authored_date: \"2026-10-01\"\n" + m[j:]
		k := strings.Index(m, "      sha256: \"") + len("      sha256: \"")
		m = m[:k] + strings.Repeat("d", 64) + m[k+64:]
		m += "  - timestamp_utc: \"2026-11-01T00:00:00Z\"\n    actor: \"stevie@builder-1\"\n    action: \"quarterly rebuild\"\n"
		f["manifest.yaml"] = []byte(m)
	})
	rc, out, errs := runCLI(t, "diff", old.pack, newer)
	if rc != exitOK {
		t.Fatalf("diff: rc=%d err=%s", rc, errs)
	}
	for _, want := range []string{
		"anchor.release_codename: bookworm -> trixie",
		"overlay.patches: + 0003-c.patch",
		"overlay.patches: - 0002-b.patch",
		"overlay.patches: ~ 0001-a.patch",
		"sbom.package_count: 3 -> 5 (+2)",
		"sbom.license_summary.MIT: 2 -> 4 (+2)",
		"cve_scan.medium: 2 -> 1 (-1)",
		"cis_benchmark.score_percent: 96.5 -> 97",
		"cis_benchmark.lynis_hardening_score: 91 -> 93 (+2)",
		"chain_of_custody: +1 entries appended",
		"quarterly rebuild",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("diff output missing %q:\n%s", want, out)
		}
	}

	rewritten := old.tampered("rewritten.tar.gz", withCustody(t, func(e []CustodyEntry) []CustodyEntry {
		e[0].Actor = "someone-else"
		return e
	}))
	rc, _, errs = runCLI(t, "diff", old.pack, rewritten)
	if rc != 3 || !strings.Contains(errs, "REWRITTEN") {
		t.Errorf("rewritten history: rc=%d err=%s", rc, errs)
	}
	// Swapping the arguments: the "new" pack lost the appended entry.
	if rc, _, errs := runCLI(t, "diff", newer, old.pack); rc != 3 || !strings.Contains(errs, "REWRITTEN") {
		t.Errorf("custody truncated: rc=%d err=%s", rc, errs)
	}
	if rc, _, _ := runCLI(t, "diff", old.pack); rc != exitUsage {
		t.Errorf("one arg: rc=%d", rc)
	}
	if rc, _, _ := runCLI(t, "diff", old.pack, filepath.Join(old.dir, "nope.tar.gz")); rc != exitUsage {
		t.Errorf("missing pack: rc=%d", rc)
	}
}

// --- end to end (scripts/check-evidence-pack.sh runs -run E2E) --------------

func TestE2E_SignVerifyAndEveryTamperFails(t *testing.T) {
	high := []vuln{{ID: "CVE-2026-12345", Severity: "HIGH"}}
	waiver := Waiver{CVEID: "CVE-2026-12345", Reason: "not reachable", Approver: "Round Table", ExpiresDate: farFuture()}
	fx := buildFixturePack(t, packOpts{
		vulns: high, waivers: []Waiver{waiver}, signCustody: true,
		patches: map[string]string{"0001-sysctl.patch": "diff\n"},
		custody: append([]CustodyEntry{{TimestampUTC: "2026-10-23T12:15:00Z", Actor: "stevie@builder-1", Action: "waiver CVE-2026-12345 approved by Round Table"}}, threeEntries()...),
	})
	verify := func(pack, iso string) (int, string) {
		rc, out, errs := runCLI(t, "verify", "--pubkey", fx.pubkey, "--iso", iso, pack)
		return rc, out + errs
	}
	if rc, out := verify(fx.pack, fx.iso); rc != exitOK {
		t.Fatalf("E2E signed pack does not verify (rc=%d):\n%s", rc, out)
	}
	badISO := filepath.Join(fx.dir, "bad.iso")
	if err := os.WriteFile(badISO, append(append([]byte(nil), fx.isoBytes...), 0), 0o600); err != nil {
		t.Fatal(err)
	}
	tampers := map[string]string{
		"manifest byte":      fx.tampered("e1.tar.gz", editText("manifest.yaml", "builder-1\"\n", "builder-9\"\n")),
		"manifest seal swap": fx.tampered("e2.tar.gz", func(f map[string][]byte) { f["manifest.yaml.sig"] = f["image.iso.sha256.sig"] }),
		"iso line":           fx.tampered("e3.tar.gz", editText("image.iso.sha256", "a", "b")),
		"cve-scan edited":    fx.tampered("e4.tar.gz", editText("cve-scan.json", `"Severity":"LOW"`, `"Severity":"CRITICAL"`)),
		"sbom edited":        fx.tampered("e5.tar.gz", editText("sbom.spdx.json", `{"SPDXID":"SPDXRef-Package-0"`, `{"SPDXID":"x"},{"SPDXID":"SPDXRef-Package-0"`)),
		"patch edited":       fx.tampered("e6.tar.gz", editText("overlay-patches/0001-sysctl.patch", "diff", "DIFF")),
		"custody reordered":  fx.tampered("e7.tar.gz", withCustody(t, func(e []CustodyEntry) []CustodyEntry { e[2], e[3] = e[3], e[2]; return e })),
		"manifest sig gone":  fx.tampered("e8.tar.gz", func(f map[string][]byte) { delete(f, "manifest.yaml.sig") }),
		"unsigned pack (todo)": fx.tampered("e9.tar.gz", func(f map[string][]byte) {
			delete(f, "manifest.yaml.sig")
			delete(f, "image.iso.sha256.sig")
			f["manifest.yaml.sig.todo"] = []byte("TODO(task #65)")
		}),
	}
	for name, pack := range tampers {
		if rc, out := verify(pack, fx.iso); rc != exitFail {
			t.Errorf("tamper %q: rc=%d, want 1\n%s", name, rc, out)
		}
	}
	if rc, out := verify(fx.pack, badISO); rc != exitFail {
		t.Errorf("tamper ISO: rc=%d, want 1\n%s", rc, out)
	}
	traversal := filepath.Join(fx.dir, "traversal.tar.gz")
	if err := os.WriteFile(traversal, makeTarGz(t, reg("manifest/../../etc/cron.d/x", "* * * * * root id\n")), 0o600); err != nil {
		t.Fatal(err)
	}
	if rc, out := verify(traversal, fx.iso); rc != exitFail {
		t.Errorf("traversal pack: rc=%d\n%s", rc, out)
	}
	// Other key: everything signed fails.
	if rc, _, _ := runCLI(t, "verify", "--pubkey", fx.otherPubkey(), "--iso", fx.iso, fx.pack); rc != exitFail {
		t.Errorf("other key: rc=%d", rc)
	}
}
