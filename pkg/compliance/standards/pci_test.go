// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package standards

import (
	"testing"

	"unheaded/pkg/compliance/controls"
)

func TestNewPCIStandard(t *testing.T) {
	s := NewPCIStandard()

	if s.ID() != "PCI-DSS" {
		t.Errorf("expected ID %q, got %q", "PCI-DSS", s.ID())
	}
	if s.Name() != "Payment Card Industry Data Security Standard" {
		t.Errorf("unexpected name: %s", s.Name())
	}
	if s.Version() != "4.0" {
		t.Errorf("expected version %q, got %q", "4.0", s.Version())
	}
}

func TestPCIControlCount(t *testing.T) {
	s := NewPCIStandard()
	ctrls := s.Controls()
	if len(ctrls) == 0 {
		t.Fatal("expected PCI to have controls")
	}
	// PCI requirements 1-12
	if len(ctrls) < 30 {
		t.Errorf("expected at least 30 controls, got %d", len(ctrls))
	}
}

func TestPCIAllControlsHaveStandard(t *testing.T) {
	s := NewPCIStandard()
	for _, c := range s.Controls() {
		if c.Standard != "PCI-DSS" {
			t.Errorf("control %s has standard %q, expected PCI-DSS", c.ID, c.Standard)
		}
	}
}

func TestPCINetworkSecurityControls(t *testing.T) {
	s := NewPCIStandard()
	netIDs := []string{"1.1", "1.2", "1.3", "1.4"}
	for _, id := range netIDs {
		c, ok := s.GetControl(id)
		if !ok {
			t.Errorf("expected to find control %s", id)
			continue
		}
		if c.Category != controls.CategoryNetworkSecurity {
			t.Errorf("control %s expected category %q, got %q", id, controls.CategoryNetworkSecurity, c.Category)
		}
	}
}

func TestPCIDataProtectionControls(t *testing.T) {
	s := NewPCIStandard()
	dpIDs := []string{"3.1", "3.2", "3.3", "3.4", "3.5", "4.1"}
	for _, id := range dpIDs {
		c, ok := s.GetControl(id)
		if !ok {
			t.Errorf("expected to find data protection control %s", id)
			continue
		}
		if c.Category != controls.CategoryDataProtection {
			t.Errorf("control %s expected category data_protection, got %q", id, c.Category)
		}
	}
}

func TestPCIAccessControlControls(t *testing.T) {
	s := NewPCIStandard()
	acIDs := []string{"7.1", "7.2", "7.3", "8.1", "8.2", "8.3"}
	for _, id := range acIDs {
		c, ok := s.GetControl(id)
		if !ok {
			t.Errorf("expected to find access control %s", id)
			continue
		}
		if c.Category != controls.CategoryAccessControl {
			t.Errorf("control %s expected category access_control, got %q", id, c.Category)
		}
	}
}

func TestPCIMonitoringControls(t *testing.T) {
	s := NewPCIStandard()
	logIDs := []string{"10.1", "10.2", "10.3", "10.4"}
	for _, id := range logIDs {
		c, ok := s.GetControl(id)
		if !ok {
			t.Errorf("expected to find monitoring control %s", id)
			continue
		}
		if c.Category != controls.CategoryAuditLogging {
			t.Errorf("control %s expected category audit_logging, got %q", id, c.Category)
		}
	}
}

func TestPCICrossFrameworkMappings(t *testing.T) {
	s := NewPCIStandard()
	tests := []struct {
		controlID string
		framework string
		expected  string
	}{
		{"1.2", "NIST", "SC-7"},
		{"3.4", "NIST", "SC-13"},
		{"4.1", "NIST", "SC-8"},
		{"5.1", "NIST", "SI-3"},
		{"6.2", "NIST", "SI-2"},
		{"7.1", "NIST", "AC-6"},
		{"8.1", "NIST", "AC-2"},
		{"10.7", "NIST", "AU-11"},
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
			t.Errorf("control %s mapping to %s: expected %q, got %q", tt.controlID, tt.framework, tt.expected, mapped)
		}
	}
}

func TestPCIPhysicalSecurityControls(t *testing.T) {
	s := NewPCIStandard()
	physIDs := []string{"9.1", "9.2", "9.5"}
	for _, id := range physIDs {
		c, ok := s.GetControl(id)
		if !ok {
			t.Errorf("expected to find physical security control %s", id)
			continue
		}
		if c.Category != controls.CategoryPhysicalSecurity {
			t.Errorf("control %s expected category physical_security, got %q", id, c.Category)
		}
	}
}

func TestPCIIncidentResponse(t *testing.T) {
	s := NewPCIStandard()
	c, ok := s.GetControl("12.10")
	if !ok {
		t.Fatal("expected to find control 12.10")
	}
	if c.Category != controls.CategoryIncidentResponse {
		t.Errorf("expected incident_response, got %q", c.Category)
	}
}

func TestPCIUniqueIDs(t *testing.T) {
	s := NewPCIStandard()
	seen := make(map[string]bool)
	for _, c := range s.Controls() {
		if seen[c.ID] {
			t.Errorf("duplicate control ID: %s", c.ID)
		}
		seen[c.ID] = true
	}
}

func TestPCISeverities(t *testing.T) {
	s := NewPCIStandard()
	severities := make(map[controls.Severity]int)
	for _, c := range s.Controls() {
		severities[c.Severity]++
	}
	if severities[controls.SeverityCritical] == 0 {
		t.Error("expected some critical severity controls")
	}
}

func TestPCIValidate(t *testing.T) {
	s := NewPCIStandard()
	if err := s.Validate(); err != nil {
		t.Errorf("expected nil validation error, got %v", err)
	}
}

func TestPCIImplementsStandard(t *testing.T) {
	var _ Standard = (*PCIStandard)(nil)
}
