package controls

import (
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Control constructor & builder helpers
// ---------------------------------------------------------------------------

func TestNewControl(t *testing.T) {
	c := NewControl("AC-1", "NIST", "Access Control", "Policy and procedures")

	if c.ID != "AC-1" {
		t.Errorf("expected ID %q, got %q", "AC-1", c.ID)
	}
	if c.Standard != "NIST" {
		t.Errorf("expected Standard %q, got %q", "NIST", c.Standard)
	}
	if c.Name != "Access Control" {
		t.Errorf("expected Name %q, got %q", "Access Control", c.Name)
	}
	if c.Description != "Policy and procedures" {
		t.Errorf("expected Description %q, got %q", "Policy and procedures", c.Description)
	}
	if c.Mappings == nil {
		t.Fatal("Mappings map must be initialized")
	}
	if len(c.Mappings) != 0 {
		t.Errorf("expected empty Mappings, got %d entries", len(c.Mappings))
	}
	if c.CreatedAt.IsZero() || c.UpdatedAt.IsZero() {
		t.Error("CreatedAt / UpdatedAt should be set")
	}
}

func TestWithCategory(t *testing.T) {
	c := NewControl("AC-1", "NIST", "n", "d").WithCategory(CategoryAccessControl)
	if c.Category != CategoryAccessControl {
		t.Errorf("expected category %q, got %q", CategoryAccessControl, c.Category)
	}
}

func TestWithSeverity(t *testing.T) {
	c := NewControl("AC-1", "NIST", "n", "d").WithSeverity(SeverityCritical)
	if c.Severity != SeverityCritical {
		t.Errorf("expected severity %q, got %q", SeverityCritical, c.Severity)
	}
}

func TestWithRequirements(t *testing.T) {
	c := NewControl("AC-1", "NIST", "n", "d").WithRequirements("r1", "r2", "r3")
	if len(c.Requirements) != 3 {
		t.Fatalf("expected 3 requirements, got %d", len(c.Requirements))
	}
	if c.Requirements[0] != "r1" || c.Requirements[2] != "r3" {
		t.Error("requirements content mismatch")
	}
}

func TestAddMapping(t *testing.T) {
	c := NewControl("AC-2", "NIST", "n", "d").
		AddMapping("SOC2", "CC6.1").
		AddMapping("PCI-DSS", "8.1")

	if len(c.Mappings) != 2 {
		t.Fatalf("expected 2 mappings, got %d", len(c.Mappings))
	}
	if c.Mappings["SOC2"] != "CC6.1" {
		t.Errorf("expected SOC2 mapping %q, got %q", "CC6.1", c.Mappings["SOC2"])
	}
	if c.Mappings["PCI-DSS"] != "8.1" {
		t.Errorf("expected PCI-DSS mapping %q, got %q", "8.1", c.Mappings["PCI-DSS"])
	}
}

func TestBuilderChaining(t *testing.T) {
	c := NewControl("AC-1", "NIST", "n", "d").
		WithCategory(CategoryAuditLogging).
		WithSeverity(SeverityHigh).
		WithRequirements("r1").
		AddMapping("SOC2", "CC1.1")

	if c.Category != CategoryAuditLogging {
		t.Error("chained category mismatch")
	}
	if c.Severity != SeverityHigh {
		t.Error("chained severity mismatch")
	}
	if len(c.Requirements) != 1 {
		t.Error("chained requirements mismatch")
	}
	if c.Mappings["SOC2"] != "CC1.1" {
		t.Error("chained mapping mismatch")
	}
}

// ---------------------------------------------------------------------------
// ControlFamily
// ---------------------------------------------------------------------------

func TestControlFamily(t *testing.T) {
	c1 := NewControl("AC-1", "NIST", "n1", "d1")
	c2 := NewControl("AC-2", "NIST", "n2", "d2")

	fam := &ControlFamily{
		ID:          "AC",
		Standard:    "NIST",
		Name:        "Access Control",
		Description: "Access control family",
		Controls:    []*Control{c1, c2},
	}

	if len(fam.Controls) != 2 {
		t.Fatalf("expected 2 controls, got %d", len(fam.Controls))
	}
	if fam.Controls[0].ID != "AC-1" {
		t.Error("first control ID mismatch")
	}
}

// ---------------------------------------------------------------------------
// ControlResult
// ---------------------------------------------------------------------------

func TestControlResultFields(t *testing.T) {
	r := &ControlResult{
		ControlID: "AC-1",
		Status:    StatusCompliant,
		Score:     0.95,
		Findings:  []Finding{{ID: "F-1", Title: "t"}},
		Evidence:  []string{"EV-1"},
		TestedAt:  time.Now(),
		TestedBy:  "automated",
		Notes:     "all good",
	}

	if r.ControlID != "AC-1" {
		t.Error("ControlID mismatch")
	}
	if r.Status != StatusCompliant {
		t.Error("Status mismatch")
	}
	if r.Score != 0.95 {
		t.Error("Score mismatch")
	}
	if len(r.Findings) != 1 {
		t.Error("Findings length mismatch")
	}
}

// ---------------------------------------------------------------------------
// Constants sanity checks
// ---------------------------------------------------------------------------

func TestControlStatusConstants(t *testing.T) {
	statuses := []ControlStatus{
		StatusNotAssessed,
		StatusCompliant,
		StatusNonCompliant,
		StatusPartial,
		StatusNotApplicable,
	}
	seen := make(map[ControlStatus]bool)
	for _, s := range statuses {
		if seen[s] {
			t.Errorf("duplicate status constant: %s", s)
		}
		seen[s] = true
	}
}

func TestControlCategoryConstants(t *testing.T) {
	categories := []ControlCategory{
		CategoryAccessControl,
		CategoryAuditLogging,
		CategoryDataProtection,
		CategoryIncidentResponse,
		CategoryRiskManagement,
		CategorySecurityAwareness,
		CategorySystemIntegrity,
		CategoryNetworkSecurity,
		CategoryChangeManagement,
		CategoryVendorManagement,
		CategoryPhysicalSecurity,
		CategoryPrivacy,
	}
	seen := make(map[ControlCategory]bool)
	for _, c := range categories {
		if seen[c] {
			t.Errorf("duplicate category constant: %s", c)
		}
		seen[c] = true
	}
	if len(categories) != 12 {
		t.Errorf("expected 12 categories, got %d", len(categories))
	}
}

func TestSeverityConstants(t *testing.T) {
	severities := []Severity{
		SeverityCritical,
		SeverityHigh,
		SeverityMedium,
		SeverityLow,
		SeverityInfo,
	}
	seen := make(map[Severity]bool)
	for _, s := range severities {
		if seen[s] {
			t.Errorf("duplicate severity constant: %s", s)
		}
		seen[s] = true
	}
}

func TestRemediationStatusConstants(t *testing.T) {
	statuses := []RemediationStatus{
		RemediationOpen,
		RemediationInProgress,
		RemediationCompleted,
		RemediationAccepted,
		RemediationDeferred,
	}
	seen := make(map[RemediationStatus]bool)
	for _, s := range statuses {
		if seen[s] {
			t.Errorf("duplicate remediation status: %s", s)
		}
		seen[s] = true
	}
}

// ---------------------------------------------------------------------------
// Remediation struct
// ---------------------------------------------------------------------------

func TestRemediation(t *testing.T) {
	now := time.Now()
	completed := now.Add(24 * time.Hour)

	r := Remediation{
		ID:          "REM-1",
		ControlID:   "AC-1",
		FindingID:   "F-1",
		Title:       "Fix policy",
		Description: "Enable policy enforcement",
		Status:      RemediationOpen,
		Priority:    SeverityHigh,
		Owner:       "team-security",
		DueDate:     now.Add(7 * 24 * time.Hour),
		CompletedAt: &completed,
		Notes:       "notes",
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if r.ID != "REM-1" {
		t.Error("Remediation ID mismatch")
	}
	if r.CompletedAt == nil {
		t.Fatal("CompletedAt should be set")
	}
	if r.CompletedAt.Before(now) {
		t.Error("CompletedAt should be after now")
	}
}

func TestRemediationNilCompleted(t *testing.T) {
	r := Remediation{
		ID:     "REM-2",
		Status: RemediationInProgress,
	}
	if r.CompletedAt != nil {
		t.Error("CompletedAt should be nil for in-progress remediation")
	}
}

// ---------------------------------------------------------------------------
// Finding struct
// ---------------------------------------------------------------------------

func TestFinding(t *testing.T) {
	f := Finding{
		ID:          "F-1",
		Title:       "Policy not enforced",
		Description: "The access control policy is not enforced",
		Severity:    SeverityHigh,
		Resource:    "policy-001",
		Location:    "/etc/policies",
		Remediation: "Enable enforcement",
		DetectedAt:  time.Now(),
	}

	if f.ID != "F-1" {
		t.Error("Finding ID mismatch")
	}
	if f.Severity != SeverityHigh {
		t.Error("Finding severity mismatch")
	}
	if f.DetectedAt.IsZero() {
		t.Error("DetectedAt should be set")
	}
}
