// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package campaigns

import (
	"context"
	"strings"
	"testing"
	"time"
)

// ─── NewRunner ───────────────────────────────────────────────────────────────

func TestNewRunner_RegistersAll6Campaigns(t *testing.T) {
	t.Parallel()
	cfg := Config{
		GatewayAddr:   "http://localhost:21000",
		WotanAddr:     "localhost:18001",
		DashboardAddr: "http://localhost:20000",
	}
	r := NewRunner(cfg)
	if r == nil {
		t.Fatal("NewRunner returned nil")
	}
	want := []string{"d1", "d2", "d3", "d4", "d5", "d6"}
	for _, name := range want {
		if _, ok := r.campaigns[name]; !ok {
			t.Errorf("expected campaign %q to be registered", name)
		}
	}
	if got := len(r.campaigns); got != 6 {
		t.Errorf("expected exactly 6 campaigns, got %d", got)
	}
}

func TestNewRunner_PreservesConfig(t *testing.T) {
	t.Parallel()
	cfg := Config{GatewayAddr: "http://gw.example", WotanAddr: "w:9000"}
	r := NewRunner(cfg)
	if r.cfg.GatewayAddr != cfg.GatewayAddr {
		t.Errorf("GatewayAddr lost: got %q, want %q", r.cfg.GatewayAddr, cfg.GatewayAddr)
	}
	if r.cfg.WotanAddr != cfg.WotanAddr {
		t.Errorf("WotanAddr lost: got %q, want %q", r.cfg.WotanAddr, cfg.WotanAddr)
	}
}

// ─── Run with unknown campaign name ──────────────────────────────────────────

func TestRunner_Run_UnknownCampaignReturnsErrorResult(t *testing.T) {
	t.Parallel()
	r := NewRunner(Config{})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	results := r.Run(ctx, []string{"d99"})
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Campaign != "d99" {
		t.Errorf("Campaign: got %q, want d99", results[0].Campaign)
	}
	if !strings.Contains(results[0].Error, "unknown campaign") {
		t.Errorf("Error should contain 'unknown campaign', got: %q", results[0].Error)
	}
}

func TestRunner_Run_EmptySelectionReturnsEmptyResults(t *testing.T) {
	t.Parallel()
	r := NewRunner(Config{})
	ctx := context.Background()
	results := r.Run(ctx, nil)
	if len(results) != 0 {
		t.Errorf("expected 0 results for empty selection, got %d", len(results))
	}
}

// ─── GenerateReport ──────────────────────────────────────────────────────────

func TestGenerateReport_NoFindings_StatesAllCampaignsPassed(t *testing.T) {
	t.Parallel()
	results := []CampaignResult{
		{Campaign: "d1", Description: "Auth", TestsRun: 5, TestsPassed: 5},
		{Campaign: "d2", Description: "Inj", TestsRun: 3, TestsPassed: 3},
	}
	report := GenerateReport(results)
	for _, want := range []string{
		"# Lich Security Audit Report",
		"## Executive Summary",
		"**Campaigns run:** 2",
		"**Total tests:** 8",
		"**Tests passed:** 8",
		"**Total findings:** 0",
		"All campaigns passed",
		"| d1 | PASS |",
		"| d2 | PASS |",
	} {
		if !strings.Contains(report, want) {
			t.Errorf("report missing %q\n--- report ---\n%s", want, report)
		}
	}
}

func TestGenerateReport_WithFindings_MarksFAILAndIncludesDetails(t *testing.T) {
	t.Parallel()
	results := []CampaignResult{
		{
			Campaign:    "d1",
			Description: "Auth Bypass",
			TestsRun:    5,
			TestsPassed: 4,
			Findings: []Finding{
				{
					ID:          "D1-001",
					Severity:    "high",
					Title:       "Default password accepted",
					Description: "Endpoint accepts admin/admin",
					Remediation: "Disable default credentials",
					Endpoint:    "/api/v1/login",
				},
			},
		},
	}
	report := GenerateReport(results)
	for _, want := range []string{
		"| d1 | FAIL |",
		"### D1-001 — Default password accepted",
		"**Severity:** high",
		"**Endpoint:** /api/v1/login",
		"**Description:** Endpoint accepts admin/admin",
		"**Remediation:** Disable default credentials",
	} {
		if !strings.Contains(report, want) {
			t.Errorf("report missing %q\n--- report ---\n%s", want, report)
		}
	}
}

func TestGenerateReport_WithError_MarksERROR(t *testing.T) {
	t.Parallel()
	results := []CampaignResult{
		{Campaign: "d4", Description: "Secrets", Error: "wotan unreachable"},
	}
	report := GenerateReport(results)
	if !strings.Contains(report, "| d4 | ERROR |") {
		t.Errorf("report should mark errored campaign as ERROR, got:\n%s", report)
	}
}

// ─── Campaign interface conformance — sanity-check all 6 implementations ────

func TestAllCampaignsHaveNonEmptyNameAndDescription(t *testing.T) {
	t.Parallel()
	r := NewRunner(Config{})
	for key, c := range r.campaigns {
		if c.Name() == "" {
			t.Errorf("campaign %q has empty Name()", key)
		}
		if c.Description() == "" {
			t.Errorf("campaign %q has empty Description()", key)
		}
	}
}
