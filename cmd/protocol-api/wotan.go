// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	pb "unheaded/proto/unheaded/v1"
)

// wotanServer implements per-flow memory management
// Each flow has 40 bytes: 32 bytes state + 8 bytes sequence metadata
type wotanServer struct {
	pb.UnimplementedWotanServiceServer
	mockStorage map[uint32]*pb.MemorySlot
	mu          sync.RWMutex
}

var (
	// Global Wotan instance
	wotanInstance *wotanServer
	wotanOnce     sync.Once
)

// getWotanServer returns the singleton Wotan server instance
func getWotanServer() *wotanServer {
	wotanOnce.Do(func() {
		wotanInstance = &wotanServer{
			mockStorage: make(map[uint32]*pb.MemorySlot),
		}
	})
	return wotanInstance
}

// Read retrieves the 40-byte memory slot for a flow
func (s *wotanServer) Read(ctx context.Context, req *pb.ReadRequest) (*pb.ReadResponse, error) {
	if globalMockMode {
		return s.readMock(req)
	}

	return s.readBPF(req)
}

// readMock reads from in-memory mock storage
func (s *wotanServer) readMock(req *pb.ReadRequest) (*pb.ReadResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	slot, found := s.mockStorage[req.FlowLabel]
	if !found {
		return &pb.ReadResponse{
			Found:        false,
			ErrorMessage: fmt.Sprintf("Flow label %d not found", req.FlowLabel),
		}, nil
	}

	return &pb.ReadResponse{
		Slot:  slot,
		Found: true,
	}, nil
}

// readBPF reads from BPF pinned maps
func (s *wotanServer) readBPF(req *pb.ReadRequest) (*pb.ReadResponse, error) {
	// In a real implementation, this would:
	// 1. Open the pinned BPF map at /sys/fs/bpf/unheaded/wotan/[flow_label]
	// 2. Read the 40-byte value
	// 3. Parse into MemorySlot proto
	// 4. Return with found=true/false

	// For now, fall back to mock
	return s.readMock(req)
}

// Write stores the 40-byte memory slot for a flow
func (s *wotanServer) Write(ctx context.Context, req *pb.WriteRequest) (*pb.WriteResponse, error) {
	if globalMockMode {
		return s.writeMock(req)
	}

	return s.writeBPF(req)
}

