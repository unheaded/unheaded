// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cloudflare/circl/sign/mldsa/mldsa65"
)

// Test fixtures for whole evidence packs. Keys are throwaway seeds written
// 0600 into t.TempDir(); nothing here is a real key and nothing is printed.

type vuln struct{ ID, Severity string }

type packOpts struct {
	// editManifest rewrites the manifest YAML before anything is signed.
	editManifest func(string) string
	sbomPackages int // default 3 (matches minimal.yaml's package_count)
	vulns        []vuln
	waivers      []Waiver
	custody      []CustodyEntry // extra entries after the build entry
	patches      map[string]string
	cisJSON      string // default {"score_percent": 96.5, "lynis_hardening_score": 91}
	doctorJSON   string // default {"exit_code": 0}
	// signCustody signs every chain_of_custody entry (sign --purpose custody-entry).
	signCustody bool
}

type fixture struct {
	t        *testing.T
	dir      string
	seed     string // 0600 seed path
	pubkey   string // derived public key path
	iso      string
	isoBytes []byte
	pack     string
	files    map[string][]byte // pack files below manifest/
}

func writeSeed(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	seed := make([]byte, mldsa65.SeedSize)
	if _, err := rand.Read(seed); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, seed, mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

func mustRun(t *testing.T, args ...string) string {
	t.Helper()
	rc, out, errs := runCLI(t, args...)
	if rc != exitOK {
		t.Fatalf("%v: rc=%d\nstdout=%s\nstderr=%s", args, rc, out, errs)
	}
	return out
}

func yq(s string) string { return strconv.Quote(s) }

func renderCustody(entries []CustodyEntry) string {
	var b strings.Builder
	b.WriteString("chain_of_custody:\n")
	for _, e := range entries {
		fmt.Fprintf(&b, "  - timestamp_utc: %s\n    actor: %s\n    action: %s\n", yq(e.TimestampUTC), yq(e.Actor), yq(e.Action))
		if e.Signature != "" {
			fmt.Fprintf(&b, "    signature: %s\n", yq(e.Signature))
		}
	}
	return b.String()
}

func renderWaivers(ws []Waiver) string {
	if len(ws) == 0 {
		return "  waivers: []\n"
	}
	var b strings.Builder
	b.WriteString("  waivers:\n")
	for _, w := range ws {
		fmt.Fprintf(&b, "    - cve_id: %s\n      reason: %s\n      approver: %s\n      expires_date: %s\n",
			yq(w.CVEID), yq(w.Reason), yq(w.Approver), yq(w.ExpiresDate))
	}
	return b.String()
}

// manifestText renders minimal.yaml for this ISO, waivers and custody.
func manifestText(t *testing.T, isoSHA string, isoSize int, o packOpts, custody []CustodyEntry) string {
	t.Helper()
	base, err := os.ReadFile("testdata/manifests/valid/minimal.yaml")
	if err != nil {
		t.Fatal(err)
	}
	s := string(base)
	s = strings.Replace(s, `iso_sha256: "`+strings.Repeat("0", 64)+`"`, `iso_sha256: "`+isoSHA+`"`, 1)
	s = strings.Replace(s, "iso_size_bytes: 4096", "iso_size_bytes: "+strconv.Itoa(isoSize), 1)
	s = strings.Replace(s, "  waivers: []\n", renderWaivers(o.waivers), 1)
	if len(o.patches) > 0 {
		var names []string
		for n := range o.patches {
			names = append(names, n)
		}
		sort.Strings(names)
		var b strings.Builder
		fmt.Fprintf(&b, "  patch_count: %d\n  patches:\n", len(names))
		for _, n := range names {
			h := sha256.Sum256([]byte(o.patches[n]))
			fmt.Fprintf(&b, "    - filename: %s\n      sha256: %s\n      subject: \"s\"\n      reason: \"r\"\n      upstream_status: \"downstream-only\"\n      authored_date: \"2026-05-11\"\n",
				yq(n), yq(hex.EncodeToString(h[:])))
		}
		s = strings.Replace(s, "  patch_count: 0\n  patches: []\n", b.String(), 1)
	}
	i := strings.Index(s, "chain_of_custody:\n")
	if i < 0 {
		t.Fatal("minimal.yaml has no chain_of_custody")
	}
	s = s[:i] + renderCustody(custody)
	if o.editManifest != nil {
		s = o.editManifest(s)
	}
	return s
}

func tarGz(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: "manifest/", Typeflag: tar.TypeDir, Mode: 0o755}); err != nil {
		t.Fatal(err)
	}
	var names []string
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	dirs := map[string]bool{}
	for _, n := range names {
		if d := filepath.Dir(n); d != "." && !dirs[d] {
			dirs[d] = true
			if err := tw.WriteHeader(&tar.Header{Name: "manifest/" + d + "/", Typeflag: tar.TypeDir, Mode: 0o755}); err != nil {
				t.Fatal(err)
			}
		}
		b := files[n]
		if err := tw.WriteHeader(&tar.Header{Name: "manifest/" + n, Typeflag: tar.TypeReg, Mode: 0o644, Size: int64(len(b))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(b); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// buildFixturePack writes a fake ISO, a signed pack for it, and the key pair.
func buildFixturePack(t *testing.T, o packOpts) *fixture {
	t.Helper()
	dir := t.TempDir()
	fx := &fixture{t: t, dir: dir, seed: filepath.Join(dir, "build.key"), pubkey: filepath.Join(dir, "build.pubkey"), iso: filepath.Join(dir, "yggdrasil-0.1.0-amd64.iso")}
	writeSeed(t, fx.seed, 0o600)
	mustRun(t, "pubkey", "--key", fx.seed, "--out", fx.pubkey)

	fx.isoBytes = []byte("fake ISO 9660 image for tests\x00\x01\x02")
	if err := os.WriteFile(fx.iso, fx.isoBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(fx.isoBytes)
	isoSHA := hex.EncodeToString(sum[:])

	custody := append([]CustodyEntry{{
		TimestampUTC: "2026-10-23T12:10:00Z",
		Actor:        "yggdrasil-build-pipeline@builder-1",
		Action:       "build image yggdrasil-0.1.0-amd64.iso",
	}}, o.custody...)
	mtext := manifestText(t, isoSHA, len(fx.isoBytes), o, custody)
	mpath := filepath.Join(dir, "manifest.yaml")
	if o.signCustody {
		if err := os.WriteFile(mpath, []byte(mtext), 0o600); err != nil {
			t.Fatal(err)
		}
		for i := range custody {
			out := filepath.Join(dir, fmt.Sprintf("custody-%d.sig", i))
			mustRun(t, "sign", "--key", fx.seed, "--in", mpath, "--purpose", "custody-entry", "--custody-index", strconv.Itoa(i), "--out", out)
			b, err := os.ReadFile(out) // #nosec G304 -- test temp file
			if err != nil {
				t.Fatal(err)
			}
			custody[i].Signature = strings.TrimSpace(string(b))
		}
		mtext = manifestText(t, isoSHA, len(fx.isoBytes), o, custody)
	}
	if err := os.WriteFile(mpath, []byte(mtext), 0o600); err != nil {
		t.Fatal(err)
	}
	shaLine := isoSHA + "  yggdrasil-0.1.0-amd64.iso\n"
	shaPath := filepath.Join(dir, "image.iso.sha256")
	if err := os.WriteFile(shaPath, []byte(shaLine), 0o600); err != nil {
		t.Fatal(err)
	}
	mustRun(t, "sign", "--key", fx.seed, "--in", mpath, "--out", mpath+".sig", "--purpose", "manifest")
	mustRun(t, "sign", "--key", fx.seed, "--in", shaPath, "--out", shaPath+".sig", "--purpose", "iso-sha256")

	read := func(p string) []byte {
		b, err := os.ReadFile(p) // #nosec G304 -- test temp file
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	n := o.sbomPackages
	if n == 0 {
		n = 3
	}
	var pkgs []map[string]string
	for i := 0; i < n; i++ {
		pkgs = append(pkgs, map[string]string{"name": fmt.Sprintf("pkg%d", i), "SPDXID": fmt.Sprintf("SPDXRef-Package-%d", i)})
	}
	spdx, _ := json.Marshal(map[string]any{"spdxVersion": "SPDX-2.3", "packages": pkgs})
	var vs []map[string]string
	for _, v := range o.vulns {
		vs = append(vs, map[string]string{"VulnerabilityID": v.ID, "Severity": v.Severity})
	}
	medLow := []map[string]string{
		{"VulnerabilityID": "CVE-2026-0001", "Severity": "MEDIUM"},
		{"VulnerabilityID": "CVE-2026-0002", "Severity": "MEDIUM"},
		{"VulnerabilityID": "CVE-2026-0003", "Severity": "LOW"},
		{"VulnerabilityID": "CVE-2026-0004", "Severity": "LOW"},
		{"VulnerabilityID": "CVE-2026-0005", "Severity": "LOW"},
		{"VulnerabilityID": "CVE-2026-0006", "Severity": "LOW"},
		{"VulnerabilityID": "CVE-2026-0007", "Severity": "LOW"},
	}
	cve, _ := json.Marshal(map[string]any{"SchemaVersion": 2, "Results": []any{
		map[string]any{"Target": "rootfs", "Vulnerabilities": medLow},
		map[string]any{"Target": "extra", "Vulnerabilities": vs},
	}})
	cis := o.cisJSON
	if cis == "" {
		cis = `{"score_percent": 96.5, "lynis_hardening_score": 91}`
	}
	doctor := o.doctorJSON
	if doctor == "" {
		doctor = `{"exit_code": 0}`
	}
	fx.files = map[string][]byte{
		"manifest.yaml":        read(mpath),
		"manifest.yaml.sig":    read(mpath + ".sig"),
		"image.iso.sha256":     read(shaPath),
		"image.iso.sha256.sig": read(shaPath + ".sig"),
		"sbom.spdx.json":       spdx,
		"sbom.cyclonedx.json":  []byte(`{"bomFormat":"CycloneDX","specVersion":"1.5"}`),
		"cve-scan.json":        cve,
		"cis-benchmark.json":   []byte(cis),
		"upc-doctor.json":      []byte(doctor),
		"chain-of-custody.txt": []byte("see manifest.yaml chain_of_custody\n"),
		"anchor.json":          []byte(`{"release_codename":"bookworm"}`),
	}
	for n, c := range o.patches {
		fx.files["overlay-patches/"+n] = []byte(c)
	}
	fx.pack = fx.writePack("evidence-pack.tar.gz", fx.files)
	return fx
}

// writePack tars files into dir/name and returns the path.
func (fx *fixture) writePack(name string, files map[string][]byte) string {
	fx.t.Helper()
	p := filepath.Join(fx.dir, name)
	if err := os.WriteFile(p, tarGz(fx.t, files), 0o600); err != nil {
		fx.t.Fatal(err)
	}
	return p
}

// tampered returns a pack whose files were edited AFTER signing.
func (fx *fixture) tampered(name string, edit func(map[string][]byte)) string {
	fx.t.Helper()
	files := map[string][]byte{}
	for k, v := range fx.files {
		files[k] = append([]byte(nil), v...)
	}
	edit(files)
	return fx.writePack(name, files)
}

// otherPubkey writes a different, valid public key.
func (fx *fixture) otherPubkey() string {
	fx.t.Helper()
	seed := filepath.Join(fx.dir, "other.key")
	writeSeed(fx.t, seed, 0o600)
	pub := filepath.Join(fx.dir, "other.pubkey")
	mustRun(fx.t, "pubkey", "--key", seed, "--out", pub)
	return pub
}

func farFuture() string { return time.Now().AddDate(2, 0, 0).Format("2006-01-02") }
