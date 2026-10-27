// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

// Package casktest builds Sealed Cask tarballs in memory for tests: good
// casks signed with a throwaway key, and hostile ones (traversal, links,
// devices). Keys are generated at run time and never written to the repo.
package casktest

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"testing"

	"github.com/cloudflare/circl/sign/mldsa/mldsa65"

	"unheaded/pkg/gungnir"
)

// Root is the top-level directory of every cask this package builds.
const Root = "sealed-cask-20261027-000000"

// Purpose is the evidence purpose casks are signed for (sealedcask.Purpose,
// repeated here so this package does not import the one it tests).
const Purpose = "sealed-cask-v1"

// Entry is one raw tar member.
type Entry struct {
	Name     string
	Type     byte // tar.TypeReg when zero
	Body     []byte
	Linkname string
	Mode     int64 // 0o644 when zero
}

// Key is a throwaway ML-DSA-65 build key.
type Key struct {
	Signer *gungnir.Signer
	Pub    *mldsa65.PublicKey
	seed   [mldsa65.SeedSize]byte
}

// NewKey makes a fresh random key.
func NewKey(t testing.TB) *Key {
	t.Helper()
	k := &Key{}
	if _, err := rand.Read(k.seed[:]); err != nil {
		t.Fatal(err)
	}
	pub, priv := mldsa65.NewKeyFromSeed(&k.seed)
	s, err := gungnir.NewSignerWithID(priv)
	if err != nil {
		t.Fatal(err)
	}
	k.Signer, k.Pub = s, pub
	return k
}

// WriteFiles writes the seed (0600) and the packed public key into dir and
// returns their paths.
func (k *Key) WriteFiles(t testing.TB, dir string) (seedPath, pubPath string) {
	t.Helper()
	seedPath = filepath.Join(dir, "build.key")
	pubPath = filepath.Join(dir, "build.pubkey")
	if err := os.WriteFile(seedPath, k.seed[:], 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pubPath, k.Pub.Bytes(), 0o644); err != nil { // #nosec G306 -- public key in a test temp dir
		t.Fatal(err)
	}
	return seedPath, pubPath
}

// Manifest renders files (path below the root -> content) the way
// build-sealed-cask.sh does: `sha256sum` lines "<hex>  ./<path>", sorted.
func Manifest(files map[string][]byte) []byte {
	var b bytes.Buffer
	for _, n := range sortedNames(files) {
		fmt.Fprintf(&b, "%x  ./%s\n", sha256.Sum256(files[n]), n)
	}
	return b.Bytes()
}

// Seal signs manifest as an evidence seal for purpose (JSON + newline).
func (k *Key) Seal(t testing.TB, manifest []byte, purpose string) []byte {
	t.Helper()
	seal, err := k.Signer.SignEvidence(manifest, purpose)
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(seal)
	if err != nil {
		t.Fatal(err)
	}
	return append(b, '\n')
}

// Entries lays out a good cask: the root directory, each file under it
// (parent directories first), then the manifest and, when sig is non-nil,
// the signature.
func Entries(files map[string][]byte, manifest, sig []byte) []Entry {
	es := []Entry{{Name: Root + "/", Type: tar.TypeDir, Mode: 0o755}}
	dirs := map[string]bool{}
	for _, n := range sortedNames(files) {
		var parents []string
		for d := path.Dir(n); d != "."; d = path.Dir(d) {
			parents = append([]string{d}, parents...)
		}
		for _, d := range parents {
			if !dirs[d] {
				dirs[d] = true
				es = append(es, Entry{Name: Root + "/" + d + "/", Type: tar.TypeDir, Mode: 0o755})
			}
		}
		mode := int64(0o644)
		if path.Dir(n) == "bin" {
			mode = 0o755
		}
		es = append(es, Entry{Name: Root + "/" + n, Body: files[n], Mode: mode})
	}
	es = append(es, Entry{Name: Root + "/BINDING_RUNE.sha256", Body: manifest})
	if sig != nil {
		es = append(es, Entry{Name: Root + "/BINDING_RUNE.sig", Body: sig})
	}
	return es
}

// Tar gzips a tar of entries.
func Tar(t testing.TB, entries []Entry) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		typ := e.Type
		if typ == 0 {
			typ = tar.TypeReg
		}
		mode := e.Mode
		if mode == 0 {
			mode = 0o644
		}
		h := &tar.Header{Name: e.Name, Typeflag: typ, Mode: mode, Linkname: e.Linkname}
		if typ == tar.TypeReg {
			h.Size = int64(len(e.Body))
		}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if typ == tar.TypeReg {
			if _, err := tw.Write(e.Body); err != nil {
				t.Fatal(err)
			}
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

// Files is a small default payload: a binary (exec bit), a config, a nested
// runbook and a migration.
func Files() map[string][]byte {
	return map[string][]byte{
		"bin/wotan":                  []byte("\x7fELF pretend binary"),
		"configs/wotan.yaml":         []byte("topics: {}\n"),
		"runbooks/security/a.yaml":   []byte("kind: Runbook\n"),
		"db/migrations/001_init.sql": []byte("select 1;\n"),
	}
}

// Good returns a correctly signed cask of Files() and the key that signed it.
func Good(t testing.TB) ([]byte, *Key) {
	t.Helper()
	k := NewKey(t)
	files := Files()
	m := Manifest(files)
	return Tar(t, Entries(files, m, k.Seal(t, m, Purpose))), k
}

func sortedNames(files map[string][]byte) []string {
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
