// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package controls

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Checker defines the interface for control checks.
type Checker interface {
	Check(ctx context.Context, control *Control) (*ControlResult, error)
	Name() string
	Supports(control *Control) bool
}

// CheckFunc is a function type for custom control checks.
type CheckFunc func(ctx context.Context, control *Control) (*ControlResult, error)

// CheckRegistry manages registered control checkers.
type CheckRegistry struct {
	mu       sync.RWMutex
	checkers map[string]Checker
	funcs    map[string]CheckFunc
}

// NewCheckRegistry creates a new check registry.
func NewCheckRegistry() *CheckRegistry {
	return &CheckRegistry{
		checkers: make(map[string]Checker),
		funcs:    make(map[string]CheckFunc),
	}
}

// Register registers a checker for a control pattern.
func (r *CheckRegistry) Register(pattern string, checker Checker) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.checkers[pattern] = checker
}

// RegisterFunc registers a check function for a specific control ID.
func (r *CheckRegistry) RegisterFunc(controlID string, fn CheckFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.funcs[controlID] = fn
}

// GetChecker returns the checker for a control.
func (r *CheckRegistry) GetChecker(control *Control) (Checker, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// First check for exact match function
	if fn, ok := r.funcs[control.ID]; ok {
		return &funcChecker{
			name: control.ID,
			fn:   fn,
		}, true
	}

	// Then check registered checkers
	for _, checker := range r.checkers {
		if checker.Supports(control) {
			return checker, true
		}
	}

	return nil, false
}

// funcChecker wraps a CheckFunc as a Checker.
type funcChecker struct {
	name string
	fn   CheckFunc
}

func (f *funcChecker) Check(ctx context.Context, control *Control) (*ControlResult, error) {
	return f.fn(ctx, control)
}

func (f *funcChecker) Name() string {
	return f.name
}

func (f *funcChecker) Supports(control *Control) bool {
	return control.ID == f.name
}

// CheckRunner executes control checks.
type CheckRunner struct {
	registry    *CheckRegistry
	concurrency int
	timeout     time.Duration
}

// NewCheckRunner creates a new check runner.
func NewCheckRunner(registry *CheckRegistry) *CheckRunner {
	return &CheckRunner{
		registry:    registry,
		concurrency: 10,
		timeout:     5 * time.Minute,
	}
}

// WithConcurrency sets the concurrency level.
func (r *CheckRunner) WithConcurrency(n int) *CheckRunner {
	r.concurrency = n
	return r
}

// WithTimeout sets the timeout for each check.
func (r *CheckRunner) WithTimeout(d time.Duration) *CheckRunner {
	r.timeout = d
	return r
}

// RunChecks executes checks for multiple controls.
func (r *CheckRunner) RunChecks(ctx context.Context, controls []*Control) ([]*ControlResult, error) {
	if len(controls) == 0 {
		return []*ControlResult{}, nil
	}

	results := make([]*ControlResult, len(controls))
	var mu sync.Mutex
	var firstErr error

	// Create work channel (buffered to avoid blocking)
	type workItem struct {
		idx  int
		ctrl *Control
	}
	work := make(chan workItem, len(controls))

	// Start workers first
	var wg sync.WaitGroup
	numWorkers := r.concurrency
	if numWorkers > len(controls) {
		numWorkers = len(controls)
	}

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for item := range work {
				select {
				case <-ctx.Done():
					mu.Lock()
					results[item.idx] = &ControlResult{
						ControlID: item.ctrl.ID,
						Status:    StatusNotAssessed,
						Score:     0,
						TestedAt:  time.Now(),
						Notes:     "Context cancelled",
					}
					mu.Unlock()
					continue
				default:
				}

				result, err := r.runSingleCheck(ctx, item.ctrl)
				mu.Lock()
				if err != nil && firstErr == nil {
					firstErr = err
				}
				results[item.idx] = result
				mu.Unlock()
			}
		}()
	}

	// Queue all work (this won't block since channel is buffered)
	for i, ctrl := range controls {
		work <- workItem{i, ctrl}
	}
	close(work)

	wg.Wait()
	return results, firstErr
}

// runSingleCheck executes a single control check.
func (r *CheckRunner) runSingleCheck(ctx context.Context, control *Control) (*ControlResult, error) {
	checker, ok := r.registry.GetChecker(control)
	if !ok {
		// Return not assessed if no checker available - no need for timeout
		return &ControlResult{
			ControlID: control.ID,
			Status:    StatusNotAssessed,
			Score:     0,
			TestedAt:  time.Now(),
			Notes:     "No automated checker available",
		}, nil
	}

	checkCtx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	result, err := checker.Check(checkCtx, control)
	if err != nil {
		return &ControlResult{
			ControlID: control.ID,
			Status:    StatusNotAssessed,
			Score:     0,
			TestedAt:  time.Now(),
			Notes:     fmt.Sprintf("Check failed: %v", err),
		}, err
	}

	return result, nil
}

