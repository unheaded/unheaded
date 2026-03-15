// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package response

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestNewHandler(t *testing.T) {
	h := NewHandler()

	if h == nil {
		t.Fatal("NewHandler returned nil")
	}

	if h.blockCode != http.StatusForbidden {
		t.Errorf("expected default block code %d, got %d", http.StatusForbidden, h.blockCode)
	}

	if h.headers == nil {
		t.Error("headers map should be initialized")
	}
}

func TestSetBlockPage(t *testing.T) {
	h := NewHandler()
	customPage := "<html><body>Custom Block Page</body></html>"

	h.SetBlockPage(customPage)

	if h.blockPage != customPage {
		t.Errorf("expected block page %q, got %q", customPage, h.blockPage)
	}
}

func TestSetBlockCode(t *testing.T) {
	tests := []struct {
		name string
		code int
	}{
		{"forbidden", http.StatusForbidden},
		{"bad request", http.StatusBadRequest},
		{"unauthorized", http.StatusUnauthorized},
		{"service unavailable", http.StatusServiceUnavailable},
		{"custom code", 418},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHandler()
			h.SetBlockCode(tt.code)

			if h.blockCode != tt.code {
				t.Errorf("expected block code %d, got %d", tt.code, h.blockCode)
			}
		})
	}
}

func TestSetRedirectURL(t *testing.T) {
	h := NewHandler()
	url := "https://example.com/blocked"

	h.SetRedirectURL(url)

	if h.redirectURL != url {
		t.Errorf("expected redirect URL %q, got %q", url, h.redirectURL)
	}
}

func TestSetJSONResponse(t *testing.T) {
	h := NewHandler()

	h.SetJSONResponse(true)
	if !h.jsonResponse {
		t.Error("expected jsonResponse to be true")
	}

	h.SetJSONResponse(false)
	if h.jsonResponse {
		t.Error("expected jsonResponse to be false")
	}
}

func TestSetHeader(t *testing.T) {
	h := NewHandler()

	h.SetHeader("X-WAF-Block", "true")
	h.SetHeader("X-Custom-Header", "custom-value")

	if h.headers["X-WAF-Block"] != "true" {
		t.Error("X-WAF-Block header not set correctly")
	}
	if h.headers["X-Custom-Header"] != "custom-value" {
		t.Error("X-Custom-Header header not set correctly")
	}
}

func TestBlock_HTMLResponse(t *testing.T) {
	tests := []struct {
		name          string
		reason        string
		ruleID        string
		requestID     string
		blockCode     int
		customPage    string
		expectContent []string
	}{
		{
			name:      "default block page",
			reason:    "SQL Injection detected",
			ruleID:    "SQLI-001",
			requestID: "req-12345",
			blockCode: http.StatusForbidden,
			expectContent: []string{
				"Access Denied",
				"403",
				"req-12345",
			},
		},
		{
			name:      "custom status code",
			reason:    "XSS detected",
			ruleID:    "XSS-001",
			requestID: "req-67890",
			blockCode: http.StatusBadRequest,
			expectContent: []string{
				"Access Denied",
				"req-67890",
			},
		},
		{
			name:       "custom block page with template",
			reason:     "Bot detected",
			ruleID:     "BOT-001",
			requestID:  "req-abcde",
			blockCode:  http.StatusForbidden,
			customPage: `<html><body>Blocked: {{.Reason}} - Rule: {{.RuleID}} - Request: {{.RequestID}}</body></html>`,
			expectContent: []string{
				"Blocked: Bot detected",
				"Rule: BOT-001",
				"Request: req-abcde",
			},
		},
		{
			name:      "XSS prevention in request ID",
			reason:    "Attack",
			ruleID:    "TEST-001",
			requestID: "<script>alert('xss')</script>",
			blockCode: http.StatusForbidden,
			expectContent: []string{
				"&lt;script&gt;",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHandler()
			h.SetBlockCode(tt.blockCode)
			if tt.customPage != "" {
				h.SetBlockPage(tt.customPage)
			}

			w := httptest.NewRecorder()
			h.Block(w, tt.reason, tt.ruleID, tt.requestID)

			if w.Code != tt.blockCode {
				t.Errorf("expected status %d, got %d", tt.blockCode, w.Code)
			}

			contentType := w.Header().Get("Content-Type")
			if !strings.Contains(contentType, "text/html") {
				t.Errorf("expected Content-Type text/html, got %s", contentType)
			}

			body := w.Body.String()
			for _, expected := range tt.expectContent {
				if !strings.Contains(body, expected) {
					t.Errorf("expected body to contain %q", expected)
				}
			}
		})
	}
}

