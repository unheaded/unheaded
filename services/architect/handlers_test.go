package architect

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"unheaded/pkg/httputil"
)

// ============================================================================
// HEALTH ENDPOINT TESTS
// ============================================================================

func TestHTTPHandler_Health_Success(t *testing.T) {
	handler := NewHTTPHandler(nil)
	req, _ := http.NewRequest("GET", "/health", nil)
	rr := httptest.NewRecorder()

	handler.Health(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("status = %d, want %d", status, http.StatusOK)
	}

	var resp httputil.Response
	json.NewDecoder(rr.Body).Decode(&resp)
	if healthy, ok := resp.Data.(map[string]interface{})["healthy"]; !ok || !healthy.(bool) {
		t.Errorf("healthy field missing or false")
	}
}

func TestHTTPHandler_Health_WrongMethod(t *testing.T) {
	handler := NewHTTPHandler(nil)
	req, _ := http.NewRequest("POST", "/health", nil)
	rr := httptest.NewRecorder()

	handler.Health(rr, req)

	if status := rr.Code; status != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", status, http.StatusMethodNotAllowed)
	}
}

// ============================================================================
// INFRASTRUCTURE ENDPOINTS TESTS
// ============================================================================

func TestHTTPHandler_GetInfrastructure_Success(t *testing.T) {
	service := createTestService(t)
	ctx := context.Background()
	svc := createValidService()
	service.AddService(ctx, svc)

	handler := NewHTTPHandler(service)
	req, _ := http.NewRequest("GET", "/infrastructure", nil)
	rr := httptest.NewRecorder()

	handler.GetInfrastructure(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("status = %d, want %d", status, http.StatusOK)
	}

	var resp httputil.Response
	json.NewDecoder(rr.Body).Decode(&resp)
	data := resp.Data.(map[string]interface{})
	if services, ok := data["services"]; !ok || services == nil {
		t.Errorf("services field missing")
	}
}

func TestHTTPHandler_GetInfrastructure_WrongMethod(t *testing.T) {
	handler := NewHTTPHandler(nil)
	req, _ := http.NewRequest("POST", "/infrastructure", nil)
	rr := httptest.NewRecorder()

	handler.GetInfrastructure(rr, req)

	if status := rr.Code; status != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", status, http.StatusMethodNotAllowed)
	}
}

func TestHTTPHandler_AddService_Success(t *testing.T) {
	handler := NewHTTPHandler(nil)

	svc := createValidService()
	body, _ := json.Marshal(svc)

	req, _ := http.NewRequest("POST", "/infrastructure/services", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.AddService(rr, req)

	if status := rr.Code; status != http.StatusCreated {
		t.Errorf("status = %d, want %d", status, http.StatusCreated)
	}

	var resp httputil.Response
	json.NewDecoder(rr.Body).Decode(&resp)
	data := resp.Data.(map[string]interface{})
	if svcID, ok := data["service_id"]; !ok || svcID != svc.ServiceID {
		t.Errorf("service_id mismatch")
	}
}

func TestHTTPHandler_AddService_InvalidJSON(t *testing.T) {
	handler := NewHTTPHandler(nil)

	body := bytes.NewReader([]byte("{invalid json}"))
	req, _ := http.NewRequest("POST", "/infrastructure/services", body)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.AddService(rr, req)

	if status := rr.Code; status != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", status, http.StatusBadRequest)
	}
}

func TestHTTPHandler_AddService_InvalidService(t *testing.T) {
	handler := NewHTTPHandler(nil)

	svc := createValidService()
	svc.ServiceID = "" // invalid
	body, _ := json.Marshal(svc)

	req, _ := http.NewRequest("POST", "/infrastructure/services", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.AddService(rr, req)

	if status := rr.Code; status != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", status, http.StatusBadRequest)
	}
}

func TestHTTPHandler_AddService_WrongMethod(t *testing.T) {
	handler := NewHTTPHandler(nil)

	req, _ := http.NewRequest("GET", "/infrastructure/services", nil)
	rr := httptest.NewRecorder()

	handler.AddService(rr, req)

	if status := rr.Code; status != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", status, http.StatusMethodNotAllowed)
	}
}

// ============================================================================
// NETWORK ENDPOINTS TESTS
// ============================================================================

func TestHTTPHandler_GetNetwork_Success(t *testing.T) {
	service := createTestService(t)
	ctx := context.Background()
	node := createValidNetworkNode()
	service.AddNetworkNode(ctx, node)

	handler := NewHTTPHandler(service)
	req, _ := http.NewRequest("GET", "/network", nil)
	rr := httptest.NewRecorder()

	handler.GetNetwork(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("status = %d, want %d", status, http.StatusOK)
	}

	var resp httputil.Response
	json.NewDecoder(rr.Body).Decode(&resp)
	data := resp.Data.(map[string]interface{})
	if nodes, ok := data["nodes"]; !ok || nodes == nil {
		t.Errorf("nodes field missing")
	}
}

func TestHTTPHandler_GetNetwork_WrongMethod(t *testing.T) {
	handler := NewHTTPHandler(nil)
	req, _ := http.NewRequest("DELETE", "/network", nil)
	rr := httptest.NewRecorder()

	handler.GetNetwork(rr, req)

	if status := rr.Code; status != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", status, http.StatusMethodNotAllowed)
	}
}

