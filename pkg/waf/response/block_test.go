// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package response

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// NewBlockWriter
// ---------------------------------------------------------------------------

func TestNewBlockWriter(t *testing.T) {
	h := NewHandler()
	bw := NewBlockWriter(h)
	if bw == nil {
		t.Fatal("NewBlockWriter returned nil")
	}
	if bw.handler != h {
		t.Error("handler not set")
	}
}

// ---------------------------------------------------------------------------
// WriteBlock
// ---------------------------------------------------------------------------

func TestBlockWriter_WriteBlock(t *testing.T) {
	h := NewHandler()
	bw := NewBlockWriter(h)

	reason := &BlockReason{
		Type:    "sqli",
		RuleID:  "SQLI-001",
		Message: "SQL injection detected",
	}

	w := httptest.NewRecorder()
	bw.WriteBlock(w, reason, "req-123")

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
	body := w.Body.String()
	if !strings.Contains(body, "Access Denied") {
		t.Error("expected default block page content")
	}
}

// ---------------------------------------------------------------------------
// WriteJSONBlock
// ---------------------------------------------------------------------------

func TestBlockWriter_WriteJSONBlock(t *testing.T) {
	h := NewHandler()
	bw := NewBlockWriter(h)

	reason := &BlockReason{
		Type:    "xss",
		RuleID:  "XSS-001",
		Message: "XSS attempt detected",
		Score:   15,
		Details: "Found script tag",
	}

	w := httptest.NewRecorder()
	bw.WriteJSONBlock(w, reason, "req-456")

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q", ct)
	}

	var resp struct {
		Blocked   bool         `json:"blocked"`
		RequestID string       `json:"request_id"`
		Reason    *BlockReason `json:"reason"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.Blocked {
		t.Error("Blocked should be true")
	}
	if resp.RequestID != "req-456" {
		t.Errorf("RequestID = %q", resp.RequestID)
	}
	if resp.Reason == nil {
		t.Fatal("Reason is nil")
	}
	if resp.Reason.Type != "xss" {
		t.Errorf("Reason.Type = %q", resp.Reason.Type)
	}
	if resp.Reason.RuleID != "XSS-001" {
		t.Errorf("Reason.RuleID = %q", resp.Reason.RuleID)
	}
	if resp.Reason.Score != 15 {
		t.Errorf("Reason.Score = %d", resp.Reason.Score)
	}
}

// ---------------------------------------------------------------------------
// WriteIPBlock
// ---------------------------------------------------------------------------

func TestBlockWriter_WriteIPBlock(t *testing.T) {
	h := NewHandler()
	h.SetJSONResponse(true)
	bw := NewBlockWriter(h)

	w := httptest.NewRecorder()
	bw.WriteIPBlock(w, "192.168.1.100", "req-ip-1")

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d", w.Code)
	}
	var resp BlockResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.Blocked {
		t.Error("Blocked should be true")
	}
	if !strings.Contains(resp.Reason, "IP address") {
		t.Errorf("Reason = %q, should mention IP", resp.Reason)
	}
}

// ---------------------------------------------------------------------------
// WriteRateLimit
// ---------------------------------------------------------------------------

func TestBlockWriter_WriteRateLimit(t *testing.T) {
	h := NewHandler()
	bw := NewBlockWriter(h)

	w := httptest.NewRecorder()
	bw.WriteRateLimit(w, 60, "req-rate-1")

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("status = %d, want %d", w.Code, http.StatusTooManyRequests)
	}
	if w.Header().Get("X-RateLimit-Reset") == "" {
		t.Error("X-RateLimit-Reset header should be set")
	}

	var resp struct {
		Blocked    bool         `json:"blocked"`
		RequestID  string       `json:"request_id"`
		RetryAfter int          `json:"retry_after"`
		Reason     *BlockReason `json:"reason"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.Blocked {
		t.Error("Blocked should be true")
	}
	if resp.RetryAfter != 60 {
		t.Errorf("RetryAfter = %d", resp.RetryAfter)
	}
	if resp.Reason.Type != "rate_limit" {
		t.Errorf("Reason.Type = %q", resp.Reason.Type)
	}
}

// ---------------------------------------------------------------------------
// WriteGeoBlock
// ---------------------------------------------------------------------------

func TestBlockWriter_WriteGeoBlock(t *testing.T) {
	h := NewHandler()
	h.SetJSONResponse(true)
	bw := NewBlockWriter(h)

	w := httptest.NewRecorder()
	bw.WriteGeoBlock(w, "CN", "req-geo-1")

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d", w.Code)
	}
	var resp BlockResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.Blocked {
		t.Error("Blocked should be true")
	}
	if !strings.Contains(resp.Reason, "region") {
		t.Errorf("Reason = %q, should mention region", resp.Reason)
	}
}

