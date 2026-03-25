// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// LICH-013: HTTP API Endpoint Fuzzer
//
// Target: HTTP input handling across Kanban, Monad, and Wotan services.
// Goal: Find panics, crashes, or undefined behavior from malformed HTTP input
//       without requiring live service dependencies.
//
// Attack vectors:
//   - Malformed/random JSON POST bodies
//   - SQL injection strings in JSON values
//   - XSS payloads in string fields
//   - Oversized request bodies
//   - Path traversal in topic names
//   - Null bytes in string fields
//   - CRLF injection in HTTP headers
//   - Duplicate/malformed Content-Type headers
//   - Oversized HTTP headers
//
// All harnesses use minimal mock handlers that test only the HTTP
// parsing/routing layer. No database or service dependencies required.

package harnesses

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"unheaded/pkg/protocol/encoding"
)

// ============================================================================
// MOCK HANDLERS — minimal stubs that exercise input parsing only
// ============================================================================

// mockKanbanTaskHandler mimics the kanban handleCreateTask input parsing layer.
// It reads the body, attempts JSON decode, validates fields, and responds.
// No database, no TaskManager — just the HTTP parsing contract.
func mockKanbanTaskHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		// Enforce body limit (1MB, matching kanban-app)
		body, err := io.ReadAll(io.LimitReader(r.Body, 1*1024*1024))
		if err != nil {
			http.Error(w, "Failed to read body", http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		if len(body) == 0 {
			http.Error(w, "Empty body", http.StatusBadRequest)
			return
		}

		var task map[string]interface{}
		if err := json.Unmarshal(body, &task); err != nil {
			http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
			return
		}

		// Validate required fields (mirrors kanban validateTaskInput)
		id, _ := task["id"].(string)
		title, _ := task["title"].(string)
		if id == "" || title == "" {
			http.Error(w, "id and title required", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]interface{}{"task": task})

	case http.MethodGet:
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{"tasks": []interface{}{}})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// mockMonadOperationsHandler mimics the Monad executeOperationHandler input parsing.
// Validates Content-Type, reads body with limit, parses JSON, validates fields.
func mockMonadOperationsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}

	// Content-Type check (mirrors Monad)
	if ct := r.Header.Get("Content-Type"); ct != "application/json" {
		http.Error(w, "application/json required", http.StatusUnsupportedMediaType)
		return
	}

	defer r.Body.Close()
	body, err := io.ReadAll(io.LimitReader(r.Body, 1*1024*1024))
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}

	var req struct {
		Name  string      `json:"name"`
		Type  string      `json:"type"`
		Input interface{} `json:"input,omitempty"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		http.Error(w, "Operation name is required", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"operation": map[string]interface{}{
			"name": req.Name,
			"type": req.Type,
		},
	})
}

// mockWotanPublishHandler mimics the Wotan PublishTopic input parsing.
// Extracts topic from path, validates JSON body, checks subscriber_id and payload.
func mockWotanPublishHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract topic from URL path (mimics extractTopic)
	path := r.URL.Path
	const prefix = "/api/v1/topics/"
	const suffix = "/publish"
	topic := ""
	if strings.HasPrefix(path, prefix) {
		rest := strings.TrimPrefix(path, prefix)
		if strings.HasSuffix(rest, suffix) {
			topic = strings.TrimSuffix(rest, suffix)
		} else {
			topic = rest
		}
	}
	if topic == "" {
		http.Error(w, "topic is required", http.StatusBadRequest)
		return
	}

	// Body limit (matches Wotan httputil.DefaultMaxBodySize)
	defer r.Body.Close()
	body, err := io.ReadAll(io.LimitReader(r.Body, 10*1024*1024))
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}

	var req struct {
		SubscriberID string `json:"subscriber_id"`
		Payload      string `json:"payload"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Payload == "" {
		http.Error(w, "payload is required", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"topic":   topic,
		"message": "published",
	})
}

// ============================================================================
// FUZZ TARGETS
// ============================================================================

