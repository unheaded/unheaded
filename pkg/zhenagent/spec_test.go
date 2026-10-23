// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package zhenagent

import (
	"errors"
	"strings"
	"testing"
)

var knownChecks = func(k string) bool { return k == "go-test" || k == "fixture-check" || k == "shellcheck" }

func block(body string) string { return "Fix the thing.\n\n```zhen\n" + body + "```\n" }

func TestParseSpec_HappyPath(t *testing.T) {
	s, err := ParseSpec(block("family: repo-task\ncheck: go-test\npath: ./pkg/example/...\nmax_claude_usd: 0.25\n"), knownChecks)
	if err != nil {
		t.Fatal(err)
	}
	if s.Family != "repo-task" || s.Check != "go-test" || s.Path != "./pkg/example/..." || s.MaxClaudeUSD != 0.25 {
		t.Errorf("spec = %+v", s)
	}
	if s.Task != "Fix the thing." {
		t.Errorf("task text = %q (the block is not part of it)", s.Task)
	}
}

func TestParseSpec_Refusals(t *testing.T) {
	cases := []struct {
		name string
		desc string
		want error
	}{
		{"no block", "Just do it.", ErrNoSpec},
		{"two blocks", block("family: a\ncheck: go-test\npath: .\n") + block("family: b\ncheck: go-test\npath: .\n"), ErrBadSpec},
		{"unknown key", block("family: repo-task\ncheck: go-test\npath: .\npush: true\n"), ErrBadSpec},
		{"missing check", block("family: repo-task\npath: .\n"), ErrBadSpec},
		{"missing path", block("family: repo-task\ncheck: go-test\n"), ErrBadSpec},
		{"check not allowlisted", block("family: repo-task\ncheck: rm\npath: .\n"), ErrBadSpec},
		{"check with a shell", block("family: repo-task\ncheck: \"go-test; rm -rf ~\"\npath: .\n"), ErrBadSpec},
		{"path traversal", block("family: repo-task\ncheck: go-test\npath: ../../\n"), ErrBadSpec},
		{"path traversal inside", block("family: repo-task\ncheck: go-test\npath: pkg/../../etc\n"), ErrBadSpec},
		{"absolute path", block("family: repo-task\ncheck: go-test\npath: /etc\n"), ErrBadSpec},
		{"option as path", block("family: repo-task\ncheck: go-test\npath: -exec=sh\n"), ErrBadSpec},
		{"path with a space", block("family: repo-task\ncheck: go-test\npath: a b\n"), ErrBadSpec},
		{"family with a slash", block("family: ../x\ncheck: go-test\npath: .\n"), ErrBadSpec},
		{"negative budget", block("family: repo-task\ncheck: go-test\npath: .\nmax_claude_usd: -1\n"), ErrBadSpec},
		{"huge budget", block("family: repo-task\ncheck: go-test\npath: .\nmax_claude_usd: 50\n"), ErrBadSpec},
		{"not a map", block("- a\n- b\n"), ErrBadSpec},
		{"1 MB description", block("family: repo-task\ncheck: go-test\npath: .\n") + strings.Repeat("x", 1<<20), ErrDescriptionTooLarge},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := ParseSpec(c.desc, knownChecks)
			if !errors.Is(err, c.want) {
				t.Errorf("err = %v, want %v", err, c.want)
			}
		})
	}
}

func TestParseSpec_GoPatternAllowed(t *testing.T) {
	for _, p := range []string{".", "./...", "pkg/x", "./pkg/x/...", "eval/two-brains/live/go-nil-map"} {
		if _, err := ParseSpec(block("family: repo-task\ncheck: go-test\npath: "+p+"\n"), knownChecks); err != nil {
			t.Errorf("%s: %v", p, err)
		}
	}
}

func TestEligible(t *testing.T) {
	ok := Card{ID: "c1", Status: "todo", Owner: "zhen", Type: "task"}
	if err := Eligible(ok); err != nil {
		t.Fatalf("eligible card refused: %v", err)
	}
	for name, c := range map[string]Card{
		"wrong owner":  {ID: "c1", Status: "todo", Owner: "Stevie", Type: "task"},
		"not todo":     {ID: "c1", Status: "in-progress", Owner: "zhen", Type: "task"},
		"archived":     {ID: "c1", Status: "todo", Owner: "zhen", Type: "task", Archived: true},
		"deleted":      {ID: "c1", Status: "todo", Owner: "zhen", Type: "task", Deleted: true},
		"wishlist":     {ID: "c1", Status: "todo", Owner: "zhen", Type: "wishlist"},
		"milestone":    {ID: "c1", Status: "todo", Owner: "zhen", Type: "milestone"},
		"owner spaced": {ID: "c1", Status: "todo", Owner: " zhen", Type: "task"},
	} {
		if Eligible(c) == nil {
			t.Errorf("%s: eligible", name)
		}
	}
}

func TestBranchFor(t *testing.T) {
	cases := map[string]string{"go-nil-map": "zhen/go-nil-map", "Fix_The_Thing": "zhen/fix-the-thing", "a1": "zhen/a1"}
	for id, want := range cases {
		got, err := BranchFor(id, 1)
		if err != nil || got != want {
			t.Errorf("BranchFor(%q) = %q, %v; want %q", id, got, err, want)
		}
	}
	if got, _ := BranchFor("go-nil-map", 2); got != "zhen/go-nil-map-2" {
		t.Errorf("second attempt = %q", got)
	}
	for _, bad := range []string{"", "-x", "a/b", "a..b", strings.Repeat("a", 61), "é"} {
		if b, err := BranchFor(bad, 1); err == nil {
			t.Errorf("BranchFor(%q) = %q, want an error", bad, b)
		}
	}
}

func TestNewRunID(t *testing.T) {
	seen := map[string]bool{}
	for range 100 {
		id := NewRunID()
		if !ValidRunID(id) || seen[id] {
			t.Fatalf("id %q", id)
		}
		seen[id] = true
	}
	if ValidRunID("zzzz") || ValidRunID("") {
		t.Error("ValidRunID accepts garbage")
	}
}
