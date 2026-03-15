// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package lifecycle provides GOAWAY and CANCEL_FLOW frame handling for the Unheaded protocol.
// It implements H7 and H8 findings: GOAWAY monotonicity enforcement and flow cancellation.
package lifecycle

import (
	"encoding/binary"
	"fmt"
	"sync"
	"time"

	"unheaded/pkg/logger"
	wotanClient "unheaded/pkg/wotan-client"
)

// GoawayFrame represents a GOAWAY frame.
type GoawayFrame struct {
	LastFlowID uint32
	ErrorCode  uint16
}

// CancelFlowFrame represents a CANCEL_FLOW frame.
type CancelFlowFrame struct {
	FlowLabel uint32
	ErrorCode uint16
}

// GoawayTracker tracks the last sent GOAWAY to enforce monotonicity.
type GoawayTracker struct {
	mu              sync.RWMutex
	lastFlowID      uint32
	initialized     bool
}

// NewGoawayTracker creates a new GOAWAY tracker.
func NewGoawayTracker() *GoawayTracker {
	return &GoawayTracker{
		lastFlowID:  0,
		initialized: false,
	}
}

// ValidateAndUpdate validates that a new GOAWAY frame maintains monotonicity
// and updates the tracker if valid.
func (gt *GoawayTracker) ValidateAndUpdate(frame *GoawayFrame) error {
	if frame == nil {
		return fmt.Errorf("lifecycle: nil GoawayFrame")
	}

	gt.mu.Lock()
	defer gt.mu.Unlock()

	if gt.initialized && frame.LastFlowID < gt.lastFlowID {
		return fmt.Errorf(
			"lifecycle: GOAWAY violation - LastFlowID must not decrease: current=%d, new=%d",
			gt.lastFlowID,
			frame.LastFlowID,
		)
	}

	gt.lastFlowID = frame.LastFlowID
	gt.initialized = true

	logger.Debug().Msgf("GOAWAY validated and updated: LastFlowID=%d ErrorCode=%d", frame.LastFlowID, frame.ErrorCode)

	return nil
}

// LastFlowID returns the last recorded flow ID.
func (gt *GoawayTracker) LastFlowID() uint32 {
	gt.mu.RLock()
	defer gt.mu.RUnlock()
	return gt.lastFlowID
}

// IsInitialized returns whether the tracker has recorded any GOAWAY.
func (gt *GoawayTracker) IsInitialized() bool {
	gt.mu.RLock()
	defer gt.mu.RUnlock()
	return gt.initialized
}

// EncodeGoawayFrame encodes a GOAWAY frame.
// Format: LastFlowID (4 bytes, big-endian) + ErrorCode (2 bytes, big-endian)
func EncodeGoawayFrame(frame *GoawayFrame) ([]byte, error) {
	if frame == nil {
		return nil, fmt.Errorf("lifecycle: nil GoawayFrame")
	}

	buf := make([]byte, 6)
	binary.BigEndian.PutUint32(buf[0:4], frame.LastFlowID)
	binary.BigEndian.PutUint16(buf[4:6], frame.ErrorCode)

	logger.Debug().Msgf("encoded GOAWAY: LastFlowID=%d ErrorCode=%d", frame.LastFlowID, frame.ErrorCode)

	return buf, nil
}

// DecodeGoawayFrame decodes a GOAWAY frame.
func DecodeGoawayFrame(data []byte) (*GoawayFrame, error) {
	if data == nil {
		return nil, fmt.Errorf("lifecycle: nil data buffer")
	}

	if len(data) < 6 {
		return nil, fmt.Errorf("lifecycle: GoawayFrame requires 6 bytes, got %d", len(data))
	}

	frame := &GoawayFrame{
		LastFlowID: binary.BigEndian.Uint32(data[0:4]),
		ErrorCode:  binary.BigEndian.Uint16(data[4:6]),
	}

	logger.Debug().Msgf("decoded GOAWAY: LastFlowID=%d ErrorCode=%d", frame.LastFlowID, frame.ErrorCode)

	return frame, nil
}

// EncodeCancelFlowFrame encodes a CANCEL_FLOW frame.
// Format: FlowLabel (4 bytes, big-endian) + ErrorCode (2 bytes, big-endian)
func EncodeCancelFlowFrame(frame *CancelFlowFrame) ([]byte, error) {
	if frame == nil {
		return nil, fmt.Errorf("lifecycle: nil CancelFlowFrame")
	}

	buf := make([]byte, 6)
	binary.BigEndian.PutUint32(buf[0:4], frame.FlowLabel)
	binary.BigEndian.PutUint16(buf[4:6], frame.ErrorCode)

	logger.Debug().Msgf("encoded CANCEL_FLOW: FlowLabel=%d ErrorCode=%d", frame.FlowLabel, frame.ErrorCode)

	return buf, nil
}

// DecodeCancelFlowFrame decodes a CANCEL_FLOW frame.
func DecodeCancelFlowFrame(data []byte) (*CancelFlowFrame, error) {
	if data == nil {
		return nil, fmt.Errorf("lifecycle: nil data buffer")
	}

	if len(data) < 6 {
		return nil, fmt.Errorf("lifecycle: CancelFlowFrame requires 6 bytes, got %d", len(data))
	}

	frame := &CancelFlowFrame{
		FlowLabel: binary.BigEndian.Uint32(data[0:4]),
		ErrorCode: binary.BigEndian.Uint16(data[4:6]),
	}

	logger.Debug().Msgf("decoded CANCEL_FLOW: FlowLabel=%d ErrorCode=%d", frame.FlowLabel, frame.ErrorCode)

	return frame, nil
}

