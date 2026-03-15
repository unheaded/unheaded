// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package inspection provides request and response inspection for the WAF
package inspection

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
)

// ContentType represents the content type of a request body
type ContentType int

const (
	ContentTypeUnknown ContentType = iota
	ContentTypeJSON
	ContentTypeXML
	ContentTypeFormURLEncoded
	ContentTypeMultipart
	ContentTypePlainText
	ContentTypeHTML
	ContentTypeJavaScript
	ContentTypeCSS
	ContentTypeBinary
)

// String returns the string representation of content type
func (ct ContentType) String() string {
	switch ct {
	case ContentTypeJSON:
		return "application/json"
	case ContentTypeXML:
		return "application/xml"
	case ContentTypeFormURLEncoded:
		return "application/x-www-form-urlencoded"
	case ContentTypeMultipart:
		return "multipart/form-data"
	case ContentTypePlainText:
		return "text/plain"
	case ContentTypeHTML:
		return "text/html"
	case ContentTypeJavaScript:
		return "application/javascript"
	case ContentTypeCSS:
		return "text/css"
	case ContentTypeBinary:
		return "application/octet-stream"
	default:
		return "unknown"
	}
}

// BodyInspector inspects HTTP request bodies
type BodyInspector struct {
	// Configuration
	maxBodySize     int64
	enabledTypes    map[ContentType]bool
	strictParsing   bool

	// Detection patterns
	jsonPatterns    []*Pattern
	xmlPatterns     []*Pattern
	formPatterns    []*Pattern
	genericPatterns []*Pattern

	// Statistics
	stats *InspectionStats

	mu sync.RWMutex
}

// Pattern represents a detection pattern
type Pattern struct {
	Name        string
	Regex       *regexp.Regexp
	Score       int
	Category    string
	Description string
}

// InspectionStats holds inspection statistics
type InspectionStats struct {
	TotalInspections  int64
	DetectionsFound   int64
	ByContentType     map[ContentType]int64
	ByCategory        map[string]int64
	AverageBodySize   float64
	totalBodySize     int64
	mu                sync.RWMutex
}

// InspectionResult represents the result of body inspection
type InspectionResult struct {
	ContentType    ContentType
	BodySize       int64
	Detected       bool
	Findings       []Finding
	TotalScore     int
	ParsedData     map[string]interface{}
	RawBody        []byte
	Truncated      bool
}

// Finding represents a single detection finding
type Finding struct {
	Pattern     string
	Match       string
	Location    string
	Score       int
	Category    string
	Description string
	Context     string // Surrounding context
}

// NewBodyInspector creates a new body inspector
func NewBodyInspector() *BodyInspector {
	inspector := &BodyInspector{
		maxBodySize:   1024 * 1024, // 1MB default
		enabledTypes:  make(map[ContentType]bool),
		strictParsing: false,
		stats:         NewInspectionStats(),
	}

	// Enable all types by default
	inspector.enabledTypes[ContentTypeJSON] = true
	inspector.enabledTypes[ContentTypeXML] = true
	inspector.enabledTypes[ContentTypeFormURLEncoded] = true
	inspector.enabledTypes[ContentTypeMultipart] = true
	inspector.enabledTypes[ContentTypePlainText] = true
	inspector.enabledTypes[ContentTypeHTML] = true

	// Load default patterns
	inspector.loadDefaultPatterns()

	return inspector
}

// NewInspectionStats creates new inspection statistics
func NewInspectionStats() *InspectionStats {
	return &InspectionStats{
		ByContentType: make(map[ContentType]int64),
		ByCategory:    make(map[string]int64),
	}
}

