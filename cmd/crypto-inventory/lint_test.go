// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func lintFixture(t *testing.T, fx string) map[string]bool {
	t.Helper()
	assets, err := scanAll(fixtureTree(t, fx))
	if err != nil {
		t.Fatal(err)
	}
	keys := map[string]bool{}
	for _, v := range lintAssets(assets) {
		keys[v.Key()] = true
	}
	return keys
}

func TestLint_Violations(t *testing.T) {
	keys := map[string]bool{}
	for _, fx := range []string{"go", "rust", "config"} {
		for k := range lintFixture(t, fx) {
			keys[k] = true
		}
	}
	want := []string{
		// CurvePreferences with no hybrid group (literal, alias, assignment).
		"pkg/fx/a_classical.go " + ruleCurvePin,
		"pkg/fx/d_alias.go " + ruleCurvePin,
		"pkg/fx/e_assign.go " + ruleCurvePin,
		// A first-party floor below TLS 1.3, however it is set.
		"pkg/fx/c_floor12.go " + ruleFloor,
		"pkg/fx/f_grpc.go " + ruleFloor,
		"pkg/fx/k_floorfield.go " + ruleFloor,
		// A cap below 1.3 forces a classical session.
		"pkg/fx/e_assign.go " + ruleCap,
		// GODEBUG kill switches in code, a unit and go.mod.
		"cmd/godebugfx/main.go " + ruleGodebug,
		"pkg/fx/k_floorfield.go " + ruleGodebug,
		"deploy/units/app.service " + ruleGodebug,
		"tools/new/go.mod " + ruleGodebug,
		"go.mod " + ruleGoDirective,
		// rustls with ring: a provider call and a ring-only dependency.
		"crates/ringy/src/lib.rs " + ruleRustRing,
		"crates/ringy/Cargo.toml " + ruleRustRing,
		// The Well below PG 18 with TLS on.
		"stack.yml " + rulePGPre18,
		// sshd lists with no hybrid at all (sntrup counts as hybrid).
		"runbooks/security/ssh.yaml " + ruleSSHD,
		"nix/patches/0001-sshd.patch " + ruleSSHD,
	}
	for _, k := range want {
		if !keys[k] {
			t.Errorf("missing violation %q", k)
		}
	}
	notWant := []string{
		"pkg/fx/b_hybrid.go " + ruleFloor,            // 1.3 floor
		"pkg/fx/g_strict.go " + ruleCurvePin,         // hybrid in the list
		"pkg/fx/g_pqonly.go " + ruleCurvePin,         // pure ML-KEM is not classical
		"pkg/fx/h_ignored_test.go " + ruleCurvePin,   // test files
		"cmd/alert-mailer/smtp.go " + ruleFloor,      // third-party peer keeps 1.2
		"cmd/lich-security/probe.go " + ruleCurvePin, // exempt: probes weak TLS on purpose
		"cmd/lich-security/probe.go " + ruleCap,
		"crates/awsy/src/lib.rs " + ruleRustRing,
		"crates/awsy/Cargo.toml " + ruleRustRing,
		"crates/bothy/Cargo.toml " + ruleRustRing,  // both providers: the call site decides
		"crates/tonicy/Cargo.toml " + ruleRustRing, // unused feature
		"crates/reqy/Cargo.toml " + ruleRustRing,   // unused feature
		"deploy/pg18/compose.yml " + rulePGPre18,
		"deploy/pg18/compose.yml " + rulePGGroups, // a hybrid ssl_groups exists
		"deploy/ssh/sshd_config " + ruleSSHD,      // hybrid first, classical fallback
		"deploy/ssh/sshd_strict.conf " + ruleSSHD,
		"scripts/wg/setup.sh " + ruleSSHD,
	}
	for _, k := range notWant {
		if keys[k] {
			t.Errorf("unexpected violation %q", k)
		}
	}
}

func TestLint_KeyHasNoLineNumber(t *testing.T) {
	v := Violation{Path: "a/b.go", Rule: ruleCurvePin, Line: 42}
	if v.Key() != "a/b.go "+ruleCurvePin {
		t.Errorf("key = %q; line numbers must not churn the baseline", v.Key())
	}
}

func TestLint_PGGroupsClassical(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "db/postgresql.conf", "ssl = on\nssl_groups = 'X25519:prime256v1'\n")
	writeFile(t, root, "compose.yml", "services:\n  db:\n    image: postgres:18.0-alpine\n")
	tr, err := dirTree(root)
	if err != nil {
		t.Fatal(err)
	}
	assets, err := scanAll(tr)
	if err != nil {
		t.Fatal(err)
	}
	keys := map[string]bool{}
	for _, v := range lintAssets(assets) {
		keys[v.Key()] = true
	}
	for _, k := range []string{"db/postgresql.conf " + rulePGGroups, "compose.yml " + rulePGGroups} {
		if !keys[k] {
			t.Errorf("missing %q in %v", k, keys)
		}
	}
}

