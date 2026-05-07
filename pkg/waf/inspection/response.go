// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package inspection provides response inspection for the WAF
package inspection

import (
	"bytes"
	"io"
	"net/http"
	"regexp"
	"sync"
)

// ResponseInspector inspects HTTP responses for security issues
type ResponseInspector struct {
	// Configuration
	maxResponseSize    int64
	enabledStatusCodes map[int]bool
	inspectErrors      bool // Inspect 4xx/5xx responses for info disclosure

	// Detection patterns
	infoDisclosurePatterns []*Pattern
	errorMessagePatterns   []*Pattern
	sensitiveDataPatterns  []*Pattern
	headerPatterns         []*Pattern

	// Statistics
	stats *ResponseStats

	mu sync.RWMutex
}

// ResponseStats holds response inspection statistics
type ResponseStats struct {
	TotalInspections    int64
	DetectionsFound     int64
	ByStatusCode        map[int]int64
	ByCategory          map[string]int64
	AverageResponseSize float64
	totalResponseSize   int64
	InfoDisclosures     int64
	SensitiveDataLeaks  int64
	mu                  sync.RWMutex
}

// ResponseInspectionResult represents the result of response inspection
type ResponseInspectionResult struct {
	StatusCode          int
	ContentType         ContentType
	ResponseSize        int64
	Detected            bool
	Findings            []ResponseFinding
	TotalScore          int
	HeaderFindings      []ResponseFinding
	SensitiveDataFound  bool
	InfoDisclosureFound bool
	Truncated           bool
}

// ResponseFinding represents a finding in the response
type ResponseFinding struct {
	Pattern     string
	Match       string
	Location    string
	Score       int
	Category    string
	Description string
	Context     string
	IsSensitive bool
	Redacted    string // Redacted version of the match
}

// NewResponseInspector creates a new response inspector
func NewResponseInspector() *ResponseInspector {
	inspector := &ResponseInspector{
		maxResponseSize:    5 * 1024 * 1024, // 5MB default
		enabledStatusCodes: make(map[int]bool),
		inspectErrors:      true,
		stats:              NewResponseStats(),
	}

	// Enable inspection for all status codes by default
	for i := 200; i < 600; i++ {
		inspector.enabledStatusCodes[i] = true
	}

	// Load default patterns
	inspector.loadDefaultPatterns()

	return inspector
}

// NewResponseStats creates new response statistics
func NewResponseStats() *ResponseStats {
	return &ResponseStats{
		ByStatusCode: make(map[int]int64),
		ByCategory:   make(map[string]int64),
	}
}