// loadDefaultPatterns loads default detection patterns
func (bi *BodyInspector) loadDefaultPatterns() {
	// SQL injection patterns for JSON
	bi.jsonPatterns = []*Pattern{
		{Name: "sqli_union", Regex: regexp.MustCompile(`(?i)union\s+(all\s+)?select`), Score: 20, Category: "sqli", Description: "SQL UNION injection"},
		{Name: "sqli_or", Regex: regexp.MustCompile(`(?i)'\s*(or|and)\s+['0-9]`), Score: 15, Category: "sqli", Description: "SQL boolean injection"},
		{Name: "sqli_comment", Regex: regexp.MustCompile(`--\s*$|\/\*.*\*\/`), Score: 10, Category: "sqli", Description: "SQL comment injection"},
		{Name: "sqli_sleep", Regex: regexp.MustCompile(`(?i)(sleep|benchmark|waitfor|delay)\s*\(`), Score: 20, Category: "sqli", Description: "SQL time-based injection"},
	}

	// XSS patterns for JSON/HTML
	bi.genericPatterns = []*Pattern{
		{Name: "xss_script", Regex: regexp.MustCompile(`(?i)<\s*script[^>]*>`), Score: 20, Category: "xss", Description: "Script tag injection"},
		{Name: "xss_event", Regex: regexp.MustCompile(`(?i)\bon\w+\s*=`), Score: 15, Category: "xss", Description: "Event handler injection"},
		{Name: "xss_javascript", Regex: regexp.MustCompile(`(?i)javascript\s*:`), Score: 15, Category: "xss", Description: "JavaScript protocol"},
		{Name: "xss_svg", Regex: regexp.MustCompile(`(?i)<\s*svg[^>]*onload`), Score: 20, Category: "xss", Description: "SVG XSS"},
		{Name: "xss_iframe", Regex: regexp.MustCompile(`(?i)<\s*iframe[^>]*>`), Score: 10, Category: "xss", Description: "Iframe injection"},
	}

	// XML patterns
	bi.xmlPatterns = []*Pattern{
		{Name: "xxe_entity", Regex: regexp.MustCompile(`(?i)<!ENTITY[^>]*>`), Score: 25, Category: "xxe", Description: "XML external entity"},
		{Name: "xxe_system", Regex: regexp.MustCompile(`(?i)SYSTEM\s+["'][^"']+["']`), Score: 25, Category: "xxe", Description: "XML SYSTEM directive"},
		{Name: "xxe_doctype", Regex: regexp.MustCompile(`(?i)<!DOCTYPE[^>]*\[`), Score: 15, Category: "xxe", Description: "DOCTYPE with internal subset"},
		{Name: "xxe_parameter", Regex: regexp.MustCompile(`(?i)%[a-zA-Z0-9_]+;`), Score: 15, Category: "xxe", Description: "Parameter entity reference"},
	}

	// Form data patterns
	bi.formPatterns = []*Pattern{
		{Name: "path_traversal", Regex: regexp.MustCompile(`\.\.\/|\.\.\\`), Score: 20, Category: "traversal", Description: "Path traversal"},
		{Name: "cmd_injection", Regex: regexp.MustCompile(`;\s*\w|\|\s*\w|&&\s*\w`), Score: 20, Category: "rce", Description: "Command chaining"},
		{Name: "cmd_subst", Regex: regexp.MustCompile(`\$\([^)]+\)|` + "`[^`]+`"), Score: 20, Category: "rce", Description: "Command substitution"},
		{Name: "ssrf_localhost", Regex: regexp.MustCompile(`(?i)localhost|127\.0\.0\.1|0\.0\.0\.0`), Score: 15, Category: "ssrf", Description: "Localhost access"},
		{Name: "ssrf_metadata", Regex: regexp.MustCompile(`169\.254\.169\.254`), Score: 25, Category: "ssrf", Description: "Cloud metadata access"},
	}
}

// Inspect inspects a request body
func (bi *BodyInspector) Inspect(req *http.Request) (*InspectionResult, error) {
	bi.mu.RLock()
	defer bi.mu.RUnlock()

	bi.stats.mu.Lock()
	bi.stats.TotalInspections++
	bi.stats.mu.Unlock()

	result := &InspectionResult{
		Findings:   make([]Finding, 0),
		ParsedData: make(map[string]interface{}),
	}

	// Determine content type
	result.ContentType = bi.detectContentType(req)

	// Check if content type is enabled
	if !bi.enabledTypes[result.ContentType] {
		return result, nil
	}

	// Read body
	if req.Body == nil {
		return result, nil
	}

	body, err := bi.readBody(req)
	if err != nil {
		return result, err
	}

	result.RawBody = body
	result.BodySize = int64(len(body))
	result.Truncated = int64(len(body)) >= bi.maxBodySize

	// Update stats
	bi.stats.mu.Lock()
	bi.stats.totalBodySize += result.BodySize
	bi.stats.AverageBodySize = float64(bi.stats.totalBodySize) / float64(bi.stats.TotalInspections)
	bi.stats.ByContentType[result.ContentType]++
	bi.stats.mu.Unlock()

	// Inspect based on content type
	switch result.ContentType {
	case ContentTypeJSON:
		bi.inspectJSON(body, result)
	case ContentTypeXML:
		bi.inspectXML(body, result)
	case ContentTypeFormURLEncoded:
		bi.inspectFormURLEncoded(body, result)
	case ContentTypeMultipart:
		bi.inspectMultipart(req, body, result)
	case ContentTypeHTML:
		bi.inspectHTML(body, result)
	default:
		bi.inspectGeneric(body, result)
	}

	// Apply generic patterns to all content types
	bi.applyPatterns(body, bi.genericPatterns, "body", result)

	result.Detected = len(result.Findings) > 0

	// Update detection stats
	if result.Detected {
		bi.stats.mu.Lock()
		bi.stats.DetectionsFound++
		for _, f := range result.Findings {
			bi.stats.ByCategory[f.Category]++
		}
		bi.stats.mu.Unlock()
	}

	return result, nil
}

