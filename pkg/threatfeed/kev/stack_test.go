// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package kev

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

const testStack = `
categories:
  - name: linux-kernel
    title: Linux kernel
    match:
      - {vendor: Linux, product: Kernel}
  - name: host-userland
    title: Host userland
    match:
      - {vendor: GNU}
      - {vendor: Sudo}
`

func TestClassify(t *testing.T) {
	s, err := ParseStack([]byte(testStack))
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		vendor, product string
		want            string
	}{
		{"Linux", "Kernel", "linux-kernel"},
		{"linux", "KERNEL", "linux-kernel"}, // case is not a difference
		{"Android", "Kernel", ""},           // product alone does not match
		{"Linux", "Kernel Module", ""},      // exact product, not substring
		{"GNU", "GNU C Library", "host-userland"},
		{"BeyondTrust", "Remote Support", ""}, // "rust" is not a vendor
	}
	for _, c := range cases {
		got := strings.Join(s.Classify(Vulnerability{VendorProject: c.vendor, Product: c.product}), ",")
		if got != c.want {
			t.Errorf("%s / %s: got %q, want %q", c.vendor, c.product, got, c.want)
		}
	}
	var none *Stack
	if none.Classify(Vulnerability{VendorProject: "Linux", Product: "Kernel"}) != nil || none.Relevant([]Vulnerability{{VendorProject: "Linux"}}) != nil {
		t.Fatal("a nil stack must match nothing")
	}
}

func TestParseStackRejects(t *testing.T) {
	for name, y := range map[string]string{
		"empty":         "categories: []",
		"misspelt key":  "categories:\n  - name: a\n    match:\n      - {vendor: Linux, prodcut: Kernel}\n",
		"no vendor":     "categories:\n  - name: a\n    match:\n      - {product: Kernel}\n",
		"no rules":      "categories:\n  - name: a\n    match: []\n",
		"duplicate":     "categories:\n  - {name: a, match: [{vendor: X}]}\n  - {name: a, match: [{vendor: Y}]}\n",
		"unnamed":       "categories:\n  - {match: [{vendor: X}]}\n",
		"not even yaml": "categories: [",
	} {
		if _, err := ParseStack([]byte(y)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestRepoStackFileLoads(t *testing.T) {
	s, err := LoadStack("../../../config/kev-poller/stack.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Classify(Vulnerability{VendorProject: "Linux", Product: "Kernel"}); len(got) != 1 || got[0] != "linux-kernel" {
		t.Fatalf("repo stack does not tag the Linux kernel: %v", got)
	}
}

func TestWriteReport(t *testing.T) {
	s, _ := ParseStack([]byte(testStack))
	c := &Catalog{CatalogVersion: "2026.10.16", Vulnerabilities: []Vulnerability{
		{CVEID: "CVE-2026-0001", VendorProject: "Linux", Product: "Kernel", VulnerabilityName: "OOB | write", DateAdded: "2026-09-18", DueDate: "2026-09-21"},
		{CVEID: "CVE-2026-0002", VendorProject: "Acme", Product: "Widget", VulnerabilityName: "RCE", DateAdded: "2026-09-01"},
		{CVEID: "CVE-2020-0003", VendorProject: "Sudo", Product: "Sudo", VulnerabilityName: "old", DateAdded: "2021-11-03"},
	}}
	c.Count = len(c.Vulnerabilities)
	var b bytes.Buffer
	st := &State{SHA256: strings.Repeat("ab", 32), FetchedAt: time.Date(2026, 10, 19, 4, 42, 0, 0, time.UTC)}
	if err := WriteReport(&b, c, st, s, ReportOptions{Since: "2026-04-27", Today: "2026-10-19", StackPath: "stack.yaml"}); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	for _, want := range []string{
		"### 2026-10-19: kev-poller report",
		"3 entries, dateAdded 2021-11-03 to 2026-09-18",
		"Added since 2026-04-27: **2**",
		"**1** of them; 2 in the whole catalog",
		"| 2026-09-18 | CVE-2026-0001 | Linux / Kernel | OOB / write | linux-kernel | 2026-09-21 |",
		"    pro fix CVE-2026-0001 --dry-run",
		"1 of 1 shown",
		"| 2026-09-01 | CVE-2026-0002 | Acme / Widget | RCE |",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("report lacks %q\n%s", want, out)
		}
	}
	if strings.Contains(out, "CVE-2020-0003 |") && strings.Contains(out, "pro fix CVE-2020-0003") {
		t.Error("an entry older than --since is listed as new")
	}

	b.Reset()
	if err := WriteReport(&b, c, nil, nil, ReportOptions{Since: "2026-04-27", Today: "x"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "No stack file given") {
		t.Errorf("no-stack report does not say so:\n%s", b.String())
	}
	if err := WriteReport(&b, nil, nil, s, ReportOptions{}); err == nil {
		t.Error("report with no catalog")
	}
}
