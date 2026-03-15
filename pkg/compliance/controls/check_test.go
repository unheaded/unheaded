// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package controls

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func makeControl(id string, cat ControlCategory) *Control {
	return NewControl(id, "TEST", "name-"+id, "desc").WithCategory(cat)
}

// ---------------------------------------------------------------------------
// CheckRegistry
// ---------------------------------------------------------------------------

func TestNewCheckRegistry(t *testing.T) {
	r := NewCheckRegistry()
	if r == nil {
		t.Fatal("expected non-nil registry")
	}
	if r.checkers == nil || r.funcs == nil {
		t.Fatal("internal maps must be initialized")
	}
}

func TestRegisterAndGetChecker(t *testing.T) {
	r := NewCheckRegistry()
	checker := NewAccessControlChecker([]AccessPolicy{
		{ID: "p1", Name: "Policy1", Enforced: true},
	})
	r.Register("access", checker)

	ctrl := makeControl("AC-1", CategoryAccessControl)
	got, ok := r.GetChecker(ctrl)
	if !ok {
		t.Fatal("expected to find checker")
	}
	if got.Name() != "access_control_checker" {
		t.Errorf("expected name %q, got %q", "access_control_checker", got.Name())
	}
}

func TestRegisterFunc(t *testing.T) {
	r := NewCheckRegistry()

	called := false
	r.RegisterFunc("CTRL-1", func(ctx context.Context, control *Control) (*ControlResult, error) {
		called = true
		return &ControlResult{ControlID: control.ID, Status: StatusCompliant, Score: 1.0, TestedAt: time.Now()}, nil
	})

	ctrl := makeControl("CTRL-1", CategoryAccessControl)
	checker, ok := r.GetChecker(ctrl)
	if !ok {
		t.Fatal("expected to find func checker")
	}
	if checker.Name() != "CTRL-1" {
		t.Errorf("funcChecker name mismatch")
	}

	result, err := checker.Check(context.Background(), ctrl)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("check function was not called")
	}
	if result.Status != StatusCompliant {
		t.Errorf("expected StatusCompliant, got %s", result.Status)
	}
}

func TestRegisterFuncPrecedence(t *testing.T) {
	r := NewCheckRegistry()

	// Register a broad checker that supports all access-control
	r.Register("access", NewAccessControlChecker(nil))

	// Register a specific func for CTRL-X
	r.RegisterFunc("CTRL-X", func(ctx context.Context, control *Control) (*ControlResult, error) {
		return &ControlResult{ControlID: "CTRL-X", Status: StatusPartial, Score: 0.5, TestedAt: time.Now()}, nil
	})

	ctrl := makeControl("CTRL-X", CategoryAccessControl)
	checker, ok := r.GetChecker(ctrl)
	if !ok {
		t.Fatal("expected to find checker")
	}
	// Func should take precedence
	result, err := checker.Check(context.Background(), ctrl)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != StatusPartial {
		t.Errorf("expected func checker to take precedence, got status %s", result.Status)
	}
}

func TestGetCheckerNotFound(t *testing.T) {
	r := NewCheckRegistry()
	ctrl := makeControl("UNKNOWN-1", CategoryPhysicalSecurity)
	_, ok := r.GetChecker(ctrl)
	if ok {
		t.Error("expected no checker found for unregistered control")
	}
}

// ---------------------------------------------------------------------------
// funcChecker
// ---------------------------------------------------------------------------

func TestFuncCheckerSupports(t *testing.T) {
	fc := &funcChecker{name: "CTRL-1"}
	ctrl := makeControl("CTRL-1", CategoryAccessControl)
	if !fc.Supports(ctrl) {
		t.Error("funcChecker should support control with matching ID")
	}
	ctrl2 := makeControl("CTRL-2", CategoryAccessControl)
	if fc.Supports(ctrl2) {
		t.Error("funcChecker should not support control with different ID")
	}
}

// ---------------------------------------------------------------------------
// CheckRunner
// ---------------------------------------------------------------------------

