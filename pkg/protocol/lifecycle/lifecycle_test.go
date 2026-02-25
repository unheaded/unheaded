// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package lifecycle

import (
	"encoding/binary"
	"testing"
	"time"
)

func TestNewGoawayTracker(t *testing.T) {
	tracker := NewGoawayTracker()

	if tracker == nil {
		t.Fatal("NewGoawayTracker returned nil")
	}

	if tracker.IsInitialized() {
		t.Error("new tracker should not be initialized")
	}
}

func TestGoawayTrackerValidateAndUpdate(t *testing.T) {
	tests := []struct {
		name      string
		frames    []*GoawayFrame
		wantError bool
	}{
		{
			name: "single frame",
			frames: []*GoawayFrame{
				{LastFlowID: 100, ErrorCode: 0},
			},
			wantError: false,
		},
		{
			name: "increasing flow IDs",
			frames: []*GoawayFrame{
				{LastFlowID: 100, ErrorCode: 0},
				{LastFlowID: 200, ErrorCode: 0},
				{LastFlowID: 300, ErrorCode: 0},
			},
			wantError: false,
		},
		{
			name: "same flow ID",
			frames: []*GoawayFrame{
				{LastFlowID: 100, ErrorCode: 0},
				{LastFlowID: 100, ErrorCode: 0},
			},
			wantError: false,
		},
		{
			name: "decreasing flow IDs",
			frames: []*GoawayFrame{
				{LastFlowID: 300, ErrorCode: 0},
				{LastFlowID: 200, ErrorCode: 0},
			},
			wantError: true,
		},
		{
			name: "non-monotonic sequence",
			frames: []*GoawayFrame{
				{LastFlowID: 100, ErrorCode: 0},
				{LastFlowID: 300, ErrorCode: 0},
				{LastFlowID: 200, ErrorCode: 0},
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tracker := NewGoawayTracker()

			var lastErr error
			for _, frame := range tt.frames {
				lastErr = tracker.ValidateAndUpdate(frame)
				if lastErr != nil && !tt.wantError {
					t.Fatalf("unexpected error: %v", lastErr)
				}
				if lastErr == nil && tt.wantError {
					continue // Might error on subsequent frame
				}
			}

			if (lastErr != nil) != tt.wantError {
				t.Errorf("got error %v, wantError %v", lastErr, tt.wantError)
			}
		})
	}
}

func TestGoawayTrackerNilFrame(t *testing.T) {
	tracker := NewGoawayTracker()
	err := tracker.ValidateAndUpdate(nil)
	if err == nil {
		t.Error("ValidateAndUpdate(nil) should return error")
	}
}

func TestGoawayTrackerLastFlowID(t *testing.T) {
	tracker := NewGoawayTracker()

	frame := &GoawayFrame{LastFlowID: 42}
	tracker.ValidateAndUpdate(frame)

	if got := tracker.LastFlowID(); got != 42 {
		t.Errorf("LastFlowID() = %d, want 42", got)
	}
}

func TestEncodeDecodeGoawayFrameRoundtrip(t *testing.T) {
	tests := []struct {
		name  string
		frame *GoawayFrame
	}{
		{
			name:  "zero values",
			frame: &GoawayFrame{LastFlowID: 0, ErrorCode: 0},
		},
		{
			name:  "normal values",
			frame: &GoawayFrame{LastFlowID: 12345, ErrorCode: 1},
		},
		{
			name:  "large values",
			frame: &GoawayFrame{LastFlowID: 0xFFFFFFFF, ErrorCode: 0xFFFF},
		},
		{
			name:  "mixed values",
			frame: &GoawayFrame{LastFlowID: 0x12345678, ErrorCode: 0xABCD},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Encode
			encoded, err := EncodeGoawayFrame(tt.frame)
			if err != nil {
				t.Fatalf("Encode failed: %v", err)
			}

			if len(encoded) != 6 {
				t.Errorf("encoded length = %d, want 6", len(encoded))
			}

			// Decode
			decoded, err := DecodeGoawayFrame(encoded)
			if err != nil {
				t.Fatalf("Decode failed: %v", err)
			}

			if decoded.LastFlowID != tt.frame.LastFlowID {
				t.Errorf("LastFlowID: got %d, want %d", decoded.LastFlowID, tt.frame.LastFlowID)
			}
			if decoded.ErrorCode != tt.frame.ErrorCode {
				t.Errorf("ErrorCode: got %d, want %d", decoded.ErrorCode, tt.frame.ErrorCode)
			}
		})
	}
}

func TestEncodeGoawayFrameNil(t *testing.T) {
	_, err := EncodeGoawayFrame(nil)
	if err == nil {
		t.Error("EncodeGoawayFrame(nil) should return error")
	}
}