// detectContentType detects the content type from request headers
func (bi *BodyInspector) detectContentType(req *http.Request) ContentType {
	contentType := req.Header.Get("Content-Type")
	if contentType == "" {
		return ContentTypeUnknown
	}

	// Extract the main type (ignore parameters like charset)
	if idx := strings.IndexByte(contentType, ';'); idx != -1 {
		contentType = contentType[:idx]
	}
	contentType = strings.TrimSpace(strings.ToLower(contentType))

	switch {
	case strings.Contains(contentType, "json"):
		return ContentTypeJSON
	case strings.Contains(contentType, "xml"):
		return ContentTypeXML
	case contentType == "application/x-www-form-urlencoded":
		return ContentTypeFormURLEncoded
	case strings.Contains(contentType, "multipart"):
		return ContentTypeMultipart
	case strings.Contains(contentType, "html"):
		return ContentTypeHTML
	case strings.Contains(contentType, "javascript"):
		return ContentTypeJavaScript
	case strings.Contains(contentType, "css"):
		return ContentTypeCSS
	case strings.Contains(contentType, "text"):
		return ContentTypePlainText
	default:
		return ContentTypeUnknown
	}
}

// readBody reads the request body up to max size
func (bi *BodyInspector) readBody(req *http.Request) ([]byte, error) {
	if req.Body == nil {
		return nil, nil
	}

	// Read up to max size
	limited := io.LimitReader(req.Body, bi.maxBodySize)
	body, err := readAllBytes(limited)
	if err != nil {
		return nil, err
	}

	// Restore body for further use
	req.Body = nopCloser{bytes.NewReader(body)}

	return body, nil
}

// inspectJSON inspects JSON body content
func (bi *BodyInspector) inspectJSON(body []byte, result *InspectionResult) {
	// Parse JSON structure
	values := extractJSONValues(body)

	// Apply JSON-specific patterns
	bi.applyPatterns(body, bi.jsonPatterns, "json", result)

	// Check extracted values
	for key, value := range values {
		bi.applyPatterns([]byte(value), bi.jsonPatterns, "json."+key, result)
		bi.applyPatterns([]byte(value), bi.genericPatterns, "json."+key, result)
		result.ParsedData[key] = value
	}
}

// inspectXML inspects XML body content
func (bi *BodyInspector) inspectXML(body []byte, result *InspectionResult) {
	// Apply XML-specific patterns
	bi.applyPatterns(body, bi.xmlPatterns, "xml", result)

	// Extract text content from XML
	values := extractXMLValues(body)
	for key, value := range values {
		bi.applyPatterns([]byte(value), bi.genericPatterns, "xml."+key, result)
		result.ParsedData[key] = value
	}
}

// inspectFormURLEncoded inspects URL-encoded form data
func (bi *BodyInspector) inspectFormURLEncoded(body []byte, result *InspectionResult) {
	// Parse form values
	values, err := url.ParseQuery(string(body))
	if err != nil {
		// If parsing fails, inspect raw
		bi.applyPatterns(body, bi.formPatterns, "form", result)
		return
	}

	// Inspect each form field
	for key, vals := range values {
		for _, val := range vals {
			bi.applyPatterns([]byte(val), bi.formPatterns, "form."+key, result)
			bi.applyPatterns([]byte(val), bi.genericPatterns, "form."+key, result)
			result.ParsedData[key] = val
		}
	}
}

// inspectMultipart inspects multipart form data
func (bi *BodyInspector) inspectMultipart(req *http.Request, body []byte, result *InspectionResult) {
	// For multipart, we inspect the raw body with generic patterns
	// Full multipart parsing would require more complex handling
	bi.applyPatterns(body, bi.formPatterns, "multipart", result)
}

// inspectHTML inspects HTML body content
func (bi *BodyInspector) inspectHTML(body []byte, result *InspectionResult) {
	// HTML might contain various injection patterns
	bi.applyPatterns(body, bi.genericPatterns, "html", result)
}

