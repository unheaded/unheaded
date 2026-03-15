// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package errors

import (
	"errors"
	"testing"
)

func TestAllCoreErrorCodesExist(t *testing.T) {
	testCases := []ErrorCode{
		UNHD_NO_ERROR,
		UNHD_PROTOCOL_ERROR,
		UNHD_INVALID_FRAME,
		UNHD_FLOW_CONTROL_ERROR,
		UNHD_SETTINGS_TIMEOUT,
		UNHD_STREAM_CLOSED,
		UNHD_FRAME_SIZE_ERROR,
		UNHD_REFUSED_STREAM,
		UNHD_CANCEL,
		UNHD_COMPRESSION_ERROR,
		UNHD_CONNECT_ERROR,
		UNHD_ENHANCE_YOUR_CALM,
		UNHD_INTERNAL_ERROR,
	}

	for _, code := range testCases {
		t.Run("Lookup_"+string(rune(code)), func(t *testing.T) {
			info, err := Lookup(code)
			if err != nil {
				t.Fatalf("failed to lookup code 0x%04x: %v", code, err)
			}
			if info == nil {
				t.Fatalf("got nil ErrorInfo for code 0x%04x", code)
			}
			if info.Code != code {
				t.Errorf("expected code 0x%04x, got 0x%04x", code, info.Code)
			}
			if info.Description == "" {
				t.Errorf("description is empty for code 0x%04x", code)
			}
		})
	}
}

func TestLookupUnknownCode(t *testing.T) {
	unknownCode := ErrorCode(0x9999)
	info, err := Lookup(unknownCode)
	if err == nil {
		t.Fatalf("expected error for unknown code, got nil")
	}
	if info != nil {
		t.Fatalf("expected nil ErrorInfo for unknown code")
	}
}

func TestErrorLevelClassification(t *testing.T) {
	testCases := []struct {
		code     ErrorCode
		expected ErrorLevel
	}{
		{UNHD_NO_ERROR, FlowLevel},
		{UNHD_PROTOCOL_ERROR, SystemLevel},
		{UNHD_INVALID_FRAME, DomainLevel},
		{UNHD_FLOW_CONTROL_ERROR, DomainLevel},
		{UNHD_SETTINGS_TIMEOUT, SystemLevel},
		{UNHD_STREAM_CLOSED, FlowLevel},
		{UNHD_FRAME_SIZE_ERROR, DomainLevel},
		{UNHD_REFUSED_STREAM, FlowLevel},
		{UNHD_CANCEL, FlowLevel},
		{UNHD_COMPRESSION_ERROR, DomainLevel},
		{UNHD_CONNECT_ERROR, SystemLevel},
		{UNHD_ENHANCE_YOUR_CALM, SystemLevel},
		{UNHD_INTERNAL_ERROR, SystemLevel},
	}

	for _, tc := range testCases {
		t.Run("Level_"+string(rune(tc.code)), func(t *testing.T) {
			info, err := Lookup(tc.code)
			if err != nil {
				t.Fatalf("failed to lookup code: %v", err)
			}
			if info.Level != tc.expected {
				t.Errorf("expected level %d, got %d", tc.expected, info.Level)
			}
		})
	}
}

func TestRetryableClassification(t *testing.T) {
	testCases := []struct {
		code      ErrorCode
		retryable bool
	}{
		{UNHD_NO_ERROR, false},
		{UNHD_PROTOCOL_ERROR, true},
		{UNHD_INVALID_FRAME, false},
		{UNHD_FLOW_CONTROL_ERROR, true},
		{UNHD_SETTINGS_TIMEOUT, true},
		{UNHD_STREAM_CLOSED, true},
		{UNHD_FRAME_SIZE_ERROR, true},
		{UNHD_REFUSED_STREAM, true},
		{UNHD_CANCEL, true},
		{UNHD_COMPRESSION_ERROR, false},
		{UNHD_CONNECT_ERROR, true},
		{UNHD_ENHANCE_YOUR_CALM, true},
		{UNHD_INTERNAL_ERROR, true},
	}

	for _, tc := range testCases {
		t.Run("Retryable_"+string(rune(tc.code)), func(t *testing.T) {
			info, err := Lookup(tc.code)
			if err != nil {
				t.Fatalf("failed to lookup code: %v", err)
			}
			if info.Retryable != tc.retryable {
				t.Errorf("expected retryable %v, got %v", tc.retryable, info.Retryable)
			}
		})
	}
}