func TestBlock_JSONResponse(t *testing.T) {
	tests := []struct {
		name      string
		reason    string
		ruleID    string
		requestID string
		blockCode int
	}{
		{
			name:      "basic JSON response",
			reason:    "SQL Injection detected",
			ruleID:    "SQLI-001",
			requestID: "req-12345",
			blockCode: http.StatusForbidden,
		},
		{
			name:      "empty fields",
			reason:    "",
			ruleID:    "",
			requestID: "",
			blockCode: http.StatusForbidden,
		},
		{
			name:      "special characters",
			reason:    `Attack with "quotes" and <tags>`,
			ruleID:    "TEST-001",
			requestID: "req-special-!@#$%",
			blockCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHandler()
			h.SetJSONResponse(true)
			h.SetBlockCode(tt.blockCode)

			w := httptest.NewRecorder()
			h.Block(w, tt.reason, tt.ruleID, tt.requestID)

			if w.Code != tt.blockCode {
				t.Errorf("expected status %d, got %d", tt.blockCode, w.Code)
			}

			contentType := w.Header().Get("Content-Type")
			if contentType != "application/json" {
				t.Errorf("expected Content-Type application/json, got %s", contentType)
			}

			var resp BlockResponse
			if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
				t.Fatalf("failed to decode JSON response: %v", err)
			}

			if !resp.Blocked {
				t.Error("expected Blocked to be true")
			}
			if resp.Reason != tt.reason {
				t.Errorf("expected Reason %q, got %q", tt.reason, resp.Reason)
			}
			if resp.RuleID != tt.ruleID {
				t.Errorf("expected RuleID %q, got %q", tt.ruleID, resp.RuleID)
			}
			if resp.RequestID != tt.requestID {
				t.Errorf("expected RequestID %q, got %q", tt.requestID, resp.RequestID)
			}
		})
	}
}

func TestBlock_CustomHeaders(t *testing.T) {
	h := NewHandler()
	h.SetHeader("X-WAF-Block", "true")
	h.SetHeader("X-Block-Reason", "SQL Injection")

	w := httptest.NewRecorder()
	h.Block(w, "SQL Injection", "SQLI-001", "req-12345")

	if w.Header().Get("X-WAF-Block") != "true" {
		t.Error("X-WAF-Block header not set")
	}
	if w.Header().Get("X-Block-Reason") != "SQL Injection" {
		t.Error("X-Block-Reason header not set")
	}
}

func TestRedirect(t *testing.T) {
	tests := []struct {
		name           string
		handlerURL     string
		overrideURL    string
		expectedTarget string
	}{
		{
			name:           "use override URL",
			handlerURL:     "https://example.com/default",
			overrideURL:    "https://example.com/override",
			expectedTarget: "https://example.com/override",
		},
		{
			name:           "fallback to handler URL",
			handlerURL:     "https://example.com/default",
			overrideURL:    "",
			expectedTarget: "https://example.com/default",
		},
		{
			name:           "relative URL",
			handlerURL:     "/blocked",
			overrideURL:    "",
			expectedTarget: "/blocked",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHandler()
			h.SetRedirectURL(tt.handlerURL)
			h.SetHeader("X-WAF-Redirect", "true")

			req := httptest.NewRequest("GET", "http://example.com/path", nil)
			w := httptest.NewRecorder()

			h.Redirect(w, req, tt.overrideURL)

			if w.Code != http.StatusFound {
				t.Errorf("expected status %d, got %d", http.StatusFound, w.Code)
			}

			location := w.Header().Get("Location")
			if location != tt.expectedTarget {
				t.Errorf("expected Location %q, got %q", tt.expectedTarget, location)
			}

			if w.Header().Get("X-WAF-Redirect") != "true" {
				t.Error("custom header not set on redirect")
			}
		})
	}
}

