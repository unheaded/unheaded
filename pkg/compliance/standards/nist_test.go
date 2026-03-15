// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package standards

import (
	"testing"

	"unheaded/pkg/compliance/controls"
)

func TestNewNISTStandard(t *testing.T) {
	s := NewNISTStandard()

	if s.ID() != "NIST-800-53" {
		t.Errorf("expected ID %q, got %q", "NIST-800-53", s.ID())
	}
	if s.Name() != "NIST Special Publication 800-53" {
		t.Errorf("unexpected name: %s", s.Name())
	}
	if s.Version() != "Rev 5" {
		t.Errorf("expected version %q, got %q", "Rev 5", s.Version())
	}
	if s.Description() == "" {
		t.Error("expected non-empty description")
	}
}

func TestNISTControlCount(t *testing.T) {
	s := NewNISTStandard()
	ctrls := s.Controls()
	if len(ctrls) == 0 {
		t.Fatal("expected NIST to have controls")
	}
	// The standard defines controls across 8 families
	// AC(12) + AU(9) + CA(4) + CM(6) + CP(5) + IA(4) + IR(5) + SC/SI(7) = 52
	// Just verify a reasonable minimum
	if len(ctrls) < 40 {
		t.Errorf("expected at least 40 controls, got %d", len(ctrls))
	}
}

func TestNISTAllControlsHaveStandard(t *testing.T) {
	s := NewNISTStandard()
	for _, c := range s.Controls() {
		if c.Standard != "NIST-800-53" {
			t.Errorf("control %s has standard %q, expected NIST-800-53", c.ID, c.Standard)
		}
	}
}

func TestNISTAccessControlFamily(t *testing.T) {
	s := NewNISTStandard()
	knownAC := []string{"AC-1", "AC-2", "AC-3", "AC-4", "AC-5", "AC-6", "AC-7"}
	for _, id := range knownAC {
		c, ok := s.GetControl(id)
		if !ok {
			t.Errorf("expected to find control %s", id)
			continue
		}
		if c.Severity == "" {
			t.Errorf("control %s should have severity set", id)
		}
		if c.Name == "" {
			t.Errorf("control %s should have a name", id)
		}
	}
}

func TestNISTAuditFamily(t *testing.T) {
	s := NewNISTStandard()
	auditIDs := []string{"AU-1", "AU-2", "AU-3", "AU-6", "AU-9", "AU-12"}
	for _, id := range auditIDs {
		c, ok := s.GetControl(id)
		if !ok {
			t.Errorf("expected to find audit control %s", id)
			continue
		}
		if c.Category != controls.CategoryAuditLogging {
			t.Errorf("control %s expected category %q, got %q", id, controls.CategoryAuditLogging, c.Category)
		}
	}
}

func TestNISTCrossFrameworkMappings(t *testing.T) {
	s := NewNISTStandard()

	tests := []struct {
		controlID string
		framework string
		expected  string
	}{
		{"AC-2", "SOC2", "CC6.1"},
		{"AC-2", "PCI-DSS", "8.1"},
		{"AC-6", "PCI-DSS", "7.1"},
		{"AU-2", "SOC2", "CC7.2"},
		{"SC-7", "SOC2", "CC6.6"},
		{"SC-8", "PCI-DSS", "4.1"},
		{"SI-4", "SOC2", "CC7.1"},
	}

	for _, tt := range tests {
		c, ok := s.GetControl(tt.controlID)
		if !ok {
			t.Errorf("control %s not found", tt.controlID)
			continue
		}
		mapped, hasMapped := c.Mappings[tt.framework]
		if !hasMapped {
			t.Errorf("control %s missing %s mapping", tt.controlID, tt.framework)
			continue
		}
		if mapped != tt.expected {
			t.Errorf("control %s mapping to %s: expected %q, got %q", tt.controlID, tt.framework, tt.expected, mapped)
		}
	}
}

func TestNISTCategories(t *testing.T) {
	s := NewNISTStandard()
	categories := make(map[controls.ControlCategory]int)
	for _, c := range s.Controls() {
		categories[c.Category]++
	}

	// NIST should have controls across multiple categories
	if len(categories) < 5 {
		t.Errorf("expected at least 5 distinct categories, got %d", len(categories))
	}

	// Access control should have several controls
	if categories[controls.CategoryAccessControl] == 0 {
		t.Error("expected access control category to have controls")
	}
	if categories[controls.CategoryAuditLogging] == 0 {
		t.Error("expected audit logging category to have controls")
	}
}

func TestNISTSeverities(t *testing.T) {
	s := NewNISTStandard()
	severities := make(map[controls.Severity]int)
	for _, c := range s.Controls() {
		severities[c.Severity]++
	}

	if severities[controls.SeverityCritical] == 0 {
		t.Error("expected some critical severity controls")
	}
	if severities[controls.SeverityHigh] == 0 {
		t.Error("expected some high severity controls")
	}
}

func TestNISTUniqueIDs(t *testing.T) {
	s := NewNISTStandard()
	seen := make(map[string]bool)
	for _, c := range s.Controls() {
		if seen[c.ID] {
			t.Errorf("duplicate control ID: %s", c.ID)
		}
		seen[c.ID] = true
	}
}

func TestNISTGetControlMiss(t *testing.T) {
	s := NewNISTStandard()
	_, ok := s.GetControl("NONEXISTENT-999")
	if ok {
		t.Error("should not find non-existent control")
	}
}

func TestNISTValidate(t *testing.T) {
	s := NewNISTStandard()
	if err := s.Validate(); err != nil {
		t.Errorf("expected nil validation error, got %v", err)
	}
}

func TestNISTImplementsStandard(t *testing.T) {
	var _ Standard = (*NISTStandard)(nil)
}
