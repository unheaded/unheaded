//go:build !integration

// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package metrics

import (
	"context"
	"testing"
	"time"
)

func TestParseSystemdUnits(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantCount int
		wantErr   bool
	}{
		{
			name: "valid systemd units",
			input: `[
  {"unit":"test.service","load":"loaded","active":"active","sub":"running","description":"Test Service"},
  {"unit":"other.service","load":"loaded","active":"inactive","sub":"dead","description":"Other Service"}
]`,
			wantCount: 2,
			wantErr:   false,
		},
		{
			name:      "empty array",
			input:     "[]",
			wantCount: 0,
			wantErr:   false,
		},
		{
			name:      "empty string",
			input:     "",
			wantCount: 0,
			wantErr:   false,
		},
		{
			name:      "invalid json",
			input:     "{invalid}",
			wantCount: 0,
			wantErr:   true,
		},
		{
			name: "single unit",
			input: `[
  {"unit":"nginx.service","load":"loaded","active":"active","sub":"running","description":"Nginx Web Server"}
]`,
			wantCount: 1,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			collector := NewNixOSCollector()
			units, err := collector.parseSystemdUnits(tt.input)

			if (err != nil) != tt.wantErr {
				t.Errorf("parseSystemdUnits() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if len(units) != tt.wantCount {
				t.Errorf("parseSystemdUnits() got %d units, want %d", len(units), tt.wantCount)
			}
		})
	}
}

func TestParseJournalErrors(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantCount int
		wantErr   bool
	}{
		{
			name: "multiple errors",
			input: `[
  {"MESSAGE":"Error 1","PRIORITY":"3"},
  {"MESSAGE":"Error 2","PRIORITY":"3"},
  {"MESSAGE":"Error 3","PRIORITY":"3"}
]`,
			wantCount: 3,
			wantErr:   false,
		},
		{
			name:      "no errors",
			input:     "[]",
			wantCount: 0,
			wantErr:   false,
		},
		{
			name:      "empty string",
			input:     "",
			wantCount: 0,
			wantErr:   false,
		},
		{
			name:      "invalid json",
			input:     "{invalid}",
			wantCount: 0,
			wantErr:   true,
		},
		{
			name: "single error",
			input: `[
  {"MESSAGE":"System failed to start service","PRIORITY":"3"}
]`,
			wantCount: 1,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			collector := NewNixOSCollector()
			count, err := collector.parseJournalErrors(tt.input)

			if (err != nil) != tt.wantErr {
				t.Errorf("parseJournalErrors() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if count != tt.wantCount {
				t.Errorf("parseJournalErrors() got %d, want %d", count, tt.wantCount)
			}
		})
	}
}

func TestNixOSCollector_Name(t *testing.T) {
	collector := NewNixOSCollector()
	if name := collector.Name(); name != "nixos" {
		t.Errorf("Name() = %q, want %q", name, "nixos")
	}
}

func TestNixOSCollector_Describe(t *testing.T) {
	collector := NewNixOSCollector()
	desc := collector.Describe()

	if len(desc) == 0 {
		t.Error("Describe() returned empty list")
	}

	for _, d := range desc {
		if d == "" {
			t.Error("Describe() contains empty string")
		}
	}
}

func TestNixOSCollector_Collect(t *testing.T) {
	// Use a nonexistent systemctl path to simulate unavailable systemd
	collector := NewNixOSCollector()
	collector.systemctlPath = "/nonexistent/systemctl"
	collector.journalctlPath = "/nonexistent/journalctl"

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	samples, err := collector.Collect(ctx)

	// Should not fail, should return degraded availability
	if err != nil {
		t.Errorf("Collect() should not fail when systemctl is unavailable, got: %v", err)
	}

	// Should return at least the availability sample
	if len(samples) < 1 {
		t.Error("Collect() should return at least one sample")
	}

	// Check for availability sample
	foundAvailability := false
	for _, s := range samples {
		if s.Name == "unheaded_systemd_available" {
			foundAvailability = true
			// Should be 0 since systemctl is unavailable
			if s.Value != 0 {
				t.Errorf("Expected availability=0 when systemctl unavailable, got %f", s.Value)
			}
			break
		}
	}

	if !foundAvailability {
		t.Error("Expected unheaded_systemd_available sample")
	}
}

func TestParseSystemdUnitsStateCount(t *testing.T) {
	input := `[
  {"unit":"service1.service","load":"loaded","active":"active","sub":"running","description":"Service 1"},
  {"unit":"service2.service","load":"loaded","active":"active","sub":"running","description":"Service 2"},
  {"unit":"service3.service","load":"loaded","active":"inactive","sub":"dead","description":"Service 3"},
  {"unit":"service4.service","load":"loaded","active":"failed","sub":"failed","description":"Service 4"}
]`

	collector := NewNixOSCollector()
	units, err := collector.parseSystemdUnits(input)
	if err != nil {
		t.Fatalf("parseSystemdUnits() error = %v", err)
	}

	if len(units) != 4 {
		t.Errorf("expected 4 units, got %d", len(units))
	}

	// Count by state
	stateCounts := make(map[string]int)
	for _, unit := range units {
		stateCounts[unit.ActiveState]++
	}

	if stateCounts["active"] != 2 {
		t.Errorf("expected 2 active units, got %d", stateCounts["active"])
	}

	if stateCounts["inactive"] != 1 {
		t.Errorf("expected 1 inactive unit, got %d", stateCounts["inactive"])
	}

	if stateCounts["failed"] != 1 {
		t.Errorf("expected 1 failed unit, got %d", stateCounts["failed"])
	}
}
