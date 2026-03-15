// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package rules

import (
	"context"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// ============================================================================
// S77 BUG #17: XFF Spoofing — Integration Tests
// Verifies that the Engine, ChainedRule rate limiter, and extractSecureClientIP
// work together end-to-end to prevent IP spoofing via X-Forwarded-For headers.
// ============================================================================

func TestEngine_XFFSpoofingBlocked_Integration(t *testing.T) {
	// Create an engine with a rule that matches on IP target.
	// The attacker sends XFF headers to try to evade IP-based detection.
	engine := NewEngine()

	rs := NewRuleSet("test", "Test rules")
	// Rule that blocks a specific IP pattern
	rule, err := NewRule("block-ip", "Block 8.8.8.8", `^8\.8\.8\.8$`, []Target{TargetIP}, ActionBlock, SeverityHigh)
	if err != nil {
		t.Fatal(err)
	}
	rs.AddRule(rule)
	engine.AddRuleSet(rs)

	// External attacker at 8.8.8.8 tries to spoof XFF to bypass IP rule
	req := httptest.NewRequest("GET", "http://example.com/api/data", nil)
	req.RemoteAddr = "8.8.8.8:54321"
	req.Header.Set("X-Forwarded-For", "10.0.0.1") // Spoofed internal IP

	result, err := engine.Evaluate(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}

	// The rule MUST still match because extractSecureClientIP ignores XFF from
	// external peers — the actual IP (8.8.8.8) should be used.
	if result.Action != ActionBlock {
		t.Errorf("XFF spoofing bypass: Action = %v, want ActionBlock (IP should be 8.8.8.8, not spoofed 10.0.0.1)", result.Action)
	}
}

func TestEngine_XFFTrustedProxy_Integration(t *testing.T) {
	engine := NewEngine()

	rs := NewRuleSet("test", "Test rules")
	// Rule that blocks a specific external IP
	rule, err := NewRule("block-ip", "Block attacker", `^203\.0\.113\.50$`, []Target{TargetIP}, ActionBlock, SeverityHigh)
	if err != nil {
		t.Fatal(err)
	}
	rs.AddRule(rule)
	engine.AddRuleSet(rs)

	// Request arrives through internal proxy (10.10.10.100) with real client IP in XFF
	req := httptest.NewRequest("GET", "http://example.com/api/data", nil)
	req.RemoteAddr = "10.10.10.100:12345" // Internal proxy
	req.Header.Set("X-Forwarded-For", "203.0.113.50")

	result, err := engine.Evaluate(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}

	// XFF should be trusted from internal proxy — should detect 203.0.113.50
	if result.Action != ActionBlock {
		t.Errorf("XFF from trusted proxy: Action = %v, want ActionBlock", result.Action)
	}
}

func TestChainedRule_RateLimiter_UsesSecureIP(t *testing.T) {
	// Verify that the chain rate limiter uses extractSecureClientIP (via getClientKey)
	// so that an external attacker can't bypass rate limits by spoofing XFF.

	rule, err := NewRule("rate-test", "Rate limit test", `.*`, []Target{TargetPath}, ActionLog, SeverityLow)
	if err != nil {
		t.Fatal(err)
	}

	cr := NewChainedRule(rule)
	cr.SetRateLimit(3, time.Minute, ActionBlock)

	// Attacker sends 4 requests from 8.8.8.8, each with a different XFF
	for i := 0; i < 4; i++ {
		req := httptest.NewRequest("GET", "http://example.com/", nil)
		req.RemoteAddr = "8.8.8.8:12345"
		// Each request spoofs a different XFF to try to get a fresh rate limit bucket
		req.Header.Set("X-Forwarded-For", "192.168.1."+string(rune('1'+i)))

		data := &requestData{
			URL:    req.URL.String(),
			Path:   req.URL.Path,
			IP:     extractSecureClientIP(req),
			Method: "GET",
		}

		result := cr.EvaluateChain(context.Background(), req, data)

		if i < 3 {
			// First 3 should be allowed (under rate limit)
			if result.RateLimited {
				t.Errorf("request %d: rate limited too early", i+1)
			}
		} else {
			// 4th should be rate limited because all 4 requests share the same
			// key (8.8.8.8) — XFF spoofing didn't bypass the rate limiter
			if !result.RateLimited {
				t.Errorf("request %d: NOT rate limited — XFF spoofing bypassed rate limiter", i+1)
			}
		}
	}
}

func TestChainedRule_RateLimiter_TrustedProxySeparateKeys(t *testing.T) {
	// When requests come through a trusted proxy, different XFF IPs should
	// get separate rate limit buckets (because the real client IPs differ).

	rule, err := NewRule("rate-test", "Rate limit test", `.*`, []Target{TargetPath}, ActionLog, SeverityLow)
	if err != nil {
		t.Fatal(err)
	}

	cr := NewChainedRule(rule)
	cr.SetRateLimit(2, time.Minute, ActionBlock)

	// Two different real clients coming through the same internal proxy
	clients := []string{"1.2.3.4", "5.6.7.8"}
	for _, clientIP := range clients {
		for i := 0; i < 2; i++ {
			req := httptest.NewRequest("GET", "http://example.com/", nil)
			req.RemoteAddr = "10.10.10.100:12345" // Internal proxy
			req.Header.Set("X-Forwarded-For", clientIP)

			data := &requestData{
				URL:    req.URL.String(),
				Path:   req.URL.Path,
				IP:     extractSecureClientIP(req),
				Method: "GET",
			}

			result := cr.EvaluateChain(context.Background(), req, data)
			if result.RateLimited {
				t.Errorf("client %s request %d: should not be rate limited (each client gets own bucket)", clientIP, i+1)
			}
		}
	}
}

func TestExtractSecureClientIP_IPv6(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr string
		xff        string
		wantIP     string
	}{
		{
			name:       "IPv6 loopback trusts XFF",
			remoteAddr: "[::1]:12345",
			xff:        "2001:db8::1",
			wantIP:     "2001:db8::1",
		},
		{
			name:       "IPv6 external ignores XFF",
			remoteAddr: "[2001:db8::bad]:12345",
			xff:        "10.0.0.1",
			wantIP:     "2001:db8::bad",
		},
		{
			name:       "IPv6 link-local (private) trusts XFF",
			remoteAddr: "[fd00::1]:12345",
			xff:        "203.0.113.50",
			wantIP:     "203.0.113.50",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "http://example.com/", nil)
			req.RemoteAddr = tt.remoteAddr
			if tt.xff != "" {
				req.Header.Set("X-Forwarded-For", tt.xff)
			}

			got := extractSecureClientIP(req)
			if got != tt.wantIP {
				t.Errorf("extractSecureClientIP() = %q, want %q", got, tt.wantIP)
			}
		})
	}
}

