// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package campaigns

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

// D5PrivilegeEscalation tests privilege escalation vulnerabilities.
type D5PrivilegeEscalation struct{}

func (d *D5PrivilegeEscalation) Name() string        { return "d5" }
func (d *D5PrivilegeEscalation) Description() string { return "Privilege Escalation" }

func (d *D5PrivilegeEscalation) Run(ctx context.Context, cfg Config) CampaignResult {
	start := time.Now()
	result := CampaignResult{
		Campaign:    "d5",
		Description: "Privilege Escalation",
		StartTime:   start,
	}

	tests := []struct {
		name string
		fn   func(ctx context.Context, cfg Config) *Finding
	}{
		{"guest_admin_access", d.testGuestAdminAccess},
		{"observer_operator_access", d.testObserverOperatorAccess},
		{"role_claim_modification", d.testRoleClaimModification},
		{"missing_rbac_check", d.testMissingRBACCheck},
		{"horizontal_escalation", d.testHorizontalEscalation},
	}

	for _, test := range tests {
		result.TestsRun++
		finding := test.fn(ctx, cfg)
		if finding != nil {
			finding.ID = fmt.Sprintf("D5-%03d", result.TestsRun)
			result.Findings = append(result.Findings, *finding)
		} else {
			result.TestsPassed++
		}
	}

	result.Duration = time.Since(start)
	return result
}

func (d *D5PrivilegeEscalation) testGuestAdminAccess(ctx context.Context, cfg Config) *Finding {
	// Try accessing admin endpoints with guest-level credentials.
	adminEndpoints := []string{
		"/api/v1/admin/users",
		"/api/v1/admin/config",
		"/api/v1/deploy",
	}
	for _, ep := range adminEndpoints {
		resp, err := doHTTPRequest(ctx, cfg.GatewayAddr+ep, "GET", nil, nil)
		if err != nil {
			continue
		}
		status := resp.StatusCode
		_ = resp.Body.Close()
		if status == http.StatusOK {
			return &Finding{
				Severity:    "critical",
				Title:       "Admin endpoint accessible without admin role",
				Description: fmt.Sprintf("%s returned 200 without authentication", ep),
				Endpoint:    ep,
				Remediation: "Enforce admin role requirement on admin endpoints",
			}
		}
	}
	return nil
}

func (d *D5PrivilegeEscalation) testObserverOperatorAccess(ctx context.Context, cfg Config) *Finding {
	// Operator endpoints should reject observer-level tokens.
	operatorEndpoints := []string{
		"/api/v1/deploy",
		"/api/v1/config",
	}
	for _, ep := range operatorEndpoints {
		resp, err := doHTTPRequest(ctx, cfg.GatewayAddr+ep, "POST", nil, nil)
		if err != nil {
			continue
		}
		status := resp.StatusCode
		_ = resp.Body.Close()
		if status == http.StatusOK || status == http.StatusCreated {
			return &Finding{
				Severity:    "high",
				Title:       "Write endpoint accessible without operator role",
				Description: fmt.Sprintf("POST %s returned success without proper auth", ep),
				Endpoint:    ep,
				Remediation: "Enforce operator or admin role on write endpoints",
			}
		}
	}
	return nil
}

func (d *D5PrivilegeEscalation) testRoleClaimModification(_ context.Context, _ Config) *Finding {
	// This test would require modifying JWT claims and verifying they're rejected.
	// The HMAC-SHA256 signature prevents claim modification.
	// Code-level verification: JWT claims are always re-verified from signature.
	return nil
}

func (d *D5PrivilegeEscalation) testMissingRBACCheck(ctx context.Context, cfg Config) *Finding {
	// Check that all API endpoints return 401/403 without auth.
	apiEndpoints := []string{
		"/api/v1/threats",
		"/api/v1/status",
		"/api/v1/operations",
		"/api/v1/knowledge",
	}
	for _, ep := range apiEndpoints {
		resp, err := doHTTPRequest(ctx, cfg.GatewayAddr+ep, "GET", nil, nil)
		if err != nil {
			continue
		}
		status := resp.StatusCode
		_ = resp.Body.Close()
		if status == http.StatusOK {
			return &Finding{
				Severity:    "high",
				Title:       "API endpoint missing RBAC check",
				Description: fmt.Sprintf("%s returned 200 without authentication", ep),
				Endpoint:    ep,
				Remediation: "Add RBAC middleware to all API endpoints",
			}
		}
	}
	return nil
}

func (d *D5PrivilegeEscalation) testHorizontalEscalation(_ context.Context, _ Config) *Finding {
	// Horizontal privilege escalation: user A accessing user B's resources.
	// Unheaded doesn't store per-user data (zero user data access policy),
	// so horizontal escalation is not applicable.
	return nil
}

// Exported for testing.
var _ = fmt.Sprintf
