// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The repo's generated files match its policy, and every hand-written rule
// is urgency none. CI runs the same check (security.yml, prometheus-rules).
func TestRepoIsCurrent(t *testing.T) {
	if errs := run(filepath.Join("..", ".."), true); len(errs) > 0 {
		t.Fatal(errs)
	}
}

// tree copies the policy and rules into a temp repo for the edit cases.
func tree(t *testing.T, policyEdit func(string) string, extraRule string) string {
	t.Helper()
	root := t.TempDir()
	for _, d := range []string{"observability/alerting", rulesDir, "monitoring/alertmanager"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	p, err := os.ReadFile(filepath.Join("..", "..", policyPath))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, policyPath), []byte(policyEdit(string(p))), 0o644); err != nil {
		t.Fatal(err)
	}
	if extraRule != "" {
		if err := os.WriteFile(filepath.Join(root, rulesDir, "extra.yml"), []byte(extraRule), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func same(s string) string { return s }

func TestRefusals(t *testing.T) {
	replace := func(old, new string) func(string) string {
		return func(s string) string {
			if !strings.Contains(s, old) {
				t.Fatalf("anchor %q not in policy", old)
			}
			return strings.Replace(s, old, new, 1)
		}
	}
	cases := []struct {
		name  string
		edit  func(string) string
		rule  string
		wants string
	}{
		{"hand rule that pages", same, "groups:\n  - name: x\n    rules:\n      - alert: Loud\n        expr: vector(1)\n        labels:\n          urgency: page\n", "hand-written rules are urgency \"none\""},
		{"hand rule without urgency", same, "groups:\n  - name: x\n    rules:\n      - alert: Unlabelled\n        expr: vector(1)\n", "Unlabelled has urgency \"\""},
		{"objective the fast tier can never reach", replace("objective: 99.5", "objective: 90"), "", "want 95 <= objective < 100"},
		{"cap of zero", replace("max_per_day: 6", "max_per_day: 0"), "", "max_per_day 0"},
		{"cap too big to mean anything", replace("max_per_day: 6", "max_per_day: 500"), "", "max_per_day 500"},
		{"unknown mode", replace("mode: dry-run", "mode: yolo"), "", `mailer.mode "yolo"`},
		{"template in text", replace("description: The message bus answers on both hosts", "description: hi {{ $labels }}"), "", "may not contain {{"},
		{"duplicate slo", replace("  - name: dashboard\n", "  - name: wotan\n"), "", "duplicate name"},
		{"promotion of an alert that does not exist", replace("promote: []", "promote:\n  - alert: Nope\n    urgency: ticket\n    reason: test"), "", "Nope is not an alert"},
		{"promotion without a reason", replace("promote: []", "promote:\n  - alert: HostDiskFull\n    urgency: ticket\n    reason: \"\""), "", "a reason is required"},
		{"unknown policy key", replace("window: 28d", "window: 28d\nwindw: 7d"), "", "field windw not found"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := tree(t, c.edit, c.rule)
			// Hand rules from the repo, so promote: can name real alerts.
			matches, _ := filepath.Glob(filepath.Join("..", "..", rulesDir, "*.yml"))
			for _, m := range matches {
				b, _ := os.ReadFile(m)
				_ = os.WriteFile(filepath.Join(root, rulesDir, filepath.Base(m)), b, 0o644)
			}
			errs := run(root, false)
			var all []string
			for _, e := range errs {
				all = append(all, e.Error())
			}
			if !strings.Contains(strings.Join(all, "\n"), c.wants) {
				t.Fatalf("want an error containing %q, got %v", c.wants, all)
			}
		})
	}
}

// A promotion routes exactly that alert, ahead of the urgency routes.
func TestPromotionRoutesOneAlert(t *testing.T) {
	root := tree(t, func(s string) string {
		return strings.Replace(s, "promote: []", "promote:\n  - alert: HostDiskFull\n    urgency: ticket\n    reason: test", 1)
	}, "")
	matches, _ := filepath.Glob(filepath.Join("..", "..", rulesDir, "*.yml"))
	for _, m := range matches {
		b, _ := os.ReadFile(m)
		_ = os.WriteFile(filepath.Join(root, rulesDir, filepath.Base(m)), b, 0o644)
	}
	if errs := run(root, false); len(errs) > 0 {
		t.Fatal(errs)
	}
	am, _ := os.ReadFile(filepath.Join(root, amConfig))
	s := string(am)
	promo := strings.Index(s, `alertname="HostDiskFull"`)
	page := strings.Index(s, `- urgency="page"`)
	if promo < 0 || page < 0 || promo > page {
		t.Fatalf("promotion route missing or after the urgency routes:\n%s", s)
	}
}
