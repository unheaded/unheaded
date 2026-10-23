// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"strings"
	"testing"
)

// fixtureTree loads a testdata subtree as if it were the repository root.
func fixtureTree(t *testing.T, dir string) *Tree {
	t.Helper()
	tr, err := dirTree("testdata/" + dir)
	if err != nil {
		t.Fatalf("dirTree(%s): %v", dir, err)
	}
	return tr
}

// at returns the assets at path (and line, when line > 0) of the given kind.
func at(assets []Asset, path string, line int, kind string) []Asset {
	var out []Asset
	for _, a := range assets {
		if a.Path == path && a.Kind == kind && (line == 0 || a.Line == line) {
			out = append(out, a)
		}
	}
	return out
}

// one asserts exactly one asset matches and returns it.
func one(t *testing.T, assets []Asset, path string, line int, kind string) Asset {
	t.Helper()
	got := at(assets, path, line, kind)
	if len(got) != 1 {
		var refs []string
		for _, a := range assets {
			if a.Path == path {
				refs = append(refs, a.Ref()+"="+a.PQStatus)
			}
		}
		t.Fatalf("want 1 %s asset at %s:%d, got %d (assets in file: %v)", kind, path, line, len(got), refs)
	}
	return got[0]
}

func scanGoFixtures(t *testing.T) []Asset {
	t.Helper()
	assets, err := scanGo(fixtureTree(t, "go"))
	if err != nil {
		t.Fatalf("scanGo: %v", err)
	}
	return assets
}

func TestScanGo_Classification(t *testing.T) {
	assets := scanGoFixtures(t)
	tests := []struct {
		name   string
		path   string
		line   int
		kind   string
		status string
	}{
		{"(a) classical curve pin", "pkg/fx/a_classical.go", 8, kindTLSConfig, pqClassical},
		{"(b) nil curves + TLS 1.3", "pkg/fx/b_hybrid.go", 8, kindTLSConfig, pqHybrid},
		{"(c) server with a 1.2 floor", "pkg/fx/c_floor12.go", 9, kindTLSConfig, pqHybridOpp},
		{"(d) aliased import", "pkg/fx/d_alias.go", 8, kindTLSConfig, pqClassical},
		{"(e) literal before the assignment", "pkg/fx/e_assign.go", 9, kindTLSConfig, pqHybrid},
		{"(e) curve pin by assignment", "pkg/fx/e_assign.go", 10, kindTLSGroups, pqClassical},
		{"MaxVersion 1.2 by assignment", "pkg/fx/e_assign.go", 16, kindTLSCap, pqClassical},
		{"(f) grpc server creds from files", "pkg/fx/f_grpc.go", 12, kindGRPCCreds, pqHybridOpp},
		{"(g) hybrid-only list", "pkg/fx/g_strict.go", 8, kindTLSConfig, pqHybridStrict},
		{"pure ML-KEM group", "pkg/fx/g_pqonly.go", 11, kindTLSConfig, pqOnly},
		{"hybrid first, classical fallback", "pkg/fx/g_strict.go", 14, kindTLSConfig, pqHybrid},
		{"(i) ML-DSA-65 import", "pkg/fx/i_mldsa.go", 5, kindSigImport, pqSignature},
		{"(j) Ed25519 import", "pkg/fx/j_ed25519.go", 5, kindSigImport, pqClassicalSig},
		{"in-tree PQC wrapper, ML-DSA-65", "pkg/fx/l_pqc.go", 5, kindSigImport, pqSignature},
		{"floor via another struct's MinVersion", "pkg/fx/k_floorfield.go", 15, kindTLSFloor, pqHybridOpp},
		{"//go:debug kill switch", "cmd/godebugfx/main.go", 3, kindGodebug, pqClassical},
		{"GODEBUG kill switch in a string", "pkg/fx/k_floorfield.go", 20, kindGodebug, pqClassical},
		{"third-party SMTP relay", "cmd/alert-mailer/smtp.go", 8, kindTLSConfig, pqHybridOpp},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := one(t, assets, tt.path, tt.line, tt.kind)
			if a.PQStatus != tt.status {
				t.Errorf("%s: pq-status = %q, want %q (detail %q)", a.Ref(), a.PQStatus, tt.status, a.Detail)
			}
			if a.Scanner != "go" {
				t.Errorf("%s: scanner = %q, want go", a.Ref(), a.Scanner)
			}
		})
	}
}

func TestScanGo_IgnoresTestFiles(t *testing.T) {
	for _, a := range scanGoFixtures(t) {
		if strings.HasSuffix(a.Path, "_test.go") {
			t.Errorf("(h) %s inventoried; _test.go files must be ignored", a.Ref())
		}
	}
}