func TestDecodeGoawayFrameErrors(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		wantErr bool
	}{
		{
			name:    "nil data",
			data:    nil,
			wantErr: true,
		},
		{
			name:    "empty data",
			data:    []byte{},
			wantErr: true,
		},
		{
			name:    "short data",
			data:    []byte{0x00, 0x00, 0x00},
			wantErr: true,
		},
		{
			name:    "exact size",
			data:    []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
			wantErr: false,
		},
		{
			name:    "extra data",
			data:    []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xFF},
			wantErr: false, // Decoder should only read first 6 bytes
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := DecodeGoawayFrame(tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("DecodeGoawayFrame error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestEncodeDecodeCancelFlowFrameRoundtrip(t *testing.T) {
	tests := []struct {
		name  string
		frame *CancelFlowFrame
	}{
		{
			name:  "zero values",
			frame: &CancelFlowFrame{FlowLabel: 0, ErrorCode: 0},
		},
		{
			name:  "normal values",
			frame: &CancelFlowFrame{FlowLabel: 54321, ErrorCode: 2},
		},
		{
			name:  "large values",
			frame: &CancelFlowFrame{FlowLabel: 0xFFFFFFFF, ErrorCode: 0xFFFF},
		},
		{
			name:  "mixed values",
			frame: &CancelFlowFrame{FlowLabel: 0x87654321, ErrorCode: 0x1234},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Encode
			encoded, err := EncodeCancelFlowFrame(tt.frame)
			if err != nil {
				t.Fatalf("Encode failed: %v", err)
			}

			if len(encoded) != 6 {
				t.Errorf("encoded length = %d, want 6", len(encoded))
			}

			// Decode
			decoded, err := DecodeCancelFlowFrame(encoded)
			if err != nil {
				t.Fatalf("Decode failed: %v", err)
			}

			if decoded.FlowLabel != tt.frame.FlowLabel {
				t.Errorf("FlowLabel: got %d, want %d", decoded.FlowLabel, tt.frame.FlowLabel)
			}
			if decoded.ErrorCode != tt.frame.ErrorCode {
				t.Errorf("ErrorCode: got %d, want %d", decoded.ErrorCode, tt.frame.ErrorCode)
			}
		})
	}
}

func TestEncodeCancelFlowFrameNil(t *testing.T) {
	_, err := EncodeCancelFlowFrame(nil)
	if err == nil {
		t.Error("EncodeCancelFlowFrame(nil) should return error")
	}
}

func TestDecodeCancelFlowFrameErrors(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		wantErr bool
	}{
		{
			name:    "nil data",
			data:    nil,
			wantErr: true,
		},
		{
			name:    "empty data",
			data:    []byte{},
			wantErr: true,
		},
		{
			name:    "short data",
			data:    []byte{0xFF, 0xFF, 0xFF},
			wantErr: true,
		},
		{
			name:    "exact size",
			data:    []byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := DecodeCancelFlowFrame(tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("DecodeCancelFlowFrame error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateMonotonicity(t *testing.T) {
	tests := []struct {
		name    string
		ids     []uint32
		wantErr bool
	}{
		{
			name:    "empty",
			ids:     []uint32{},
			wantErr: false,
		},
		{
			name:    "single element",
			ids:     []uint32{1},
			wantErr: false,
		},
		{
			name:    "increasing",
			ids:     []uint32{1, 2, 3, 4, 5},
			wantErr: false,
		},
		{
			name:    "equal",
			ids:     []uint32{1, 1, 1},
			wantErr: false,
		},
		{
			name:    "decreasing",
			ids:     []uint32{5, 4, 3, 2, 1},
			wantErr: true,
		},
		{
			name:    "non-monotonic",
			ids:     []uint32{1, 3, 2, 4},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateMonotonicity(tt.ids)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateMonotonicity error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestGoawayFrameBuilder(t *testing.T) {
	frame := NewGoawayFrameBuilder().
		WithLastFlowID(999).
		WithErrorCode(42).
		Build()

	if frame.LastFlowID != 999 {
		t.Errorf("LastFlowID = %d, want 999", frame.LastFlowID)
	}
	if frame.ErrorCode != 42 {
		t.Errorf("ErrorCode = %d, want 42", frame.ErrorCode)
	}
}

func TestCancelFlowFrameBuilder(t *testing.T) {
	frame := NewCancelFlowFrameBuilder().
		WithFlowLabel(777).
		WithErrorCode(99).
		Build()

	if frame.FlowLabel != 777 {
		t.Errorf("FlowLabel = %d, want 777", frame.FlowLabel)
	}
	if frame.ErrorCode != 99 {
		t.Errorf("ErrorCode = %d, want 99", frame.ErrorCode)
	}
}

func TestShutdownSequenceExecuteNilClient(t *testing.T) {
	ss := NewShutdownSequence(time.Second, nil)
	err := ss.Execute(100, 0)
	if err == nil {
		t.Error("Execute with nil client should return error")
	}
}

func TestShutdownSequenceTracker(t *testing.T) {
	ss := NewShutdownSequence(time.Second, nil)
	tracker := ss.Tracker()

	if tracker == nil {
		t.Fatal("Tracker() returned nil")
	}

	if tracker.IsInitialized() {
		t.Error("tracker should not be initialized initially")
	}
}

func TestGoawayFrameEncoding(t *testing.T) {
	// Test specific encoding format
	frame := &GoawayFrame{LastFlowID: 0x12345678, ErrorCode: 0xABCD}

	encoded, err := EncodeGoawayFrame(frame)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	// Verify encoding
	expectedFlowID := binary.BigEndian.Uint32(encoded[0:4])
	expectedErrorCode := binary.BigEndian.Uint16(encoded[4:6])

	if expectedFlowID != 0x12345678 {
		t.Errorf("encoded LastFlowID = 0x%X, want 0x12345678", expectedFlowID)
	}
	if expectedErrorCode != 0xABCD {
		t.Errorf("encoded ErrorCode = 0x%X, want 0xABCD", expectedErrorCode)
	}
}

func TestCancelFlowFrameEncoding(t *testing.T) {
	// Test specific encoding format
	frame := &CancelFlowFrame{FlowLabel: 0x87654321, ErrorCode: 0x1234}

	encoded, err := EncodeCancelFlowFrame(frame)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	// Verify encoding
	expectedFlowLabel := binary.BigEndian.Uint32(encoded[0:4])
	expectedErrorCode := binary.BigEndian.Uint16(encoded[4:6])

	if expectedFlowLabel != 0x87654321 {
		t.Errorf("encoded FlowLabel = 0x%X, want 0x87654321", expectedFlowLabel)
	}
	if expectedErrorCode != 0x1234 {
		t.Errorf("encoded ErrorCode = 0x%X, want 0x1234", expectedErrorCode)
	}
}
