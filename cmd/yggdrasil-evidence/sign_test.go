// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"unheaded/pkg/gungnir"
)

func TestSign_RefusesReadableKey(t *testing.T) {
	dir := t.TempDir()
	key := filepath.Join(dir, "build.key")
	writeSeed(t, key, 0o644)
	in := filepath.Join(dir, "image.iso.sha256")
	if err := os.WriteFile(in, []byte(strings.Repeat("a", 64)+"  x.iso\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "sig")
	rc, _, errs := runCLI(t, "sign", "--key", key, "--in", in, "--out", out, "--purpose", "iso-sha256")
	if rc != exitFail || !strings.Contains(errs, gungnir.ErrKeyFilePerms.Error()) {
		t.Errorf("0644 key: rc=%d err=%q", rc, errs)
	}
	if _, err := os.Stat(out); err == nil {
		t.Error("signature written despite refusal")
	}
	if rc, _, _ := runCLI(t, "pubkey", "--key", key, "--out", filepath.Join(dir, "pub")); rc != exitFail {
		t.Errorf("pubkey from a 0644 key: rc=%d", rc)
	}
}

func TestSign_Usage(t *testing.T) {
	dir := t.TempDir()
	key := filepath.Join(dir, "k")
	writeSeed(t, key, 0o600)
	in := filepath.Join(dir, "in")
	if err := os.WriteFile(in, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out")
	for name, args := range map[string][]string{
		"no purpose":            {"sign", "--key", key, "--in", in, "--out", out},
		"unknown purpose":       {"sign", "--key", key, "--in", in, "--out", out, "--purpose", "config"},
		"no in":                 {"sign", "--key", key, "--out", out, "--purpose", "manifest"},
		"no out":                {"sign", "--key", key, "--in", in, "--purpose", "manifest"},
		"index without custody": {"sign", "--key", key, "--in", in, "--out", out, "--purpose", "iso-sha256", "--custody-index", "0"},
		"custody without index": {"sign", "--key", key, "--in", in, "--out", out, "--purpose", "custody-entry"},
		"missing input":         {"sign", "--key", key, "--in", filepath.Join(dir, "nope"), "--out", out, "--purpose", "iso-sha256"},
		"pubkey no out":         {"pubkey", "--key", key},
		"pubkey extra arg":      {"pubkey", "--key", key, "--out", out, "extra"},
	} {
		if rc, _, _ := runCLI(t, args...); rc != exitUsage {
			t.Errorf("%s: rc=%d, want 2", name, rc)
		}
	}
	// A manifest that does not validate is not signed.
	bad, err := os.ReadFile("testdata/manifests/invalid/critical-1.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(in, bad, 0o600); err != nil {
		t.Fatal(err)
	}
	if rc, _, errs := runCLI(t, "sign", "--key", key, "--in", in, "--out", out, "--purpose", "manifest"); rc != exitFail || !strings.Contains(errs, "cve_scan.critical") {
		t.Errorf("invalid manifest signed: rc=%d err=%q", rc, errs)
	}
}

func TestPubkey_DerivesTheSigningKey(t *testing.T) {
	fx := buildFixturePack(t, packOpts{})
	pub, err := gungnir.LoadPublicKeyFile(fx.pubkey)
	if err != nil {
		t.Fatal(err)
	}
	seal, err := gungnir.ParseEvidenceSeal(fx.files["manifest.yaml.sig"])
	if err != nil {
		t.Fatal(err)
	}
	if seal.KeyId != gungnir.KeyID(pub) {
		t.Errorf("seal key %s, pubkey %s", seal.KeyId, gungnir.KeyID(pub))
	}
	if err := gungnir.VerifyEvidence(fx.files["manifest.yaml"], seal, pub, "manifest"); err != nil {
		t.Errorf("pubkey does not verify sign's output: %v", err)
	}
	// Deterministic derivation: same seed, same public key.
	again := filepath.Join(fx.dir, "again.pubkey")
	mustRun(t, "pubkey", "--key", fx.seed, "--out", again)
	a, _ := os.ReadFile(fx.pubkey) // #nosec G304 -- test temp file
	b, _ := os.ReadFile(again)     // #nosec G304 -- test temp file
	if !bytes.Equal(a, b) || len(a) == 0 {
		t.Error("pubkey is not a deterministic derivation")
	}
	st, err := os.Stat(fx.pubkey)
	if err != nil || st.Mode().Perm() != 0o644 {
		t.Errorf("pubkey mode %v", st.Mode())
	}
}

func TestVerifySignature(t *testing.T) {
	fx := buildFixturePack(t, packOpts{})
	if rc, out, errs := runCLI(t, "verify-signature", "--pubkey", fx.pubkey, fx.pack); rc != exitOK {
		t.Fatalf("untouched pack: rc=%d out=%s err=%s", rc, out, errs)
	}
	cases := map[string]string{
		"manifest byte changed": fx.tampered("t1.tar.gz", func(f map[string][]byte) {
			f["manifest.yaml"] = bytes.Replace(f["manifest.yaml"], []byte("builder-1\""), []byte("builder-2\""), 1)
		}),
		"manifest sig swapped for the iso seal": fx.tampered("t2.tar.gz", func(f map[string][]byte) {
			f["manifest.yaml.sig"] = f["image.iso.sha256.sig"]
		}),
		"iso sha line changed": fx.tampered("t3.tar.gz", func(f map[string][]byte) {
			f["image.iso.sha256"] = bytes.Replace(f["image.iso.sha256"], []byte("iso"), []byte("img"), 1)
		}),
		"manifest sig missing": fx.tampered("t4.tar.gz", func(f map[string][]byte) { delete(f, "manifest.yaml.sig") }),
		"iso sig missing":      fx.tampered("t5.tar.gz", func(f map[string][]byte) { delete(f, "image.iso.sha256.sig") }),
		"sig not json":         fx.tampered("t6.tar.gz", func(f map[string][]byte) { f["manifest.yaml.sig"] = []byte("TODO(task #65)") }),
	}
	for name, pack := range cases {
		if rc, _, _ := runCLI(t, "verify-signature", "--pubkey", fx.pubkey, pack); rc != exitFail {
			t.Errorf("%s: rc=%d, want 1", name, rc)
		}
	}
	if rc, _, _ := runCLI(t, "verify-signature", "--pubkey", fx.otherPubkey(), fx.pack); rc != exitFail {
		t.Errorf("different pubkey: rc=%d, want 1", rc)
	}
	if rc, _, _ := runCLI(t, "verify-signature", "--pubkey", filepath.Join(fx.dir, "nope"), fx.pack); rc != exitUsage {
		t.Errorf("missing pubkey: rc=%d, want 2", rc)
	}
	if rc, _, _ := runCLI(t, "verify-signature", "--pubkey", fx.pubkey, filepath.Join(fx.dir, "nope.tar.gz")); rc != exitUsage {
		t.Errorf("missing pack: rc=%d, want 2", rc)
	}
	notPack := filepath.Join(fx.dir, "not-a-pack")
	if err := os.WriteFile(notPack, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	if rc, _, _ := runCLI(t, "verify-signature", "--pubkey", fx.pubkey, notPack); rc != exitFail {
		t.Errorf("garbage pack: rc=%d, want 1", rc)
	}
}

func TestVerifyISO(t *testing.T) {
	fx := buildFixturePack(t, packOpts{})
	if rc, out, errs := runCLI(t, "verify-iso", "--iso", fx.iso, "--pubkey", fx.pubkey, fx.pack); rc != exitOK {
		t.Fatalf("right ISO: rc=%d out=%s err=%s", rc, out, errs)
	}
	other := filepath.Join(fx.dir, "other.iso")
	flipped := append([]byte(nil), fx.isoBytes...)
	flipped[3] ^= 1
	if err := os.WriteFile(other, flipped, 0o600); err != nil {
		t.Fatal(err)
	}
	if rc, _, _ := runCLI(t, "verify-iso", "--iso", other, "--pubkey", fx.pubkey, fx.pack); rc != exitFail {
		t.Errorf("one-byte-different ISO: rc=%d, want 1", rc)
	}

	// The manifest disagrees with the (correctly signed) image.iso.sha256.
	mismatch := buildFixturePack(t, packOpts{editManifest: func(s string) string {
		i := strings.Index(s, "iso_sha256: \"")
		return s[:i+13] + strings.Repeat("e", 64) + s[i+13+64:]
	}})
	if rc, _, errs := runCLI(t, "verify-iso", "--iso", mismatch.iso, "--pubkey", mismatch.pubkey, mismatch.pack); rc != exitFail || !strings.Contains(errs, "manifest") {
		t.Errorf("manifest vs image.iso.sha256 mismatch: rc=%d err=%q", rc, errs)
	}

	// An attacker who swaps the ISO and rewrites the unsigned hash line.
	swapped := fx.tampered("swap.tar.gz", func(f map[string][]byte) {
		f["image.iso.sha256"] = []byte(strings.Repeat("f", 64) + "  yggdrasil-0.1.0-amd64.iso\n")
	})
	if rc, _, _ := runCLI(t, "verify-iso", "--iso", fx.iso, "--pubkey", fx.pubkey, swapped); rc != exitFail {
		t.Errorf("rewritten hash line: rc=%d, want 1", rc)
	}
	if rc, _, _ := runCLI(t, "verify-iso", "--pubkey", fx.pubkey, fx.pack); rc != exitUsage {
		t.Errorf("no --iso: rc=%d, want 2", rc)
	}
	if rc, _, _ := runCLI(t, "verify-iso", "--iso", filepath.Join(fx.dir, "nope.iso"), "--pubkey", fx.pubkey, fx.pack); rc != exitUsage {
		t.Errorf("missing ISO: rc=%d, want 2", rc)
	}
}