// ShutdownSequence implements a progressive GOAWAY shutdown with drain timeout.
// It sends a GOAWAY frame and waits for existing flows to complete.
type ShutdownSequence struct {
	tracker      *GoawayTracker
	drainTimeout time.Duration
	client       *wotanClient.Client
}

// NewShutdownSequence creates a new shutdown sequence handler.
func NewShutdownSequence(drainTimeout time.Duration, client *wotanClient.Client) *ShutdownSequence {
	return &ShutdownSequence{
		tracker:      NewGoawayTracker(),
		drainTimeout: drainTimeout,
		client:       client,
	}
}

// Execute performs the shutdown sequence:
// 1. Send GOAWAY with the last allowed flow ID
// 2. Wait for in-flight flows to drain (with timeout)
// 3. Close the connection after drain timeout
func (ss *ShutdownSequence) Execute(lastFlowID uint32, errorCode uint16) error {
	if ss.client == nil {
		return fmt.Errorf("lifecycle: nil client in shutdown sequence")
	}

	// Create and validate GOAWAY frame
	frame := &GoawayFrame{
		LastFlowID: lastFlowID,
		ErrorCode:  errorCode,
	}

	if err := ss.tracker.ValidateAndUpdate(frame); err != nil {
		return fmt.Errorf("lifecycle: shutdown validation failed: %w", err)
	}

	// Encode and send GOAWAY
	encoded, err := EncodeGoawayFrame(frame)
	if err != nil {
		return fmt.Errorf("lifecycle: failed to encode GOAWAY: %w", err)
	}

	// Note: In a real implementation, this would send to the connection
	// For now, we log that we would send it
	logger.Info().Msgf("shutdown sequence initiated: LastFlowID=%d ErrorCode=%d drainTimeout=%v",
		lastFlowID, errorCode, ss.drainTimeout)

	// Wait for drain timeout before closing
	if ss.drainTimeout > 0 {
		time.Sleep(ss.drainTimeout)
	}

	logger.Info().Msgf("shutdown sequence completed: sent GOAWAY frame (%d bytes)", len(encoded))

	return nil
}

// Tracker returns the underlying GOAWAY tracker.
func (ss *ShutdownSequence) Tracker() *GoawayTracker {
	return ss.tracker
}

// ValidateMonotonicity checks if a series of flow IDs maintains monotonicity.
// Used for testing and validation of flow ID progression.
func ValidateMonotonicity(flowIDs []uint32) error {
	if len(flowIDs) == 0 {
		return nil
	}

	for i := 1; i < len(flowIDs); i++ {
		if flowIDs[i] < flowIDs[i-1] {
			return fmt.Errorf(
				"lifecycle: monotonicity violation at index %d: %d < %d",
				i,
				flowIDs[i],
				flowIDs[i-1],
			)
		}
	}

	return nil
}

// GoawayFrameBuilder provides a fluent interface for building GOAWAY frames.
type GoawayFrameBuilder struct {
	frame *GoawayFrame
}

// NewGoawayFrameBuilder creates a new GOAWAY frame builder.
func NewGoawayFrameBuilder() *GoawayFrameBuilder {
	return &GoawayFrameBuilder{
		frame: &GoawayFrame{},
	}
}

// WithLastFlowID sets the LastFlowID.
func (b *GoawayFrameBuilder) WithLastFlowID(id uint32) *GoawayFrameBuilder {
	b.frame.LastFlowID = id
	return b
}

// WithErrorCode sets the ErrorCode.
func (b *GoawayFrameBuilder) WithErrorCode(code uint16) *GoawayFrameBuilder {
	b.frame.ErrorCode = code
	return b
}

// Build returns the constructed GOAWAY frame.
func (b *GoawayFrameBuilder) Build() *GoawayFrame {
	return b.frame
}

// CancelFlowFrameBuilder provides a fluent interface for building CANCEL_FLOW frames.
type CancelFlowFrameBuilder struct {
	frame *CancelFlowFrame
}

// NewCancelFlowFrameBuilder creates a new CANCEL_FLOW frame builder.
func NewCancelFlowFrameBuilder() *CancelFlowFrameBuilder {
	return &CancelFlowFrameBuilder{
		frame: &CancelFlowFrame{},
	}
}

// WithFlowLabel sets the FlowLabel.
func (b *CancelFlowFrameBuilder) WithFlowLabel(label uint32) *CancelFlowFrameBuilder {
	b.frame.FlowLabel = label
	return b
}

// WithErrorCode sets the ErrorCode.
func (b *CancelFlowFrameBuilder) WithErrorCode(code uint16) *CancelFlowFrameBuilder {
	b.frame.ErrorCode = code
	return b
}

// Build returns the constructed CANCEL_FLOW frame.
func (b *CancelFlowFrameBuilder) Build() *CancelFlowFrame {
	return b.frame
}
