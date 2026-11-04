// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	recordsDir = "../../pkg/threatfeed/osv/testdata/records"
	kevFixture = "../../pkg/threatfeed/osv/testdata/kev.json"
)

// tree writes a small tree with one affected component per ecosystem.
func tree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"go.mod":           "module m\n\nrequire (\n\texample.test/alpha v1.2.3\n\texample.test/clean v9.9.9\n)\n",
		"requirements.txt": "chroma-fixture==1.5.9\nranged>=1\n",
		"Cargo.lock":       "[[package]]\nname = \"serde-fixture\"\nversion = \"1.0.230\"\nsource = \"registry+x\"\n",
	}
	for n, b := range files {
		if err := os.WriteFile(filepath.Join(root, n), []byte(b), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestRunFindsAndOrders(t *testing.T) {
	var out, errb bytes.Buffer
	cfg := config{root: tree(t), osvDir: recordsDir, kevFile: kevFixture, jsonOut: true}
	code, err := run(cfg, &out, &errb)
	if err != nil || code != 0 {
		t.Fatalf("run = %d, %v", code, err)
	}
	var got []Finding
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d findings: %+v", len(got), got)
	}
	if !got[0].KEV || got[0].ID != "FIXTURE-PY-0001" || got[0].FixedIn != "" {
		t.Errorf("first finding %+v, want the KEV one with no fix", got[0])
	}
	if got[1].ID != "FIXTURE-GO-0001" || got[1].KEV || got[1].FixedIn != "1.3.0" {
		t.Errorf("second finding %+v", got[1])
	}
	if got[2].ID != "FIXTURE-RS-0001" {
		t.Errorf("third finding %+v", got[2])
	}
	log := errb.String()
	if !strings.Contains(log, "not checked, not an exact pin: requirements.txt: ranged>=1") {
		t.Errorf("stderr lacks the unpinned line: %q", log)
	}
	if !strings.Contains(log, "4 components, 4 records, 3 findings (1 in KEV), 1 not an exact pin") {
		t.Errorf("stderr lacks the summary: %q", log)
	}
}

func TestFailOnFindings(t *testing.T) {
	var out, errb bytes.Buffer
	cfg := config{root: tree(t), osvDir: recordsDir, failOnFindings: true}
	code, err := run(cfg, &out, &errb)
	if err != nil || code != 1 {
		t.Fatalf("run = %d, %v, want 1", code, err)
	}
	if !strings.Contains(out.String(), "FIXTURE-GO-0001") || strings.Contains(out.String(), "FIXTURE-WD-0001") {
		t.Errorf("table: %q", out.String())
	}
}

func TestNoDataIsAnError(t *testing.T) {
	var out, errb bytes.Buffer
	if _, err := run(config{root: tree(t)}, &out, &errb); err == nil ||
		!strings.Contains(err.Error(), "--osv-dir is required") {
		t.Errorf("err = %v", err)
	}
}
