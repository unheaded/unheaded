// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package standards

import (
	"context"
	"testing"
	"time"

	"unheaded/pkg/compliance/controls"
)

// ---------------------------------------------------------------------------
// BaseStandard
// ---------------------------------------------------------------------------

func TestNewBaseStandard(t *testing.T) {
	s := NewBaseStandard("STD-1", "Test Standard", "1.0", "A test standard")

	if s.ID() != "STD-1" {
		t.Errorf("expected ID %q, got %q", "STD-1", s.ID())
	}
	if s.Name() != "Test Standard" {
		t.Errorf("expected Name %q, got %q", "Test Standard", s.Name())
	}
	if s.Version() != "1.0" {
		t.Errorf("expected Version %q, got %q", "1.0", s.Version())
	}
	if s.Description() != "A test standard" {
		t.Errorf("expected Description %q, got %q", "A test standard", s.Description())
	}
	if len(s.Controls()) != 0 {
		t.Errorf("expected 0 controls initially, got %d", len(s.Controls()))
	}
	if len(s.Families()) != 0 {
		t.Errorf("expected 0 families initially, got %d", len(s.Families()))
	}
}

func TestBaseStandardAddControl(t *testing.T) {
	s := NewBaseStandard("STD", "n", "v", "d")

	c1 := controls.NewControl("C-1", "STD", "Control 1", "desc")
	c2 := controls.NewControl("C-2", "STD", "Control 2", "desc")
	s.AddControl(c1)
	s.AddControl(c2)

	if len(s.Controls()) != 2 {
		t.Fatalf("expected 2 controls, got %d", len(s.Controls()))
	}

	got, ok := s.GetControl("C-1")
	if !ok {
		t.Fatal("expected to find C-1")
	}
	if got.Name != "Control 1" {
		t.Errorf("expected name %q, got %q", "Control 1", got.Name)
	}

	_, ok = s.GetControl("C-MISSING")
	if ok {
		t.Error("should not find non-existent control")
	}
}

func TestBaseStandardAddFamily(t *testing.T) {
	s := NewBaseStandard("STD", "n", "v", "d")

	f := &controls.ControlFamily{
		ID:   "FAM-1",
		Name: "Family 1",
	}
	s.AddFamily(f)

	families := s.Families()
	if len(families) != 1 {
		t.Fatalf("expected 1 family, got %d", len(families))
	}
	if families[0].ID != "FAM-1" {
		t.Error("family ID mismatch")
	}
}

func TestBaseStandardValidate(t *testing.T) {
	s := NewBaseStandard("STD", "n", "v", "d")
	if err := s.Validate(); err != nil {
		t.Errorf("expected nil validation error, got %v", err)
	}
}

// Verify BaseStandard satisfies the Standard interface.
func TestBaseStandardImplementsStandard(t *testing.T) {
	var _ Standard = (*BaseStandard)(nil)
}

// ---------------------------------------------------------------------------
// StandardRegistry
// ---------------------------------------------------------------------------

func TestNewStandardRegistry(t *testing.T) {
	r := NewStandardRegistry()
	if r == nil {
		t.Fatal("expected non-nil registry")
	}
}

func TestStandardRegistryRegisterAndGet(t *testing.T) {
	r := NewStandardRegistry()
	s := NewBaseStandard("STD-1", "Test", "1.0", "desc")
	r.Register(s)

	got, ok := r.Get("STD-1")
	if !ok {
		t.Fatal("expected to find STD-1")
	}
	if got.Name() != "Test" {
		t.Errorf("expected name %q, got %q", "Test", got.Name())
	}
}

func TestStandardRegistryGetNotFound(t *testing.T) {
	r := NewStandardRegistry()
	_, ok := r.Get("NONEXISTENT")
	if ok {
		t.Error("should not find non-existent standard")
	}
}

func TestStandardRegistryList(t *testing.T) {
	r := NewStandardRegistry()
	r.Register(NewBaseStandard("A", "A-name", "1", "d"))
	r.Register(NewBaseStandard("B", "B-name", "2", "d"))
	r.Register(NewBaseStandard("C", "C-name", "3", "d"))

	list := r.List()
	if len(list) != 3 {
		t.Fatalf("expected 3 standards, got %d", len(list))
	}

	ids := make(map[string]bool)
	for _, s := range list {
		ids[s.ID()] = true
	}
	for _, id := range []string{"A", "B", "C"} {
		if !ids[id] {
			t.Errorf("expected %q in list", id)
		}
	}
}

