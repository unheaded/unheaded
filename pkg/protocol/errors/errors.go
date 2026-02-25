// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package errors

import (
	"fmt"

	"unheaded/pkg/protocol/registry"
)

// ErrorCode represents a 16-bit error code in the Unheaded protocol
type ErrorCode uint16

// ErrorLevel indicates the scope of an error
type ErrorLevel uint8

const (
	// Error levels
	FlowLevel   ErrorLevel = iota // Error specific to a flow
	DomainLevel                    // Error affecting a domain or connection
	SystemLevel                    // Error affecting the entire system
)

// Core error codes as specified by H2 finding
const (
	UNHD_NO_ERROR              ErrorCode = 0x0000
	UNHD_PROTOCOL_ERROR        ErrorCode = 0x0001
	UNHD_INVALID_FRAME         ErrorCode = 0x0002
	UNHD_FLOW_CONTROL_ERROR    ErrorCode = 0x0003
	UNHD_SETTINGS_TIMEOUT      ErrorCode = 0x0004
	UNHD_STREAM_CLOSED         ErrorCode = 0x0005
	UNHD_FRAME_SIZE_ERROR      ErrorCode = 0x0006
	UNHD_REFUSED_STREAM        ErrorCode = 0x0007
	UNHD_CANCEL                ErrorCode = 0x0008
	UNHD_COMPRESSION_ERROR     ErrorCode = 0x0009
	UNHD_CONNECT_ERROR         ErrorCode = 0x000a
	UNHD_ENHANCE_YOUR_CALM     ErrorCode = 0x000b
	UNHD_INTERNAL_ERROR        ErrorCode = 0x000c
)

// IANA allocation ranges
const (
	StandardsMin    ErrorCode = 0x0000
	StandardsMax    ErrorCode = 0x003f
	ExtensionMin    ErrorCode = 0x0040
	ExtensionMax    ErrorCode = 0x00ff
	TestingPrefix   ErrorCode = 0x1f00 // Testing range: 0x1F*N+0x21
)

// ErrorInfo contains metadata about an error code
type ErrorInfo struct {
	Code              ErrorCode
	Level             ErrorLevel
	Description       string
	RecommendedAction string
	Retryable         bool
}

// codeRegistry is the shared registry for error codes.
// Uses a RangePolicy to enforce that StandardsRange (0x0000-0x003f) can only be
// registered once during init, per RFC 8126 "Specification Required" policy.
var codeRegistry = registry.New[ErrorCode, *ErrorInfo](
	"Unheaded Error Codes",
	errorCodePolicy,
)

// errorCodePolicy validates that error codes don't exceed the valid range
// and rejects registration in the core standards range (0x0000-0x003f) if already registered.
func errorCodePolicy(code ErrorCode) error {
	if code > ExtensionMax {
		return fmt.Errorf("registry: error code 0x%04x outside valid range [0x%04x-0x%04x]",
			code, StandardsMin, ExtensionMax)
	}
	return nil
}

func init() {
	// Register all core error codes
	registerDefaults()
	// Freeze the registry to prevent post-init modifications
	codeRegistry.Freeze()
}

