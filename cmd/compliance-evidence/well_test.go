// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"io"
	"strings"
	"testing"

	"unheaded/pkg/compliance/well"
)

func TestWellSyncNeedsTheWell(t *testing.T) {
	t.Setenv("WELL_HOST", "127.0.0.1")
	t.Setenv("WELL_PORT", "1")
	t.Setenv("WELL_CONFIG_USER", "config_admin")
	t.Setenv("WELL_CONFIG_PASSWORD", "x")
	err := runWell([]string{"sync", "-dry-run", "-repo-dir", "../.."}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "unheaded_config") {
		t.Fatalf("got %v, want a loud unheaded_config failure (writes never fall back)", err)
	}
	if err := runWell([]string{"push"}, io.Discard); err == nil {
		t.Error("unknown subcommand accepted")
	}
}

func TestRenderSync(t *testing.T) {
	r := &well.SyncReport{
		Created:  map[string]int{"compliance_fixes": 10},
		Updated:  map[string]int{},
		Same:     map[string]int{"compliance_profiles": 4},
		Diverged: []string{"binding:west"},
	}
	out := renderSync(r, true)
	for _, want := range []string{"dry run", "compliance_fixes", "10 new", "binding:west"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
}