// AccessControlChecker checks access control related controls.
type AccessControlChecker struct {
	policies []AccessPolicy
}

// AccessPolicy represents an access control policy.
type AccessPolicy struct {
	ID          string
	Name        string
	Enforced    bool
	LastChecked time.Time
}

// NewAccessControlChecker creates a new access control checker.
func NewAccessControlChecker(policies []AccessPolicy) *AccessControlChecker {
	return &AccessControlChecker{policies: policies}
}

func (c *AccessControlChecker) Name() string {
	return "access_control_checker"
}

func (c *AccessControlChecker) Supports(control *Control) bool {
	return control.Category == CategoryAccessControl
}

func (c *AccessControlChecker) Check(ctx context.Context, control *Control) (*ControlResult, error) {
	findings := []Finding{}
	compliant := 0
	total := len(c.policies)

	for _, policy := range c.policies {
		if policy.Enforced {
			compliant++
		} else {
			findings = append(findings, Finding{
				ID:          fmt.Sprintf("AC-%s", policy.ID),
				Title:       fmt.Sprintf("Policy %s not enforced", policy.Name),
				Description: fmt.Sprintf("Access control policy '%s' is not currently enforced", policy.Name),
				Severity:    SeverityHigh,
				Resource:    policy.ID,
				Remediation: "Enable and enforce the access control policy",
				DetectedAt:  time.Now(),
			})
		}
	}

	score := float64(compliant) / float64(total)
	status := StatusCompliant
	if score < 1.0 {
		if score >= 0.8 {
			status = StatusPartial
		} else {
			status = StatusNonCompliant
		}
	}

	return &ControlResult{
		ControlID: control.ID,
		Status:    status,
		Score:     score,
		Findings:  findings,
		TestedAt:  time.Now(),
		TestedBy:  "automated",
	}, nil
}

// AuditLogChecker checks audit logging controls.
type AuditLogChecker struct {
	logsEnabled     bool
	retentionDays   int
	requiredEvents  []string
	configuredEvents []string
}

// NewAuditLogChecker creates a new audit log checker.
func NewAuditLogChecker(enabled bool, retention int, required, configured []string) *AuditLogChecker {
	return &AuditLogChecker{
		logsEnabled:      enabled,
		retentionDays:    retention,
		requiredEvents:   required,
		configuredEvents: configured,
	}
}

func (c *AuditLogChecker) Name() string {
	return "audit_log_checker"
}

func (c *AuditLogChecker) Supports(control *Control) bool {
	return control.Category == CategoryAuditLogging
}

func (c *AuditLogChecker) Check(ctx context.Context, control *Control) (*ControlResult, error) {
	findings := []Finding{}
	issues := 0

	if !c.logsEnabled {
		issues++
		findings = append(findings, Finding{
			ID:          "AL-001",
			Title:       "Audit logging disabled",
			Description: "Audit logging is not enabled for the system",
			Severity:    SeverityCritical,
			Remediation: "Enable audit logging immediately",
			DetectedAt:  time.Now(),
		})
	}

	if c.retentionDays < 90 {
		issues++
		findings = append(findings, Finding{
			ID:          "AL-002",
			Title:       "Insufficient log retention",
			Description: fmt.Sprintf("Log retention is %d days, minimum 90 days required", c.retentionDays),
			Severity:    SeverityHigh,
			Remediation: "Increase log retention to at least 90 days",
			DetectedAt:  time.Now(),
		})
	}

	// Check for missing required events
	configuredSet := make(map[string]bool)
	for _, e := range c.configuredEvents {
		configuredSet[e] = true
	}

	for _, required := range c.requiredEvents {
		if !configuredSet[required] {
			issues++
			findings = append(findings, Finding{
				ID:          fmt.Sprintf("AL-EVT-%s", required),
				Title:       fmt.Sprintf("Missing event: %s", required),
				Description: fmt.Sprintf("Required audit event '%s' is not configured", required),
				Severity:    SeverityMedium,
				Remediation: fmt.Sprintf("Configure audit logging for %s events", required),
				DetectedAt:  time.Now(),
			})
		}
	}

	totalChecks := 2 + len(c.requiredEvents)
	score := float64(totalChecks-issues) / float64(totalChecks)

	status := StatusCompliant
	if issues > 0 {
		if score >= 0.8 {
			status = StatusPartial
		} else {
			status = StatusNonCompliant
		}
	}

	return &ControlResult{
		ControlID: control.ID,
		Status:    status,
		Score:     score,
		Findings:  findings,
		TestedAt:  time.Now(),
		TestedBy:  "automated",
	}, nil
}
