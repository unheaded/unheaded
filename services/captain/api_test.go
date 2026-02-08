package captain

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"unheaded/pkg/httputil"
)

func createTestServer(t *testing.T) (*Service, *HTTPServer) {
	storage := NewMemoryStorage()
	busboy := newMockBusboy()

	svc, err := NewService(Config{
		Storage: storage,
		Busboy:  busboy,
	})
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	httpServer, err := NewHTTPServer(svc, "localhost:0")
	if err != nil {
		t.Fatalf("NewHTTPServer failed: %v", err)
	}

	return svc, httpServer
}

func TestHTTPServer_Health(t *testing.T) {
	svc, httpServer := createTestServer(t)
	defer svc.Close()

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	httpServer.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Errorf("failed to decode response: %v", err)
	}

	if resp["status"] != "healthy" {
		t.Errorf("expected status 'healthy', got '%s'", resp["status"])
	}
}

func TestHTTPServer_Ready(t *testing.T) {
	svc, httpServer := createTestServer(t)

	tests := []struct {
		name        string
		closed      bool
		wantStatus  int
	}{
		{
			name:       "ready",
			closed:     false,
			wantStatus: http.StatusOK,
		},
		{
			name:       "closed",
			closed:     true,
			wantStatus: http.StatusServiceUnavailable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.closed {
				svc.Close()
			}

			req := httptest.NewRequest("GET", "/ready", nil)
			w := httptest.NewRecorder()

			httpServer.server.Handler.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("expected status %d, got %d", tt.wantStatus, w.Code)
			}
		})
	}
}

func TestHTTPServer_Vision(t *testing.T) {
	svc, httpServer := createTestServer(t)
	defer svc.Close()

	req := httptest.NewRequest("GET", "/api/v1/vision", nil)
	w := httptest.NewRecorder()

	httpServer.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp httputil.Response
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Errorf("failed to decode response: %v", err)
	}

	if !resp.Success {
		t.Error("expected success=true")
	}
}

func TestHTTPServer_Strategy(t *testing.T) {
	svc, httpServer := createTestServer(t)
	defer svc.Close()

	req := httptest.NewRequest("GET", "/api/v1/strategy", nil)
	w := httptest.NewRecorder()

	httpServer.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp httputil.Response
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Errorf("failed to decode response: %v", err)
	}

	if !resp.Success {
		t.Error("expected success=true")
	}
}

func TestHTTPServer_Metrics(t *testing.T) {
	svc, httpServer := createTestServer(t)
	defer svc.Close()

	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()

	httpServer.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "captain_http_requests_total") {
		t.Error("metrics should contain captain_http_requests_total")
	}
}

func TestHTTPServer_CreateDecision(t *testing.T) {
	svc, httpServer := createTestServer(t)
	defer svc.Close()

	decision := &Decision{
		Title:    "Deploy to production",
		Content:  "All systems ready",
		Owner:    "captain",
		Priority: PriorityHigh,
	}

	body, _ := json.Marshal(decision)
	req := httptest.NewRequest("POST", "/api/v1/decisions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	httpServer.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected status %d, got %d", http.StatusCreated, w.Code)
	}

	var resp httputil.Response
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Errorf("failed to decode response: %v", err)
	}

	if !resp.Success {
		t.Error("expected success=true")
	}
}

func TestHTTPServer_CreateDecision_Invalid(t *testing.T) {
	svc, httpServer := createTestServer(t)
	defer svc.Close()

	tests := []struct {
		name       string
		decision   *Decision
		wantStatus int
	}{
		{
			name: "empty title",
			decision: &Decision{
				Title:    "",
				Content:  "Content",
				Owner:    "captain",
				Priority: PriorityHigh,
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "empty content",
			decision: &Decision{
				Title:    "Title",
				Content:  "",
				Owner:    "captain",
				Priority: PriorityHigh,
			},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.decision)
			req := httptest.NewRequest("POST", "/api/v1/decisions", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			httpServer.server.Handler.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("expected status %d, got %d", tt.wantStatus, w.Code)
			}
		})
	}
}