// writeMock writes to in-memory mock storage
func (s *wotanServer) writeMock(req *pb.WriteRequest) (*pb.WriteResponse, error) {
	// Validate state data length (must be 32 bytes)
	if len(req.StateData) != 32 {
		return &pb.WriteResponse{
			FlowLabel:    req.FlowLabel,
			Success:      false,
			ErrorMessage: fmt.Sprintf("state_data must be 32 bytes, got %d", len(req.StateData)),
		}, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := uint64(time.Now().UnixMilli())

	slot := &pb.MemorySlot{
		FlowLabel:      req.FlowLabel,
		StateData:      req.StateData,
		SequenceNumber: req.SequenceNumber,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	// Check if this is an update
	if existing, found := s.mockStorage[req.FlowLabel]; found {
		slot.CreatedAt = existing.CreatedAt
	}

	s.mockStorage[req.FlowLabel] = slot

	return &pb.WriteResponse{
		FlowLabel: req.FlowLabel,
		Success:   true,
		WrittenAt: now,
	}, nil
}

// writeBPF writes to BPF pinned maps
func (s *wotanServer) writeBPF(req *pb.WriteRequest) (*pb.WriteResponse, error) {
	// In a real implementation, this would:
	// 1. Validate state_data length (32 bytes)
	// 2. Pack into 40-byte structure (32 state + 8 sequence)
	// 3. Open or create pinned BPF map at /sys/fs/bpf/unheaded/wotan/[flow_label]
	// 4. Write with atomic guarantees
	// 5. Return written_at timestamp

	// For now, fall back to mock
	return s.writeMock(req)
}

// Delete removes a memory slot (development/testing only)
func (s *wotanServer) Delete(ctx context.Context, flowLabel uint32) error {
	if !globalMockMode {
		return fmt.Errorf("deletion only supported in mock mode")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.mockStorage, flowLabel)
	return nil
}

// Clear removes all memory slots (development/testing only)
func (s *wotanServer) Clear(ctx context.Context) error {
	if !globalMockMode {
		return fmt.Errorf("clear only supported in mock mode")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.mockStorage = make(map[uint32]*pb.MemorySlot)
	return nil
}

// ListFlowLabels returns all flow labels with stored memory
func (s *wotanServer) ListFlowLabels(ctx context.Context) ([]uint32, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	labels := make([]uint32, 0, len(s.mockStorage))
	for label := range s.mockStorage {
		labels = append(labels, label)
	}

	return labels, nil
}

// InitializeWithStateData initializes a memory slot with raw state bytes (development)
func (s *wotanServer) InitializeWithStateData(ctx context.Context, flowLabel uint32, stateData []byte, sequence uint64) error {
	if len(stateData) != 32 {
		return fmt.Errorf("state_data must be exactly 32 bytes, got %d", len(stateData))
	}

	writeReq := &pb.WriteRequest{
		FlowLabel:      flowLabel,
		StateData:      stateData,
		SequenceNumber: sequence,
	}

	_, err := s.writeMock(writeReq)
	return err
}

// GetFlowCount returns the number of flows with memory allocated
func (s *wotanServer) GetFlowCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return len(s.mockStorage)
}

// WotanStateMetadata provides information about a flow's state
type WotanStateMetadata struct {
	FlowLabel      uint32
	StateDataSize  int
	SequenceNumber uint64
	CreatedAt      uint64
	UpdatedAt      uint64
	AgeMillis      uint64
}

// GetStateMetadata returns metadata about a flow's state
func (s *wotanServer) GetStateMetadata(ctx context.Context, flowLabel uint32) (*WotanStateMetadata, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	slot, found := s.mockStorage[flowLabel]
	if !found {
		return nil, fmt.Errorf("flow label %d not found", flowLabel)
	}

	now := uint64(time.Now().UnixMilli())
	return &WotanStateMetadata{
		FlowLabel:      slot.FlowLabel,
		StateDataSize:  len(slot.StateData),
		SequenceNumber: slot.SequenceNumber,
		CreatedAt:      slot.CreatedAt,
		UpdatedAt:      slot.UpdatedAt,
		AgeMillis:      now - slot.CreatedAt,
	}, nil
}

// UpdateSequenceOnly updates only the sequence number without modifying state
func (s *wotanServer) UpdateSequenceOnly(ctx context.Context, flowLabel uint32, newSequence uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	slot, found := s.mockStorage[flowLabel]
	if !found {
		return fmt.Errorf("flow label %d not found", flowLabel)
	}

	slot.SequenceNumber = newSequence
	slot.UpdatedAt = uint64(time.Now().UnixMilli())

	return nil
}

// CompactMemoryLayout represents the wire format for Wotan storage
// 40 bytes total: bytes 0-31 state, bytes 32-39 sequence number (big-endian uint64)
type CompactMemoryLayout struct {
	State    [32]byte
	Sequence uint64
}

// MarshalToBytes converts a MemorySlot to its compact 40-byte representation
func MarshalToBytes(slot *pb.MemorySlot) [40]byte {
	var layout CompactMemoryLayout
	copy(layout.State[:], slot.StateData)
	layout.Sequence = slot.SequenceNumber
	// In real implementation, would use binary.BigEndian.PutUint64
	var result [40]byte
	copy(result[:32], layout.State[:])
	// Sequence would be encoded in bytes 32-39
	return result
}

// UnmarshalFromBytes converts 40 bytes back to a MemorySlot
func UnmarshalFromBytes(data [40]byte, flowLabel uint32) *pb.MemorySlot {
	return &pb.MemorySlot{
		FlowLabel: flowLabel,
		StateData: data[:32],
		// Sequence would be decoded from bytes 32-39
	}
}