func TestChallenge(t *testing.T) {
	tests := []struct {
		name          string
		requestID     string
		expectContent []string
	}{
		{
			name:      "basic challenge",
			requestID: "req-12345",
			expectContent: []string{
				"Security Check",
				"req-12345",
				"Please wait",
			},
		},
		{
			name:      "XSS prevention in request ID",
			requestID: "<script>alert('xss')</script>",
			expectContent: []string{
				"&lt;script&gt;",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHandler()
			h.SetHeader("X-WAF-Challenge", "true")

			w := httptest.NewRecorder()
			h.Challenge(w, tt.requestID)

			if w.Code != http.StatusForbidden {
				t.Errorf("expected status %d, got %d", http.StatusForbidden, w.Code)
			}

			contentType := w.Header().Get("Content-Type")
			if !strings.Contains(contentType, "text/html") {
				t.Errorf("expected Content-Type text/html, got %s", contentType)
			}

			if w.Header().Get("X-WAF-Challenge") != "true" {
				t.Error("custom header not set on challenge")
			}

			body := w.Body.String()
			for _, expected := range tt.expectContent {
				if !strings.Contains(body, expected) {
					t.Errorf("expected body to contain %q", expected)
				}
			}
		})
	}
}

func TestRateLimited_HTMLResponse(t *testing.T) {
	tests := []struct {
		name       string
		retryAfter int
	}{
		{
			name:       "basic rate limit",
			retryAfter: 60,
		},
		{
			name:       "short retry",
			retryAfter: 5,
		},
		{
			name:       "long retry",
			retryAfter: 3600,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHandler()
			h.SetHeader("X-WAF-RateLimit", "true")

			w := httptest.NewRecorder()
			h.RateLimited(w, tt.retryAfter)

			if w.Code != http.StatusTooManyRequests {
				t.Errorf("expected status %d, got %d", http.StatusTooManyRequests, w.Code)
			}

			if w.Header().Get("X-WAF-RateLimit") != "true" {
				t.Error("custom header not set on rate limit response")
			}

			body := w.Body.String()
			if !strings.Contains(body, "429") {
				t.Error("expected body to contain '429'")
			}
			if !strings.Contains(body, "Too Many Requests") {
				t.Error("expected body to contain 'Too Many Requests'")
			}
		})
	}
}

func TestRateLimited_JSONResponse(t *testing.T) {
	tests := []struct {
		name       string
		retryAfter int
	}{
		{
			name:       "basic rate limit",
			retryAfter: 60,
		},
		{
			name:       "zero retry",
			retryAfter: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHandler()
			h.SetJSONResponse(true)

			w := httptest.NewRecorder()
			h.RateLimited(w, tt.retryAfter)

			if w.Code != http.StatusTooManyRequests {
				t.Errorf("expected status %d, got %d", http.StatusTooManyRequests, w.Code)
			}

			var resp map[string]interface{}
			if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
				t.Fatalf("failed to decode JSON response: %v", err)
			}

			if resp["error"] != "rate_limited" {
				t.Errorf("expected error 'rate_limited', got %v", resp["error"])
			}

			retryAfterVal, ok := resp["retry_after"].(float64)
			if !ok {
				t.Fatal("retry_after not found or not a number")
			}
			if int(retryAfterVal) != tt.retryAfter {
				t.Errorf("expected retry_after %d, got %v", tt.retryAfter, retryAfterVal)
			}
		})
	}
}

