// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package cmd

import (
	"testing"
)

// parseNetworkPolicy is a CLI helper that parses a simplified key:value
// YAML-ish network policy. It currently has no command callers (per
// golangci-lint unused warning) — these tests pin its contract so the
// helper isn't accidentally deleted before the consuming command lands.

func TestParseNetworkPolicy_BasicKeyValue(t *testing.T) {
	t.Parallel()
	got, err := parseNetworkPolicy("name: web-allow\naction: allow\n")
	if err != nil {
		t.Fatalf("parseNetworkPolicy: %v", err)
	}
	if got["name"] != "web-allow" {
		t.Errorf("name: got %v, want web-allow", got["name"])
	}
	if got["action"] != "allow" {
		t.Errorf("action: got %v, want allow", got["action"])
	}
}

func TestParseNetworkPolicy_SkipsBlankAndCommentLines(t *testing.T) {
	t.Parallel()
	got, err := parseNetworkPolicy(`
# This is a comment
name: x

# trailing comment
port: 443
`)
	if err != nil {
		t.Fatalf("parseNetworkPolicy: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 entries (name, port), got %d: %v", len(got), got)
	}
}

func TestParseNetworkPolicy_TrimsWhitespace(t *testing.T) {
	t.Parallel()
	got, err := parseNetworkPolicy("  name  :   web-allow  \n")
	if err != nil {
		t.Fatalf("parseNetworkPolicy: %v", err)
	}
	if got["name"] != "web-allow" {
		t.Errorf("got %v, want trimmed 'web-allow'", got["name"])
	}
}

func TestParseNetworkPolicy_EmptyInputReturnsEmptyMap(t *testing.T) {
	t.Parallel()
	got, err := parseNetworkPolicy("")
	if err != nil {
		t.Fatalf("parseNetworkPolicy: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("empty input should give empty map, got %v", got)
	}
}