func TestRatchet(t *testing.T) {
	current := []string{"a x", "b y", "c z"}
	tests := []struct {
		name          string
		baseline      []string
		added, solved []string
	}{
		{"equal", []string{"a x", "b y", "c z"}, nil, nil},
		{"new violation", []string{"a x", "b y"}, []string{"c z"}, nil},
		{"resolved", []string{"a x", "b y", "c z", "d w"}, nil, []string{"d w"}},
		{"both", []string{"a x", "d w"}, []string{"b y", "c z"}, []string{"d w"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			added, solved := ratchet(current, tt.baseline)
			if strings.Join(added, "|") != strings.Join(tt.added, "|") || strings.Join(solved, "|") != strings.Join(tt.solved, "|") {
				t.Errorf("ratchet = %v / %v, want %v / %v", added, solved, tt.added, tt.solved)
			}
		})
	}
}

func TestReadBaseline_CommentsBlanksDupes(t *testing.T) {
	p := filepath.Join(t.TempDir(), "b.txt")
	if err := os.WriteFile(p, []byte("# header\n\nb y\na x\n  \na x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, header, err := readBaseline(filepath.Dir(p), p)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, "|") != "a x|b y" || strings.Join(header, "|") != "# header" {
		t.Errorf("baseline = %v header = %v", got, header)
	}
	if _, _, err := readBaseline(t.TempDir(), "missing"); err == nil {
		t.Error("a missing baseline must be an error, not an empty set")
	}
	bad := filepath.Join(t.TempDir(), "bad.txt")
	if err := os.WriteFile(bad, []byte("only-one-field\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := readBaseline(filepath.Dir(bad), bad); err == nil {
		t.Error("a malformed line must be an error")
	}
}

func TestRunLint_EndToEnd(t *testing.T) {
	root := gitFixture(t, "go")
	base := filepath.Join(root, "baseline.txt")
	if rc, out, errs := runCLI("lint", "-root", root, "-print"); rc != 0 || !strings.Contains(out, ruleCurvePin) {
		t.Fatalf("lint --print: rc=%d out=%q err=%q", rc, out, errs)
	}
	// Missing baseline: fail, and --update creates it.
	if rc, _, _ := runCLI("lint", "-root", root, "-baseline", base); rc != 1 {
		t.Fatalf("missing baseline: rc=%d, want 1", rc)
	}
	if rc, _, errs := runCLI("lint", "-root", root, "-baseline", base, "-update"); rc != 0 {
		t.Fatalf("--update on a new baseline: rc=%d %s", rc, errs)
	}
	if rc, out, errs := runCLI("lint", "-root", root, "-baseline", base); rc != 0 || !strings.Contains(out, "PASS") {
		t.Fatalf("lint with its own baseline: rc=%d out=%q err=%q", rc, out, errs)
	}
	// A new classical pin (untracked file) fails and --update refuses it.
	writeFile(t, root, "pkg/tls/zz_probe.go", "package x\n\nimport \"crypto/tls\"\n\nvar C = &tls.Config{MinVersion: tls.VersionTLS13, CurvePreferences: []tls.CurveID{tls.X25519}}\n")
	rc, _, errs := runCLI("lint", "-root", root, "-baseline", base)
	if rc != 1 || !strings.Contains(errs, "pkg/tls/zz_probe.go "+ruleCurvePin) {
		t.Fatalf("new violation: rc=%d err=%q", rc, errs)
	}
	if rc, _, errs := runCLI("lint", "-root", root, "-baseline", base, "-update"); rc != 1 || !strings.Contains(errs, "REFUSED") {
		t.Fatalf("--update must refuse additions: rc=%d err=%q", rc, errs)
	}
	if err := os.Remove(filepath.Join(root, "pkg/tls/zz_probe.go")); err != nil {
		t.Fatal(err)
	}
	// Fixing a baselined violation: lint fails until the line is removed.
	p := filepath.Join(root, "pkg/fx/a_classical.go")
	src, err := os.ReadFile(p) // #nosec G304 -- test temp file
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(strings.Replace(string(src), "tls.X25519, tls.CurveP256", "tls.X25519MLKEM768", 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	rc, _, errs = runCLI("lint", "-root", root, "-baseline", base)
	if rc != 1 || !strings.Contains(errs, "resolved: remove from baseline") || !strings.Contains(errs, "pkg/fx/a_classical.go "+ruleCurvePin) {
		t.Fatalf("resolved violation: rc=%d err=%q", rc, errs)
	}
	if rc, _, _ := runCLI("lint", "-root", root, "-baseline", base, "-update"); rc != 0 {
		t.Fatalf("--update after a fix: rc=%d", rc)
	}
	b, err := os.ReadFile(base) // #nosec G304 -- test temp file
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "pkg/fx/a_classical.go") {
		t.Error("--update kept a resolved line")
	}
	if !strings.HasPrefix(string(b), "#") {
		t.Error("--update dropped the header")
	}
	// --explain names what each scanner saw.
	if rc, out, _ := runCLI("lint", "-root", root, "-explain", "pkg/fx/e_assign.go"); rc != 0 || !strings.Contains(out, "tls-groups") || !strings.Contains(out, ruleCurvePin) {
		t.Fatalf("--explain: rc=%d out=%q", rc, out)
	}
}
