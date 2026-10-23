// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// copyTree copies a fixture subtree into a temp dir so a test can edit it.
func copyTree(t *testing.T, fixture string) string {
	t.Helper()
	src := filepath.Join("testdata", fixture)
	dst := t.TempDir()
	err := filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		if d.IsDir() {
			return os.MkdirAll(filepath.Join(dst, rel), 0o750)
		}
		b, err := os.ReadFile(p) // #nosec G304 -- test fixture
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dst, rel), b, 0o600)
	})
	if err != nil {
		t.Fatal(err)
	}
	return dst
}

func mustGenerate(t *testing.T, tr *Tree) []byte {
	t.Helper()
	b, _, err := generate(tr)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	return b
}

func TestScanDeterministic(t *testing.T) {
	for _, fx := range []string{"go", "rust", "config"} {
		t.Run(fx, func(t *testing.T) {
			tr := fixtureTree(t, fx)
			first := mustGenerate(t, tr)
			second := mustGenerate(t, tr)
			if !bytes.Equal(first, second) {
				t.Fatal("two scans of the same tree differ")
			}
			shuffled := &Tree{Root: tr.Root, Files: append([]string(nil), tr.Files...)}
			rng := rand.New(rand.NewSource(42)) // #nosec G404 -- test shuffle
			rng.Shuffle(len(shuffled.Files), func(i, j int) {
				shuffled.Files[i], shuffled.Files[j] = shuffled.Files[j], shuffled.Files[i]
			})
			if third := mustGenerate(t, shuffled); !bytes.Equal(first, third) {
				t.Fatal("file order changed the output")
			}
		})
	}
}

func TestGenerate_IsValidShape(t *testing.T) {
	b := mustGenerate(t, fixtureTree(t, "config"))
	var bom BOM
	if err := json.Unmarshal(b, &bom); err != nil {
		t.Fatal(err)
	}
	if bom.BOMFormat != "CycloneDX" || bom.SpecVersion != "1.6" || bom.Version != 1 || len(bom.Components) == 0 {
		t.Fatalf("header = %+v (%d components)", bom.Metadata, len(bom.Components))
	}
	refs := map[string]bool{}
	for _, c := range bom.Components {
		if c.Type != "cryptographic-asset" || c.Name == "" || c.CryptoProperties.AssetType == "" {
			t.Errorf("component %+v lacks required fields", c)
		}
		if refs[c.BOMRef] {
			t.Errorf("duplicate bom-ref %s", c.BOMRef)
		}
		refs[c.BOMRef] = true
		for _, p := range []string{propSource, propPQStatus, propLinkState, propOwner, propCNSA2} {
			if componentProp(c, p) == "" {
				t.Errorf("%s lacks %s", c.BOMRef, p)
			}
		}
	}
	if bytes.Contains(b, []byte("timestamp")) || bytes.Contains(b, []byte("serialNumber")) {
		t.Error("cbom carries a timestamp or serial number; it would never be fresh")
	}
}

func TestCheckDetectsStale(t *testing.T) {
	root := copyTree(t, "go")
	tr, err := dirTree(root)
	if err != nil {
		t.Fatal(err)
	}
	checkedIn := mustGenerate(t, tr)
	if err := compareCBOM(mustGenerate(t, tr), checkedIn); err != nil {
		t.Fatalf("fresh cbom reported stale: %v", err)
	}

	// Edit one fixture: the classical pin becomes hybrid-only.
	p := filepath.Join(root, "pkg/fx/a_classical.go")
	src, err := os.ReadFile(p) // #nosec G304 -- test temp file
	if err != nil {
		t.Fatal(err)
	}
	src = bytes.Replace(src, []byte("tls.X25519, tls.CurveP256"), []byte("tls.X25519MLKEM768"), 1)
	if err := os.WriteFile(p, src, 0o600); err != nil {
		t.Fatal(err)
	}
	err = compareCBOM(mustGenerate(t, tr), checkedIn)
	if !errors.Is(err, errStale) {
		t.Fatalf("edited tree: err = %v, want errStale", err)
	}
	if !strings.Contains(err.Error(), "~ pkg/fx/a_classical.go:8:tls-config") {
		t.Errorf("error does not name the changed component:\n%v", err)
	}
}

func TestCompareCBOM_AddedRemovedAndGarbage(t *testing.T) {
	tr := fixtureTree(t, "go")
	full := mustGenerate(t, tr)
	var bom BOM
	if err := json.Unmarshal(full, &bom); err != nil {
		t.Fatal(err)
	}
	dropped := bom
	dropped.Components = bom.Components[1:]
	b, err := encodeBOM(dropped)
	if err != nil {
		t.Fatal(err)
	}
	err = compareCBOM(full, b)
	if err == nil || !strings.Contains(err.Error(), "+ "+bom.Components[0].BOMRef) {
		t.Errorf("a component missing from the file must be reported '+': %v", err)
	}
	extra := bom
	extra.Components = append(append([]Component(nil), bom.Components...), Component{Type: "cryptographic-asset", BOMRef: "zz:1:fake", Name: "fake"})
	b, _ = encodeBOM(extra)
	if err := compareCBOM(full, b); err == nil || !strings.Contains(err.Error(), "- zz:1:fake") {
		t.Errorf("a component the tree lacks must be reported '-': %v", err)
	}
	if err := compareCBOM(full, []byte("{not json")); !errors.Is(err, errStale) {
		t.Errorf("garbage checked-in file: err = %v, want errStale", err)
	}
	if err := compareCBOM(full, append([]byte(" "), full...)); !errors.Is(err, errStale) {
		t.Errorf("formatting drift must still be stale: %v", err)
	}
}

func TestSummarize(t *testing.T) {
	s := summarize([]Asset{
		{PQStatus: pqClassical, Owner: ownerFirstParty},
		{PQStatus: pqClassical, Owner: ownerThirdParty},
		{PQStatus: pqHybrid, Owner: ownerFirstParty},
	})
	for _, want := range []string{"classical", "hybrid", "all"} {
		if !strings.Contains(s, want) {
			t.Errorf("summary lacks %q:\n%s", want, s)
		}
	}
}
