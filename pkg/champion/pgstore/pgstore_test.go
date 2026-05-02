// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Steven Bellis. All rights reserved.

package pgstore

import (
	"strings"
	"testing"
)

// TestSchema_Embedded verifies the go:embed directive is wired and
// the SQL contains the expected table definition. Doesn't run the
// SQL — that requires a live PG (covered by the package-level
// integration test in pkg/champion/integration_test.go).
func TestSchema_Embedded(t *testing.T) {
	s := Schema()
	if s == "" {
		t.Fatal("Schema() returned empty — go:embed not wired")
	}
	for _, must := range []string{
		"CREATE TABLE IF NOT EXISTS zhen_actions",
		"BIGSERIAL",
		"PRIMARY KEY",
		"session_id",
		"action_type",
		"planned_at",
		"completed_at",
		"elapsed_ms",
		"CREATE INDEX IF NOT EXISTS zhen_actions_session_id_idx",
	} {
		if !strings.Contains(s, must) {
			t.Errorf("schema missing %q", must)
		}
	}
}

func TestNullableString(t *testing.T) {
	cases := []struct {
		in   string
		want any
	}{
		{"", nil},
		{"hello", "hello"},
		{" ", " "}, // whitespace is content
	}
	for _, c := range cases {
		got := nullableString(c.in)
		if got != c.want {
			t.Errorf("nullableString(%q) = %v; want %v", c.in, got, c.want)
		}
	}
}

func TestNew_NotNil(t *testing.T) {
	s := New(nil) // permissive constructor; nil DB is the caller's problem
	if s == nil {
		t.Fatal("New returned nil")
	}
}
