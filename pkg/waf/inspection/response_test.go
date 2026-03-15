// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package inspection

import (
	"bytes"
	"io"
	"net/http"
	"testing"
)

func newTestResponse(statusCode int, headers map[string]string, body string) *http.Response {
	resp := &http.Response{
		StatusCode: statusCode,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewBufferString(body)),
	}
	for k, v := range headers {
		resp.Header.Set(k, v)
	}
	return resp
}

func TestNewResponseInspector(t *testing.T) {
	ri := NewResponseInspector()
	if ri == nil {
		t.Fatal("expected non-nil inspector")
	}
	if ri.maxResponseSize != 5*1024*1024 {
		t.Errorf("maxResponseSize = %d, want 5MB", ri.maxResponseSize)
	}
	if !ri.inspectErrors {
		t.Error("inspectErrors should default to true")
	}
	if len(ri.infoDisclosurePatterns) == 0 {
		t.Error("expected default info disclosure patterns")
	}
	if len(ri.sensitiveDataPatterns) == 0 {
		t.Error("expected default sensitive data patterns")
	}
}

func TestResponseInspector_InspectCleanResponse(t *testing.T) {
	ri := NewResponseInspector()
	resp := newTestResponse(200, map[string]string{
		"Content-Type":           "application/json",
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Content-Security-Policy": "default-src 'self'",
		"Strict-Transport-Security": "max-age=31536000",
	}, `{"status":"ok"}`)

	result, err := ri.Inspect(resp)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if result.StatusCode != 200 {
		t.Errorf("StatusCode = %d, want 200", result.StatusCode)
	}
	if result.ContentType != ContentTypeJSON {
		t.Errorf("ContentType = %d, want JSON", result.ContentType)
	}
	if result.SensitiveDataFound {
		t.Error("should not find sensitive data in clean response")
	}
}

func TestResponseInspector_DetectStackTrace(t *testing.T) {
	ri := NewResponseInspector()

	tests := []struct {
		name string
		body string
	}{
		{"java", `at MyClass.myMethod(MyClass.java:42)`},
		{"python", `File "/app/main.py", line 42, in handle_request`},
		{"go", `goroutine 1 [running]:`},
		{"node", `at handleRequest (/app/server.js:42:15)`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := newTestResponse(500, map[string]string{"Content-Type": "text/html"}, tt.body)
			result, err := ri.Inspect(resp)
			if err != nil {
				t.Fatalf("Inspect: %v", err)
			}
			if !result.InfoDisclosureFound {
				t.Error("expected info disclosure for stack trace")
			}
		})
	}
}

func TestResponseInspector_DetectVersionDisclosure(t *testing.T) {
	ri := NewResponseInspector()
	resp := newTestResponse(200, map[string]string{
		"Content-Type": "text/html",
		"Server":       "Apache/2.4.51",
	}, `<html>Powered by nginx/1.21.0</html>`)

	result, err := ri.Inspect(resp)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if !result.InfoDisclosureFound {
		t.Error("expected info disclosure for version")
	}
}

func TestResponseInspector_DetectSensitiveData(t *testing.T) {
	ri := NewResponseInspector()

	tests := []struct {
		name string
		body string
	}{
		{"password", `password=supersecretpassword123 other data`},
		{"api_key", `api_key=abcdefghijklmnopqrstuvwxyz1234567890 other data`},
		{"private_key", `-----BEGIN PRIVATE KEY-----`},
		{"aws_key", `AKIAIOSFODNN7EXAMPLE`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := newTestResponse(200, map[string]string{"Content-Type": "application/json"}, tt.body)
			result, err := ri.Inspect(resp)
			if err != nil {
				t.Fatalf("Inspect: %v", err)
			}
			if !result.SensitiveDataFound {
				t.Errorf("expected sensitive data detection for %s", tt.name)
			}
			// Check redaction
			for _, f := range result.Findings {
				if f.IsSensitive && f.Redacted == "" {
					t.Error("sensitive finding should have redacted version")
				}
			}
		})
	}
}

