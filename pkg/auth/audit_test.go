// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package auth

import (
	"bytes"
	"encoding/json"
	"errors"
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

type failingWriter struct{ short bool }

func (f failingWriter) Write(p []byte) (int, error) {
	if f.short {
		return len(p) / 2, nil
	}
	return 0, errors.New("disk full")
}

// A failed audit write is counted, never silent (ADR-147, BlackMage F11;
// NIST AU-5): it used to be `_, _ = a.w.Write(data)`.
func TestFailedAuditWriteCounted(t *testing.T) {
	before := auditWriteFailures.Value()
	NewAuditLogger(failingWriter{}, "svc").Log(AuditEvent{EventType: "test"})
	if got := auditWriteFailures.Value() - before; got != 1 {
		t.Fatalf("failures counted %v, want 1", got)
	}
}

func TestShortAuditWriteCounted(t *testing.T) {
	before := auditWriteFailures.Value()
	NewAuditLogger(failingWriter{short: true}, "svc").Log(AuditEvent{EventType: "test"})
	if got := auditWriteFailures.Value() - before; got != 1 {
		t.Fatalf("short write counted %v, want 1", got)
	}
}