func TestStandardRegistryOverwrite(t *testing.T) {
	r := NewStandardRegistry()
	r.Register(NewBaseStandard("STD", "Original", "1", "d"))
	r.Register(NewBaseStandard("STD", "Replacement", "2", "d"))

	got, _ := r.Get("STD")
	if got.Name() != "Replacement" {
		t.Errorf("expected overwritten name %q, got %q", "Replacement", got.Name())
	}
	if len(r.List()) != 1 {
		t.Error("should still have only 1 standard after overwrite")
	}
}

func TestStandardRegistryListEmpty(t *testing.T) {
	r := NewStandardRegistry()
	list := r.List()
	if len(list) != 0 {
		t.Errorf("expected empty list, got %d items", len(list))
	}
}

// ---------------------------------------------------------------------------
// StandardAssessor
// ---------------------------------------------------------------------------

func TestNewStandardAssessor(t *testing.T) {
	s := NewBaseStandard("STD", "n", "v", "d")
	reg := controls.NewCheckRegistry()
	runner := controls.NewCheckRunner(reg)
	a := NewStandardAssessor(s, runner)
	if a == nil {
		t.Fatal("expected non-nil assessor")
	}
}

func TestAssessEmptyStandard(t *testing.T) {
	s := NewBaseStandard("STD", "n", "v", "d")
	reg := controls.NewCheckRegistry()
	runner := controls.NewCheckRunner(reg)
	a := NewStandardAssessor(s, runner)

	result, err := a.Assess(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TotalControls != 0 {
		t.Errorf("expected 0 total controls, got %d", result.TotalControls)
	}
	if result.ComplianceRate != 0 {
		t.Errorf("expected compliance rate 0, got %f", result.ComplianceRate)
	}
}

func TestAssessAllCompliant(t *testing.T) {
	s := NewBaseStandard("STD", "n", "v", "d")
	c1 := controls.NewControl("C-1", "STD", "n1", "d1")
	c2 := controls.NewControl("C-2", "STD", "n2", "d2")
	s.AddControl(c1)
	s.AddControl(c2)

	reg := controls.NewCheckRegistry()
	for _, id := range []string{"C-1", "C-2"} {
		reg.RegisterFunc(id, func(ctx context.Context, c *controls.Control) (*controls.ControlResult, error) {
			return &controls.ControlResult{
				ControlID: c.ID,
				Status:    controls.StatusCompliant,
				Score:     1.0,
				TestedAt:  time.Now(),
			}, nil
		})
	}

	runner := controls.NewCheckRunner(reg)
	a := NewStandardAssessor(s, runner)

	result, err := a.Assess(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TotalControls != 2 {
		t.Errorf("expected 2 total, got %d", result.TotalControls)
	}
	if result.Compliant != 2 {
		t.Errorf("expected 2 compliant, got %d", result.Compliant)
	}
	if result.ComplianceRate != 1.0 {
		t.Errorf("expected compliance rate 1.0, got %f", result.ComplianceRate)
	}
	if result.Standard != "STD" {
		t.Errorf("expected standard STD, got %s", result.Standard)
	}
	if result.Version != "v" {
		t.Errorf("expected version v, got %s", result.Version)
	}
}

func TestAssessMixedStatuses(t *testing.T) {
	s := NewBaseStandard("STD", "n", "v", "d")
	// Create 5 controls with different statuses
	statusMap := map[string]controls.ControlStatus{
		"C-1": controls.StatusCompliant,
		"C-2": controls.StatusNonCompliant,
		"C-3": controls.StatusPartial,
		"C-4": controls.StatusNotApplicable,
		"C-5": controls.StatusNotAssessed,
	}

	for id := range statusMap {
		s.AddControl(controls.NewControl(id, "STD", "n-"+id, "d"))
	}

	reg := controls.NewCheckRegistry()
	for id, status := range statusMap {
		st := status // capture for closure
		reg.RegisterFunc(id, func(ctx context.Context, c *controls.Control) (*controls.ControlResult, error) {
			return &controls.ControlResult{
				ControlID: c.ID,
				Status:    st,
				Score:     0.5,
				TestedAt:  time.Now(),
			}, nil
		})
	}

	runner := controls.NewCheckRunner(reg)
	a := NewStandardAssessor(s, runner)

	result, err := a.Assess(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TotalControls != 5 {
		t.Errorf("expected 5 total, got %d", result.TotalControls)
	}
	if result.Compliant != 1 {
		t.Errorf("expected 1 compliant, got %d", result.Compliant)
	}
	if result.NonCompliant != 1 {
		t.Errorf("expected 1 non-compliant, got %d", result.NonCompliant)
	}
	if result.Partial != 1 {
		t.Errorf("expected 1 partial, got %d", result.Partial)
	}
	if result.NotApplicable != 1 {
		t.Errorf("expected 1 not-applicable, got %d", result.NotApplicable)
	}
	if result.NotAssessed != 1 {
		t.Errorf("expected 1 not-assessed, got %d", result.NotAssessed)
	}

	// assessable = 5 - 1(NA) - 1(NotAssessed) = 3, compliant = 1 => rate = 1/3
	expectedRate := 1.0 / 3.0
	if result.ComplianceRate < expectedRate-0.01 || result.ComplianceRate > expectedRate+0.01 {
		t.Errorf("expected compliance rate ~%f, got %f", expectedRate, result.ComplianceRate)
	}
}

func TestAssessAllNotApplicable(t *testing.T) {
	s := NewBaseStandard("STD", "n", "v", "d")
	s.AddControl(controls.NewControl("C-1", "STD", "n", "d"))

	reg := controls.NewCheckRegistry()
	reg.RegisterFunc("C-1", func(ctx context.Context, c *controls.Control) (*controls.ControlResult, error) {
		return &controls.ControlResult{
			ControlID: c.ID,
			Status:    controls.StatusNotApplicable,
			TestedAt:  time.Now(),
		}, nil
	})

	runner := controls.NewCheckRunner(reg)
	a := NewStandardAssessor(s, runner)

	result, err := a.Assess(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// assessable = 0 => compliance rate should be 0 (no division by zero)
	if result.ComplianceRate != 0 {
		t.Errorf("expected rate 0 when all NA, got %f", result.ComplianceRate)
	}
}

func TestAssessResultsLength(t *testing.T) {
	s := NewBaseStandard("STD", "n", "v", "d")
	for i := 0; i < 10; i++ {
		s.AddControl(controls.NewControl(
			"C-"+string(rune('A'+i)),
			"STD", "name", "desc",
		))
	}

	runner := controls.NewCheckRunner(controls.NewCheckRegistry())
	a := NewStandardAssessor(s, runner)

	result, err := a.Assess(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Results) != 10 {
		t.Errorf("expected 10 results, got %d", len(result.Results))
	}
}

// ---------------------------------------------------------------------------
// AssessmentResult
// ---------------------------------------------------------------------------

func TestAssessmentResultFields(t *testing.T) {
	r := AssessmentResult{
		Standard:       "SOC2",
		Version:        "2017",
		TotalControls:  50,
		Compliant:      40,
		NonCompliant:   5,
		Partial:        3,
		NotAssessed:    1,
		NotApplicable:  1,
		ComplianceRate: 0.833,
	}
	if r.Standard != "SOC2" {
		t.Error("standard mismatch")
	}
	if r.TotalControls != 50 {
		t.Error("total mismatch")
	}
}

// ---------------------------------------------------------------------------
// Integration: Register all concrete standards in a registry
// ---------------------------------------------------------------------------

func TestRegistryWithAllStandards(t *testing.T) {
	r := NewStandardRegistry()
	r.Register(NewNISTStandard())
	r.Register(NewPCIStandard())
	r.Register(NewGDPRStandard())
	r.Register(NewHIPAAStandard())
	r.Register(NewSOC2Standard())

	if len(r.List()) != 5 {
		t.Errorf("expected 5 standards registered, got %d", len(r.List()))
	}

	for _, id := range []string{"NIST-800-53", "PCI-DSS", "GDPR", "HIPAA", "SOC2"} {
		s, ok := r.Get(id)
		if !ok {
			t.Errorf("expected to find standard %q", id)
			continue
		}
		if len(s.Controls()) == 0 {
			t.Errorf("standard %q should have controls", id)
		}
	}
}