func TestResponseInspector_DetectSQLError(t *testing.T) {
	ri := NewResponseInspector()
	resp := newTestResponse(500, map[string]string{"Content-Type": "text/html"},
		`<html>SQL error: syntax error near "SELECT" at line 1</html>`)

	result, err := ri.Inspect(resp)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if !result.Detected {
		t.Error("expected detection of SQL error in error response")
	}
}

func TestResponseInspector_DetectPathDisclosure(t *testing.T) {
	ri := NewResponseInspector()
	resp := newTestResponse(500, map[string]string{"Content-Type": "text/html"},
		`Error: file not found at /var/www/html/config.php`)

	result, err := ri.Inspect(resp)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if !result.InfoDisclosureFound {
		t.Error("expected info disclosure for path")
	}
}

func TestResponseInspector_MissingSecurityHeaders(t *testing.T) {
	ri := NewResponseInspector()
	resp := newTestResponse(200, map[string]string{
		"Content-Type": "text/html",
	}, `<html>ok</html>`)

	result, err := ri.Inspect(resp)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}

	// Should report missing security headers
	foundMissingHeader := false
	for _, f := range result.HeaderFindings {
		if f.Category == "security_header" {
			foundMissingHeader = true
			break
		}
	}
	if !foundMissingHeader {
		t.Error("expected findings for missing security headers")
	}
}

func TestResponseInspector_ServerVersionHeader(t *testing.T) {
	ri := NewResponseInspector()
	resp := newTestResponse(200, map[string]string{
		"Content-Type": "text/html",
		"Server":       "Apache/2.4.51",
	}, `ok`)

	result, err := ri.Inspect(resp)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}

	foundServerVersion := false
	for _, f := range result.HeaderFindings {
		if f.Category == "header_disclosure" {
			foundServerVersion = true
			break
		}
	}
	if !foundServerVersion {
		t.Error("expected header disclosure for server version")
	}
}

func TestResponseInspector_NilBody(t *testing.T) {
	ri := NewResponseInspector()
	resp := &http.Response{
		StatusCode: 204,
		Header:     make(http.Header),
		Body:       nil,
	}

	result, err := ri.Inspect(resp)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if result.ResponseSize != 0 {
		t.Errorf("ResponseSize = %d, want 0", result.ResponseSize)
	}
}

func TestResponseInspector_DisabledStatusCode(t *testing.T) {
	ri := NewResponseInspector()
	ri.DisableStatusCode(200)

	resp := newTestResponse(200, map[string]string{"Content-Type": "text/html"},
		`goroutine 1 [running]:`) // would normally be detected

	result, err := ri.Inspect(resp)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if result.Detected {
		t.Error("should not detect anything when status code is disabled")
	}
}

func TestResponseInspector_SetMaxResponseSize(t *testing.T) {
	ri := NewResponseInspector()
	ri.SetMaxResponseSize(10)

	resp := newTestResponse(200, map[string]string{"Content-Type": "text/html"},
		`This is a long body that should be truncated to 10 bytes`)

	result, err := ri.Inspect(resp)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if result.ResponseSize > 10 {
		t.Errorf("ResponseSize = %d, should be <= 10", result.ResponseSize)
	}
	if !result.Truncated {
		t.Error("expected truncated flag")
	}
}

func TestResponseInspector_SetInspectErrors(t *testing.T) {
	ri := NewResponseInspector()
	ri.SetInspectErrors(false)

	resp := newTestResponse(500, map[string]string{"Content-Type": "text/html"},
		`SQL error: syntax error near "DROP"`)

	result, err := ri.Inspect(resp)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	// Error message patterns should not be checked
	foundErrorDisclosure := false
	for _, f := range result.Findings {
		if f.Category == "error_disclosure" {
			foundErrorDisclosure = true
		}
	}
	if foundErrorDisclosure {
		t.Error("should not inspect error response patterns when disabled")
	}
}

func TestResponseInspector_AddPattern(t *testing.T) {
	ri := NewResponseInspector()
	err := ri.AddPattern("custom_pattern", `CUSTOM_TOKEN_[A-Z]+`, 25, "custom", "Custom token found")
	if err != nil {
		t.Fatalf("AddPattern: %v", err)
	}

	resp := newTestResponse(200, map[string]string{"Content-Type": "text/plain"},
		`Your token is CUSTOM_TOKEN_ABCDEF`)

	result, err := ri.Inspect(resp)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}

	foundCustom := false
	for _, f := range result.Findings {
		if f.Pattern == "custom_pattern" {
			foundCustom = true
		}
	}
	if !foundCustom {
		t.Error("expected custom pattern detection")
	}
}

