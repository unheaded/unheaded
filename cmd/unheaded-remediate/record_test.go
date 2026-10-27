// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// check, then -record-existing: how the eight 2026-10-01 lab fixes get a
// journal without being applied again (UH-REMED-01).
func TestCheckAndRecordExisting(t *testing.T) {
	model, root, state := setup(t)
	flags := []string{"-model", model, "-root", root, "-state", state, "-host", "east"}
	if code, out, _ := cli(append(flags, "check", "kmod-cis")...); code != exitDeviates || !strings.Contains(out, "not in place") {
		t.Fatalf("check before apply: %d %s", code, out)
	}
	if code, _, errs := cli(append(flags, "-record-existing", "kmod-cis")...); code == 0 || !strings.Contains(errs, "not in place") {
		t.Fatalf("recorded a fix that is not in place: %d %s", code, errs)
	}
	if _, err := os.Stat(filepath.Join(state, "journal.jsonl")); err == nil {
		t.Fatal("a refused record wrote the journal")
	}
	// Put the fix in place the way remediate.sh did, outside the journal.
	if code, out, errs := cli(append(flags, "-state", t.TempDir(), "apply", "kmod-cis")...); code != 0 {
		t.Fatalf("apply: %d %s %s", code, out, errs)
	}
	if code, out, _ := cli(append(flags, "check", "kmod-cis")...); code != 0 || !strings.Contains(out, "in place") {
		t.Fatalf("check after apply: %d %s", code, out)
	}
	code, out, errs := cli(append(flags, "-record-existing", "kmod-cis", "-applied-by", "remediate.sh (2026-10-01)")...)
	if code != 0 || !strings.Contains(out, "nothing changed") {
		t.Fatalf("record-existing: %d %s %s", code, out, errs)
	}
	b, err := os.ReadFile(filepath.Join(state, "journal.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"backfilled":true`) || !strings.Contains(string(b), "applied_by remediate.sh (2026-10-01), recorded ") ||
		strings.Count(string(b), "\n") != 1 {
		t.Errorf("journal:\n%s", b)
	}
	if code, _, _ := cli(append(flags, "-record-existing", "kmod-cis", "plan", "kmod-cis")...); code != 2 {
		t.Errorf("-record-existing with a verb: %d, want usage", code)
	}
}

// Recording writes the journal, so the model must be trustworthy for it.
func TestRecordExistingRefusesAWritableModel(t *testing.T) {
	model, root, state := setup(t)
	if err := os.Chmod(model, 0o666); err != nil {
		t.Fatal(err)
	}
	code, _, errs := cli("-model", model, "-root", root, "-state", state, "-host", "east", "-record-existing", "kmod-cis")
	if code == 0 || !strings.Contains(errs, "refusing the model") {
		t.Fatalf("%d %s", code, errs)
	}
}