// inspectGeneric inspects generic content
func (bi *BodyInspector) inspectGeneric(body []byte, result *InspectionResult) {
	bi.applyPatterns(body, bi.genericPatterns, "body", result)
}

// applyPatterns applies patterns to content and adds findings
func (bi *BodyInspector) applyPatterns(content []byte, patterns []*Pattern, location string, result *InspectionResult) {
	for _, pattern := range patterns {
		matches := pattern.Regex.FindAll(content, -1)
		for _, match := range matches {
			finding := Finding{
				Pattern:     pattern.Name,
				Match:       string(match),
				Location:    location,
				Score:       pattern.Score,
				Category:    pattern.Category,
				Description: pattern.Description,
				Context:     getContext(content, match),
			}
			result.Findings = append(result.Findings, finding)
			result.TotalScore += pattern.Score
		}
	}
}

// SetMaxBodySize sets the maximum body size to inspect
func (bi *BodyInspector) SetMaxBodySize(size int64) {
	bi.mu.Lock()
	defer bi.mu.Unlock()
	bi.maxBodySize = size
}

// EnableContentType enables inspection for a content type
func (bi *BodyInspector) EnableContentType(ct ContentType) {
	bi.mu.Lock()
	defer bi.mu.Unlock()
	bi.enabledTypes[ct] = true
}

// DisableContentType disables inspection for a content type
func (bi *BodyInspector) DisableContentType(ct ContentType) {
	bi.mu.Lock()
	defer bi.mu.Unlock()
	bi.enabledTypes[ct] = false
}

// AddPattern adds a custom detection pattern
func (bi *BodyInspector) AddPattern(name, pattern string, score int, category, description string) error {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return err
	}

	bi.mu.Lock()
	defer bi.mu.Unlock()

	p := &Pattern{
		Name:        name,
		Regex:       re,
		Score:       score,
		Category:    category,
		Description: description,
	}

	bi.genericPatterns = append(bi.genericPatterns, p)
	return nil
}

// GetStats returns inspection statistics
func (bi *BodyInspector) GetStats() map[string]interface{} {
	bi.stats.mu.RLock()
	defer bi.stats.mu.RUnlock()

	stats := make(map[string]interface{})
	stats["total_inspections"] = bi.stats.TotalInspections
	stats["detections_found"] = bi.stats.DetectionsFound
	stats["average_body_size"] = bi.stats.AverageBodySize

	byType := make(map[string]int64)
	for ct, count := range bi.stats.ByContentType {
		byType[ct.String()] = count
	}
	stats["by_content_type"] = byType

	byCategory := make(map[string]int64)
	for cat, count := range bi.stats.ByCategory {
		byCategory[cat] = count
	}
	stats["by_category"] = byCategory

	return stats
}

// Helper functions

// nopCloser wraps a Reader to implement ReadCloser with a no-op Close
type nopCloser struct {
	io.Reader
}

func (nopCloser) Close() error { return nil }

func readAllBytes(r io.Reader) ([]byte, error) {
	var buf bytes.Buffer
	_, err := buf.ReadFrom(r)
	return buf.Bytes(), err
}

// extractJSONValues extracts string values from JSON (simple implementation)
func extractJSONValues(body []byte) map[string]string {
	values := make(map[string]string)

	// Simple extraction of string values using regex
	// A production implementation would use proper JSON parsing
	re := regexp.MustCompile(`"([^"]+)"\s*:\s*"([^"]*)"`)
	matches := re.FindAllSubmatch(body, -1)
	for _, m := range matches {
		if len(m) >= 3 {
			values[string(m[1])] = string(m[2])
		}
	}

	return values
}

// extractXMLValues extracts text content from XML (simple implementation)
func extractXMLValues(body []byte) map[string]string {
	values := make(map[string]string)

	// Simple extraction of element content
	re := regexp.MustCompile(`<([a-zA-Z][a-zA-Z0-9_]*)[^>]*>([^<]*)</`)
	matches := re.FindAllSubmatch(body, -1)
	for _, m := range matches {
		if len(m) >= 3 {
			values[string(m[1])] = string(m[2])
		}
	}

	return values
}

// getContext extracts surrounding context for a match
func getContext(content, match []byte) string {
	idx := bytes.Index(content, match)
	if idx < 0 {
		return ""
	}

	start := idx - 50
	if start < 0 {
		start = 0
	}

	end := idx + len(match) + 50
	if end > len(content) {
		end = len(content)
	}

	return string(content[start:end])
}