func TestResponseInspector_AddPattern_InvalidRegex(t *testing.T) {
	ri := NewResponseInspector()
	err := ri.AddPattern("bad", `[invalid`, 0, "", "")
	if err == nil {
		t.Error("expected error for invalid regex")
	}
}

func TestResponseInspector_GetStats(t *testing.T) {
	ri := NewResponseInspector()
	resp := newTestResponse(200, map[string]string{"Content-Type": "text/plain"},
		`goroutine 1 [running]:`)

	ri.Inspect(resp)

	stats := ri.GetStats()
	if stats["total_inspections"].(int64) != 1 {
		t.Errorf("total_inspections = %v, want 1", stats["total_inspections"])
	}
}

func TestResponseInspector_ContentTypeDetection(t *testing.T) {
	ri := NewResponseInspector()

	tests := []struct {
		contentType string
		expected    ContentType
	}{
		{"application/json; charset=utf-8", ContentTypeJSON},
		{"application/xml", ContentTypeXML},
		{"text/html", ContentTypeHTML},
		{"application/javascript", ContentTypeJavaScript},
		{"text/css", ContentTypeCSS},
		{"text/plain", ContentTypePlainText},
		{"application/octet-stream", ContentTypeUnknown},
		{"", ContentTypeUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.contentType, func(t *testing.T) {
			headers := map[string]string{}
			if tt.contentType != "" {
				headers["Content-Type"] = tt.contentType
			}
			resp := newTestResponse(200, headers, "body")
			result, err := ri.Inspect(resp)
			if err != nil {
				t.Fatalf("Inspect: %v", err)
			}
			if result.ContentType != tt.expected {
				t.Errorf("ContentType = %d, want %d for %q", result.ContentType, tt.expected, tt.contentType)
			}
		})
	}
}

func TestResponseInspector_EnableStatusCode(t *testing.T) {
	ri := NewResponseInspector()
	ri.DisableStatusCode(200)
	ri.EnableStatusCode(200)

	resp := newTestResponse(200, map[string]string{"Content-Type": "text/html"},
		`goroutine 1 [running]:`)

	result, err := ri.Inspect(resp)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if !result.Detected {
		t.Error("should detect after re-enabling status code")
	}
}

func TestRedactSensitive(t *testing.T) {
	tests := []struct {
		input, expected string
	}{
		{"ab", "****"},
		{"abcdef", "ab****ef"},
		{"a", "****"},
	}
	for _, tt := range tests {
		got := redactSensitive(tt.input)
		if got != tt.expected {
			t.Errorf("redactSensitive(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestRedactContext(t *testing.T) {
	context := "password: mysecretpassword here"
	sensitive := "mysecretpassword"
	result := redactContext(context, sensitive)
	if result == context {
		t.Error("expected context to be redacted")
	}
}

func TestHelperFunctions(t *testing.T) {
	// indexOfByte
	if indexOfByte("hello", 'l') != 2 {
		t.Error("indexOfByte wrong")
	}
	if indexOfByte("hello", 'z') != -1 {
		t.Error("indexOfByte should return -1 for missing byte")
	}

	// toLowerASCII
	if toLowerASCII("HELLO") != "hello" {
		t.Error("toLowerASCII wrong")
	}

	// trimSpace
	if trimSpace("  hello  ") != "hello" {
		t.Error("trimSpace wrong")
	}
	if trimSpace("hello") != "hello" {
		t.Error("trimSpace should not trim when no spaces")
	}

	// containsString
	if !containsString("hello world", "world") {
		t.Error("containsString should find substring")
	}
	if containsString("hello", "world") {
		t.Error("containsString should not find missing substring")
	}
}

func TestNewResponseStats(t *testing.T) {
	stats := NewResponseStats()
	if stats.TotalInspections != 0 {
		t.Error("expected 0 initial inspections")
	}
	if len(stats.ByStatusCode) != 0 {
		t.Error("expected empty status code map")
	}
}