func TestConcurrentAccess(t *testing.T) {
	h := NewHandler()
	var wg sync.WaitGroup

	// Concurrent setters
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			h.SetBlockCode(http.StatusForbidden)
			h.SetJSONResponse(i%2 == 0)
			h.SetHeader("X-Test", "value")
		}(i)
	}

	// Concurrent Block calls
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := httptest.NewRecorder()
			h.Block(w, "test", "TEST-001", "req-123")
		}()
	}

	// Concurrent Challenge calls
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := httptest.NewRecorder()
			h.Challenge(w, "req-123")
		}()
	}

	// Concurrent RateLimited calls
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := httptest.NewRecorder()
			h.RateLimited(w, 60)
		}()
	}

	// Concurrent Redirect calls
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest("GET", "http://example.com", nil)
			w := httptest.NewRecorder()
			h.Redirect(w, req, "http://example.com/blocked")
		}()
	}

	wg.Wait()
}

func TestBlockResponse_JSONMarshaling(t *testing.T) {
	tests := []struct {
		name     string
		response BlockResponse
	}{
		{
			name: "full response",
			response: BlockResponse{
				Blocked:   true,
				Reason:    "SQL Injection",
				RuleID:    "SQLI-001",
				RequestID: "req-12345",
				Timestamp: "2024-01-15T12:00:00Z",
			},
		},
		{
			name: "minimal response",
			response: BlockResponse{
				Blocked: true,
			},
		},
		{
			name: "response with special characters",
			response: BlockResponse{
				Blocked:   true,
				Reason:    `Attack with "quotes" and <tags>`,
				RuleID:    "TEST-001",
				RequestID: "req-!@#$%",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.response)
			if err != nil {
				t.Fatalf("failed to marshal: %v", err)
			}

			var decoded BlockResponse
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("failed to unmarshal: %v", err)
			}

			if decoded.Blocked != tt.response.Blocked {
				t.Errorf("Blocked mismatch: expected %v, got %v", tt.response.Blocked, decoded.Blocked)
			}
			if decoded.Reason != tt.response.Reason {
				t.Errorf("Reason mismatch: expected %v, got %v", tt.response.Reason, decoded.Reason)
			}
			if decoded.RuleID != tt.response.RuleID {
				t.Errorf("RuleID mismatch: expected %v, got %v", tt.response.RuleID, decoded.RuleID)
			}
			if decoded.RequestID != tt.response.RequestID {
				t.Errorf("RequestID mismatch: expected %v, got %v", tt.response.RequestID, decoded.RequestID)
			}
		})
	}
}

func TestDefaultPages_XSSPrevention(t *testing.T) {
	xssPayloads := []string{
		"<script>alert('xss')</script>",
		"<img src=x onerror=alert('xss')>",
		"javascript:alert('xss')",
		"<svg onload=alert('xss')>",
		`" onclick="alert('xss')`,
		`'><script>alert('xss')</script>`,
		"<iframe src=\"javascript:alert('xss')\">",
	}

	for _, payload := range xssPayloads {
		t.Run("block page XSS: "+payload[:min(20, len(payload))], func(t *testing.T) {
			page := defaultBlockPage("test reason", payload)

			// Should not contain raw XSS payload
			if strings.Contains(page, payload) && !strings.Contains(page, "&lt;") {
				t.Errorf("block page contains unescaped XSS payload: %s", payload)
			}
		})

		t.Run("challenge page XSS: "+payload[:min(20, len(payload))], func(t *testing.T) {
			page := defaultChallengePage(payload)

			// Should not contain raw XSS payload
			if strings.Contains(page, payload) && !strings.Contains(page, "&lt;") {
				t.Errorf("challenge page contains unescaped XSS payload: %s", payload)
			}
		})
	}
}

