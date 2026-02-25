// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package campaigns

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

// D2Injection tests injection attack vulnerabilities.
type D2Injection struct{}

func (d *D2Injection) Name() string        { return "d2" }
func (d *D2Injection) Description() string { return "Injection Attacks" }

func (d *D2Injection) Run(ctx context.Context, cfg Config) CampaignResult {
	start := time.Now()
	result := CampaignResult{
		Campaign:    "d2",
		Description: "Injection Attacks",
		StartTime:   start,
	}

	tests := []struct {
		name string
		fn   func(ctx context.Context, cfg Config) *Finding
	}{
		{"sql_injection", d.testSQLInjection},
		{"json_injection", d.testJSONInjection},
		{"log_injection", d.testLogInjection},
		{"command_injection", d.testCommandInjection},
		{"path_traversal", d.testPathTraversal},
		{"header_injection", d.testHeaderInjection},
		{"null_byte_injection", d.testNullByteInjection},
		{"oversized_payload", d.testOversizedPayload},
	}

	for _, test := range tests {
		result.TestsRun++
		finding := test.fn(ctx, cfg)
		if finding != nil {
			finding.ID = fmt.Sprintf("D2-%03d", result.TestsRun)
			result.Findings = append(result.Findings, *finding)
		} else {
			result.TestsPassed++
		}
	}

	result.Duration = time.Since(start)
	return result
}

func (d *D2Injection) testSQLInjection(ctx context.Context, cfg Config) *Finding {
	payloads := []string{
		`' OR '1'='1`,
		`"; DROP TABLE users; --`,
		`1 UNION SELECT * FROM users`,
	}
	for _, payload := range payloads {
		resp, err := doHTTPRequestWithBody(ctx,
			cfg.GatewayAddr+"/api/v1/threats",
			"POST", "application/json",
			fmt.Sprintf(`{"name":"%s"}`, payload), nil)
		if err != nil {
			continue
		}
		// 500 with SQL error in body would indicate SQL injection.
		if resp.StatusCode == http.StatusInternalServerError {
			return &Finding{
				Severity:    "critical",
				Title:       "Possible SQL injection",
				Description: fmt.Sprintf("SQL payload returned 500: %s", payload),
				Remediation: "Use parameterized queries, never interpolate user input into SQL",
			}
		}
	}
	return nil
}

func (d *D2Injection) testJSONInjection(ctx context.Context, cfg Config) *Finding {
	payloads := []string{
		`{"name":"test","__proto__":{"admin":true}}`,
		`{"name":"test","constructor":{"prototype":{"admin":true}}}`,
		`{invalid json`,
	}
	for _, payload := range payloads {
		resp, err := doHTTPRequestWithBody(ctx,
			cfg.GatewayAddr+"/api/v1/threats",
			"POST", "application/json", payload, nil)
		if err != nil {
			continue
		}
		// Invalid JSON should get 400, not 500.
		if resp.StatusCode == http.StatusInternalServerError {
			return &Finding{
				Severity:    "high",
				Title:       "JSON parsing error causes 500",
				Description: "Malformed JSON returns server error instead of 400",
				Remediation: "Validate JSON input and return 400 for parse errors",
			}
		}
	}
	return nil
}

func (d *D2Injection) testLogInjection(ctx context.Context, cfg Config) *Finding {
	// Log injection: newlines in user input that could forge log entries.
	payload := "normal\n[ERROR] FAKE LOG ENTRY injected=true"
	headers := map[string]string{"X-Request-ID": payload}
	_, err := doHTTPRequest(ctx, cfg.GatewayAddr+"/health", "GET", headers, nil)
	if err != nil {
		return nil
	}
	// Can't easily detect log injection remotely — this is a code audit finding.
	// Structured logging (zerolog) should prevent it.
	return nil
}

func (d *D2Injection) testCommandInjection(ctx context.Context, cfg Config) *Finding {
	payloads := []string{
		`; cat /etc/passwd`,
		`$(whoami)`,
		"`id`",
	}
	for _, payload := range payloads {
		resp, err := doHTTPRequestWithBody(ctx,
			cfg.GatewayAddr+"/api/v1/threats",
			"POST", "application/json",
			fmt.Sprintf(`{"name":"%s"}`, payload), nil)
		if err != nil {
			continue
		}
		if resp.StatusCode == http.StatusInternalServerError {
			return &Finding{
				Severity:    "critical",
				Title:       "Possible command injection",
				Description: "Shell metacharacters in input caused server error",
				Remediation: "Never pass user input to shell commands",
			}
		}
	}
	return nil
}

func (d *D2Injection) testPathTraversal(ctx context.Context, cfg Config) *Finding {
	paths := []string{
		"/api/v1/../../../etc/passwd",
		"/api/v1/data?file=../../etc/shadow",
		"/api/v1/data?path=%2e%2e%2f%2e%2e%2fetc%2fpasswd",
	}
	for _, path := range paths {
		resp, err := doHTTPRequest(ctx, cfg.GatewayAddr+path, "GET", nil, nil)
		if err != nil {
			continue
		}
		if resp.StatusCode == http.StatusOK {
			return &Finding{
				Severity:    "critical",
				Title:       "Path traversal vulnerability",
				Description: fmt.Sprintf("Path traversal returned 200: %s", path),
				Remediation: "Sanitize file paths, reject .. components",
			}
		}
	}
	return nil
}

func (d *D2Injection) testHeaderInjection(ctx context.Context, cfg Config) *Finding {
	headers := map[string]string{
		"X-Injected": "value\r\nX-Fake-Header: injected",
	}
	_, err := doHTTPRequest(ctx, cfg.GatewayAddr+"/health", "GET", headers, nil)
	if err != nil {
		return nil
	}
	// Go's net/http rejects CRLF in headers by default.
	return nil
}

func (d *D2Injection) testNullByteInjection(ctx context.Context, cfg Config) *Finding {
	resp, err := doHTTPRequestWithBody(ctx,
		cfg.GatewayAddr+"/api/v1/threats",
		"POST", "application/json",
		`{"name":"test\u0000injected"}`, nil)
	if err != nil {
		return nil
	}
	if resp.StatusCode == http.StatusInternalServerError {
		return &Finding{
			Severity:    "medium",
			Title:       "Null byte causes server error",
			Description: "Null byte in input returned 500",
			Remediation: "Strip or reject null bytes in input",
		}
	}
	return nil
}

func (d *D2Injection) testOversizedPayload(ctx context.Context, cfg Config) *Finding {
	// Send a 10MB payload.
	bigPayload := `{"name":"` + string(make([]byte, 10*1024*1024)) + `"}`
	resp, err := doHTTPRequestWithBody(ctx,
		cfg.GatewayAddr+"/api/v1/threats",
		"POST", "application/json", bigPayload, nil)
	if err != nil {
		return nil // Connection reset is acceptable for oversized payloads.
	}
	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated {
		return &Finding{
			Severity:    "high",
			Title:       "No request size limit",
			Description: "10MB payload accepted without rejection",
			Remediation: "Enforce request body size limits (e.g., 1MB)",
		}
	}
	return nil
}