// ---------------------------------------------------------------------------
// WriteSQLiBlock
// ---------------------------------------------------------------------------

func TestBlockWriter_WriteSQLiBlock(t *testing.T) {
	h := NewHandler()
	h.SetJSONResponse(true)
	bw := NewBlockWriter(h)

	w := httptest.NewRecorder()
	bw.WriteSQLiBlock(w, "SQLI-002", "req-sqli-1")

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d", w.Code)
	}
	var resp BlockResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.RuleID != "SQLI-002" {
		t.Errorf("RuleID = %q", resp.RuleID)
	}
	if !strings.Contains(resp.Reason, "SQL injection") {
		t.Errorf("Reason = %q", resp.Reason)
	}
}

// ---------------------------------------------------------------------------
// WriteXSSBlock
// ---------------------------------------------------------------------------

func TestBlockWriter_WriteXSSBlock(t *testing.T) {
	h := NewHandler()
	h.SetJSONResponse(true)
	bw := NewBlockWriter(h)

	w := httptest.NewRecorder()
	bw.WriteXSSBlock(w, "XSS-002", "req-xss-1")

	var resp BlockResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.RuleID != "XSS-002" {
		t.Errorf("RuleID = %q", resp.RuleID)
	}
	if !strings.Contains(resp.Reason, "Cross-site scripting") {
		t.Errorf("Reason = %q", resp.Reason)
	}
}

// ---------------------------------------------------------------------------
// WriteTraversalBlock
// ---------------------------------------------------------------------------

func TestBlockWriter_WriteTraversalBlock(t *testing.T) {
	h := NewHandler()
	h.SetJSONResponse(true)
	bw := NewBlockWriter(h)

	w := httptest.NewRecorder()
	bw.WriteTraversalBlock(w, "TRAV-001", "req-trav-1")

	var resp BlockResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !strings.Contains(resp.Reason, "Path traversal") {
		t.Errorf("Reason = %q", resp.Reason)
	}
}

// ---------------------------------------------------------------------------
// WriteRCEBlock
// ---------------------------------------------------------------------------

func TestBlockWriter_WriteRCEBlock(t *testing.T) {
	h := NewHandler()
	h.SetJSONResponse(true)
	bw := NewBlockWriter(h)

	w := httptest.NewRecorder()
	bw.WriteRCEBlock(w, "RCE-001", "req-rce-1")

	var resp BlockResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !strings.Contains(resp.Reason, "Remote code execution") {
		t.Errorf("Reason = %q", resp.Reason)
	}
}

// ---------------------------------------------------------------------------
// WriteSSRFBlock
// ---------------------------------------------------------------------------

func TestBlockWriter_WriteSSRFBlock(t *testing.T) {
	h := NewHandler()
	h.SetJSONResponse(true)
	bw := NewBlockWriter(h)

	w := httptest.NewRecorder()
	bw.WriteSSRFBlock(w, "SSRF-001", "req-ssrf-1")

	var resp BlockResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !strings.Contains(resp.Reason, "Server-side request forgery") {
		t.Errorf("Reason = %q", resp.Reason)
	}
}

// ---------------------------------------------------------------------------
// WriteCustomBlock
// ---------------------------------------------------------------------------

func TestBlockWriter_WriteCustomBlock(t *testing.T) {
	h := NewHandler()
	h.SetJSONResponse(true)
	bw := NewBlockWriter(h)

	w := httptest.NewRecorder()
	bw.WriteCustomBlock(w, "custom_type", "Custom block message", "CUSTOM-001", "req-custom-1")

	var resp BlockResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.RuleID != "CUSTOM-001" {
		t.Errorf("RuleID = %q", resp.RuleID)
	}
	if resp.Reason != "Custom block message" {
		t.Errorf("Reason = %q", resp.Reason)
	}
}

// ---------------------------------------------------------------------------
// BlockReason JSON marshaling
// ---------------------------------------------------------------------------

func TestBlockReason_JSONMarshal(t *testing.T) {
	reason := BlockReason{
		Type:    "test",
		RuleID:  "TEST-001",
		Score:   10,
		Message: "Test message",
		Details: "Test details",
	}

	data, err := json.Marshal(reason)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded BlockReason
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.Type != reason.Type {
		t.Errorf("Type = %q", decoded.Type)
	}
	if decoded.RuleID != reason.RuleID {
		t.Errorf("RuleID = %q", decoded.RuleID)
	}
	if decoded.Score != reason.Score {
		t.Errorf("Score = %d", decoded.Score)
	}
	if decoded.Message != reason.Message {
		t.Errorf("Message = %q", decoded.Message)
	}
	if decoded.Details != reason.Details {
		t.Errorf("Details = %q", decoded.Details)
	}
}
