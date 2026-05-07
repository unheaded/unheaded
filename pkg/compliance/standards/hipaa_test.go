// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package standards

import (
	"testing"

	"unheaded/pkg/compliance/controls"
)

func TestNewHIPAAStandard(t *testing.T) {
	s := NewHIPAAStandard()

	if s.ID() != "HIPAA" {
		t.Errorf("expected ID %q, got %q", "HIPAA", s.ID())
	}
	if s.Name() != "Health Insurance Portability and Accountability Act" {
		t.Errorf("unexpected name: %s", s.Name())
	}
	if s.Version() != "2013" {
		t.Errorf("expected version %q, got %q", "2013", s.Version())
	}
}

func TestHIPAAControlCount(t *testing.T) {
	s := NewHIPAAStandard()
	ctrls := s.Controls()
	if len(ctrls) == 0 {
		t.Fatal("expected HIPAA to have controls")
	}
	if len(ctrls) < 30 {
		t.Errorf("expected at least 30 controls, got %d", len(ctrls))
	}
}

func TestHIPAAAllControlsHaveStandard(t *testing.T) {
	s := NewHIPAAStandard()
	for _, c := range s.Controls() {
		if c.Standard != "HIPAA" {
			t.Errorf("control %s has standard %q, expected HIPAA", c.ID, c.Standard)
		}
	}
}

func TestHIPAAAdministrativeSafeguards(t *testing.T) {
	s := NewHIPAAStandard()
	adminIDs := []string{
		"164.308(a)(1)",
		"164.308(a)(1)(i)",
		"164.308(a)(3)",
		"164.308(a)(4)",
		"164.308(a)(5)",
		"164.308(a)(6)",
		"164.308(a)(7)",
	}
	for _, id := range adminIDs {
		_, ok := s.GetControl(id)
		if !ok {
			t.Errorf("expected to find administrative safeguard %s", id)
		}
	}
}

func TestHIPAAPhysicalSafeguards(t *testing.T) {
	s := NewHIPAAStandard()
	physIDs := []string{
		"164.310(a)(1)",
		"164.310(b)",
		"164.310(c)",
		"164.310(d)(1)",
	}
	for _, id := range physIDs {
		_, ok := s.GetControl(id)
		if !ok {
			t.Errorf("expected to find physical safeguard %s", id)
		}
	}
}

func TestHIPAATechnicalSafeguards(t *testing.T) {
	s := NewHIPAAStandard()
	techIDs := []string{
		"164.312(a)(1)",
		"164.312(a)(2)(i)",
		"164.312(a)(2)(iv)",
		"164.312(b)",
		"164.312(c)(1)",
		"164.312(d)",
		"164.312(e)(1)",
	}
	for _, id := range techIDs {
		_, ok := s.GetControl(id)
		if !ok {
			t.Errorf("expected to find technical safeguard %s", id)
		}
	}
}

func TestHIPAAAccessControlCategory(t *testing.T) {
	s := NewHIPAAStandard()
	accessControls := []string{
		"164.308(a)(3)",    // Workforce Security
		"164.312(a)(1)",    // Access Control
		"164.312(a)(2)(i)", // Unique User ID
		"164.312(d)",       // Person Authentication
	}
	for _, id := range accessControls {
		c, ok := s.GetControl(id)
		if !ok {
			t.Errorf("expected to find control %s", id)
			continue
		}
		if c.Category != controls.CategoryAccessControl {
			t.Errorf("control %s expected category access_control, got %q", id, c.Category)
		}
	}
}

func TestHIPAADataProtectionCategory(t *testing.T) {
	s := NewHIPAAStandard()
	dpControls := []string{
		"164.308(a)(7)(ii)(A)", // Data Backup Plan
		"164.310(d)(1)",        // Device and Media Controls
		"164.312(a)(2)(iv)",    // Encryption and Decryption
		"164.312(c)(1)",        // Integrity
		"164.312(e)(1)",        // Transmission Security
	}
	for _, id := range dpControls {
		c, ok := s.GetControl(id)
		if !ok {
			t.Errorf("expected to find control %s", id)
			continue
		}
		if c.Category != controls.CategoryDataProtection {
			t.Errorf("control %s expected category data_protection, got %q", id, c.Category)
		}
	}
}

