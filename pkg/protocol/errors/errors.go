// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package errors

import "fmt"

// Code is an Unheaded protocol error code (8 bits, foundation-06 registry).
type Code uint8

// Registry values (foundation-06 §Error Code Registry).
const (
	NoError                 Code = 0x00
	CRCValidationFailed     Code = 0x01
	VersionNotSupported     Code = 0x02
	FlowLabelInvalid        Code = 0x03
	ArithmeticOverflow      Code = 0x04
	WotanBoundsCheckFailed  Code = 0x05
	MultipleHbHHeaders      Code = 0x06
	UnknownCriticalTLV      Code = 0x07
	WALSeqnoDiscontinuity   Code = 0x08
	InsufficientBufferSpace Code = 0x09
	FlowStateCorruption     Code = 0x0A
	TLSHandshakeFailure     Code = 0x0B
	QUICVersionMismatch     Code = 0x0C
	Reserved0D              Code = 0x0D

	// Greasing values (error-registry.md Section 3): accept and ignore.
	Greasing0 Code = 0x1F
	Greasing1 Code = 0xFF
)

// Allocation ranges.
const (
	UnallocatedMin Code = 0x0E
	UnallocatedMax Code = 0x1E
	PrivateUseMin  Code = 0x20
	PrivateUseMax  Code = 0xFE
)

// Level is the scope an error affects (error-registry.md "Error Level Hierarchy").
type Level uint8

// Levels. LevelNone covers NO_ERROR, reserved, greasing and private-use codes.
const (
	LevelNone Level = iota
	LevelFlow
	LevelDomain
	LevelSystem
)

// String returns the registry's spelling of the level.
func (l Level) String() string {
	switch l {
	case LevelFlow:
		return "Flow"
	case LevelDomain:
		return "Domain"
	case LevelSystem:
		return "System"
	default:
		return "N/A"
	}
}

// Info is one registry row.
type Info struct {
	Code              Code
	Name              string
	Level             Level
	RecommendedAction string
}

// registry holds the single-code rows of error-registry.md Section 1.
var registry = map[Code]Info{
	NoError:                 {NoError, "NO_ERROR", LevelNone, "Continue processing"},
	CRCValidationFailed:     {CRCValidationFailed, "CRC_VALIDATION_FAILED", LevelFlow, "Drop packet, log event, optionally alert operator"},
	VersionNotSupported:     {VersionNotSupported, "VERSION_NOT_SUPPORTED", LevelDomain, "Drop packet, no response (silence is correct)"},
	FlowLabelInvalid:        {FlowLabelInvalid, "FLOW_LABEL_INVALID", LevelFlow, "Drop packet, return error response (optional)"},
	ArithmeticOverflow:      {ArithmeticOverflow, "ARITHMETIC_OVERFLOW", LevelFlow, "Reject operation, return error to program, do not crash"},
	WotanBoundsCheckFailed:  {WotanBoundsCheckFailed, "WOTAN_BOUNDS_CHECK_FAILED", LevelFlow, "Return error from helper, do not access memory"},
	MultipleHbHHeaders:      {MultipleHbHHeaders, "MULTIPLE_HBH_HEADERS", LevelDomain, "Drop packet immediately, no error response"},
	UnknownCriticalTLV:      {UnknownCriticalTLV, "UNKNOWN_CRITICAL_TLV", LevelDomain, "Drop packet, return error code in response (optional)"},
	WALSeqnoDiscontinuity:   {WALSeqnoDiscontinuity, "WAL_SEQNO_DISCONTINUITY", LevelDomain, "Log alert, replay from last known good seqno, may lose data"},
	InsufficientBufferSpace: {InsufficientBufferSpace, "INSUFFICIENT_BUFFER_SPACE", LevelSystem, "Reject new entry, return error to client"},
	FlowStateCorruption:     {FlowStateCorruption, "FLOW_STATE_CORRUPTION", LevelFlow, "Drop flow, close connection, log critical alert"},
	TLSHandshakeFailure:     {TLSHandshakeFailure, "TLS_HANDSHAKE_FAILURE", LevelDomain, "Close connection, log error, do not retry immediately"},
	QUICVersionMismatch:     {QUICVersionMismatch, "QUIC_VERSION_MISMATCH", LevelDomain, "Drop packet, close connection (HTTP/3 integration)"},
	Reserved0D:              {Reserved0D, "RESERVED", LevelNone, "Do not use in current implementations"},
	Greasing0:               {Greasing0, "GREASING_0", LevelNone, "Implementations MUST accept and ignore gracefully"},
	Greasing1:               {Greasing1, "GREASING_1", LevelNone, "Implementations MUST accept and ignore gracefully"},
}

// Lookup returns the registry row for a single-code entry. Unallocated and
// private-use codes have no row.
func Lookup(c Code) (Info, bool) {
	info, ok := registry[c]
	return info, ok
}

// String returns the registry name, or UNALLOCATED(0xNN) / PRIVATE_USE(0xNN).
func (c Code) String() string {
	if info, ok := registry[c]; ok {
		return info.Name
	}
	if c >= UnallocatedMin && c <= UnallocatedMax {
		return fmt.Sprintf("UNALLOCATED(0x%02x)", uint8(c))
	}
	return fmt.Sprintf("PRIVATE_USE(0x%02x)", uint8(c))
}

// Level returns the registry level; LevelNone for codes without one.
func (c Code) Level() Level {
	return registry[c].Level
}

// IsStandard reports whether c is one of the 13 normative codes (0x00-0x0C).
func (c Code) IsStandard() bool { return c <= QUICVersionMismatch }

// IsGrease reports whether c is a greasing value; receivers ignore these.
func (c Code) IsGrease() bool { return c == Greasing0 || c == Greasing1 }

// ProtocolError carries a registry code with context.
type ProtocolError struct {
	code Code
	msg  string
	err  error
}

// New returns a ProtocolError for code with a message.
func New(code Code, msg string) *ProtocolError {
	return &ProtocolError{code: code, msg: msg}
}

// Wrap returns a ProtocolError for code that wraps cause.
func Wrap(code Code, msg string, cause error) *ProtocolError {
	return &ProtocolError{code: code, msg: msg, err: cause}
}

// Error formats as "[Level] 0xNN NAME: msg[: cause]".
func (e *ProtocolError) Error() string {
	s := fmt.Sprintf("[%s] 0x%02x %s: %s", e.code.Level(), uint8(e.code), e.code, e.msg)
	if e.err != nil {
		s += ": " + e.err.Error()
	}
	return s
}

// Code returns the registry code.
func (e *ProtocolError) Code() Code { return e.code }

// Unwrap returns the cause.
func (e *ProtocolError) Unwrap() error { return e.err }
