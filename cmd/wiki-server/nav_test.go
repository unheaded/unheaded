// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"os"
	"regexp"
	"testing"
)

// Doom left this repository, so the wiki's navigation must not link to it.
// This lives here, not with the move-out residue table: a path named under
// scripts/ makes the live-path inventory read wiki-server as a tool.
func TestNavigationHasNoDoomLink(t *testing.T) {
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	if loc := regexp.MustCompile(`:16666|>Doom<`).Find(src); loc != nil {
		t.Errorf("main.go still links to Doom: %q", loc)
	}
}