func TestProtocolError(t *testing.T) {
	t.Run("NewProtocolError", func(t *testing.T) {
		pe := NewProtocolError(UNHD_PROTOCOL_ERROR, "test error")
		if pe == nil {
			t.Fatal("got nil ProtocolError")
		}
		if pe.Code() != UNHD_PROTOCOL_ERROR {
			t.Errorf("expected code 0x%04x, got 0x%04x", UNHD_PROTOCOL_ERROR, pe.Code())
		}
		msg := pe.Error()
		if msg == "" {
			t.Fatal("error message is empty")
		}
		if !contains(msg, "test error") {
			t.Errorf("error message should contain 'test error', got: %s", msg)
		}
	})

	t.Run("NewProtocolErrorWithCause", func(t *testing.T) {
		cause := errors.New("root cause")
		pe := NewProtocolErrorWithCause(UNHD_FLOW_CONTROL_ERROR, "flow error", cause)
		if pe == nil {
			t.Fatal("got nil ProtocolError")
		}
		if pe.Code() != UNHD_FLOW_CONTROL_ERROR {
			t.Errorf("expected code 0x%04x, got 0x%04x", UNHD_FLOW_CONTROL_ERROR, pe.Code())
		}
		if pe.Unwrap() != cause {
			t.Errorf("unwrap should return original cause")
		}
		msg := pe.Error()
		if !contains(msg, "root cause") {
			t.Errorf("error message should contain cause, got: %s", msg)
		}
	})
}

func TestProtocolErrorInterface(t *testing.T) {
	pe := NewProtocolError(UNHD_FRAME_SIZE_ERROR, "frame too large")
	var _ error = pe
}

func TestIsRetryable(t *testing.T) {
	t.Run("RetryableError", func(t *testing.T) {
		pe := NewProtocolError(UNHD_FLOW_CONTROL_ERROR, "flow control")
		if !pe.IsRetryable() {
			t.Errorf("UNHD_FLOW_CONTROL_ERROR should be retryable")
		}
	})

	t.Run("NonRetryableError", func(t *testing.T) {
		pe := NewProtocolError(UNHD_INVALID_FRAME, "invalid")
		if pe.IsRetryable() {
			t.Errorf("UNHD_INVALID_FRAME should not be retryable")
		}
	})
}

func TestErrorLevel(t *testing.T) {
	t.Run("FlowLevelError", func(t *testing.T) {
		pe := NewProtocolError(UNHD_STREAM_CLOSED, "stream closed")
		if pe.Level() != FlowLevel {
			t.Errorf("expected FlowLevel, got %d", pe.Level())
		}
	})

	t.Run("DomainLevelError", func(t *testing.T) {
		pe := NewProtocolError(UNHD_INVALID_FRAME, "invalid frame")
		if pe.Level() != DomainLevel {
			t.Errorf("expected DomainLevel, got %d", pe.Level())
		}
	})

	t.Run("SystemLevelError", func(t *testing.T) {
		pe := NewProtocolError(UNHD_CONNECT_ERROR, "connect failed")
		if pe.Level() != SystemLevel {
			t.Errorf("expected SystemLevel, got %d", pe.Level())
		}
	})
}

func TestRegisterCustomCode(t *testing.T) {
	customCode := ErrorCode(0x0050)
	customInfo := &ErrorInfo{
		Code:              customCode,
		Level:             FlowLevel,
		Description:       "Custom error",
		RecommendedAction: "Take action",
		Retryable:         true,
	}

	// Registry is frozen after init, so registration should fail
	err := Register(customCode, customInfo)
	if err == nil {
		t.Fatalf("expected error due to frozen registry")
	}

	// The code should not be in the registry
	_, lookupErr := Lookup(customCode)
	if lookupErr == nil {
		t.Fatalf("expected lookup error for unregistered code")
	}
}