func TestHTTPHandler_AddNetworkNode_Success(t *testing.T) {
	handler := NewHTTPHandler(nil)

	node := createValidNetworkNode()
	body, _ := json.Marshal(node)

	req, _ := http.NewRequest("POST", "/network/nodes", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.AddNetworkNode(rr, req)

	if status := rr.Code; status != http.StatusCreated {
		t.Errorf("status = %d, want %d", status, http.StatusCreated)
	}

	var resp httputil.Response
	json.NewDecoder(rr.Body).Decode(&resp)
	data := resp.Data.(map[string]interface{})
	if nodeID, ok := data["node_id"]; !ok || nodeID != node.NodeID {
		t.Errorf("node_id mismatch")
	}
}

func TestHTTPHandler_AddNetworkNode_InvalidNode(t *testing.T) {
	handler := NewHTTPHandler(nil)

	node := createValidNetworkNode()
	node.NodeID = "" // invalid
	body, _ := json.Marshal(node)

	req, _ := http.NewRequest("POST", "/network/nodes", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.AddNetworkNode(rr, req)

	if status := rr.Code; status != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", status, http.StatusBadRequest)
	}
}

// ============================================================================
// DESIGN ENDPOINTS TESTS
// ============================================================================

func TestHTTPHandler_LogDesign_Success(t *testing.T) {
	handler := NewHTTPHandler(nil)

	decision := createValidDecision()
	body, _ := json.Marshal(decision)

	req, _ := http.NewRequest("POST", "/design", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.LogDesign(rr, req)

	if status := rr.Code; status != http.StatusCreated {
		t.Errorf("status = %d, want %d", status, http.StatusCreated)
	}

	var resp httputil.Response
	json.NewDecoder(rr.Body).Decode(&resp)
	data := resp.Data.(map[string]interface{})
	if title, ok := data["title"]; !ok || title != decision.Title {
		t.Errorf("title mismatch")
	}
}

func TestHTTPHandler_LogDesign_InvalidDecision(t *testing.T) {
	handler := NewHTTPHandler(nil)

	decision := createValidDecision()
	decision.Title = "" // invalid
	body, _ := json.Marshal(decision)

	req, _ := http.NewRequest("POST", "/design", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.LogDesign(rr, req)

	if status := rr.Code; status != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", status, http.StatusBadRequest)
	}
}

func TestHTTPHandler_GetDesignDecisions_Success(t *testing.T) {
	service := createTestService(t)
	ctx := context.Background()
	decision := createValidDecision()
	service.LogDecision(ctx, decision)

	handler := NewHTTPHandler(service)
	req, _ := http.NewRequest("GET", "/design", nil)
	rr := httptest.NewRecorder()

	handler.GetDesignDecisions(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("status = %d, want %d", status, http.StatusOK)
	}

	var resp httputil.Response
	json.NewDecoder(rr.Body).Decode(&resp)
	decisions := resp.Data.([]interface{})
	if len(decisions) != 1 {
		t.Errorf("decisions count = %d, want 1", len(decisions))
	}
}

func TestHTTPHandler_GetDesignDecisions_WrongMethod(t *testing.T) {
	handler := NewHTTPHandler(nil)
	req, _ := http.NewRequest("POST", "/design", nil)
	rr := httptest.NewRecorder()

	handler.GetDesignDecisions(rr, req)

	if status := rr.Code; status != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", status, http.StatusMethodNotAllowed)
	}
}

// ============================================================================
// ERROR RESPONSE FORMAT TESTS
// ============================================================================

func TestHTTPHandler_ErrorResponse_Format(t *testing.T) {
	handler := NewHTTPHandler(nil)
	req, _ := http.NewRequest("INVALID", "/health", nil)
	rr := httptest.NewRecorder()

	handler.Health(rr, req)

	var resp httputil.Response
	json.NewDecoder(rr.Body).Decode(&resp)

	if resp.Error.Code == "" {
		t.Errorf("error code empty")
	}
	if resp.Error.Message == "" {
		t.Errorf("error message empty")
	}
}

func TestHTTPHandler_SuccessResponse_HasTimestamp(t *testing.T) {
	handler := NewHTTPHandler(nil)
	req, _ := http.NewRequest("GET", "/health", nil)
	rr := httptest.NewRecorder()

	handler.Health(rr, req)

	var resp httputil.Response
	json.NewDecoder(rr.Body).Decode(&resp)

	if resp.Meta == nil || resp.Meta.Timestamp.IsZero() {
		t.Errorf("timestamp not set")
	}
}

// ============================================================================
// HANDLER CREATION TESTS
// ============================================================================

func TestNewHTTPHandler_WithNilService(t *testing.T) {
	handler := NewHTTPHandler(nil)
	if handler == nil {
		t.Errorf("NewHTTPHandler(nil) = nil, want non-nil")
	}
	if handler.service == nil {
		t.Errorf("handler.service = nil, want non-nil")
	}
}

func TestNewHTTPHandler_WithService(t *testing.T) {
	service := createTestService(t)
	handler := NewHTTPHandler(service)
	if handler == nil {
		t.Errorf("NewHTTPHandler() = nil, want non-nil")
	}
	if handler.service != service {
		t.Errorf("handler.service != service")
	}
}
