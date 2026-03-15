// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package audit

import (
	"reflect"
	"testing"
	"time"
)

func TestPQCEventTypesAreDistinct(t *testing.T) {
	types := []PQCEventType{
		PQCVerifySuccess,
		PQCVerifyFail,
		PQCKeyRotate,
		PQCKeyRevoke,
		PQCTierChange,
		PQCVerifierHealth,
		PQCMapUtilization,
		PQCEntropyLow,
	}

	if len(types) != 8 {
		t.Fatalf("expected 8 PQC event types, got %d", len(types))
	}

	seen := make(map[PQCEventType]bool, len(types))
	for _, et := range types {
		if et == "" {
			t.Error("event type must not be empty")
		}
		if seen[et] {
			t.Errorf("duplicate event type: %s", et)
		}
		seen[et] = true
	}
}

func TestNewPQCEventSetsTimestamp(t *testing.T) {
	before := time.Now()
	event := NewPQCEvent(PQCVerifySuccess)
	after := time.Now()

	if event.Type != PQCVerifySuccess {
		t.Errorf("expected type %s, got %s", PQCVerifySuccess, event.Type)
	}

	if event.Timestamp.Before(before) || event.Timestamp.After(after) {
		t.Errorf("timestamp %v not between %v and %v", event.Timestamp, before, after)
	}

	if event.Details == nil {
		t.Error("details map must be initialized")
	}
}

func TestPQCEventBuilderPattern(t *testing.T) {
	event := NewPQCEvent(PQCKeyRotate).
		WithService(42).
		WithAlgo(3).
		WithKeyRef(1001).
		WithSigRef(500_000).
		WithTier(2).
		WithDetail("reason", "scheduled").
		WithTrace("trace-abc-123")

	if event.ServiceID != 42 {
		t.Errorf("expected ServiceID 42, got %d", event.ServiceID)
	}
	if event.AlgoID != 3 {
		t.Errorf("expected AlgoID 3, got %d", event.AlgoID)
	}
	if event.KeyRef != 1001 {
		t.Errorf("expected KeyRef 1001, got %d", event.KeyRef)
	}
	if event.SigRef != 500_000 {
		t.Errorf("expected SigRef 500000, got %d", event.SigRef)
	}
	if event.Tier != 2 {
		t.Errorf("expected Tier 2, got %d", event.Tier)
	}
	if event.Details["reason"] != "scheduled" {
		t.Errorf("expected detail reason=scheduled, got %v", event.Details["reason"])
	}
	if event.TraceID != "trace-abc-123" {
		t.Errorf("expected TraceID trace-abc-123, got %s", event.TraceID)
	}
}

func TestWotanTopicsAreUniqueAndNonEmpty(t *testing.T) {
	v := reflect.ValueOf(WotanTopics)
	numFields := v.NumField()

	if numFields != 10 {
		t.Fatalf("expected 10 Wotan topics, got %d", numFields)
	}

	seen := make(map[string]string, numFields)
	tp := reflect.TypeOf(WotanTopics)

	for i := 0; i < numFields; i++ {
		fieldName := tp.Field(i).Name
		topic := v.Field(i).String()

		if topic == "" {
			t.Errorf("Wotan topic %s must not be empty", fieldName)
		}

		if prevField, exists := seen[topic]; exists {
			t.Errorf("duplicate Wotan topic %q: used by both %s and %s", topic, prevField, fieldName)
		}
		seen[topic] = fieldName
	}
}