func TestNewCheckRunner(t *testing.T) {
	reg := NewCheckRegistry()
	runner := NewCheckRunner(reg)
	if runner == nil {
		t.Fatal("expected non-nil runner")
	}
	if runner.concurrency != 10 {
		t.Errorf("expected default concurrency 10, got %d", runner.concurrency)
	}
	if runner.timeout != 5*time.Minute {
		t.Errorf("expected default timeout 5m, got %s", runner.timeout)
	}
}

func TestWithConcurrency(t *testing.T) {
	runner := NewCheckRunner(NewCheckRegistry()).WithConcurrency(5)
	if runner.concurrency != 5 {
		t.Errorf("expected concurrency 5, got %d", runner.concurrency)
	}
}

func TestWithTimeout(t *testing.T) {
	runner := NewCheckRunner(NewCheckRegistry()).WithTimeout(30 * time.Second)
	if runner.timeout != 30*time.Second {
		t.Errorf("expected timeout 30s, got %s", runner.timeout)
	}
}

func TestRunChecksEmpty(t *testing.T) {
	runner := NewCheckRunner(NewCheckRegistry())
	results, err := runner.RunChecks(context.Background(), []*Control{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestRunChecksNoChecker(t *testing.T) {
	runner := NewCheckRunner(NewCheckRegistry())
	ctrls := []*Control{makeControl("X-1", CategoryPrivacy)}

	results, err := runner.RunChecks(context.Background(), ctrls)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Status != StatusNotAssessed {
		t.Errorf("expected StatusNotAssessed, got %s", results[0].Status)
	}
	if results[0].ControlID != "X-1" {
		t.Errorf("expected control ID X-1, got %s", results[0].ControlID)
	}
}

func TestRunChecksSingleSuccess(t *testing.T) {
	reg := NewCheckRegistry()
	reg.RegisterFunc("C-1", func(ctx context.Context, c *Control) (*ControlResult, error) {
		return &ControlResult{ControlID: c.ID, Status: StatusCompliant, Score: 1.0, TestedAt: time.Now()}, nil
	})
	runner := NewCheckRunner(reg).WithConcurrency(1)

	results, err := runner.RunChecks(context.Background(), []*Control{makeControl("C-1", CategoryAccessControl)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if results[0].Status != StatusCompliant {
		t.Errorf("expected StatusCompliant, got %s", results[0].Status)
	}
}

func TestRunChecksMultipleControls(t *testing.T) {
	reg := NewCheckRegistry()
	var callCount atomic.Int32

	for _, id := range []string{"C-1", "C-2", "C-3", "C-4", "C-5"} {
		id := id
		reg.RegisterFunc(id, func(ctx context.Context, c *Control) (*ControlResult, error) {
			callCount.Add(1)
			return &ControlResult{ControlID: c.ID, Status: StatusCompliant, Score: 1.0, TestedAt: time.Now()}, nil
		})
	}

	controls := make([]*Control, 5)
	for i := range controls {
		controls[i] = makeControl([]string{"C-1", "C-2", "C-3", "C-4", "C-5"}[i], CategoryAccessControl)
	}

	runner := NewCheckRunner(reg).WithConcurrency(3)
	results, err := runner.RunChecks(context.Background(), controls)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 5 {
		t.Fatalf("expected 5 results, got %d", len(results))
	}
	if int(callCount.Load()) != 5 {
		t.Errorf("expected 5 calls, got %d", callCount.Load())
	}
}

func TestRunChecksReturnsFirstError(t *testing.T) {
	reg := NewCheckRegistry()
	reg.RegisterFunc("ERR-1", func(ctx context.Context, c *Control) (*ControlResult, error) {
		return nil, errors.New("check failed")
	})

	runner := NewCheckRunner(reg).WithConcurrency(1)
	results, err := runner.RunChecks(context.Background(), []*Control{makeControl("ERR-1", CategoryAccessControl)})
	if err == nil {
		t.Fatal("expected error from RunChecks")
	}
	if err.Error() != "check failed" {
		t.Errorf("unexpected error: %v", err)
	}
	// Even on error, a result should be filled in
	if len(results) != 1 {
		t.Fatalf("expected 1 result even on error, got %d", len(results))
	}
	if results[0].Status != StatusNotAssessed {
		t.Errorf("expected StatusNotAssessed on error, got %s", results[0].Status)
	}
}

func TestRunChecksContextCancelled(t *testing.T) {
	reg := NewCheckRegistry()
	reg.RegisterFunc("SLOW-1", func(ctx context.Context, c *Control) (*ControlResult, error) {
		<-ctx.Done()
		return &ControlResult{ControlID: c.ID, Status: StatusNotAssessed, TestedAt: time.Now()}, ctx.Err()
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	runner := NewCheckRunner(reg).WithConcurrency(1).WithTimeout(100 * time.Millisecond)
	results, _ := runner.RunChecks(ctx, []*Control{makeControl("SLOW-1", CategoryAccessControl)})
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
}

func TestRunChecksConcurrencyCapToControlCount(t *testing.T) {
	// When concurrency > len(controls), workers should be capped
	reg := NewCheckRegistry()
	reg.RegisterFunc("C-1", func(ctx context.Context, c *Control) (*ControlResult, error) {
		return &ControlResult{ControlID: c.ID, Status: StatusCompliant, Score: 1.0, TestedAt: time.Now()}, nil
	})

	runner := NewCheckRunner(reg).WithConcurrency(100) // way more than 1 control
	results, err := runner.RunChecks(context.Background(), []*Control{makeControl("C-1", CategoryAccessControl)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
}

// ---------------------------------------------------------------------------
// AccessControlChecker
// ---------------------------------------------------------------------------

func TestAccessControlCheckerName(t *testing.T) {
	c := NewAccessControlChecker(nil)
	if c.Name() != "access_control_checker" {
		t.Errorf("expected name %q, got %q", "access_control_checker", c.Name())
	}
}

func TestAccessControlCheckerSupports(t *testing.T) {
	c := NewAccessControlChecker(nil)
	if !c.Supports(makeControl("AC-1", CategoryAccessControl)) {
		t.Error("should support access control category")
	}
	if c.Supports(makeControl("AU-1", CategoryAuditLogging)) {
		t.Error("should not support audit logging category")
	}
}

func TestAccessControlCheckerAllEnforced(t *testing.T) {
	policies := []AccessPolicy{
		{ID: "p1", Name: "P1", Enforced: true},
		{ID: "p2", Name: "P2", Enforced: true},
		{ID: "p3", Name: "P3", Enforced: true},
	}
	checker := NewAccessControlChecker(policies)

	result, err := checker.Check(context.Background(), makeControl("AC-1", CategoryAccessControl))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != StatusCompliant {
		t.Errorf("expected StatusCompliant, got %s", result.Status)
	}
	if result.Score != 1.0 {
		t.Errorf("expected score 1.0, got %f", result.Score)
	}
	if len(result.Findings) != 0 {
		t.Errorf("expected 0 findings, got %d", len(result.Findings))
	}
}

func TestAccessControlCheckerPartial(t *testing.T) {
	// 4 out of 5 enforced = 0.8 => StatusPartial
	policies := []AccessPolicy{
		{ID: "p1", Name: "P1", Enforced: true},
		{ID: "p2", Name: "P2", Enforced: true},
		{ID: "p3", Name: "P3", Enforced: true},
		{ID: "p4", Name: "P4", Enforced: true},
		{ID: "p5", Name: "P5", Enforced: false},
	}
	checker := NewAccessControlChecker(policies)

	result, err := checker.Check(context.Background(), makeControl("AC-1", CategoryAccessControl))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != StatusPartial {
		t.Errorf("expected StatusPartial, got %s", result.Status)
	}
	if result.Score != 0.8 {
		t.Errorf("expected score 0.8, got %f", result.Score)
	}
	if len(result.Findings) != 1 {
		t.Errorf("expected 1 finding, got %d", len(result.Findings))
	}
}

func TestAccessControlCheckerNonCompliant(t *testing.T) {
	// 1 out of 5 enforced = 0.2 => StatusNonCompliant
	policies := []AccessPolicy{
		{ID: "p1", Name: "P1", Enforced: true},
		{ID: "p2", Name: "P2", Enforced: false},
		{ID: "p3", Name: "P3", Enforced: false},
		{ID: "p4", Name: "P4", Enforced: false},
		{ID: "p5", Name: "P5", Enforced: false},
	}
	checker := NewAccessControlChecker(policies)

	result, err := checker.Check(context.Background(), makeControl("AC-1", CategoryAccessControl))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != StatusNonCompliant {
		t.Errorf("expected StatusNonCompliant, got %s", result.Status)
	}
	if len(result.Findings) != 4 {
		t.Errorf("expected 4 findings, got %d", len(result.Findings))
	}
}

func TestAccessControlCheckerFindingSeverity(t *testing.T) {
	policies := []AccessPolicy{
		{ID: "p1", Name: "MFA", Enforced: false},
	}
	checker := NewAccessControlChecker(policies)

	result, err := checker.Check(context.Background(), makeControl("AC-1", CategoryAccessControl))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(result.Findings))
	}
	f := result.Findings[0]
	if f.Severity != SeverityHigh {
		t.Errorf("expected severity %q, got %q", SeverityHigh, f.Severity)
	}
	if f.Resource != "p1" {
		t.Errorf("expected resource %q, got %q", "p1", f.Resource)
	}
}

func TestAccessControlCheckerTestedBy(t *testing.T) {
	checker := NewAccessControlChecker([]AccessPolicy{{ID: "p1", Enforced: true}})
	result, _ := checker.Check(context.Background(), makeControl("AC-1", CategoryAccessControl))
	if result.TestedBy != "automated" {
		t.Errorf("expected TestedBy %q, got %q", "automated", result.TestedBy)
	}
}

// ---------------------------------------------------------------------------
// AuditLogChecker
// ---------------------------------------------------------------------------

func TestAuditLogCheckerName(t *testing.T) {
	c := NewAuditLogChecker(true, 90, nil, nil)
	if c.Name() != "audit_log_checker" {
		t.Errorf("expected name %q, got %q", "audit_log_checker", c.Name())
	}
}

func TestAuditLogCheckerSupports(t *testing.T) {
	c := NewAuditLogChecker(true, 90, nil, nil)
	if !c.Supports(makeControl("AU-1", CategoryAuditLogging)) {
		t.Error("should support audit logging category")
	}
	if c.Supports(makeControl("AC-1", CategoryAccessControl)) {
		t.Error("should not support access control category")
	}
}

func TestAuditLogCheckerFullCompliance(t *testing.T) {
	required := []string{"login", "logout", "admin"}
	configured := []string{"login", "logout", "admin", "extra"}
	checker := NewAuditLogChecker(true, 90, required, configured)

	result, err := checker.Check(context.Background(), makeControl("AU-1", CategoryAuditLogging))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != StatusCompliant {
		t.Errorf("expected StatusCompliant, got %s", result.Status)
	}
	if result.Score != 1.0 {
		t.Errorf("expected score 1.0, got %f", result.Score)
	}
	if len(result.Findings) != 0 {
		t.Errorf("expected 0 findings, got %d", len(result.Findings))
	}
}

func TestAuditLogCheckerLogsDisabled(t *testing.T) {
	checker := NewAuditLogChecker(false, 90, nil, nil)
	result, err := checker.Check(context.Background(), makeControl("AU-1", CategoryAuditLogging))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	for _, f := range result.Findings {
		if f.ID == "AL-001" {
			found = true
			if f.Severity != SeverityCritical {
				t.Errorf("expected severity %q, got %q", SeverityCritical, f.Severity)
			}
		}
	}
	if !found {
		t.Error("expected finding AL-001 for disabled logs")
	}
}

func TestAuditLogCheckerInsufficientRetention(t *testing.T) {
	checker := NewAuditLogChecker(true, 30, nil, nil)
	result, err := checker.Check(context.Background(), makeControl("AU-1", CategoryAuditLogging))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	for _, f := range result.Findings {
		if f.ID == "AL-002" {
			found = true
			if f.Severity != SeverityHigh {
				t.Errorf("expected severity %q, got %q", SeverityHigh, f.Severity)
			}
		}
	}
	if !found {
		t.Error("expected finding AL-002 for insufficient retention")
	}
}

func TestAuditLogCheckerRetention89Days(t *testing.T) {
	checker := NewAuditLogChecker(true, 89, nil, nil)
	result, err := checker.Check(context.Background(), makeControl("AU-1", CategoryAuditLogging))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status == StatusCompliant {
		t.Error("89 days retention should not be compliant")
	}
}

func TestAuditLogCheckerMissingEvents(t *testing.T) {
	required := []string{"login", "logout", "admin"}
	configured := []string{"login"}
	checker := NewAuditLogChecker(true, 90, required, configured)

	result, err := checker.Check(context.Background(), makeControl("AU-1", CategoryAuditLogging))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 2 missing events out of 5 total checks => 3/5 = 0.6 => non-compliant
	if result.Status == StatusCompliant {
		t.Error("expected non-compliant status with missing events")
	}

	// Should have findings for each missing event
	missingCount := 0
	for _, f := range result.Findings {
		if f.Severity == SeverityMedium {
			missingCount++
		}
	}
	if missingCount != 2 {
		t.Errorf("expected 2 missing event findings, got %d", missingCount)
	}
}

func TestAuditLogCheckerAllIssues(t *testing.T) {
	// Everything wrong: disabled, short retention, missing events
	required := []string{"login", "logout"}
	configured := []string{}
	checker := NewAuditLogChecker(false, 10, required, configured)

	result, err := checker.Check(context.Background(), makeControl("AU-1", CategoryAuditLogging))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != StatusNonCompliant {
		t.Errorf("expected StatusNonCompliant, got %s", result.Status)
	}
	// 4 issues: disabled (1) + retention (1) + missing events (2) = 4
	if len(result.Findings) != 4 {
		t.Errorf("expected 4 findings, got %d", len(result.Findings))
	}
	// Score: totalChecks=4, issues=4, score=0/4=0
	if result.Score != 0.0 {
		t.Errorf("expected score 0.0, got %f", result.Score)
	}
}

func TestAuditLogCheckerPartialCompliance(t *testing.T) {
	// logs enabled, retention ok, 1 of 5 required events missing
	// totalChecks = 2 + 5 = 7, issues = 1, score = 6/7 ~= 0.857 => Partial
	required := []string{"a", "b", "c", "d", "e"}
	configured := []string{"a", "b", "c", "d"} // missing "e"
	checker := NewAuditLogChecker(true, 90, required, configured)

	result, err := checker.Check(context.Background(), makeControl("AU-1", CategoryAuditLogging))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != StatusPartial {
		t.Errorf("expected StatusPartial, got %s", result.Status)
	}
	expectedScore := 6.0 / 7.0
	if result.Score < expectedScore-0.01 || result.Score > expectedScore+0.01 {
		t.Errorf("expected score ~%f, got %f", expectedScore, result.Score)
	}
}

func TestAuditLogCheckerTestedBy(t *testing.T) {
	checker := NewAuditLogChecker(true, 90, nil, nil)
	result, _ := checker.Check(context.Background(), makeControl("AU-1", CategoryAuditLogging))
	if result.TestedBy != "automated" {
		t.Errorf("expected TestedBy %q, got %q", "automated", result.TestedBy)
	}
}

// ---------------------------------------------------------------------------
// runSingleCheck
// ---------------------------------------------------------------------------

func TestRunSingleCheckTimeout(t *testing.T) {
	reg := NewCheckRegistry()
	reg.RegisterFunc("SLOW", func(ctx context.Context, c *Control) (*ControlResult, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(5 * time.Second):
			return &ControlResult{ControlID: c.ID, Status: StatusCompliant, TestedAt: time.Now()}, nil
		}
	})

	runner := NewCheckRunner(reg).WithTimeout(50 * time.Millisecond)
	ctrl := makeControl("SLOW", CategoryAccessControl)
	result, err := runner.runSingleCheck(context.Background(), ctrl)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if result.Status != StatusNotAssessed {
		t.Errorf("expected StatusNotAssessed on timeout, got %s", result.Status)
	}
}
