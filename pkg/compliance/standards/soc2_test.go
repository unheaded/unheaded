package standards

import (
	"testing"

	"unheaded/pkg/compliance/controls"
)

func TestNewSOC2Standard(t *testing.T) {
	s := NewSOC2Standard()

	if s.ID() != "SOC2" {
		t.Errorf("expected ID %q, got %q", "SOC2", s.ID())
	}
	if s.Name() != "SOC 2 Type II" {
		t.Errorf("unexpected name: %s", s.Name())
	}
	if s.Version() != "2017" {
		t.Errorf("expected version %q, got %q", "2017", s.Version())
	}
}

func TestSOC2ControlCount(t *testing.T) {
	s := NewSOC2Standard()
	ctrls := s.Controls()
	if len(ctrls) == 0 {
		t.Fatal("expected SOC2 to have controls")
	}
	if len(ctrls) < 25 {
		t.Errorf("expected at least 25 controls, got %d", len(ctrls))
	}
}

func TestSOC2AllControlsHaveStandard(t *testing.T) {
	s := NewSOC2Standard()
	for _, c := range s.Controls() {
		if c.Standard != "SOC2" {
			t.Errorf("control %s has standard %q, expected SOC2", c.ID, c.Standard)
		}
	}
}

func TestSOC2SecurityControls(t *testing.T) {
	s := NewSOC2Standard()
	ccIDs := []string{
		"CC1.1", "CC1.2",
		"CC2.1", "CC2.2",
		"CC3.1", "CC3.2", "CC3.3",
		"CC5.1", "CC5.2",
		"CC6.1", "CC6.2", "CC6.3", "CC6.6", "CC6.7",
		"CC7.1", "CC7.2", "CC7.3", "CC7.4",
		"CC8.1",
		"CC9.1", "CC9.2",
	}
	for _, id := range ccIDs {
		_, ok := s.GetControl(id)
		if !ok {
			t.Errorf("expected to find security control %s", id)
		}
	}
}

func TestSOC2AvailabilityControls(t *testing.T) {
	s := NewSOC2Standard()
	availIDs := []string{"A1.1", "A1.2", "A1.3"}
	for _, id := range availIDs {
		_, ok := s.GetControl(id)
		if !ok {
			t.Errorf("expected to find availability control %s", id)
		}
	}
}

func TestSOC2ProcessingIntegrityControls(t *testing.T) {
	s := NewSOC2Standard()
	piIDs := []string{"PI1.1", "PI1.2", "PI1.3"}
	for _, id := range piIDs {
		_, ok := s.GetControl(id)
		if !ok {
			t.Errorf("expected to find processing integrity control %s", id)
		}
	}
}

func TestSOC2ConfidentialityControls(t *testing.T) {
	s := NewSOC2Standard()
	confIDs := []string{"C1.1", "C1.2"}
	for _, id := range confIDs {
		c, ok := s.GetControl(id)
		if !ok {
			t.Errorf("expected to find confidentiality control %s", id)
			continue
		}
		if c.Category != controls.CategoryDataProtection {
			t.Errorf("control %s expected category data_protection, got %q", id, c.Category)
		}
	}
}

func TestSOC2PrivacyControls(t *testing.T) {
	s := NewSOC2Standard()
	privIDs := []string{"P1.1", "P2.1", "P3.1", "P4.1", "P5.1", "P6.1", "P7.1", "P8.1"}
	for _, id := range privIDs {
		c, ok := s.GetControl(id)
		if !ok {
			t.Errorf("expected to find privacy control %s", id)
			continue
		}
		if c.Category != controls.CategoryPrivacy {
			t.Errorf("control %s expected category privacy, got %q", id, c.Category)
		}
	}
}

func TestSOC2AccessControlCategory(t *testing.T) {
	s := NewSOC2Standard()
	acIDs := []string{"CC6.1", "CC6.2"}
	for _, id := range acIDs {
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

func TestSOC2IncidentResponseCategory(t *testing.T) {
	s := NewSOC2Standard()
	irIDs := []string{"CC7.3", "CC7.4", "A1.2", "A1.3"}
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

func TestSOC2CrossFrameworkMappings(t *testing.T) {
	s := NewSOC2Standard()
	c, ok := s.GetControl("CC6.1")
	if !ok {
		t.Fatal("expected to find CC6.1")
	}
	mapped, ok := c.Mappings["NIST"]
	if !ok {
		t.Fatal("CC6.1 should have NIST mapping")
	}
	if mapped != "AC-2" {
		t.Errorf("expected NIST mapping %q, got %q", "AC-2", mapped)
	}
}

func TestSOC2VendorManagement(t *testing.T) {
	s := NewSOC2Standard()
	c, ok := s.GetControl("CC9.2")
	if !ok {
		t.Fatal("expected to find CC9.2")
	}
	if c.Category != controls.CategoryVendorManagement {
		t.Errorf("expected vendor_management, got %q", c.Category)
	}
}

func TestSOC2ChangeManagement(t *testing.T) {
	s := NewSOC2Standard()
	c, ok := s.GetControl("CC8.1")
	if !ok {
		t.Fatal("expected to find CC8.1")
	}
	if c.Category != controls.CategoryChangeManagement {
		t.Errorf("expected change_management, got %q", c.Category)
	}
}

func TestSOC2UniqueIDs(t *testing.T) {
	s := NewSOC2Standard()
	seen := make(map[string]bool)
	for _, c := range s.Controls() {
		if seen[c.ID] {
			t.Errorf("duplicate control ID: %s", c.ID)
		}
		seen[c.ID] = true
	}
}

func TestSOC2Categories(t *testing.T) {
	s := NewSOC2Standard()
	categories := make(map[controls.ControlCategory]int)
	for _, c := range s.Controls() {
		categories[c.Category]++
	}
	if len(categories) < 5 {
		t.Errorf("expected at least 5 distinct categories, got %d", len(categories))
	}
}

func TestSOC2Severities(t *testing.T) {
	s := NewSOC2Standard()
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
	if severities[controls.SeverityMedium] == 0 {
		t.Error("expected some medium severity controls")
	}
}

func TestSOC2TrustServiceCategories(t *testing.T) {
	s := NewSOC2Standard()
	// Verify SOC2 covers all five trust service categories
	// Security (CC), Availability (A), Processing Integrity (PI),
	// Confidentiality (C), Privacy (P)
	prefixes := map[string]bool{
		"CC": false,
		"A":  false,
		"PI": false,
		"C":  false,
		"P":  false,
	}

	for _, c := range s.Controls() {
		for prefix := range prefixes {
			if len(c.ID) >= len(prefix) && c.ID[:len(prefix)] == prefix {
				// Make sure it's a real prefix match (not "CC" matching "C")
				if prefix == "C" && len(c.ID) > 1 && c.ID[1] == 'C' {
					continue // This is a CC prefix, not C
				}
				prefixes[prefix] = true
			}
		}
	}

	for prefix, found := range prefixes {
		if !found {
			t.Errorf("expected controls with prefix %q for trust service category", prefix)
		}
	}
}

func TestSOC2Validate(t *testing.T) {
	s := NewSOC2Standard()
	if err := s.Validate(); err != nil {
		t.Errorf("expected nil validation error, got %v", err)
	}
}

func TestSOC2ImplementsStandard(t *testing.T) {
	var _ Standard = (*SOC2Standard)(nil)
}
