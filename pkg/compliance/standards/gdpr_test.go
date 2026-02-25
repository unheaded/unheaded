// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package standards

import (
	"testing"

	"unheaded/pkg/compliance/controls"
)

func TestNewGDPRStandard(t *testing.T) {
	s := NewGDPRStandard()

	if s.ID() != "GDPR" {
		t.Errorf("expected ID %q, got %q", "GDPR", s.ID())
	}
	if s.Name() != "General Data Protection Regulation" {
		t.Errorf("unexpected name: %s", s.Name())
	}
	if s.Version() != "2016/679" {
		t.Errorf("expected version %q, got %q", "2016/679", s.Version())
	}
}

func TestGDPRControlCount(t *testing.T) {
	s := NewGDPRStandard()
	ctrls := s.Controls()
	if len(ctrls) == 0 {
		t.Fatal("expected GDPR to have controls")
	}
	if len(ctrls) < 30 {
		t.Errorf("expected at least 30 controls, got %d", len(ctrls))
	}
}

func TestGDPRAllControlsHaveStandard(t *testing.T) {
	s := NewGDPRStandard()
	for _, c := range s.Controls() {
		if c.Standard != "GDPR" {
			t.Errorf("control %s has standard %q, expected GDPR", c.ID, c.Standard)
		}
	}
}

func TestGDPRDataProcessingPrinciples(t *testing.T) {
	s := NewGDPRStandard()
	principleIDs := []string{
		"Art.5(1)(a)", "Art.5(1)(b)", "Art.5(1)(c)",
		"Art.5(1)(d)", "Art.5(1)(e)", "Art.5(1)(f)", "Art.5(2)",
	}
	for _, id := range principleIDs {
		c, ok := s.GetControl(id)
		if !ok {
			t.Errorf("expected to find control %s", id)
			continue
		}
		if c.Name == "" {
			t.Errorf("control %s should have a name", id)
		}
	}
}

func TestGDPRPrivacyControls(t *testing.T) {
	s := NewGDPRStandard()
	privacyCount := 0
	for _, c := range s.Controls() {
		if c.Category == controls.CategoryPrivacy {
			privacyCount++
		}
	}
	// GDPR is heavily privacy-focused
	if privacyCount < 10 {
		t.Errorf("expected at least 10 privacy controls, got %d", privacyCount)
	}
}

func TestGDPRDataSubjectRights(t *testing.T) {
	s := NewGDPRStandard()
	rightIDs := []string{
		"Art.15(1)", // Right of access
		"Art.16",    // Right to rectification
		"Art.17(1)", // Right to erasure
		"Art.18(1)", // Right to restriction
		"Art.20(1)", // Right to data portability
		"Art.21(1)", // Right to object
		"Art.22(1)", // Automated decision-making
	}
	for _, id := range rightIDs {
		_, ok := s.GetControl(id)
		if !ok {
			t.Errorf("expected to find data subject right control %s", id)
		}
	}
}

func TestGDPRSecurityControls(t *testing.T) {
	s := NewGDPRStandard()
	secIDs := []string{"Art.32(1)", "Art.32(1)(a)", "Art.32(1)(b)", "Art.32(1)(c)", "Art.32(1)(d)"}
	for _, id := range secIDs {
		_, ok := s.GetControl(id)
		if !ok {
			t.Errorf("expected to find security control %s", id)
		}
	}
}

func TestGDPRBreachNotification(t *testing.T) {
	s := NewGDPRStandard()
	breachIDs := []string{"Art.33(1)", "Art.33(3)", "Art.33(5)", "Art.34(1)"}
	for _, id := range breachIDs {
		c, ok := s.GetControl(id)
		if !ok {
			t.Errorf("expected to find breach notification control %s", id)
			continue
		}
		if c.Category != controls.CategoryIncidentResponse && c.Category != controls.CategoryAuditLogging {
			t.Errorf("control %s expected incident_response or audit_logging category, got %q", id, c.Category)
		}
	}
}

func TestGDPRTransferControls(t *testing.T) {
	s := NewGDPRStandard()
	transferIDs := []string{"Art.44", "Art.45(1)", "Art.46(1)", "Art.46(2)"}
	for _, id := range transferIDs {
		_, ok := s.GetControl(id)
		if !ok {
			t.Errorf("expected to find transfer control %s", id)
		}
	}
}

func TestGDPRAccountabilityControls(t *testing.T) {
	s := NewGDPRStandard()
	accountIDs := []string{"Art.25(1)", "Art.25(2)", "Art.28(1)", "Art.28(3)", "Art.30(1)", "Art.35(1)", "Art.37(1)"}
	for _, id := range accountIDs {
		_, ok := s.GetControl(id)
		if !ok {
			t.Errorf("expected to find accountability control %s", id)
		}
	}
}

func TestGDPRCrossFrameworkMappings(t *testing.T) {
	s := NewGDPRStandard()
	tests := []struct {
		controlID string
		framework string
		expected  string
	}{
		{"Art.5(1)(b)", "SOC2", "P4.1"},
		{"Art.5(1)(c)", "SOC2", "P3.1"},
		{"Art.7(1)", "SOC2", "P2.1"},
		{"Art.32(1)(a)", "NIST", "SC-13"},
		{"Art.32(1)(c)", "NIST", "CP-10"},
		{"Art.28(1)", "SOC2", "CC9.2"},
		{"Art.44", "SOC2", "P7.1"},
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

func TestGDPRUniqueIDs(t *testing.T) {
	s := NewGDPRStandard()
	seen := make(map[string]bool)
	for _, c := range s.Controls() {
		if seen[c.ID] {
			t.Errorf("duplicate control ID: %s", c.ID)
		}
		seen[c.ID] = true
	}
}

func TestGDPRCategories(t *testing.T) {
	s := NewGDPRStandard()
	categories := make(map[controls.ControlCategory]int)
	for _, c := range s.Controls() {
		categories[c.Category]++
	}
	// GDPR should span multiple categories
	if len(categories) < 5 {
		t.Errorf("expected at least 5 distinct categories, got %d", len(categories))
	}
}

func TestGDPRValidate(t *testing.T) {
	s := NewGDPRStandard()
	if err := s.Validate(); err != nil {
		t.Errorf("expected nil validation error, got %v", err)
	}
}

func TestGDPRImplementsStandard(t *testing.T) {
	var _ Standard = (*GDPRStandard)(nil)
}