func TestExtractSecureClientIP_ConcurrentSafe(t *testing.T) {
	// Verify extractSecureClientIP is safe to call concurrently
	const goroutines = 100
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			req := httptest.NewRequest("GET", "http://example.com/", nil)
			req.RemoteAddr = "8.8.8.8:12345"
			req.Header.Set("X-Forwarded-For", "1.2.3.4")

			ip := extractSecureClientIP(req)
			if ip != "8.8.8.8" {
				t.Errorf("concurrent extractSecureClientIP() = %q, want 8.8.8.8", ip)
			}
		}()
	}
	wg.Wait()
}

func TestEngine_ExtractRequestData_UsesSecureIP(t *testing.T) {
	// Verify that the engine's extractRequestData uses extractSecureClientIP
	engine := NewEngine()

	// External peer with spoofed XFF
	req := httptest.NewRequest("GET", "http://example.com/test", nil)
	req.RemoteAddr = "198.51.100.1:9999"
	req.Header.Set("X-Forwarded-For", "127.0.0.1") // Trying to look like localhost

	data := engine.extractRequestData(req)
	if data.IP != "198.51.100.1" {
		t.Errorf("extractRequestData().IP = %q, want %q (XFF from external peer should be ignored)",
			data.IP, "198.51.100.1")
	}
}

func TestEngine_ExtractRequestData_TrustedProxyXFF(t *testing.T) {
	engine := NewEngine()

	// Internal proxy forwarding real client IP
	req := httptest.NewRequest("GET", "http://example.com/test", nil)
	req.RemoteAddr = "127.0.0.1:9999" // Loopback = trusted
	req.Header.Set("X-Forwarded-For", "198.51.100.1")

	data := engine.extractRequestData(req)
	if data.IP != "198.51.100.1" {
		t.Errorf("extractRequestData().IP = %q, want %q (XFF from loopback should be trusted)",
			data.IP, "198.51.100.1")
	}
}
