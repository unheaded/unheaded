// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeRule writes a one-alert rule file under root.
func writeRule(t *testing.T, root, rel, alert, urgency, runbook string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	s := "groups:\n  - name: x\n    rules:\n      - alert: " + alert + "\n        expr: vector(1)\n        labels:\n          urgency: " + urgency + "\n"
	if runbook != "" {
		s += "        annotations:\n          runbook_url: \"" + runbook + "\"\n"
	}
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

// A module's rules (ADR-149) obey the same urgency rule as the platform's.
func TestCheckHandRules_ReadsModuleRules(t *testing.T) {
	root := t.TempDir()
	writeRule(t, root, rulesDir+"/a.yml", "A", "none", "")
	writeRule(t, root, "modules/zhen/prometheus/rules/z.yml", "Z", "ticket", "")
	errs := checkHandRules(root, &Policy{})
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "modules/zhen/prometheus/rules/z.yml") {
		t.Fatalf("errs = %v, want one naming the module's file", errs)
	}
}

// A promotion may name a module's alert; one naming no alert anywhere fails.
func TestCheckHandRules_PromotesAModuleAlert(t *testing.T) {
	root := t.TempDir()
	writeRule(t, root, rulesDir+"/a.yml", "A", "none", "")
	writeRule(t, root, "modules/zhen/prometheus/rules/z.yml", "Z", "none", "")
	if errs := checkHandRules(root, &Policy{Promote: []Promotion{{Alert: "Z"}}}); len(errs) != 0 {
		t.Fatalf("promoting a module alert: %v", errs)
	}
	errs := checkHandRules(root, &Policy{Promote: []Promotion{{Alert: "Q"}}})
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "modules/*/prometheus/rules") {
		t.Fatalf("errs = %v, want one promote error naming both directories", errs)
	}
}

// The runbook_url gate reads a module's rules too.
func TestRunbookURLs_ReadsModuleRules(t *testing.T) {
	root := t.TempDir()
	writeRule(t, root, rulesDir+"/a.yml", "A", "none", "runbooks/observe/ok.yaml")
	writeRule(t, root, "modules/zhen/prometheus/rules/z.yml", "Z", "none", "runbooks/none.yaml")
	if err := os.MkdirAll(filepath.Join(root, "runbooks/observe"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "runbooks/observe/ok.yaml"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	errs := checkRunbookURLs(root)
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "Z: runbooks/none.yaml does not exist") {
		t.Fatalf("errs = %v", errs)
	}
}