func TestHTTPServer_ListDecisions(t *testing.T) {
	svc, httpServer := createTestServer(t)
	defer svc.Close()

	// Create some decisions
	for i := 0; i < 3; i++ {
		decision := &Decision{
			Title:    "Decision",
			Content:  "Content",
			Owner:    "captain",
			Priority: PriorityHigh,
		}
		svc.LogDecision(context.Background(), decision)
	}

	req := httptest.NewRequest("GET", "/api/v1/decisions?limit=2&offset=0", nil)
	w := httptest.NewRecorder()

	httpServer.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp httputil.Response
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Errorf("failed to decode response: %v", err)
	}

	if !resp.Success {
		t.Error("expected success=true")
	}
}

func TestHTTPServer_GetDecision(t *testing.T) {
	svc, httpServer := createTestServer(t)
	defer svc.Close()

	// Create a decision
	decision := &Decision{
		Title:    "Test Decision",
		Content:  "Content",
		Owner:    "captain",
		Priority: PriorityHigh,
	}
	svc.LogDecision(context.Background(), decision)

	req := httptest.NewRequest("GET", "/api/v1/decisions/"+decision.ID, nil)
	w := httptest.NewRecorder()

	httpServer.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp httputil.Response
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Errorf("failed to decode response: %v", err)
	}

	if !resp.Success {
		t.Error("expected success=true")
	}
}

func TestHTTPServer_GetDecision_NotFound(t *testing.T) {
	svc, httpServer := createTestServer(t)
	defer svc.Close()

	req := httptest.NewRequest("GET", "/api/v1/decisions/nonexistent", nil)
	w := httptest.NewRecorder()

	httpServer.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestHTTPServer_UpdateDecision(t *testing.T) {
	svc, httpServer := createTestServer(t)
	defer svc.Close()

	// Create a decision
	decision := &Decision{
		Title:    "Test Decision",
		Content:  "Content",
		Owner:    "captain",
		Priority: PriorityHigh,
	}
	svc.LogDecision(context.Background(), decision)

	// Update status
	updateReq := map[string]string{"status": "approved"}
	body, _ := json.Marshal(updateReq)

	req := httptest.NewRequest("PATCH", "/api/v1/decisions/"+decision.ID, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	httpServer.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp httputil.Response
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Errorf("failed to decode response: %v", err)
	}

	if !resp.Success {
		t.Error("expected success=true")
	}
}

func TestHTTPServer_InvalidMethod(t *testing.T) {
	svc, httpServer := createTestServer(t)
	defer svc.Close()

	tests := []struct {
		name   string
		path   string
		method string
	}{
		{
			name:   "DELETE on health",
			path:   "/health",
			method: "DELETE",
		},
		{
			name:   "POST on vision",
			path:   "/api/v1/vision",
			method: "POST",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			w := httptest.NewRecorder()

			httpServer.server.Handler.ServeHTTP(w, req)

			if w.Code != http.StatusMethodNotAllowed {
				t.Errorf("expected status %d, got %d", http.StatusMethodNotAllowed, w.Code)
			}
		})
	}
}

func TestHTTPServer_ContentType(t *testing.T) {
	svc, httpServer := createTestServer(t)
	defer svc.Close()

	decision := &Decision{
		Title:    "Test",
		Content:  "Content",
		Owner:    "captain",
		Priority: PriorityHigh,
	}

	body, _ := json.Marshal(decision)
	req := httptest.NewRequest("POST", "/api/v1/decisions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "text/plain") // Wrong content type
	w := httptest.NewRecorder()

	httpServer.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnsupportedMediaType {
		t.Errorf("expected status %d, got %d", http.StatusUnsupportedMediaType, w.Code)
	}
}

func TestHTTPServer_LargePayload(t *testing.T) {
	svc, httpServer := createTestServer(t)
	defer svc.Close()

	// Create a very large decision
	decision := &Decision{
		Title:    "Test",
		Content:  string(make([]byte, 20*1024*1024)), // 20MB
		Owner:    "captain",
		Priority: PriorityHigh,
	}

	body, _ := json.Marshal(decision)
	req := httptest.NewRequest("POST", "/api/v1/decisions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	httpServer.server.Handler.ServeHTTP(w, req)

	// Should fail due to size limit
	if w.Code == http.StatusOK {
		t.Error("expected error for large payload")
	}
}

func TestHTTPServer_MalformedJSON(t *testing.T) {
	svc, httpServer := createTestServer(t)
	defer svc.Close()

	req := httptest.NewRequest("POST", "/api/v1/decisions", strings.NewReader("{invalid json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	httpServer.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}