func TestHIPAACrossFrameworkMappings(t *testing.T) {
	s := NewHIPAAStandard()
	tests := []struct {
		controlID string
		framework string
		expected  string
	}{
		{"164.308(a)(1)(i)", "NIST", "CA-2"},
		{"164.308(a)(1)(ii)(C)", "NIST", "AU-6"},
		{"164.308(a)(3)(ii)(C)", "SOC2", "CC6.3"},
		{"164.308(a)(5)(ii)(B)", "NIST", "SI-3"},
		{"164.308(a)(6)", "NIST", "IR-4"},
		{"164.308(a)(7)", "NIST", "CP-2"},
		{"164.312(a)(1)", "NIST", "AC-3"},
		{"164.312(b)", "NIST", "AU-2"},
		{"164.312(d)", "NIST", "IA-5"},
		{"164.312(e)(1)", "NIST", "SC-8"},
	}

	for _, tt := range tests {
		c, ok := s.GetControl(tt.controlID)
		if !ok {
			t.Errorf("control %s not found", tt.controlID)
			continue
		}
		mapped, ok := c.Mappings[tt.framework]
		if !ok {
			t.Errorf("control %s missing %s mapping", tt.controlID, tt.framework)
			continue
		}
		if mapped != tt.expected {
			t.Errorf("control %s -> %s: expected %q, got %q", tt.controlID, tt.framework, tt.expected, mapped)
		}
	}
}

func TestHIPAAIncidentResponseControls(t *testing.T) {
	s := NewHIPAAStandard()
	irIDs := []string{
		"164.308(a)(6)",
		"164.308(a)(7)",
		"164.308(a)(7)(ii)(B)",
		"164.308(a)(7)(ii)(C)",
	}
	for _, id := range irIDs {
		c, ok := s.GetControl(id)
		if !ok {
			t.Errorf("expected to find control %s", id)
			continue
		}
		if c.Category != controls.CategoryIncidentResponse {
			t.Errorf("control %s expected category incident_response, got %q", id, c.Category)
		}
	}
}

func TestHIPAABusinessAssociates(t *testing.T) {
	s := NewHIPAAStandard()
	c, ok := s.GetControl("164.308(b)(1)")
	if !ok {
		t.Fatal("expected to find business associate control")
	}
	if c.Category != controls.CategoryVendorManagement {
		t.Errorf("expected vendor_management, got %q", c.Category)
	}
	if c.Severity != controls.SeverityCritical {
		t.Errorf("expected critical severity, got %q", c.Severity)
	}
}

func TestHIPAAUniqueIDs(t *testing.T) {
	s := NewHIPAAStandard()
	seen := make(map[string]bool)
	for _, c := range s.Controls() {
		if seen[c.ID] {
			t.Errorf("duplicate control ID: %s", c.ID)
		}
		seen[c.ID] = true
	}
}

func TestHIPAACategories(t *testing.T) {
	s := NewHIPAAStandard()
	categories := make(map[controls.ControlCategory]int)
	for _, c := range s.Controls() {
		categories[c.Category]++
	}
	if len(categories) < 5 {
		t.Errorf("expected at least 5 distinct categories, got %d", len(categories))
	}
}

func TestHIPAASeverities(t *testing.T) {
	s := NewHIPAAStandard()
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

func TestHIPAAValidate(t *testing.T) {
	s := NewHIPAAStandard()
	if err := s.Validate(); err != nil {
		t.Errorf("expected nil validation error, got %v", err)
	}
}

func TestHIPAAImplementsStandard(t *testing.T) {
	var _ Standard = (*HIPAAStandard)(nil)
}