func registerDefaults() {
	codes := []ErrorInfo{
		{
			Code:              UNHD_NO_ERROR,
			Level:             FlowLevel,
			Description:       "The connection is closing gracefully",
			RecommendedAction: "No action required",
			Retryable:         false,
		},
		{
			Code:              UNHD_PROTOCOL_ERROR,
			Level:             SystemLevel,
			Description:       "An unspecified protocol error occurred",
			RecommendedAction: "Close the connection and reconnect",
			Retryable:         true,
		},
		{
			Code:              UNHD_INVALID_FRAME,
			Level:             DomainLevel,
			Description:       "Received frame with invalid format or values",
			RecommendedAction: "Reset the stream or connection",
			Retryable:         false,
		},
		{
			Code:              UNHD_FLOW_CONTROL_ERROR,
			Level:             DomainLevel,
			Description:       "Flow control window exceeded",
			RecommendedAction: "Wait for window update or close connection",
			Retryable:         true,
		},
		{
			Code:              UNHD_SETTINGS_TIMEOUT,
			Level:             SystemLevel,
			Description:       "Settings handshake timed out",
			RecommendedAction: "Reconnect with adjusted timeout",
			Retryable:         true,
		},
		{
			Code:              UNHD_STREAM_CLOSED,
			Level:             FlowLevel,
			Description:       "Stream has been closed",
			RecommendedAction: "Create new stream",
			Retryable:         true,
		},
		{
			Code:              UNHD_FRAME_SIZE_ERROR,
			Level:             DomainLevel,
			Description:       "Frame size exceeds configured limits",
			RecommendedAction: "Check payload size and retry with smaller data",
			Retryable:         true,
		},
		{
			Code:              UNHD_REFUSED_STREAM,
			Level:             FlowLevel,
			Description:       "Stream was refused before processing",
			RecommendedAction: "Retry with new stream",
			Retryable:         true,
		},
		{
			Code:              UNHD_CANCEL,
			Level:             FlowLevel,
			Description:       "Stream was cancelled",
			RecommendedAction: "Retry operation or create new stream",
			Retryable:         true,
		},
		{
			Code:              UNHD_COMPRESSION_ERROR,
			Level:             DomainLevel,
			Description:       "Compression or decompression failed",
			RecommendedAction: "Reset connection compression context",
			Retryable:         false,
		},
		{
			Code:              UNHD_CONNECT_ERROR,
			Level:             SystemLevel,
			Description:       "Connection establishment failed",
			RecommendedAction: "Attempt new connection",
			Retryable:         true,
		},
		{
			Code:              UNHD_ENHANCE_YOUR_CALM,
			Level:             SystemLevel,
			Description:       "Peer is sending excessive frames or operating abusively",
			RecommendedAction: "Reduce frame rate or implement backoff",
			Retryable:         true,
		},
		{
			Code:              UNHD_INTERNAL_ERROR,
			Level:             SystemLevel,
			Description:       "Internal implementation error occurred",
			RecommendedAction: "Report error and reconnect",
			Retryable:         true,
		},
	}

	for _, info := range codes {
		infoCopy := info
		codeRegistry.MustRegister(info.Code, &infoCopy)
	}
}

// Lookup retrieves error information for a given code
func Lookup(code ErrorCode) (*ErrorInfo, error) {
	if code == 0 {
		code = UNHD_NO_ERROR
	}

	info, exists := codeRegistry.Lookup(code)
	if !exists {
		return nil, fmt.Errorf("unknown error code: 0x%04x", code)
	}

	return info, nil
}

// Register adds a new error code in the registry if not already registered.
// This will fail if the registry is frozen (after init).
// For use in extension registrations (if registry is unfrozen).
func Register(code ErrorCode, info *ErrorInfo) error {
	if info == nil {
		return fmt.Errorf("error info cannot be nil")
	}

	if info.Code == 0 {
		info.Code = code
	}

	return codeRegistry.Register(code, info)
}

// ProtocolError implements the Go error interface with a Code() method
type ProtocolError struct {
	code    ErrorCode
	message string
	err     error
}

// NewProtocolError creates a new protocol error
func NewProtocolError(code ErrorCode, message string) *ProtocolError {
	return &ProtocolError{
		code:    code,
		message: message,
	}
}

// NewProtocolErrorWithCause creates a new protocol error with an underlying cause
func NewProtocolErrorWithCause(code ErrorCode, message string, err error) *ProtocolError {
	return &ProtocolError{
		code:    code,
		message: message,
		err:     err,
	}
}

// Error implements the error interface
func (pe *ProtocolError) Error() string {
	info, err := Lookup(pe.code)
	if err != nil {
		return fmt.Sprintf("protocol error 0x%04x: %s", pe.code, pe.message)
	}

	msg := fmt.Sprintf("[%s] 0x%04x: %s - %s", levelString(info.Level), pe.code, info.Description, pe.message)
	if pe.err != nil {
		msg = fmt.Sprintf("%s (cause: %v)", msg, pe.err)
	}
	return msg
}

// Code returns the error code
func (pe *ProtocolError) Code() ErrorCode {
	return pe.code
}

// Unwrap returns the underlying error
func (pe *ProtocolError) Unwrap() error {
	return pe.err
}

// IsRetryable checks if the error is retryable based on the registry
func (pe *ProtocolError) IsRetryable() bool {
	info, err := Lookup(pe.code)
	if err != nil {
		return false
	}
	return info.Retryable
}

// Level returns the severity level of the error
func (pe *ProtocolError) Level() ErrorLevel {
	info, err := Lookup(pe.code)
	if err != nil {
		return SystemLevel
	}
	return info.Level
}

// levelString converts an ErrorLevel to a human-readable string
func levelString(level ErrorLevel) string {
	switch level {
	case FlowLevel:
		return "FLOW"
	case DomainLevel:
		return "DOMAIN"
	case SystemLevel:
		return "SYSTEM"
	default:
		return "UNKNOWN"
	}
}
