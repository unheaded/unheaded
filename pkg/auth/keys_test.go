// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package auth

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// Every key in these tests is generated at run time; none is in the repo.

func TestGenerateKeyPair_WritesSeedAndPub(t *testing.T) {
	dir := t.TempDir()
	kid, err := GenerateKeyPair(dir, "cuirass")
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}

	keyPath := filepath.Join(dir, "cuirass.key")
	pubPath := filepath.Join(dir, "cuirass.pub")
	for path, want := range map[string]os.FileMode{keyPath: 0o400, pubPath: 0o444} {
		st, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %s: %v", path, err)
		}
		if got := st.Mode().Perm(); got != want {
			t.Errorf("%s mode %o, want %o", filepath.Base(path), got, want)
		}
	}

	raw, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	blk, rest := pem.Decode(raw)
	if blk == nil || blk.Type != "UNHEADED ED25519 SEED" || len(blk.Bytes) != ed25519.SeedSize {
		t.Fatalf("key file is not a 32-byte UNHEADED ED25519 SEED PEM block: %+v", blk)
	}
	if len(rest) != 0 {
		t.Errorf("trailing bytes after the seed block: %q", rest)
	}

	rawPub, err := os.ReadFile(pubPath)
	if err != nil {
		t.Fatal(err)
	}
	pb, _ := pem.Decode(rawPub)
	if pb == nil || pb.Type != "PUBLIC KEY" {
		t.Fatalf("pub file is not a PUBLIC KEY PEM block: %+v", pb)
	}

	priv := ed25519.NewKeyFromSeed(blk.Bytes)
	sum := sha256.Sum256(priv.Public().(ed25519.PublicKey))
	if want := hex.EncodeToString(sum[:])[:16]; kid != want {
		t.Errorf("kid %q, want first 16 hex of SHA-256(pub) %q", kid, want)
	}

	loaded, lkid, err := LoadPrivateKey(keyPath)
	if err != nil {
		t.Fatalf("LoadPrivateKey: %v", err)
	}
	if lkid != kid || !loaded.Equal(priv) {
		t.Errorf("LoadPrivateKey returned kid %q / a different key", lkid)
	}
}

func TestGenerateKeyPair_RefusesOverwrite(t *testing.T) {
	dir := t.TempDir()
	if _, err := GenerateKeyPair(dir, "sword"); err != nil {
		t.Fatal(err)
	}
	if _, err := GenerateKeyPair(dir, "sword"); err == nil {
		t.Fatal("a second GenerateKeyPair overwrote an existing key")
	}
}

func TestGenerateKeyPair_RefusesBadNames(t *testing.T) {
	for _, name := range []string{"", ".", "..", "../etc/x", "a/b", "A", ".hidden", "a.b", "-x", "a b", string(make([]byte, 80))} {
		if _, err := GenerateKeyPair(t.TempDir(), name); err == nil {
			t.Errorf("GenerateKeyPair accepted name %q", name)
		}
	}
}

func TestLoadPrivateKey_RefusesWorldReadable(t *testing.T) {
	dir := t.TempDir()
	if _, err := GenerateKeyPair(dir, "wotan"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "wotan.key")
	for _, mode := range []os.FileMode{0o644, 0o440, 0o404, 0o600 | 0o004} {
		if err := os.Chmod(path, mode); err != nil {
			t.Fatal(err)
		}
		if _, _, err := LoadPrivateKey(path); !errors.Is(err, ErrKeyPermissions) {
			t.Errorf("mode %o: err %v, want ErrKeyPermissions", mode, err)
		}
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadPrivateKey(path); err != nil {
		t.Errorf("mode 0600 refused: %v", err)
	}
}

func TestLoadPrivateKey_RefusesGarbage(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string][]byte{
		"empty":     nil,
		"not-pem":   []byte("hello"),
		"wrongtype": pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: make([]byte, 32)}),
		"short":     pem.EncodeToMemory(&pem.Block{Type: seedPEMType, Bytes: make([]byte, 31)}),
		"two-blocks": append(pem.EncodeToMemory(&pem.Block{Type: seedPEMType, Bytes: make([]byte, 32)}),
			pem.EncodeToMemory(&pem.Block{Type: seedPEMType, Bytes: make([]byte, 32)})...),
	} {
		p := filepath.Join(dir, name+".key")
		if err := os.WriteFile(p, body, 0o400); err != nil {
			t.Fatal(err)
		}
		if _, _, err := LoadPrivateKey(p); err == nil {
			t.Errorf("%s: LoadPrivateKey accepted it", name)
		}
	}
}