func TestRegisterNilInfo(t *testing.T) {
	err := Register(ErrorCode(0x0050), nil)
	if err == nil {
		t.Fatal("expected error when registering nil info")
	}
}

func TestRegisterOutOfRange(t *testing.T) {
	outOfRangeCode := ErrorCode(0x1234)
	info := &ErrorInfo{
		Code:        outOfRangeCode,
		Level:       SystemLevel,
		Description: "Out of range",
	}
	err := Register(outOfRangeCode, info)
	if err == nil {
		t.Fatal("expected error for out-of-range code")
	}
}

func TestRegisterCannotOverrideStandards(t *testing.T) {
	// Try to register over a standards code (frozen, so should fail)
	info := &ErrorInfo{
		Code:        UNHD_PROTOCOL_ERROR,
		Level:       SystemLevel,
		Description: "Modified protocol error",
	}
	err := Register(UNHD_PROTOCOL_ERROR, info)
	if err == nil {
		t.Fatal("expected error when trying to override standards code (frozen registry)")
	}
}

func TestLookupZeroReturnsNoError(t *testing.T) {
	info, err := Lookup(ErrorCode(0))
	if err != nil {
		t.Fatalf("Lookup(0) should return UNHD_NO_ERROR, got error: %v", err)
	}
	if info.Code != UNHD_NO_ERROR {
		t.Errorf("expected UNHD_NO_ERROR, got 0x%04x", info.Code)
	}
}

// TestRegistryFreeze verifies that the registry is frozen after init,
// preventing post-initialization registrations per RFC 9669 §7.3.
func TestRegistryFreeze(t *testing.T) {
	t.Run("RegistryIsFrozen", func(t *testing.T) {
		if !codeRegistry.IsFrozen() {
			t.Fatal("expected registry to be frozen after init")
		}
	})

	t.Run("CannotRegisterAfterFreeze", func(t *testing.T) {
		newCode := ErrorCode(0x0050)
		info := &ErrorInfo{
			Code:              newCode,
			Level:             FlowLevel,
			Description:       "Should fail",
			RecommendedAction: "None",
			Retryable:         false,
		}

		// Try to register after freeze - should fail
		err := Register(newCode, info)
		if err == nil {
			t.Fatal("expected error when registering after freeze")
		}

		// Verify the error mentions the frozen registry
		if !contains(err.Error(), "frozen") {
			t.Errorf("error should mention 'frozen', got: %v", err)
		}
	})
}

// TestMustRegisterDuplicatePanics verifies that MustRegister panics when
// attempting to register a duplicate key during initialization.
func TestMustRegisterDuplicatePanics(t *testing.T) {
	t.Run("MustRegisterDuplicateErrorMessage", func(t *testing.T) {
		defer func() {
			r := recover()
			if r == nil {
				t.Fatal("expected panic, but none occurred")
			}

			panicMsg, ok := r.(string)
			if !ok {
				t.Fatalf("expected string panic message, got %T", r)
			}

			if !contains(panicMsg, "MustRegister") {
				t.Errorf("panic message should contain 'MustRegister', got: %s", panicMsg)
			}

			if !contains(panicMsg, "frozen") {
				t.Errorf("panic message should contain 'frozen', got: %s", panicMsg)
			}
		}()

		// This should panic because the registry is frozen
		codeRegistry.MustRegister(ErrorCode(0x0099), &ErrorInfo{
			Code:        ErrorCode(0x0099),
			Level:       SystemLevel,
			Description: "This should panic",
		})
	})
}

// TestRegistryFreezePreventsAllRegistrations verifies the comprehensive
// freeze behavior, ensuring no mutations are allowed post-freeze.
func TestRegistryFreezePreventsAllRegistrations(t *testing.T) {
	// Verify all attempts to register fail consistently
	testCodes := []ErrorCode{0x0100, 0x0101, 0x0102}

	for _, code := range testCodes {
		info := &ErrorInfo{
			Code:              code,
			Level:             SystemLevel,
			Description:       "Test",
			RecommendedAction: "None",
			Retryable:         false,
		}

		err := Register(code, info)
		if err == nil {
			t.Errorf("expected error for code 0x%04x, got nil", code)
		}
	}
}

func contains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