func TestScanGo_Owner(t *testing.T) {
	assets := scanGoFixtures(t)
	if a := one(t, assets, "cmd/alert-mailer/smtp.go", 8, kindTLSConfig); a.Owner != ownerThirdParty {
		t.Errorf("SMTP relay owner = %q, want %q", a.Owner, ownerThirdParty)
	}
	if a := one(t, assets, "pkg/fx/c_floor12.go", 9, kindTLSConfig); a.Owner != ownerFirstParty {
		t.Errorf("first-party server owner = %q, want %q", a.Owner, ownerFirstParty)
	}
}

func TestScanGo_AlgorithmDetails(t *testing.T) {
	assets := scanGoFixtures(t)
	ml := one(t, assets, "pkg/fx/i_mldsa.go", 5, kindSigImport)
	if ml.ParamSet != "ML-DSA-65" || ml.NISTLevel != 3 || ml.Primitive != primSignature || ml.CNSA2 {
		t.Errorf("ML-DSA-65 asset = %+v", ml)
	}
	ed := one(t, assets, "pkg/fx/j_ed25519.go", 5, kindSigImport)
	if ed.ParamSet != "Ed25519" || ed.NISTLevel != 0 {
		t.Errorf("Ed25519 asset = %+v", ed)
	}
}

func TestScanGo_RecordsGroups(t *testing.T) {
	a := one(t, scanGoFixtures(t), "pkg/fx/a_classical.go", 8, kindTLSConfig)
	if got := strings.Join(a.Groups, ","); got != "X25519,CurveP256" {
		t.Errorf("groups = %q, want X25519,CurveP256", got)
	}
}

func TestClassifyTLS(t *testing.T) {
	tests := []struct {
		name     string
		min, max string
		groups   []string
		dynamic  bool
		want     string
	}{
		{"default everything", "", "", nil, false, pqHybridOpp},
		{"1.3 floor", "1.3", "", nil, false, pqHybrid},
		{"1.2 floor", "1.2", "", nil, false, pqHybridOpp},
		{"1.0 floor", "1.0", "", nil, false, pqHybridOpp},
		{"max 1.2", "1.3", "1.2", nil, false, pqClassical},
		{"max 1.3", "1.2", "1.3", nil, false, pqHybridOpp},
		{"classical list", "1.3", "", []string{"X25519"}, false, pqClassical},
		{"strict list", "1.2", "", []string{"X25519MLKEM768", "SecP256r1MLKEM768"}, false, pqHybridStrict},
		{"mixed list 1.3", "1.3", "", []string{"X25519MLKEM768", "X25519"}, false, pqHybrid},
		{"mixed list 1.2", "1.2", "", []string{"X25519MLKEM768", "X25519"}, false, pqHybridOpp},
		{"pure ML-KEM", "1.3", "", []string{"MLKEM1024"}, false, pqOnly},
		{"dynamic groups", "1.3", "", nil, true, pqUnknown},
		{"empty list literal = default", "1.3", "", []string{}, false, pqHybrid},
		{"floor set at run time", dynamic, "", nil, false, pqUnknown},
		{"floor at run time, strict list", dynamic, "", []string{"X25519MLKEM768"}, false, pqHybridStrict},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyTLS(tt.min, tt.max, tt.groups, tt.dynamic); got != tt.want {
				t.Errorf("classifyTLS(%q,%q,%v,%v) = %q, want %q", tt.min, tt.max, tt.groups, tt.dynamic, got, tt.want)
			}
		})
	}
}

func TestIsHybridGroup(t *testing.T) {
	for _, g := range []string{"X25519MLKEM768", "SecP256r1MLKEM768", "SecP384r1MLKEM1024"} {
		if !isHybridGroup(g) {
			t.Errorf("%s should be hybrid", g)
		}
	}
	for _, g := range []string{"X25519", "CurveP256", "CurveP384", "CurveP521", "MLKEM1024", "", "x25519mlkem768"} {
		if isHybridGroup(g) {
			t.Errorf("%q should not be hybrid", g)
		}
	}
}

func TestScanGo_ParseErrorIsLoud(t *testing.T) {
	tr := &Tree{Root: t.TempDir(), Files: []string{"bad.go"}}
	writeFile(t, tr.Root, "bad.go", "package x\nfunc {")
	if _, err := scanGo(tr); err == nil {
		t.Fatal("a file that does not parse must fail the scan, not vanish from it")
	}
}
