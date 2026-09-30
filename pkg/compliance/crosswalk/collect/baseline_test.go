// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package collect

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	"unheaded/pkg/compliance/crosswalk"
)

func baselineCatalog(t *testing.T) *crosswalk.Catalog {
	t.Helper()
	c, err := crosswalk.Load([]byte("frameworks:\n  - id: fw\n    name: FW\n    requirements: [{id: R1}]\n"), []byte(`
controls:
  - id: UH-A-01
    title: t
    statement: s
    freshness_days: 7
    evidence:
      - {kind: host-sysctl, ref: "west:fs.suid_dumpable=0"}
      - {kind: host-sshd, ref: "east:maxauthtries<=4"}
      - {kind: host-probe, ref: "east:auditd-running"}
      - {kind: host-probe, ref: "west:db-backup-recent"}
      - {kind: github-job, ref: "W/scan"}
    mappings: {fw: [R1]}
  - id: UH-B-01
    title: t
    statement: s
    freshness_days: 7
    evidence: [{kind: host-sysctl, ref: "west:fs.suid_dumpable=0"}]
    mappings: {fw: [R1]}
`))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestBaselineFromCatalog(t *testing.T) {
	b := BaselineFromCatalog(baselineCatalog(t))
	var keys []string
	for _, s := range b.Sources {
		keys = append(keys, s.Key())
	}
	// Host configuration only, deduplicated, sorted; no CI job, no operational probe.
	want := "host-probe:east:auditd-running host-sshd:east:maxauthtries<=4 host-sysctl:west:fs.suid_dumpable=0"
	if got := strings.Join(keys, " "); got != want {
		t.Errorf("sources = %s\nwant      %s", got, want)
	}
	if got := b.ForHost("east"); len(got) != 2 {
		t.Errorf("ForHost(east) = %v", got)
	}
	if got := b.ForHost("nowhere"); len(got) != 0 {
		t.Errorf("ForHost(nowhere) = %v", got)
	}
}

func TestBaseline_RoundTrip(t *testing.T) {
	b := BaselineFromCatalog(baselineCatalog(t))
	raw := b.Marshal()
	if !bytes.HasPrefix(raw, []byte("# Generated")) {
		t.Errorf("no generated header:\n%s", raw)
	}
	got, err := ParseBaseline(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Marshal(), raw) {
		t.Errorf("round trip changed the file")
	}
}

func TestParseBaseline_Rejects(t *testing.T) {
	for name, body := range map[string]string{
		"wrong version":         "version: 2\nsources: []\n",
		"unknown field":         "version: 1\nsources: []\nextra: 1\n",
		"not host scoped":       "version: 1\nsources: [{kind: github-job, ref: W/scan}]\n",
		"no host prefix":        "version: 1\nsources: [{kind: host-sysctl, ref: \"fs.suid_dumpable=0\"}]\n",
		"bad sysctl ref":        "version: 1\nsources: [{kind: host-sysctl, ref: \"west:fs.suid_dumpable!0\"}]\n",
		"sshd op needs integer": "version: 1\nsources: [{kind: host-sshd, ref: \"west:maxauthtries<=four\"}]\n",
		"unknown probe":         "version: 1\nsources: [{kind: host-probe, ref: \"west:rm-rf\"}]\n",
		"operational probe":     "version: 1\nsources: [{kind: host-probe, ref: \"west:db-backup-recent\"}]\n",
		"duplicate":             "version: 1\nsources: [{kind: host-probe, ref: \"west:auditd-running\"}, {kind: host-probe, ref: \"west:auditd-running\"}]\n",
		"empty":                 "",
	} {
		if _, err := ParseBaseline([]byte(body)); !errors.Is(err, ErrInvalidBaseline) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	if _, err := ParseBaseline(make([]byte, maxBaselineBytes+1)); !errors.Is(err, ErrInvalidBaseline) {
		t.Errorf("oversize: err = %v", err)
	}
}

// The baseline in the repo is exactly what the catalog generates: the host
// agent and the central collector check the same things.
func TestRepoBaselineInSync(t *testing.T) {
	cat, err := crosswalk.LoadFS(os.DirFS("../../../.."), "compliance/catalog/frameworks", "compliance/catalog/controls.yaml")
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile("../../../../compliance/baseline/baseline.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if want := BaselineFromCatalog(cat).Marshal(); !bytes.Equal(got, want) {
		t.Fatal("compliance/baseline/baseline.yaml is out of date: go run ./cmd/compliance-evidence baseline")
	}
}
