// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package metrics

import (
	"testing"
	"time"
)

// testMetrics is initialized once to avoid duplicate Prometheus registration panics.
var testMetrics *Metrics

func TestMain(m *testing.M) {
	testMetrics = Initialize("test_wotan")
	m.Run()
}

func TestInitialize_ReturnsNonNil(t *testing.T) {
	if testMetrics == nil {
		t.Fatal("Initialize returned nil")
	}
}

func TestInitialize_SetsDefault(t *testing.T) {
	got := Get()
	if got == nil {
		t.Fatal("Get() returned nil after Initialize")
	}
	if got != testMetrics {
		t.Error("Get() returned different instance than Initialize")
	}
}

func TestInitialize_HTTPMetrics(t *testing.T) {
	if testMetrics.HTTPRequestsTotal == nil {
		t.Error("HTTPRequestsTotal is nil")
	}
	if testMetrics.HTTPRequestDuration == nil {
		t.Error("HTTPRequestDuration is nil")
	}
	if testMetrics.HTTPRequestSize == nil {
		t.Error("HTTPRequestSize is nil")
	}
	if testMetrics.HTTPResponseSize == nil {
		t.Error("HTTPResponseSize is nil")
	}
}

func TestInitialize_RoomMetrics(t *testing.T) {
	if testMetrics.RoomsTotal == nil {
		t.Error("RoomsTotal is nil")
	}
	if testMetrics.RoomMessagesTotal == nil {
		t.Error("RoomMessagesTotal is nil")
	}
	if testMetrics.RoomBufferSize == nil {
		t.Error("RoomBufferSize is nil")
	}
	if testMetrics.RoomBufferUsage == nil {
		t.Error("RoomBufferUsage is nil")
	}
	if testMetrics.RoomBufferWrapped == nil {
		t.Error("RoomBufferWrapped is nil")
	}
}

func TestInitialize_MemberMetrics(t *testing.T) {
	if testMetrics.MembersTotal == nil {
		t.Error("MembersTotal is nil")
	}
	if testMetrics.MembersPending == nil {
		t.Error("MembersPending is nil")
	}
	if testMetrics.MembersApproved == nil {
		t.Error("MembersApproved is nil")
	}
	if testMetrics.MemberJoinRequests == nil {
		t.Error("MemberJoinRequests is nil")
	}
}

func TestInitialize_MessageMetrics(t *testing.T) {
	if testMetrics.MessagesCreated == nil {
		t.Error("MessagesCreated is nil")
	}
	if testMetrics.MessagesDeleted == nil {
		t.Error("MessagesDeleted is nil")
	}
	if testMetrics.MessageDeleteFailed == nil {
		t.Error("MessageDeleteFailed is nil")
	}
}

func TestInitialize_StreamMetrics(t *testing.T) {
	if testMetrics.StreamsActive == nil {
		t.Error("StreamsActive is nil")
	}
	if testMetrics.StreamsTotal == nil {
		t.Error("StreamsTotal is nil")
	}
	if testMetrics.StreamMessagesSent == nil {
		t.Error("StreamMessagesSent is nil")
	}
	if testMetrics.StreamErrors == nil {
		t.Error("StreamErrors is nil")
	}
}

func TestInitialize_SystemMetrics(t *testing.T) {
	if testMetrics.GoroutinesActive == nil {
		t.Error("GoroutinesActive is nil")
	}
	if testMetrics.MemoryAllocated == nil {
		t.Error("MemoryAllocated is nil")
	}
}

func TestInitPackageVars(t *testing.T) {
	// Verify package-level vars were set by Initialize
	if HTTPRequestsTotal == nil {
		t.Error("package HTTPRequestsTotal is nil")
	}
	if HTTPRequestDuration == nil {
		t.Error("package HTTPRequestDuration is nil")
	}
	if RoomsTotal == nil {
		t.Error("package RoomsTotal is nil")
	}
	if MembersTotal == nil {
		t.Error("package MembersTotal is nil")
	}
	if MessagesCreated == nil {
		t.Error("package MessagesCreated is nil")
	}
	if StreamsActive == nil {
		t.Error("package StreamsActive is nil")
	}
	if GoroutinesActive == nil {
		t.Error("package GoroutinesActive is nil")
	}
	if MemoryAllocated == nil {
		t.Error("package MemoryAllocated is nil")
	}
}

func TestRecordHTTPRequest(t *testing.T) {
	// Should not panic
	testMetrics.RecordHTTPRequest("GET", "/health", 200, 50*time.Millisecond)
	testMetrics.RecordHTTPRequest("POST", "/api/v1/rooms", 201, 100*time.Millisecond)
	testMetrics.RecordHTTPRequest("GET", "/not-found", 404, 5*time.Millisecond)
	testMetrics.RecordHTTPRequest("POST", "/api/v1/rooms", 500, 200*time.Millisecond)
}

func TestRecordMessageCreated(t *testing.T) {
	// Without overwrite
	testMetrics.RecordMessageCreated("room-1", false)
	// With overwrite (buffer wrapped)
	testMetrics.RecordMessageCreated("room-1", true)
}

func TestRecordMessageDeleted(t *testing.T) {
	// Success
	testMetrics.RecordMessageDeleted("room-1", true, "")
	// Failure
	testMetrics.RecordMessageDeleted("room-1", false, "not_found")
	testMetrics.RecordMessageDeleted("room-2", false, "permission_denied")
}

func TestUpdateRoomBufferMetrics(t *testing.T) {
	testMetrics.UpdateRoomBufferMetrics("room-1", 1024, 512, false)
	testMetrics.UpdateRoomBufferMetrics("room-2", 2048, 2048, true)
}

func TestRecordStreamCreated(t *testing.T) {
	testMetrics.RecordStreamCreated("room-1")
	testMetrics.RecordStreamCreated("room-2")
}

func TestRecordStreamClosed(t *testing.T) {
	// Open then close
	testMetrics.RecordStreamCreated("room-close-test")
	testMetrics.RecordStreamClosed("room-close-test")
}

func TestRecordStreamMessage(t *testing.T) {
	testMetrics.RecordStreamMessage("room-1", "message_created")
	testMetrics.RecordStreamMessage("room-1", "member_joined")
	testMetrics.RecordStreamMessage("room-2", "message_deleted")
}

func TestRecordStreamError(t *testing.T) {
	testMetrics.RecordStreamError("room-1", "send_failed")
	testMetrics.RecordStreamError("room-1", "connection_closed")
	testMetrics.RecordStreamError("room-2", "timeout")
}