func TestTrustDir_LoadsOnlyPub(t *testing.T) {
	keys := t.TempDir()
	kidA, err := GenerateKeyPair(keys, "a")
	if err != nil {
		t.Fatal(err)
	}
	kidB, err := GenerateKeyPair(keys, "b")
	if err != nil {
		t.Fatal(err)
	}

	trust := t.TempDir()
	copyFile(t, filepath.Join(keys, "a.pub"), filepath.Join(trust, "a.pub"))
	copyFile(t, filepath.Join(keys, "b.pub"), filepath.Join(trust, "b.pub"))
	if err := os.WriteFile(filepath.Join(trust, "junk.txt"), []byte("not a key"), 0o644); err != nil {
		t.Fatal(err)
	}

	td, err := NewTrustDir(trust)
	if err != nil {
		t.Fatalf("NewTrustDir: %v", err)
	}
	if got := len(td.Kids()); got != 2 {
		t.Fatalf("Kids() = %d keys, want 2", got)
	}
	for kid, name := range map[string]string{kidA: "a", kidB: "b"} {
		pub, ok := td.Lookup(kid)
		if !ok || len(pub) != ed25519.PublicKeySize {
			t.Errorf("Lookup(%s) failed", kid)
		}
		if _, n, ok := td.LookupName(kid); !ok || n != name {
			t.Errorf("LookupName(%s) = %q, want %q", kid, n, name)
		}
	}

	// A private key in the public directory is a misconfiguration, never
	// something to skip quietly.
	copyFile(t, filepath.Join(keys, "a.key"), filepath.Join(trust, "c.key"))
	if _, err := NewTrustDir(trust); err == nil {
		t.Fatal("NewTrustDir accepted a .key file in trust.d")
	}
	if err := td.Reload(); err == nil {
		t.Fatal("Reload accepted a .key file in trust.d")
	}
	// A failed reload keeps the last good set.
	if _, ok := td.Lookup(kidA); !ok {
		t.Error("failed Reload dropped the previous keys")
	}
}

func TestTrustDir_Reload_RemovedKeyRevokes(t *testing.T) {
	trust := t.TempDir()
	if _, err := GenerateKeyPair(trust, "x"); err != nil {
		t.Fatal(err)
	}
	// GenerateKeyPair writes x.key next to x.pub; trust.d holds only .pub.
	if err := os.Remove(filepath.Join(trust, "x.key")); err != nil {
		t.Fatal(err)
	}
	keys := t.TempDir()
	kidB, err := GenerateKeyPair(keys, "b")
	if err != nil {
		t.Fatal(err)
	}
	copyFile(t, filepath.Join(keys, "b.pub"), filepath.Join(trust, "b.pub"))

	td, err := NewTrustDir(trust)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := td.Lookup(kidB); !ok {
		t.Fatal("b not trusted before removal")
	}
	if err := os.Remove(filepath.Join(trust, "b.pub")); err != nil {
		t.Fatal(err)
	}
	if err := td.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if _, ok := td.Lookup(kidB); ok {
		t.Fatal("removed b.pub still trusted after Reload")
	}
	if len(td.Kids()) != 1 {
		t.Errorf("Kids() = %v, want only x", td.Kids())
	}
}

func TestTrustDir_RotationFilesBindToOneName(t *testing.T) {
	keys1, keys2 := t.TempDir(), t.TempDir()
	kid1, err := GenerateKeyPair(keys1, "cuirass")
	if err != nil {
		t.Fatal(err)
	}
	kid2, err := GenerateKeyPair(keys2, "cuirass")
	if err != nil {
		t.Fatal(err)
	}
	trust := t.TempDir()
	copyFile(t, filepath.Join(keys1, "cuirass.pub"), filepath.Join(trust, "cuirass.pub"))
	copyFile(t, filepath.Join(keys2, "cuirass.pub"), filepath.Join(trust, "cuirass."+kid2+".pub"))
	td, err := NewTrustDir(trust)
	if err != nil {
		t.Fatal(err)
	}
	for _, kid := range []string{kid1, kid2} {
		if _, n, ok := td.LookupName(kid); !ok || n != "cuirass" {
			t.Errorf("LookupName(%s) = %q,%v; want cuirass", kid, n, ok)
		}
	}
}

func TestTrustDir_SameKeyTwoNamesRefused(t *testing.T) {
	keys := t.TempDir()
	if _, err := GenerateKeyPair(keys, "a"); err != nil {
		t.Fatal(err)
	}
	trust := t.TempDir()
	copyFile(t, filepath.Join(keys, "a.pub"), filepath.Join(trust, "a.pub"))
	copyFile(t, filepath.Join(keys, "a.pub"), filepath.Join(trust, "b.pub"))
	if _, err := NewTrustDir(trust); err == nil {
		t.Fatal("one key under two service names was accepted")
	}
}

func TestTrustDir_MissingDir(t *testing.T) {
	if _, err := NewTrustDir(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Fatal("NewTrustDir on a missing directory succeeded")
	}
}

func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	b, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, b, 0o444); err != nil {
		t.Fatal(err)
	}
}