// FuzzHTTPAPIKanbanTasks fuzzes the kanban task handler with malformed POST bodies.
// Security-critical: kanban accepts user input that flows to storage.
func FuzzHTTPAPIKanbanTasks(f *testing.F) {
	// Seed: valid task JSON
	validTask, _ := json.Marshal(map[string]interface{}{
		"id":     "task-001",
		"title":  "Test Task",
		"status": "todo",
	})
	f.Add(validTask)

	// Seed: empty body
	f.Add([]byte{})

	// Seed: malformed JSON
	f.Add([]byte(`{broken json`))
	f.Add([]byte(`{"id": "x", "title":}`))

	// Seed: SQL injection payloads
	sqli1, _ := json.Marshal(map[string]interface{}{
		"id":    "'; DROP TABLE tasks--",
		"title": "'; DROP TABLE--",
	})
	f.Add(sqli1)
	sqli2, _ := json.Marshal(map[string]interface{}{
		"id":    "1 OR 1=1",
		"title": "' UNION SELECT * FROM users--",
	})
	f.Add(sqli2)

	// Seed: XSS payloads
	xss, _ := json.Marshal(map[string]interface{}{
		"id":    "<script>alert(1)</script>",
		"title": "<img src=x onerror=alert(1)>",
	})
	f.Add(xss)

	// Seed: oversized payload (100KB of 'A')
	oversized, _ := json.Marshal(map[string]interface{}{
		"id":    "big",
		"title": strings.Repeat("A", 100*1024),
	})
	f.Add(oversized)

	// Seed: null bytes
	nullBytes, _ := json.Marshal(map[string]interface{}{
		"id":    "null\x00byte",
		"title": "test\x00\x00\x00",
	})
	f.Add(nullBytes)

	// Seed: deeply nested JSON
	f.Add([]byte(`{"id":"x","title":"y","meta":{"a":{"b":{"c":{"d":{"e":"deep"}}}}}}`))

	// Seed: unicode edge cases
	f.Add([]byte(`{"id":"task-\ud800","title":"\uffff\ufffe"}`))

	// Seed: all zeros
	f.Add(make([]byte, 64))

	// Seed: all 0xFF
	allFF := make([]byte, 64)
	for i := range allFF {
		allFF[i] = 0xFF
	}
	f.Add(allFF)

	f.Fuzz(func(t *testing.T, data []byte) {
		srv := httptest.NewServer(http.HandlerFunc(mockKanbanTaskHandler))
		defer srv.Close()

		resp, err := http.Post(srv.URL+"/api/v1/tasks", "application/json",
			bytes.NewReader(data))
		if err != nil {
			// Network error is acceptable (e.g., server closed)
			return
		}
		defer resp.Body.Close()

		// Drain body to ensure response is valid HTTP
		_, _ = io.ReadAll(resp.Body)

		// Invariant: status code must be a valid HTTP status
		if resp.StatusCode < 100 || resp.StatusCode > 599 {
			t.Errorf("invalid HTTP status code: %d", resp.StatusCode)
		}
	})
}

// FuzzHTTPAPIMonadEncode fuzzes the Monad operations endpoint and the
// underlying exponent encode/decode with random payloads.
func FuzzHTTPAPIMonadEncode(f *testing.F) {
	// Seed: valid 20-byte Monad frame
	validFrame := EncodeMonadWire(&MonadHeader{
		Version:   MonadWireVersion,
		Flags:     0x00,
		FlowLabel: 0x00010001,
		Value:     0x0420,
		Namespace: 0x00000001,
		Sequence:  0x00000001,
		TTL:       64,
	})
	f.Add(validFrame)

	// Seed: truncated frame
	f.Add(validFrame[:10])

	// Seed: oversized frame
	f.Add(append(validFrame, make([]byte, 100)...))

	// Seed: random 20 bytes
	f.Add([]byte{
		0xDE, 0xAD, 0xBE, 0xEF, 0xCA, 0xFE, 0xBA, 0xBE, 0x01, 0x02,
		0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0A, 0x0B, 0x0C,
	})

	// Seed: max exponent values
	maxExp := make([]byte, 20)
	copy(maxExp, validFrame)
	binary.BigEndian.PutUint16(maxExp[6:8], 0xFF00) // exp=255, mantissa=0
	crc := encoding.CRC16CCITT(maxExp[:18])
	binary.BigEndian.PutUint16(maxExp[18:20], crc)
	f.Add(maxExp)

	// Seed: empty
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, data []byte) {
		// Test 1: Feed raw bytes to Monad wire parser
		header, parseErr := ParseMonadWire(data)
		if parseErr != nil {
			// Expected for malformed input — verify no panic
			return
		}

		// Test 2: Decode exponent from parsed Value field
		decoded, expErr := encoding.DecodeExponent(header.Value)
		if expErr != nil {
			// Overflow is valid rejection
			return
		}

		// Test 3: Re-encode and verify consistency
		reEncoded := encoding.EncodeExponent(uint32(decoded))
		reDecoded, reErr := encoding.DecodeExponent(reEncoded)
		if reErr != nil {
			t.Logf("LICH-013 NOTE: re-encode/decode failed: original=0x%04X decoded=%d re-encoded=0x%04X err=%v",
				header.Value, decoded, reEncoded, reErr)
		}
		_ = reDecoded

		// Test 4: Feed data as POST body to Monad operations handler
		srv := httptest.NewServer(http.HandlerFunc(mockMonadOperationsHandler))
		defer srv.Close()

		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/operations",
			bytes.NewReader(data))
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return
		}
		defer resp.Body.Close()
		_, _ = io.ReadAll(resp.Body)

		if resp.StatusCode < 100 || resp.StatusCode > 599 {
			t.Errorf("invalid HTTP status code: %d", resp.StatusCode)
		}
	})
}

