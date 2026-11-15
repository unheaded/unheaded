// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis. All rights reserved.

package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func scanOwnFixtures(t *testing.T) []Asset {
	t.Helper()
	assets, err := scanOwnPrimitives(fixtureTree(t, "own"))
	if err != nil {
		t.Fatalf("scanOwnPrimitives: %v", err)
	}
	return assets
}

// TestScanOwn_ListedFilesGiveRows: a file in the table is an own-primitive
// row at the line of its marker, and breaks no rule.
func TestScanOwn_ListedFilesGiveRows(t *testing.T) {
	assets := scanOwnFixtures(t)
	sha := one(t, assets, "crates/upc-api/src/surface_hash.rs", 3, kindOwnPrimitive)
	if sha.Name != "SHA-256 (hand-written, FIPS 180-4)" || sha.Primitive != primHash || sha.ParamSet != "SHA-256" {
		t.Errorf("listed SHA-256 asset = %+v", sha)
	}
	kdf := one(t, assets, "services/sophia/hkdf_derive.go", 6, kindOwnPrimitive)
	if kdf.Primitive != primKDF || kdf.ParamSet != "HKDF-SHA-256" {
		t.Errorf("listed HKDF asset = %+v", kdf)
	}
	for _, a := range []Asset{sha, kdf} {
		if a.AssetType != assetAlgorithm || a.Scanner != "source" || a.Owner != ownerFirstParty || a.LinkState != linkEnabled || a.PQStatus != pqNA {
			t.Errorf("%s: %+v", a.Ref(), a)
		}
		if len(rulesFor(a)) != 0 {
			t.Errorf("%s: a listed primitive breaks %v", a.Ref(), rulesFor(a))
		}
	}
}

// TestLint_HandWrittenSHA256OutsideTheTableFails: the SHA-256 round
// constants in a source file the table does not list are a lint violation,
// in Go and in Rust, whatever the case of the hex digits.
func TestLint_HandWrittenSHA256OutsideTheTableFails(t *testing.T) {
	keys := lintFixture(t, "own")
	for _, k := range []string{
		"pkg/rogue/sha.go " + ruleOwnPrimitive,
		"crates/rogue/src/lib.rs " + ruleOwnPrimitive,
	} {
		if !keys[k] {
			t.Errorf("missing violation %q in %v", k, keys)
		}
	}
	for _, k := range []string{
		"crates/upc-api/src/surface_hash.rs " + ruleOwnPrimitive, // listed
		"services/sophia/hkdf_derive.go " + ruleOwnPrimitive,     // listed
		"pkg/rogue/sha_test.go " + ruleOwnPrimitive,              // a Go test file
		"crates/rogue/tests/vectors.rs " + ruleOwnPrimitive,      // a Rust integration test
		"docs/notes.md " + ruleOwnPrimitive,                      // prose
	} {
		if keys[k] {
			t.Errorf("unexpected violation %q", k)
		}
	}
}

// TestScanOwn_UnlistedRowNamesWhatToDo: the row for an unlisted file says
// it is not in the table, so the CBOM finding reads on its own.
func TestScanOwn_UnlistedRowNamesWhatToDo(t *testing.T) {
	a := one(t, scanOwnFixtures(t), "pkg/rogue/sha.go", 7, kindOwnPrimitive)
	if !strings.Contains(a.Name, "not in the own-primitive table") || a.Primitive != primHash {
		t.Errorf("unlisted asset = %+v", a)
	}
}

// TestRunLint_OwnPrimitiveMessage: the failure names the rule and says what
// to do; it does not give the key-exchange advice.
func TestRunLint_OwnPrimitiveMessage(t *testing.T) {
	root := gitFixture(t, "go")
	base := filepath.Join(root, "baseline.txt")
	if rc, _, errs := runCLI("lint", "-root", root, "-baseline", base, "-update"); rc != 0 {
		t.Fatalf("--update on a new baseline: rc=%d %s", rc, errs)
	}
	writeFile(t, root, "pkg/zz/sha.go", "package zz\n\nvar k = uint32(0x428a2f98)\n")
	rc, _, errs := runCLI("lint", "-root", root, "-baseline", base)
	if rc != 1 || !strings.Contains(errs, "pkg/zz/sha.go "+ruleOwnPrimitive) {
		t.Fatalf("hand-written SHA-256: rc=%d err=%q", rc, errs)
	}
	if !strings.Contains(errs, "hand-written cryptographic primitive") || !strings.Contains(errs, "ownPrimitives") {
		t.Errorf("the failure does not say what to do: %q", errs)
	}
	if strings.Contains(errs, "classical-only key exchange configured") {
		t.Errorf("the failure gives key-exchange advice for a hash: %q", errs)
	}
	if rc, _, errs := runCLI("lint", "-root", root, "-baseline", base, "-update"); rc != 1 || !strings.Contains(errs, "REFUSED") {
		t.Fatalf("--update must refuse to baseline it: rc=%d err=%q", rc, errs)
	}
}

// TestOwnPrimitiveTableMatchesRepo: every table entry names a file in this
// repository that still holds its marker. A moved or deleted file fails
// here instead of dropping silently out of the CBOM.
func TestOwnPrimitiveTableMatchesRepo(t *testing.T) {
	if len(ownPrimitives) == 0 {
		t.Fatal("the own-primitive table is empty")
	}
	repo := os.DirFS(filepath.Join("..", ".."))
	for _, p := range ownPrimitives {
		b, err := fs.ReadFile(repo, p.path)
		if err != nil {
			t.Errorf("%s: %v", p.path, err)
			continue
		}
		if !p.marker.Match(b) {
			t.Errorf("%s no longer holds %q", p.path, p.marker.String())
		}
	}
}
