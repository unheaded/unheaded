// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"unheaded/pkg/health"
)

// ─── loadConfig ──────────────────────────────────────────────────────────────

func TestLoadConfig_ParsesValidYAML(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "akira.yaml")
	body := `targets:
  - name: wotan
    host: 10.10.10.10
    port: 18000
    health_path: /health
  - name: timeguru
    host: 10.10.10.20
    port: 19000
    health_path: /api/health
`
	if err := os.WriteFile(cfgPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	targets, err := loadConfig(cfgPath)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if len(targets) != 2 {
		t.Fatalf("got %d targets, want 2", len(targets))
	}
	if targets[0].Name != "wotan" || targets[0].Port != 18000 {
		t.Errorf("targets[0] = %+v", targets[0])
	}
	if targets[1].HealthPath != "/api/health" {
		t.Errorf("targets[1].HealthPath = %q, want /api/health", targets[1].HealthPath)
	}
}

func TestLoadConfig_DefaultsHostAndHealthPath(t *testing.T) {
	t.Parallel()
	// Per loadConfig: host="" → "localhost"; health_path="" → "/health".
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "akira.yaml")
	body := `targets:
  - name: minimal
    port: 19999
`
	if err := os.WriteFile(cfgPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	targets, err := loadConfig(cfgPath)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("got %d targets, want 1", len(targets))
	}
	if targets[0].Host != "localhost" {
		t.Errorf("missing-host should default to localhost; got %q", targets[0].Host)
	}
	if targets[0].HealthPath != "/health" {
		t.Errorf("missing-health-path should default to /health; got %q", targets[0].HealthPath)
	}
}

func TestLoadConfig_MissingFileErrors(t *testing.T) {
	t.Parallel()
	_, err := loadConfig("/nonexistent/akira.yaml")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
	if !os.IsNotExist(err) && !strings.Contains(err.Error(), "no such file") {
		t.Errorf("expected file-not-found error, got: %v", err)
	}
}

func TestLoadConfig_InvalidYAMLErrors(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "broken.yaml")
	if err := os.WriteFile(cfgPath, []byte("targets: [not-a-list-but-string"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := loadConfig(cfgPath)
	if err == nil {
		t.Fatal("expected YAML parse error, got nil")
	}
}

// ─── findPort ────────────────────────────────────────────────────────────────

func TestFindPort_KnownNameReturnsPort(t *testing.T) {
	t.Parallel()
	targets := []health.ServiceTarget{
		{Name: "wotan", Port: 18000},
		{Name: "monad", Port: 19004},
	}
	if got := findPort("monad", targets); got != 19004 {
		t.Errorf("findPort(monad) = %d, want 19004", got)
	}
}

func TestFindPort_UnknownNameReturnsZero(t *testing.T) {
	t.Parallel()
	targets := []health.ServiceTarget{
		{Name: "wotan", Port: 18000},
	}
	if got := findPort("unknown", targets); got != 0 {
		t.Errorf("findPort(unknown) = %d, want 0", got)
	}
	if got := findPort("", nil); got != 0 {
		t.Errorf("findPort('', nil) = %d, want 0", got)
	}
}

// ─── defaultTargets invariants ──────────────────────────────────────────────

func TestDefaultTargets_NamesUnique(t *testing.T) {
	t.Parallel()
	seen := make(map[string]int)
	for _, tgt := range defaultTargets {
		seen[tgt.Name]++
	}
	for name, count := range seen {
		if count > 1 {
			t.Errorf("duplicate Name in defaultTargets: %s appears %d times", name, count)
		}
	}
}

func TestDefaultTargets_PortsUnique(t *testing.T) {
	t.Parallel()
	seen := make(map[int]string)
	for _, tgt := range defaultTargets {
		if existing, ok := seen[tgt.Port]; ok {
			t.Errorf("duplicate port %d: %s and %s", tgt.Port, existing, tgt.Name)
		}
		seen[tgt.Port] = tgt.Name
	}
}

func TestDefaultTargets_AllPortsInDoomRange(t *testing.T) {
	t.Parallel()
	// CLAUDE.md: Doom Range 16666-26666.
	for _, tgt := range defaultTargets {
		if tgt.Port < 16666 || tgt.Port > 26666 {
			t.Errorf("%s port %d outside Doom Range (16666-26666)", tgt.Name, tgt.Port)
		}
	}
}

func TestDefaultTargets_AllUseHealthPath(t *testing.T) {
	t.Parallel()
	for _, tgt := range defaultTargets {
		if tgt.HealthPath != "/health" {
			t.Errorf("%s HealthPath = %q, want /health (convention)", tgt.Name, tgt.HealthPath)
		}
	}
}

// ─── /ready ──────────────────────────────────────────────────────────────────

type fakeSweeper uint64

func (f fakeSweeper) Sweeps() uint64 { return uint64(f) }

// Ready means one full sweep has completed. Before that /api/v1/status is
// empty, and a constant 200 would publish a readiness nothing measured.
func TestReadyHandler(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		sweeps uint64
		want   int
	}{{0, http.StatusServiceUnavailable}, {1, http.StatusOK}, {7, http.StatusOK}} {
		rec := httptest.NewRecorder()
		readyHandler(fakeSweeper(tc.sweeps)).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ready", nil))
		if rec.Code != tc.want {
			t.Errorf("sweeps=%d: status %d, want %d", tc.sweeps, rec.Code, tc.want)
		}
		if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
			t.Errorf("sweeps=%d: Content-Type %q", tc.sweeps, ct)
		}
	}
}
