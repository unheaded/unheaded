// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// runCLI runs the CLI in-process and returns exit code, stdout, stderr.
func runCLI(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := run(args, &out, &errb)
	return rc, out.String(), errb.String()
}

func TestUsageHasAllSubcommands(t *testing.T) {
	var b bytes.Buffer
	usage(&b)
	out := b.String()
	for _, sub := range []string{
		"validate", "sign", "pubkey", "verify",
		"verify-signature", "verify-iso", "verify-gates", "verify-custody",
		"diff",
	} {
		if !strings.Contains(out, sub) {
			t.Errorf("usage output missing subcommand %q\nfull output:\n%s", sub, out)
		}
	}
	for _, ref := range []string{
		"nix/yggdrasil/evidence-pack/README.md",
		"nix/yggdrasil/evidence-pack/runbooks",
	} {
		if !strings.Contains(out, ref) {
			t.Errorf("usage output missing doc reference %q", ref)
		}
	}
}

func TestSubcommandRoutingTable(t *testing.T) {
	if rc, _, _ := runCLI(t); rc != exitUsage {
		t.Errorf("no args: rc=%d", rc)
	}
	if rc, _, errs := runCLI(t, "frobnicate"); rc != exitUsage || !strings.Contains(errs, "unknown subcommand") {
		t.Errorf("unknown subcommand: rc=%d %q", rc, errs)
	}
	for _, h := range []string{"help", "-h", "--help"} {
		if rc, out, _ := runCLI(t, h); rc != exitOK || !strings.Contains(out, "Subcommands:") {
			t.Errorf("%s: rc=%d", h, rc)
		}
	}
	// Every subcommand is routed: called without its arguments it reports
	// usage (exit 2), never "unknown subcommand".
	for _, sub := range []string{
		"validate", "sign", "pubkey", "verify",
		"verify-signature", "verify-iso", "verify-gates", "verify-custody",
		"diff",
	} {
		rc, _, errs := runCLI(t, sub)
		if rc != exitUsage || strings.Contains(errs, "unknown subcommand") {
			t.Errorf("%s with no args: rc=%d stderr=%q", sub, rc, errs)
		}
	}
}

func TestValidateCommand(t *testing.T) {
	valid := "testdata/manifests/valid/minimal.yaml"
	if rc, out, errs := runCLI(t, "validate", valid); rc != exitOK || !strings.Contains(out, "OK") {
		t.Errorf("valid: rc=%d out=%q err=%q", rc, out, errs)
	}
	if rc, _, errs := runCLI(t, "validate", "testdata/manifests/invalid/critical-1.yaml"); rc != exitFail || !strings.Contains(errs, "cve_scan.critical") {
		t.Errorf("invalid: rc=%d err=%q", rc, errs)
	}
	if rc, _, _ := runCLI(t, "validate", "testdata/manifests/does-not-exist.yaml"); rc != exitUsage {
		t.Errorf("missing file: rc=%d", rc)
	}
	if rc, _, _ := runCLI(t, "validate", "testdata/manifests"); rc != exitUsage {
		t.Errorf("directory: rc=%d", rc)
	}
	if rc, _, _ := runCLI(t, "validate", "--bogus", valid); rc != exitUsage {
		t.Errorf("bad flag: rc=%d", rc)
	}
	if rc, _, errs := runCLI(t, "validate", "--schema", schemaPath, valid); rc != exitOK {
		t.Errorf("--schema matching: rc=%d err=%q", rc, errs)
	}
	schema, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatal(err)
	}
	mut := filepath.Join(t.TempDir(), "schema.yaml")
	if err := os.WriteFile(mut, bytes.Replace(schema, []byte("maximum: 50"), []byte("maximum: 60"), 1), 0o600); err != nil {
		t.Fatal(err)
	}
	if rc, _, errs := runCLI(t, "validate", "--schema", mut, valid); rc != exitUsage || !strings.Contains(errs, "patch_count") {
		t.Errorf("--schema drifted: rc=%d err=%q", rc, errs)
	}
}