// loadDefaultPatterns loads default detection patterns
func (ri *ResponseInspector) loadDefaultPatterns() {
	// Information disclosure patterns
	ri.infoDisclosurePatterns = []*Pattern{
		// Stack traces
		{Name: "stack_trace_java", Regex: regexp.MustCompile(`at [a-zA-Z0-9_$]+\.[a-zA-Z0-9_$]+\([^)]+\.java:\d+\)`), Score: 15, Category: "info_disclosure", Description: "Java stack trace"},
		{Name: "stack_trace_python", Regex: regexp.MustCompile(`File "[^"]+", line \d+, in [a-zA-Z0-9_]+`), Score: 15, Category: "info_disclosure", Description: "Python stack trace"},
		{Name: "stack_trace_php", Regex: regexp.MustCompile(`#\d+ [^\s]+ called at \[[^\]]+:\d+\]`), Score: 15, Category: "info_disclosure", Description: "PHP stack trace"},
		{Name: "stack_trace_go", Regex: regexp.MustCompile(`goroutine \d+ \[running\]:`), Score: 15, Category: "info_disclosure", Description: "Go stack trace"},
		{Name: "stack_trace_node", Regex: regexp.MustCompile(`at [^\s]+ \([^)]+\.js:\d+:\d+\)`), Score: 15, Category: "info_disclosure", Description: "Node.js stack trace"},
		{Name: "stack_trace_csharp", Regex: regexp.MustCompile(`at [a-zA-Z0-9_\.]+\([^)]*\) in [^:]+:\d+`), Score: 15, Category: "info_disclosure", Description: "C# stack trace"},

		// Version information
		{Name: "version_apache", Regex: regexp.MustCompile(`Apache/\d+\.\d+(\.\d+)?`), Score: 5, Category: "info_disclosure", Description: "Apache version disclosed"},
		{Name: "version_nginx", Regex: regexp.MustCompile(`nginx/\d+\.\d+(\.\d+)?`), Score: 5, Category: "info_disclosure", Description: "Nginx version disclosed"},
		{Name: "version_php", Regex: regexp.MustCompile(`PHP/\d+\.\d+(\.\d+)?`), Score: 5, Category: "info_disclosure", Description: "PHP version disclosed"},
		{Name: "version_aspnet", Regex: regexp.MustCompile(`ASP\.NET\s+Version:\s*\d+\.\d+`), Score: 5, Category: "info_disclosure", Description: "ASP.NET version disclosed"},

		// Path disclosure
		{Name: "path_windows", Regex: regexp.MustCompile(`[A-Z]:\\[A-Za-z0-9_\\]+`), Score: 10, Category: "info_disclosure", Description: "Windows path disclosed"},
		{Name: "path_unix", Regex: regexp.MustCompile(`/(?:home|var|www|opt|usr)/[^\s<>]+`), Score: 10, Category: "info_disclosure", Description: "Unix path disclosed"},

		// Debug information
		{Name: "debug_sql", Regex: regexp.MustCompile(`(?i)SQL\s*(error|exception|syntax)`), Score: 20, Category: "info_disclosure", Description: "SQL error message"},
		{Name: "debug_db_error", Regex: regexp.MustCompile(`(?i)(mysql|postgresql|oracle|mssql|sqlite)\s*error`), Score: 20, Category: "info_disclosure", Description: "Database error message"},
		{Name: "debug_exception", Regex: regexp.MustCompile(`(?i)(exception|error|failure)\s*:\s*[^\n]+`), Score: 10, Category: "info_disclosure", Description: "Exception message"},
	}

	// Error message patterns that might reveal too much
	ri.errorMessagePatterns = []*Pattern{
		{Name: "error_sql_syntax", Regex: regexp.MustCompile(`(?i)syntax error.{0,50}(near|at|line)`), Score: 20, Category: "error_disclosure", Description: "SQL syntax error"},
		{Name: "error_file_not_found", Regex: regexp.MustCompile(`(?i)(file|resource)\s+(not\s+found|does\s+not\s+exist):\s*[^\s<>]+`), Score: 10, Category: "error_disclosure", Description: "File path in error"},
		{Name: "error_permission", Regex: regexp.MustCompile(`(?i)(permission|access)\s+denied:\s*[^\s<>]+`), Score: 10, Category: "error_disclosure", Description: "Permission error with path"},
		{Name: "error_connection", Regex: regexp.MustCompile(`(?i)connection\s+(refused|failed|timeout)\s+to\s+[^\s<>]+`), Score: 10, Category: "error_disclosure", Description: "Connection error with target"},
	}

	// Sensitive data patterns
	ri.sensitiveDataPatterns = []*Pattern{
		// Credentials
		{Name: "sensitive_password", Regex: regexp.MustCompile(`(?i)password\s*[=:]\s*[^\s<>"']{8,}`), Score: 30, Category: "sensitive_data", Description: "Password in response"},
		{Name: "sensitive_api_key", Regex: regexp.MustCompile(`(?i)(api[_-]?key|apikey)\s*[=:]\s*[a-zA-Z0-9_-]{20,}`), Score: 30, Category: "sensitive_data", Description: "API key in response"},
		{Name: "sensitive_secret", Regex: regexp.MustCompile(`(?i)(secret|token)\s*[=:]\s*[a-zA-Z0-9_-]{20,}`), Score: 30, Category: "sensitive_data", Description: "Secret/token in response"},
		{Name: "sensitive_private_key", Regex: regexp.MustCompile(`-----BEGIN\s+(RSA\s+)?PRIVATE\s+KEY-----`), Score: 50, Category: "sensitive_data", Description: "Private key in response"},
		{Name: "sensitive_aws_key", Regex: regexp.MustCompile(`AKIA[0-9A-Z]{16}`), Score: 50, Category: "sensitive_data", Description: "AWS access key in response"},

		// PII
		{Name: "sensitive_ssn", Regex: regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`), Score: 40, Category: "pii", Description: "SSN pattern in response"},
		{Name: "sensitive_credit_card", Regex: regexp.MustCompile(`\b(?:4[0-9]{12}(?:[0-9]{3})?|5[1-5][0-9]{14}|3[47][0-9]{13}|6(?:011|5[0-9]{2})[0-9]{12})\b`), Score: 50, Category: "pii", Description: "Credit card number in response"},
		{Name: "sensitive_email_list", Regex: regexp.MustCompile(`(?:[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}\s*,?\s*){5,}`), Score: 20, Category: "pii", Description: "Multiple email addresses in response"},
	}

	// Header patterns
	ri.headerPatterns = []*Pattern{
		{Name: "header_server_version", Regex: regexp.MustCompile(`(Apache|nginx|IIS|Tomcat)/\d+`), Score: 5, Category: "header_disclosure", Description: "Server version in header"},
		{Name: "header_powered_by", Regex: regexp.MustCompile(`X-Powered-By:\s*.+`), Score: 5, Category: "header_disclosure", Description: "X-Powered-By header"},
		{Name: "header_aspnet_version", Regex: regexp.MustCompile(`X-AspNet-Version:\s*.+`), Score: 5, Category: "header_disclosure", Description: "ASP.NET version header"},
	}
}

// Inspect inspects an HTTP response
func (ri *ResponseInspector) Inspect(resp *http.Response) (*ResponseInspectionResult, error) {
	ri.mu.RLock()
	defer ri.mu.RUnlock()

	ri.stats.mu.Lock()
	ri.stats.TotalInspections++
	ri.stats.ByStatusCode[resp.StatusCode]++
	ri.stats.mu.Unlock()

	result := &ResponseInspectionResult{
		StatusCode:     resp.StatusCode,
		Findings:       make([]ResponseFinding, 0),
		HeaderFindings: make([]ResponseFinding, 0),
	}

	// Check if status code is enabled for inspection
	if !ri.enabledStatusCodes[resp.StatusCode] {
		return result, nil
	}

	// Determine content type
	result.ContentType = ri.detectContentType(resp)

	// Inspect headers first
	ri.inspectHeaders(resp.Header, result)

	// Read response body
	body, err := ri.readResponse(resp)
	if err != nil {
		return result, err
	}

	result.ResponseSize = int64(len(body))
	result.Truncated = int64(len(body)) >= ri.maxResponseSize

	// Update stats
	ri.stats.mu.Lock()
	ri.stats.totalResponseSize += result.ResponseSize
	ri.stats.AverageResponseSize = float64(ri.stats.totalResponseSize) / float64(ri.stats.TotalInspections)
	ri.stats.mu.Unlock()

	// Inspect body for information disclosure
	ri.inspectForInfoDisclosure(body, result)

	// Inspect for sensitive data
	ri.inspectForSensitiveData(body, result)

	// Inspect error responses more thoroughly
	if ri.inspectErrors && resp.StatusCode >= 400 {
		ri.inspectErrorResponse(body, result)
	}

	result.Detected = len(result.Findings) > 0 || len(result.HeaderFindings) > 0

	// Update detection stats
	if result.Detected {
		ri.stats.mu.Lock()
		ri.stats.DetectionsFound++
		if result.InfoDisclosureFound {
			ri.stats.InfoDisclosures++
		}
		if result.SensitiveDataFound {
			ri.stats.SensitiveDataLeaks++
		}
		for _, f := range result.Findings {
			ri.stats.ByCategory[f.Category]++
		}
		ri.stats.mu.Unlock()
	}

	return result, nil
}

// detectContentType detects content type from response headers
func (ri *ResponseInspector) detectContentType(resp *http.Response) ContentType {
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		return ContentTypeUnknown
	}

	// Extract the main type (ignore parameters like charset)
	if idx := indexOfByte(contentType, ';'); idx != -1 {
		contentType = contentType[:idx]
	}
	contentType = toLowerASCII(trimSpace(contentType))

	switch {
	case containsString(contentType, "json"):
		return ContentTypeJSON
	case containsString(contentType, "xml"):
		return ContentTypeXML
	case containsString(contentType, "html"):
		return ContentTypeHTML
	case containsString(contentType, "javascript"):
		return ContentTypeJavaScript
	case containsString(contentType, "css"):
		return ContentTypeCSS
	case containsString(contentType, "text"):
		return ContentTypePlainText
	default:
		return ContentTypeUnknown
	}
}

// readResponse reads the response body up to max size
func (ri *ResponseInspector) readResponse(resp *http.Response) ([]byte, error) {
	if resp.Body == nil {
		return nil, nil
	}

	// Read up to max size
	limited := io.LimitReader(resp.Body, ri.maxResponseSize)
	body, err := readAllBytes(limited)
	if err != nil {
		return nil, err
	}

	// Restore body for further use
	resp.Body = nopCloser{bytes.NewReader(body)}

	return body, nil
}

// inspectHeaders inspects response headers
func (ri *ResponseInspector) inspectHeaders(headers http.Header, result *ResponseInspectionResult) {
	// Combine headers into string for pattern matching
	var headerBuf bytes.Buffer
	for name, values := range headers {
		for _, value := range values {
			headerBuf.WriteString(name)
			headerBuf.WriteString(": ")
			headerBuf.WriteString(value)
			headerBuf.WriteByte('\n')
		}
	}

	headerStr := headerBuf.Bytes()

	// Apply header patterns
	for _, pattern := range ri.headerPatterns {
		matches := pattern.Regex.FindAll(headerStr, -1)
		for _, match := range matches {
			finding := ResponseFinding{
				Pattern:     pattern.Name,
				Match:       string(match),
				Location:    "header",
				Score:       pattern.Score,
				Category:    pattern.Category,
				Description: pattern.Description,
			}
			result.HeaderFindings = append(result.HeaderFindings, finding)
			result.TotalScore += pattern.Score
			result.InfoDisclosureFound = true
		}
	}

	// Check for security headers that should be present
	ri.checkSecurityHeaders(headers, result)
}

// checkSecurityHeaders checks for missing security headers
func (ri *ResponseInspector) checkSecurityHeaders(headers http.Header, result *ResponseInspectionResult) {
	securityHeaders := map[string]string{
		"X-Content-Type-Options":    "nosniff expected",
		"X-Frame-Options":           "frame protection recommended",
		"Content-Security-Policy":   "CSP recommended",
		"Strict-Transport-Security": "HSTS recommended for HTTPS",
	}

	for header, description := range securityHeaders {
		if headers.Get(header) == "" {
			finding := ResponseFinding{
				Pattern:     "missing_" + toLowerASCII(header),
				Match:       "Header not present",
				Location:    "header",
				Score:       3,
				Category:    "security_header",
				Description: description,
			}
			result.HeaderFindings = append(result.HeaderFindings, finding)
			result.TotalScore += 3
		}
	}
}

// inspectForInfoDisclosure inspects response body for information disclosure
func (ri *ResponseInspector) inspectForInfoDisclosure(body []byte, result *ResponseInspectionResult) {
	for _, pattern := range ri.infoDisclosurePatterns {
		matches := pattern.Regex.FindAll(body, -1)
		for _, match := range matches {
			finding := ResponseFinding{
				Pattern:     pattern.Name,
				Match:       string(match),
				Location:    "body",
				Score:       pattern.Score,
				Category:    pattern.Category,
				Description: pattern.Description,
				Context:     getContext(body, match),
			}
			result.Findings = append(result.Findings, finding)
			result.TotalScore += pattern.Score
			result.InfoDisclosureFound = true
		}
	}
}

// inspectForSensitiveData inspects response body for sensitive data
func (ri *ResponseInspector) inspectForSensitiveData(body []byte, result *ResponseInspectionResult) {
	for _, pattern := range ri.sensitiveDataPatterns {
		matches := pattern.Regex.FindAll(body, -1)
		for _, match := range matches {
			finding := ResponseFinding{
				Pattern:     pattern.Name,
				Match:       string(match),
				Location:    "body",
				Score:       pattern.Score,
				Category:    pattern.Category,
				Description: pattern.Description,
				IsSensitive: true,
				Redacted:    redactSensitive(string(match)),
				Context:     redactContext(getContext(body, match), string(match)),
			}
			result.Findings = append(result.Findings, finding)
			result.TotalScore += pattern.Score
			result.SensitiveDataFound = true
		}
	}
}

// inspectErrorResponse inspects error responses more thoroughly
func (ri *ResponseInspector) inspectErrorResponse(body []byte, result *ResponseInspectionResult) {
	for _, pattern := range ri.errorMessagePatterns {
		matches := pattern.Regex.FindAll(body, -1)
		for _, match := range matches {
			finding := ResponseFinding{
				Pattern:     pattern.Name,
				Match:       string(match),
				Location:    "body",
				Score:       pattern.Score,
				Category:    pattern.Category,
				Description: pattern.Description,
				Context:     getContext(body, match),
			}
			result.Findings = append(result.Findings, finding)
			result.TotalScore += pattern.Score
			result.InfoDisclosureFound = true
		}
	}
}

// SetMaxResponseSize sets the maximum response size to inspect
func (ri *ResponseInspector) SetMaxResponseSize(size int64) {
	ri.mu.Lock()
	defer ri.mu.Unlock()
	ri.maxResponseSize = size
}

// EnableStatusCode enables inspection for a status code
func (ri *ResponseInspector) EnableStatusCode(code int) {
	ri.mu.Lock()
	defer ri.mu.Unlock()
	ri.enabledStatusCodes[code] = true
}

// DisableStatusCode disables inspection for a status code
func (ri *ResponseInspector) DisableStatusCode(code int) {
	ri.mu.Lock()
	defer ri.mu.Unlock()
	ri.enabledStatusCodes[code] = false
}

// SetInspectErrors enables/disables thorough error response inspection
func (ri *ResponseInspector) SetInspectErrors(inspect bool) {
	ri.mu.Lock()
	defer ri.mu.Unlock()
	ri.inspectErrors = inspect
}

// AddPattern adds a custom detection pattern
func (ri *ResponseInspector) AddPattern(name, pattern string, score int, category, description string) error {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return err
	}

	ri.mu.Lock()
	defer ri.mu.Unlock()

	p := &Pattern{
		Name:        name,
		Regex:       re,
		Score:       score,
		Category:    category,
		Description: description,
	}

	ri.infoDisclosurePatterns = append(ri.infoDisclosurePatterns, p)
	return nil
}

// GetStats returns response inspection statistics
func (ri *ResponseInspector) GetStats() map[string]interface{} {
	ri.stats.mu.RLock()
	defer ri.stats.mu.RUnlock()

	stats := make(map[string]interface{})
	stats["total_inspections"] = ri.stats.TotalInspections
	stats["detections_found"] = ri.stats.DetectionsFound
	stats["info_disclosures"] = ri.stats.InfoDisclosures
	stats["sensitive_data_leaks"] = ri.stats.SensitiveDataLeaks
	stats["average_response_size"] = ri.stats.AverageResponseSize

	byStatus := make(map[int]int64)
	for code, count := range ri.stats.ByStatusCode {
		byStatus[code] = count
	}
	stats["by_status_code"] = byStatus

	byCategory := make(map[string]int64)
	for cat, count := range ri.stats.ByCategory {
		byCategory[cat] = count
	}
	stats["by_category"] = byCategory

	return stats
}

// Helper functions

func indexOfByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}

func toLowerASCII(s string) string {
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch >= 'A' && ch <= 'Z' {
			result[i] = ch + 32
		} else {
			result[i] = ch
		}
	}
	return string(result)
}

func trimSpace(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}

func containsString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// redactSensitive replaces sensitive data with asterisks
func redactSensitive(s string) string {
	if len(s) <= 4 {
		return "****"
	}
	// Show first and last 2 characters
	return s[:2] + "****" + s[len(s)-2:]
}

// redactContext redacts sensitive data in context
func redactContext(context, sensitive string) string {
	redacted := redactSensitive(sensitive)
	result := context
	for i := 0; i <= len(result)-len(sensitive); i++ {
		if result[i:i+len(sensitive)] == sensitive {
			result = result[:i] + redacted + result[i+len(sensitive):]
			break
		}
	}
	return result
}