func TestCustomBlockPage_InvalidTemplate(t *testing.T) {
	h := NewHandler()
	// Invalid template syntax
	h.SetBlockPage("{{.Invalid")

	w := httptest.NewRecorder()
	h.Block(w, "test", "TEST-001", "req-123")

	// Should fall back to default page
	if w.Code != http.StatusForbidden {
		t.Errorf("expected status %d, got %d", http.StatusForbidden, w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "Access Denied") {
		t.Error("expected fallback to default block page")
	}
}

func TestCustomBlockPage_TemplateExecution(t *testing.T) {
	h := NewHandler()
	h.SetBlockPage(`<!DOCTYPE html>
<html>
<body>
<h1>Custom Block</h1>
<p>Reason: {{.Reason}}</p>
<p>Rule: {{.RuleID}}</p>
<p>Request: {{.RequestID}}</p>
</body>
</html>`)

	w := httptest.NewRecorder()
	h.Block(w, "Test Reason", "TEST-001", "req-custom-123")

	body := w.Body.String()

	if !strings.Contains(body, "Custom Block") {
		t.Error("expected custom block page header")
	}
	if !strings.Contains(body, "Reason: Test Reason") {
		t.Error("expected reason in custom page")
	}
	if !strings.Contains(body, "Rule: TEST-001") {
		t.Error("expected rule ID in custom page")
	}
	if !strings.Contains(body, "Request: req-custom-123") {
		t.Error("expected request ID in custom page")
	}
}

func TestHandler_MultipleHeaders(t *testing.T) {
	h := NewHandler()
	h.SetHeader("X-Header-1", "value1")
	h.SetHeader("X-Header-2", "value2")
	h.SetHeader("X-Header-3", "value3")

	w := httptest.NewRecorder()
	h.Block(w, "test", "TEST-001", "req-123")

	if w.Header().Get("X-Header-1") != "value1" {
		t.Error("X-Header-1 not set")
	}
	if w.Header().Get("X-Header-2") != "value2" {
		t.Error("X-Header-2 not set")
	}
	if w.Header().Get("X-Header-3") != "value3" {
		t.Error("X-Header-3 not set")
	}
}

func TestHandler_HeaderOverwrite(t *testing.T) {
	h := NewHandler()
	h.SetHeader("X-Test", "original")
	h.SetHeader("X-Test", "overwritten")

	w := httptest.NewRecorder()
	h.Block(w, "test", "TEST-001", "req-123")

	if w.Header().Get("X-Test") != "overwritten" {
		t.Error("header should be overwritten")
	}
}

// Benchmark tests
func BenchmarkBlock_HTML(b *testing.B) {
	h := NewHandler()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		h.Block(w, "SQL Injection", "SQLI-001", "req-12345")
	}
}

func BenchmarkBlock_JSON(b *testing.B) {
	h := NewHandler()
	h.SetJSONResponse(true)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		h.Block(w, "SQL Injection", "SQLI-001", "req-12345")
	}
}

func BenchmarkBlock_CustomPage(b *testing.B) {
	h := NewHandler()
	h.SetBlockPage(`<!DOCTYPE html>
<html>
<body>
<h1>Blocked</h1>
<p>Reason: {{.Reason}}</p>
<p>Rule: {{.RuleID}}</p>
<p>Request: {{.RequestID}}</p>
</body>
</html>`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		h.Block(w, "SQL Injection", "SQLI-001", "req-12345")
	}
}

func BenchmarkChallenge(b *testing.B) {
	h := NewHandler()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		h.Challenge(w, "req-12345")
	}
}

func BenchmarkRateLimited_HTML(b *testing.B) {
	h := NewHandler()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		h.RateLimited(w, 60)
	}
}

func BenchmarkRateLimited_JSON(b *testing.B) {
	h := NewHandler()
	h.SetJSONResponse(true)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		h.RateLimited(w, 60)
	}
}

func BenchmarkRedirect(b *testing.B) {
	h := NewHandler()
	h.SetRedirectURL("https://example.com/blocked")
	req := httptest.NewRequest("GET", "http://example.com", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		h.Redirect(w, req, "")
	}
}

func BenchmarkConcurrentBlock(b *testing.B) {
	h := NewHandler()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			w := httptest.NewRecorder()
			h.Block(w, "SQL Injection", "SQLI-001", "req-12345")
		}
	})
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
