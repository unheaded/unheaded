// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package keymgmt

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHandleKeysPost(t *testing.T) {
	svc := newTestService()

	body := `{"algo_id":2,"param_set":1,"service_id":500}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/keys", strings.NewReader(body))
	w := httptest.NewRecorder()
	svc.handleKeys(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", w.Code)
	}
	var rec KeyRecord
	if err := json.NewDecoder(w.Body).Decode(&rec); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if rec.AlgoID != 2 {
		t.Errorf("AlgoID = %d, want 2", rec.AlgoID)
	}
	if rec.ServiceID != 500 {
		t.Errorf("ServiceID = %d, want 500", rec.ServiceID)
	}
}

func TestHandleKeysPost_InvalidBody(t *testing.T) {
	svc := newTestService()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/keys", strings.NewReader(`{invalid`))
	w := httptest.NewRecorder()
	svc.handleKeys(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestHandleKeysPost_MethodNotAllowed(t *testing.T) {
	svc := newTestService()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/keys", nil)
	w := httptest.NewRecorder()
	svc.handleKeys(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

func TestHandleKeysPost_LimitExceeded(t *testing.T) {
	svc := newTestService()
	svc.config.MaxKeysPerService = 1

	// First key should succeed
	body := `{"algo_id":1,"param_set":1,"service_id":100}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/keys", strings.NewReader(body))
	w := httptest.NewRecorder()
	svc.handleKeys(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("first key: status = %d, want 201", w.Code)
	}

	// Second should fail
	req = httptest.NewRequest(http.MethodPost, "/api/v1/keys", strings.NewReader(body))
	w = httptest.NewRecorder()
	svc.handleKeys(w, req)
	if w.Code != http.StatusConflict {
		t.Errorf("second key: status = %d, want 409", w.Code)
	}
}

func TestHandleKeyByID_Get(t *testing.T) {
	svc := newTestService()
	rec, _ := svc.GenerateKey(0x02, 0x01, 100)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/keys/"+rec.ID, nil)
	w := httptest.NewRecorder()
	svc.handleKeyByID(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var got KeyRecord
	json.NewDecoder(w.Body).Decode(&got)
	if got.ID != rec.ID {
		t.Errorf("ID = %s, want %s", got.ID, rec.ID)
	}
}

func TestHandleKeyByID_GetNotFound(t *testing.T) {
	svc := newTestService()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/keys/nonexistent", nil)
	w := httptest.NewRecorder()
	svc.handleKeyByID(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestHandleKeyByID_Rotate(t *testing.T) {
	svc := newTestService()
	rec, _ := svc.GenerateKey(0x01, 0x01, 100)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/keys/"+rec.ID+"/rotate", nil)
	w := httptest.NewRecorder()
	svc.handleKeyByID(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var newRec KeyRecord
	json.NewDecoder(w.Body).Decode(&newRec)
	if newRec.Status != KeyActive {
		t.Errorf("new key status = %v, want Active", newRec.Status)
	}
}

func TestHandleKeyByID_Revoke(t *testing.T) {
	svc := newTestService()
	rec, _ := svc.GenerateKey(0x01, 0x01, 100)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/keys/"+rec.ID+"/revoke", nil)
	w := httptest.NewRecorder()
	svc.handleKeyByID(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204", w.Code)
	}

	got, _ := svc.GetKey(rec.ID)
	if got.Status != KeyRevoked {
		t.Errorf("status = %v, want Revoked", got.Status)
	}
}

func TestHandleKeyByID_UnknownAction(t *testing.T) {
	svc := newTestService()

	// POST without a valid action
	req := httptest.NewRequest(http.MethodPost, "/api/v1/keys/someid", nil)
	w := httptest.NewRecorder()
	svc.handleKeyByID(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestHandleKeyByID_MethodNotAllowed(t *testing.T) {
	svc := newTestService()

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/keys/someid", nil)
	w := httptest.NewRecorder()
	svc.handleKeyByID(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

func TestHealthEndpoint(t *testing.T) {
	svc := newTestService()
	mux := http.NewServeMux()
	svc.registerRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["status"] != "healthy" {
		t.Errorf("status = %v", resp["status"])
	}
}

func TestReadyEndpoint(t *testing.T) {
	svc := newTestService()
	mux := http.NewServeMux()
	svc.registerRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

func TestMetricsEndpoint(t *testing.T) {
	svc := newTestService()
	svc.GenerateKey(0x01, 0x01, 100)
	rec, _ := svc.GenerateKey(0x02, 0x01, 100)
	svc.RevokeKey(rec.ID)

	mux := http.NewServeMux()
	svc.registerRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "keymgmt_keys_total 2") {
		t.Errorf("missing keymgmt_keys_total, body = %s", body)
	}
	if !strings.Contains(body, "keymgmt_keys_active 1") {
		t.Errorf("missing keymgmt_keys_active, body = %s", body)
	}
	if !strings.Contains(body, "keymgmt_keys_revoked 1") {
		t.Errorf("missing keymgmt_keys_revoked, body = %s", body)
	}
}

func TestCheckExpirations(t *testing.T) {
	svc := newTestService()
	svc.config.DefaultTTL = 1 * time.Millisecond

	svc.GenerateKey(0x01, 0x01, 100)
	time.Sleep(5 * time.Millisecond)

	svc.checkExpirations()

	// All keys should be expired now
	for _, rec := range svc.keys {
		if rec.Status != KeyExpired {
			t.Errorf("key %s status = %v, want Expired", rec.ID, rec.Status)
		}
	}
}

func TestStop_NoServer(t *testing.T) {
	svc := newTestService()
	// Stop without Start — server is nil
	if err := svc.Stop(); err != nil {
		t.Errorf("Stop: %v", err)
	}
}

func TestWotanDrops(t *testing.T) {
	svc := newTestService() // nil wotan → drops
	svc.GenerateKey(0x01, 0x01, 100)

	if drops := svc.WotanDrops(); drops != 1 {
		t.Errorf("WotanDrops = %d, want 1 (generate publishes event)", drops)
	}

	rec, _ := svc.GenerateKey(0x01, 0x01, 200)
	svc.RotateKey(rec.ID)

	if drops := svc.WotanDrops(); drops != 3 {
		t.Errorf("WotanDrops = %d, want 3 (2 generates + 1 rotate)", drops)
	}
}

func TestNewService_NilConfig(t *testing.T) {
	svc := NewService(newTestService().log, nil, nil)
	if svc.config.GracePeriod != 60*time.Second {
		t.Errorf("nil config should use defaults, got grace = %v", svc.config.GracePeriod)
	}
}

func TestGetKey_NotFound(t *testing.T) {
	svc := newTestService()
	_, ok := svc.GetKey("nonexistent")
	if ok {
		t.Error("expected not found")
	}
}

func TestGetKeyByRef_NotFound(t *testing.T) {
	svc := newTestService()
	_, ok := svc.GetKeyByRef(9999)
	if ok {
		t.Error("expected not found")
	}
}

func TestRotateKey_NotFound(t *testing.T) {
	svc := newTestService()
	_, err := svc.RotateKey("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent key")
	}
}

func TestRevokeKey_CancelsGraceTimer(t *testing.T) {
	svc := newTestServiceWithGrace(1 * time.Second)

	rec, _ := svc.GenerateKey(0x01, 0x01, 100)
	svc.RotateKey(rec.ID)

	// Revoke during grace period should cancel timer
	if err := svc.RevokeKey(rec.ID); err != nil {
		t.Fatalf("RevokeKey: %v", err)
	}

	got, _ := svc.GetKey(rec.ID)
	if got.Status != KeyRevoked {
		t.Errorf("status = %v, want Revoked", got.Status)
	}
}

func TestListKeys_Empty(t *testing.T) {
	svc := newTestService()
	list := svc.ListKeys(999)
	if list != nil {
		t.Errorf("expected nil for unknown service, got %v", list)
	}
}

func TestStats_WithMixedStatus(t *testing.T) {
	svc := newTestService()
	svc.GenerateKey(0x01, 0x01, 100)
	rec, _ := svc.GenerateKey(0x02, 0x01, 100)
	svc.RevokeKey(rec.ID)

	stats := svc.Stats()
	byStatus := stats["by_status"].(map[string]int)
	if byStatus["active"] != 1 {
		t.Errorf("active = %d, want 1", byStatus["active"])
	}
	if byStatus["revoked"] != 1 {
		t.Errorf("revoked = %d, want 1", byStatus["revoked"])
	}
	byAlgo := stats["by_algorithm"].(map[uint8]int)
	if byAlgo[0x01] != 1 {
		t.Errorf("algo 0x01 = %d, want 1", byAlgo[0x01])
	}
	if byAlgo[0x02] != 1 {
		t.Errorf("algo 0x02 = %d, want 1", byAlgo[0x02])
	}
}