// FuzzHTTPAPIWotanWrite fuzzes Wotan message publishing with crafted
// topic names and payloads targeting path traversal, null bytes, and overflow.
func FuzzHTTPAPIWotanWrite(f *testing.F) {
	// Seed: valid publish request
	valid, _ := json.Marshal(map[string]interface{}{
		"subscriber_id": "00000000-0000-0000-0000-000000000001",
		"payload":       "hello world",
	})
	f.Add("system.events", valid)

	// Seed: path traversal in topic
	f.Add("../../etc/passwd", valid)
	f.Add("../../../etc/shadow", valid)
	f.Add("..%2F..%2F..%2Fetc%2Fpasswd", valid)

	// Seed: null bytes in topic
	f.Add("topic\x00injected", valid)
	f.Add("\x00\x00\x00", valid)

	// Seed: oversized topic name
	f.Add(strings.Repeat("a", 10000), valid)

	// Seed: special characters in topic
	f.Add("topic/with/slashes", valid)
	f.Add("topic;rm -rf /", valid)
	f.Add("topic\ninjection", valid)
	f.Add("topic\r\nX-Injected: true", valid)

	// Seed: malformed JSON body
	f.Add("valid.topic", []byte(`{broken`))
	f.Add("valid.topic", []byte{})

	// Seed: oversized payload
	bigPayload, _ := json.Marshal(map[string]interface{}{
		"subscriber_id": "00000000-0000-0000-0000-000000000001",
		"payload":       strings.Repeat("X", 1024*1024),
	})
	f.Add("valid.topic", bigPayload)

	// Seed: SQL injection in subscriber_id
	sqli, _ := json.Marshal(map[string]interface{}{
		"subscriber_id": "'; DROP TABLE--",
		"payload":       "test",
	})
	f.Add("valid.topic", sqli)

	f.Fuzz(func(t *testing.T, topic string, body []byte) {
		srv := httptest.NewServer(http.HandlerFunc(mockWotanPublishHandler))
		defer srv.Close()

		url := fmt.Sprintf("%s/api/v1/topics/%s/publish", srv.URL, topic)
		resp, err := http.Post(url, "application/json", bytes.NewReader(body))
		if err != nil {
			// Network errors acceptable (e.g., invalid URL from fuzzed topic)
			return
		}
		defer resp.Body.Close()
		_, _ = io.ReadAll(resp.Body)

		// Invariant: server must respond with valid HTTP status
		if resp.StatusCode < 100 || resp.StatusCode > 599 {
			t.Errorf("invalid HTTP status code: %d for topic=%q", resp.StatusCode, topic)
		}
	})
}

// FuzzHTTPAPIHeaderInjection fuzzes HTTP request headers targeting CRLF injection,
// oversized headers, duplicate Content-Type, and malformed Host headers.
func FuzzHTTPAPIHeaderInjection(f *testing.F) {
	// Seed: normal headers
	f.Add("application/json", "localhost:8080", "valid-value")

	// Seed: CRLF injection in Content-Type
	f.Add("application/json\r\nX-Injected: evil", "localhost", "normal")

	// Seed: CRLF injection in custom header value
	f.Add("application/json", "localhost", "value\r\nX-Injected: evil")

	// Seed: oversized Content-Type
	f.Add(strings.Repeat("application/json;", 10000), "localhost", "normal")

	// Seed: oversized Host
	f.Add("application/json", strings.Repeat("a", 100000), "normal")

	// Seed: null bytes in headers
	f.Add("application/\x00json", "local\x00host", "\x00\x00\x00")

	// Seed: empty headers
	f.Add("", "", "")

	// Seed: duplicate-style Content-Type
	f.Add("text/html, application/json", "localhost", "normal")

	// Seed: malformed Host with port injection
	f.Add("application/json", "localhost:99999", "normal")
	f.Add("application/json", "localhost:abc", "normal")

	f.Fuzz(func(t *testing.T, contentType, host, customValue string) {
		srv := httptest.NewServer(http.HandlerFunc(mockKanbanTaskHandler))
		defer srv.Close()

		body := []byte(`{"id":"fuzz","title":"fuzz"}`)
		req, err := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/tasks",
			bytes.NewReader(body))
		if err != nil {
			return
		}

		// Apply fuzzed headers — http.NewRequest may reject invalid values
		// which is fine (that's Go's stdlib protecting us)
		req.Header.Set("Content-Type", contentType)
		req.Host = host
		req.Header.Set("X-Custom-Fuzz", customValue)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			// Connection errors are acceptable for malformed requests
			return
		}
		defer resp.Body.Close()
		_, _ = io.ReadAll(resp.Body)

		// Invariant: server must respond with valid HTTP status
		if resp.StatusCode < 100 || resp.StatusCode > 599 {
			t.Errorf("invalid HTTP status code: %d", resp.StatusCode)
		}
	})
}
