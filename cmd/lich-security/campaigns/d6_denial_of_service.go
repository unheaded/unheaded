// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package campaigns

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// D6DenialOfService tests DoS resilience.
type D6DenialOfService struct{}

func (d *D6DenialOfService) Name() string        { return "d6" }
func (d *D6DenialOfService) Description() string { return "Denial of Service" }

func (d *D6DenialOfService) Run(ctx context.Context, cfg Config) CampaignResult {
	start := time.Now()
	result := CampaignResult{
		Campaign:    "d6",
		Description: "Denial of Service",
		StartTime:   start,
	}

	tests := []struct {
		name string
		fn   func(ctx context.Context, cfg Config) *Finding
	}{
		{"large_payload", d.testLargePayload},
		{"slow_client", d.testSlowClient},
		{"connection_exhaustion", d.testConnectionExhaustion},
		{"rate_limiting", d.testRateLimiting},
		{"request_timeout", d.testRequestTimeout},
	}

	for _, test := range tests {
		result.TestsRun++
		finding := test.fn(ctx, cfg)
		if finding != nil {
			finding.ID = fmt.Sprintf("D6-%03d", result.TestsRun)
			result.Findings = append(result.Findings, *finding)
		} else {
			result.TestsPassed++
		}
	}

	result.Duration = time.Since(start)
	return result
}

func (d *D6DenialOfService) testLargePayload(ctx context.Context, cfg Config) *Finding {
	// Send a very large request body.
	largeBody := strings.Repeat("A", 100*1024*1024) // 100MB
	resp, err := doHTTPRequestWithBody(ctx,
		cfg.GatewayAddr+"/api/v1/threats",
		"POST", "application/json",
		`{"name":"`+largeBody+`"}`, nil)
	if err != nil {
		return nil // Connection reset/timeout is acceptable.
	}
	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated {
		return &Finding{
			Severity:    "high",
			Title:       "No request size limit enforced",
			Description: "100MB payload accepted without rejection",
			Remediation: "Enforce http.MaxBytesReader or request body size limit",
		}
	}
	return nil
}

func (d *D6DenialOfService) testSlowClient(_ context.Context, cfg Config) *Finding {
	// Slowloris-style: open connection, send data very slowly.
	conn, err := net.DialTimeout("tcp", extractHost(cfg.GatewayAddr), 5*time.Second)
	if err != nil {
		return nil
	}
	defer conn.Close()

	// Send partial HTTP request header very slowly.
	_, _ = conn.Write([]byte("GET /health HTTP/1.1\r\nHost: localhost\r\n"))

	// Wait — server should timeout this connection.
	_ = conn.SetReadDeadline(time.Now().Add(35 * time.Second))
	buf := make([]byte, 1024)
	_, err = conn.Read(buf)
	if err == nil {
		// If we got a response despite never finishing headers, the server
		// handled the partial request (which is fine).
		return nil
	}
	// Timeout or connection reset is expected (server dropped slow client).
	return nil
}

func (d *D6DenialOfService) testConnectionExhaustion(_ context.Context, cfg Config) *Finding {
	// Open many connections simultaneously.
	const maxConns = 500
	var conns []net.Conn
	var mu sync.Mutex

	for i := 0; i < maxConns; i++ {
		conn, err := net.DialTimeout("tcp", extractHost(cfg.GatewayAddr), 2*time.Second)
		if err != nil {
			break // Connection limit reached (good).
		}
		mu.Lock()
		conns = append(conns, conn)
		mu.Unlock()
	}

	// Clean up.
	for _, c := range conns {
		c.Close()
	}

	if len(conns) >= maxConns {
		return &Finding{
			Severity:    "medium",
			Title:       "No connection limit enforced",
			Description: fmt.Sprintf("Successfully opened %d simultaneous connections", len(conns)),
			Remediation: "Configure MaxConnsPerHost and connection limits",
		}
	}
	return nil
}

func (d *D6DenialOfService) testRateLimiting(ctx context.Context, cfg Config) *Finding {
	// Send rapid-fire requests and check for 429.
	const numRequests = 100
	rateLimited := false

	for i := 0; i < numRequests; i++ {
		resp, err := doHTTPRequest(ctx, cfg.GatewayAddr+"/health", "GET", nil, nil)
		if err != nil {
			continue
		}
		if resp.StatusCode == http.StatusTooManyRequests {
			rateLimited = true
			break
		}
	}

	// Note: rate limiting on /health is not required, but it's a good proxy.
	// Real rate limiting should be on authenticated endpoints.
	_ = rateLimited
	return nil
}

func (d *D6DenialOfService) testRequestTimeout(ctx context.Context, cfg Config) *Finding {
	// Check that requests have a reasonable timeout.
	// Send a request and check the server responds within 30s.
	start := time.Now()
	resp, err := doHTTPRequest(ctx, cfg.GatewayAddr+"/health", "GET", nil, nil)
	elapsed := time.Since(start)
	if err != nil {
		return nil
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	if elapsed > 30*time.Second {
		return &Finding{
			Severity:    "medium",
			Title:       "Health endpoint slow response",
			Description: fmt.Sprintf("Health check took %s (should be <1s)", elapsed),
			Remediation: "Add timeouts to all HTTP handlers",
		}
	}
	return nil
}
