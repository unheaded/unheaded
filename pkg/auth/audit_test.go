// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package auth

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestAuditLogger_Log(t *testing.T) {
	var buf bytes.Buffer
	logger := NewAuditLogger(&buf, "test-service")

	logger.Log(AuditEvent{
		EventType: AuditJWTSuccess,
		Subject:   "user@example.com",
		Endpoint:  "/api/v1/data",
		Method:    "GET",
		Result:    "success",
	})

	output := buf.String()
	if !strings.Contains(output, `"event_type":"jwt_validation_success"`) {
		t.Errorf("expected event_type in output: %s", output)
	}
	if !strings.Contains(output, `"service":"test-service"`) {
		t.Errorf("expected service in output: %s", output)
	}
	if !strings.Contains(output, `"subject":"user@example.com"`) {
		t.Errorf("expected subject in output: %s", output)
	}

	// Verify it's valid JSON.
	var event AuditEvent
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &event); err != nil {
		t.Errorf("output is not valid JSON: %v", err)
	}
	if event.Timestamp == "" {
		t.Error("expected non-empty timestamp")
	}
}

func TestAuditLogger_MultipleEvents(t *testing.T) {
	var buf bytes.Buffer
	logger := NewAuditLogger(&buf, "test-service")

	for i := 0; i < 10; i++ {
		logger.Log(AuditEvent{
			EventType: AuditRBACAllowed,
			Result:    "allowed",
		})
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 10 {
		t.Errorf("expected 10 lines, got %d", len(lines))
	}
}

func TestAuditLogger_LogFunc(t *testing.T) {
	var buf bytes.Buffer
	logger := NewAuditLogger(&buf, "test")
	fn := logger.LogFunc()

	fn(AuditEvent{EventType: AuditRBACDenied, Result: "denied"})

	if !strings.Contains(buf.String(), AuditRBACDenied) {
		t.Error("LogFunc did not produce expected output")
	}
}

func TestNopAuditLogger(t *testing.T) {
	fn := NopAuditLogger()
	// Should not panic.
	fn(AuditEvent{EventType: AuditJWTFailure})
}
