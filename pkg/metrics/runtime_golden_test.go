// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

//go:build linux

package metrics

import (
	"bytes"
	"os"
	"sort"
	"strings"
	"testing"
)

// Every go_* and process_* family must carry exactly the TYPE and HELP that
// client_golang v1.18.0 published, recorded in testdata before the
// dependency was removed. Names, types and help text are what dashboards and
// alerts key on. Values are covered by the derive and identity tests.
//
// This replaced the side-by-side parity tests, which needed client_golang
// in the same process.
func TestRuntimeCollectors_MatchClientGolangMetadata(t *testing.T) {
	golden, err := os.ReadFile("testdata/client_golang_v1.18.0_runtime_meta.golden")
	if err != nil {
		t.Fatal(err)
	}
	var want []string
	for _, l := range strings.Split(string(golden), "\n") {
		if strings.HasPrefix(l, "# HELP ") || strings.HasPrefix(l, "# TYPE ") {
			want = append(want, l)
		}
	}

	reg := NewRegistry()
	if err := NewGoCollector().Register(reg); err != nil {
		t.Fatal(err)
	}
	if err := NewProcessCollector("").Register(reg); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := reg.Gather(&buf); err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, l := range strings.Split(buf.String(), "\n") {
		if strings.HasPrefix(l, "# HELP ") || strings.HasPrefix(l, "# TYPE ") {
			got = append(got, l)
		}
	}
	sort.Strings(got)

	if len(want) != 68 {
		t.Fatalf("golden has %d lines, want 68 (34 families × HELP+TYPE)", len(want))
	}
	wantSet := map[string]bool{}
	for _, l := range want {
		wantSet[l] = true
	}
	gotSet := map[string]bool{}
	for _, l := range got {
		gotSet[l] = true
		if !wantSet[l] {
			t.Errorf("ours only:            %s", l)
		}
	}
	for _, l := range want {
		if !gotSet[l] {
			t.Errorf("client_golang only:   %s", l)
		}
	}
}
