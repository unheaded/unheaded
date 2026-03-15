// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	pb "unheaded/proto/unheaded/v1"
	"google.golang.org/protobuf/types/known/emptypb"
)

// flowServer implements flow lifecycle and management
type flowServer struct {
	pb.UnimplementedFlowServiceServer
	mockFlows map[uint32]*pb.FlowState
	mu        sync.RWMutex
}

var (
	// Global Flow instance
	flowInstance *flowServer
	flowOnce     sync.Once
)

// getFlowServer returns the singleton Flow server instance
func getFlowServer() *flowServer {
	flowOnce.Do(func() {
		flowInstance = &flowServer{
			mockFlows: make(map[uint32]*pb.FlowState),
		}
	})
	return flowInstance
}

// List returns all active flows
func (s *flowServer) List(ctx context.Context, req *emptypb.Empty) (*pb.FlowListResponse, error) {
	if globalMockMode {
		return s.listMock()
	}

	return s.listBPF()
}

// listMock returns flows from mock storage
func (s *flowServer) listMock() (*pb.FlowListResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	flows := make([]*pb.FlowState, 0, len(s.mockFlows))
	for _, flow := range s.mockFlows {
		flows = append(flows, flow)
	}

	return &pb.FlowListResponse{
		Flows: flows,
		Total: int32(len(flows)),
	}, nil
}

// listBPF returns flows from BPF maps
func (s *flowServer) listBPF() (*pb.FlowListResponse, error) {
	// In a real implementation, this would:
	// 1. Open the pinned BPF map for flows
	// 2. Iterate through all entries
	// 3. Return paginated results

	// For now, fall back to mock
	return s.listMock()
}

// GetState retrieves the state of a specific flow
func (s *flowServer) GetState(ctx context.Context, req *pb.ReadRequest) (*pb.FlowState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	flow, found := s.mockFlows[req.FlowLabel]
	if !found {
		return &pb.FlowState{
			FlowLabel: req.FlowLabel,
		}, nil
	}

	return flow, nil
}

// Inject injects a packet into a flow
func (s *flowServer) Inject(ctx context.Context, req *pb.InjectRequest) (*pb.InjectResponse, error) {
	if globalMockMode {
		return s.injectMock(req)
	}

	return s.injectBPF(req)
}

// injectMock injects a packet in mock mode
func (s *flowServer) injectMock(req *pb.InjectRequest) (*pb.InjectResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := uint64(time.Now().UnixMilli())

	// Create or update flow
	flow, found := s.mockFlows[req.FlowLabel]
	if !found {
		flow = &pb.FlowState{
			FlowLabel:    req.FlowLabel,
			KingdomMode:  req.KingdomMode,
			SophiaDictId: req.SophiaDictId,
			CreatedAt:    now,
			UpdatedAt:    now,
			LastPacketAt: now,
		}
		s.mockFlows[req.FlowLabel] = flow
	} else {
		flow.UpdatedAt = now
		flow.LastPacketAt = now
		if req.KingdomMode != pb.KingdomMode_KINGDOM_MODE_UNSPECIFIED {
			flow.KingdomMode = req.KingdomMode
		}
		if req.SophiaDictId > 0 {
			flow.SophiaDictId = req.SophiaDictId
		}
	}

	// Generate packet ID
	packetID := fmt.Sprintf("pkt_%d_%d", req.FlowLabel, now)

	// Publish event
	eventServer := getAnamnesisServer()
	event := CreatePacketEvent(
		req.FlowLabel,
		pb.Event_EVENT_TYPE_PACKET_SENT,
		len(req.Payload),
		map[string]string{
			"packet_id":     packetID,
			"kingdom_mode":  req.KingdomMode.String(),
			"sophia_dict_id": fmt.Sprintf("%d", req.SophiaDictId),
		},
	)
	eventServer.PublishEvent(event)

	return &pb.InjectResponse{
		PacketId:  packetID,
		InjectedAt: now,
		Success:   true,
	}, nil
}

// injectBPF injects a packet in BPF mode
func (s *flowServer) injectBPF(req *pb.InjectRequest) (*pb.InjectResponse, error) {
	// In a real implementation, this would:
	// 1. Create or lookup the flow in BPF maps
	// 2. Queue the packet for transmission
	// 3. Return packet ID and injection timestamp

	// For now, fall back to mock
	return s.injectMock(req)
}

// CreateFlow creates a new flow with given parameters
func (s *flowServer) CreateFlow(
	ctx context.Context,
	flowLabel uint32,
	kingdomMode pb.KingdomMode,
	sourceAddr, destAddr string,
	sophiaDictID uint32,
) (*pb.FlowState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := uint64(time.Now().UnixMilli())

	flow := &pb.FlowState{
		FlowLabel:          flowLabel,
		KingdomMode:        kingdomMode,
		SourceAddress:      []byte(sourceAddr),
		DestinationAddress: []byte(destAddr),
		SophiaDictId:       sophiaDictID,
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	s.mockFlows[flowLabel] = flow

	// Publish creation event
	eventServer := getAnamnesisServer()
	event := CreateFlowEvent(flowLabel, kingdomMode.String(), sourceAddr, destAddr)
	eventServer.PublishEvent(event)

	return flow, nil
}

// UpdateFlowState updates the state of an existing flow
func (s *flowServer) UpdateFlowState(
	ctx context.Context,
	flowLabel uint32,
	kingdomMode pb.KingdomMode,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	flow, found := s.mockFlows[flowLabel]
	if !found {
		return fmt.Errorf("flow %d not found", flowLabel)
	}

	oldState := flow.KingdomMode.String()
	flow.KingdomMode = kingdomMode
	flow.UpdatedAt = uint64(time.Now().UnixMilli())

	// Publish state change event
	eventServer := getAnamnesisServer()
	event := &pb.Event{
		Type:      pb.Event_EVENT_TYPE_FLOW_UPDATE,
		FlowLabel: flowLabel,
		Timestamp: flow.UpdatedAt,
		Source:    "userspace",
		Metadata: map[string]string{
			"old_state": oldState,
			"new_state": kingdomMode.String(),
		},
	}
	eventServer.PublishEvent(event)

	return nil
}

// CloseFlow closes a flow and publishes a flow_destroy event
func (s *flowServer) CloseFlow(ctx context.Context, flowLabel uint32) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	flow, found := s.mockFlows[flowLabel]
	if !found {
		return fmt.Errorf("flow %d not found", flowLabel)
	}

	// Update to CLOSING state first if not already
	if flow.KingdomMode != pb.KingdomMode_KINGDOM_MODE_CLOSING &&
		flow.KingdomMode != pb.KingdomMode_KINGDOM_MODE_CLOSED {
		flow.KingdomMode = pb.KingdomMode_KINGDOM_MODE_CLOSING
	}

	flow.UpdatedAt = uint64(time.Now().UnixMilli())

	// Publish close event
	eventServer := getAnamnesisServer()
	event := &pb.Event{
		Type:      pb.Event_EVENT_TYPE_FLOW_DESTROY,
		FlowLabel: flowLabel,
		Timestamp: flow.UpdatedAt,
		Source:    "userspace",
		Metadata: map[string]string{
			"duration_ms": fmt.Sprintf("%d", flow.UpdatedAt-flow.CreatedAt),
		},
	}
	eventServer.PublishEvent(event)

	return nil
}

// DeleteFlow removes a flow entirely (development/testing only)
func (s *flowServer) DeleteFlow(ctx context.Context, flowLabel uint32) error {
	if !globalMockMode {
		return fmt.Errorf("deletion only supported in mock mode")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.mockFlows, flowLabel)
	return nil
}

// GetFlowCount returns the number of active flows
func (s *flowServer) GetFlowCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return len(s.mockFlows)
}

// GetFlowLabels returns all active flow labels
func (s *flowServer) GetFlowLabels() []uint32 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	labels := make([]uint32, 0, len(s.mockFlows))
	for label := range s.mockFlows {
		labels = append(labels, label)
	}

	return labels
}

// ClearAllFlows removes all flows (development/testing only)
func (s *flowServer) ClearAllFlows() {
	if !globalMockMode {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.mockFlows = make(map[uint32]*pb.FlowState)
}

// FlowStatistics provides summary statistics about flows
type FlowStatistics struct {
	TotalFlows          int
	IdleFlows           int
	ActiveFlows         int
	ClosingFlows        int
	ClosedFlows         int
	AverageDurationMs   uint64
	OldestFlowTimestamp uint64
	NewestFlowTimestamp uint64
}

// GetStatistics returns flow statistics
func (s *flowServer) GetStatistics() *FlowStatistics {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := &FlowStatistics{
		TotalFlows: len(s.mockFlows),
	}

	var totalDuration uint64

	for _, flow := range s.mockFlows {
		switch flow.KingdomMode {
		case pb.KingdomMode_KINGDOM_MODE_IDLE:
			stats.IdleFlows++
		case pb.KingdomMode_KINGDOM_MODE_ACTIVE:
			stats.ActiveFlows++
		case pb.KingdomMode_KINGDOM_MODE_CLOSING:
			stats.ClosingFlows++
		case pb.KingdomMode_KINGDOM_MODE_CLOSED:
			stats.ClosedFlows++
		}

		duration := flow.UpdatedAt - flow.CreatedAt
		totalDuration += duration

		if stats.OldestFlowTimestamp == 0 || flow.CreatedAt < stats.OldestFlowTimestamp {
			stats.OldestFlowTimestamp = flow.CreatedAt
		}
		if flow.CreatedAt > stats.NewestFlowTimestamp {
			stats.NewestFlowTimestamp = flow.CreatedAt
		}
	}

	if stats.TotalFlows > 0 {
		stats.AverageDurationMs = totalDuration / uint64(stats.TotalFlows)
	}

	return stats
}

// GetFlowsByKingdomMode returns flows in a specific Kingdom Mode state
func (s *flowServer) GetFlowsByKingdomMode(mode pb.KingdomMode) []*pb.FlowState {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*pb.FlowState
	for _, flow := range s.mockFlows {
		if flow.KingdomMode == mode {
			result = append(result, flow)
		}
	}

	return result
}

// GetFlowsBySophiaDict returns flows using a specific Sophia dictionary
func (s *flowServer) GetFlowsBySophiaDict(dictID uint32) []*pb.FlowState {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*pb.FlowState
	for _, flow := range s.mockFlows {
		if flow.SophiaDictId == dictID {
			result = append(result, flow)
		}
	}

	return result
}

// UpdateWotanSequence atomically updates a flow's Wotan sequence number
func (s *flowServer) UpdateWotanSequence(ctx context.Context, flowLabel uint32, newSeq uint32) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	flow, found := s.mockFlows[flowLabel]
	if !found {
		return fmt.Errorf("flow %d not found", flowLabel)
	}

	flow.WotanSequence = newSeq
	flow.UpdatedAt = uint64(time.Now().UnixMilli())

	return nil
}

// RecordPacketActivity updates the last packet timestamp for a flow
func (s *flowServer) RecordPacketActivity(ctx context.Context, flowLabel uint32) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	flow, found := s.mockFlows[flowLabel]
	if !found {
		return fmt.Errorf("flow %d not found", flowLabel)
	}

	flow.LastPacketAt = uint64(time.Now().UnixMilli())
	return nil
}

// GetInactiveFlows returns flows that haven't sent packets recently
func (s *flowServer) GetInactiveFlows(thresholdMs uint64) []*pb.FlowState {
	s.mu.RLock()
	defer s.mu.RUnlock()

	now := uint64(time.Now().UnixMilli())
	var result []*pb.FlowState

	for _, flow := range s.mockFlows {
		age := now - flow.LastPacketAt
		if age > thresholdMs {
			result = append(result, flow)
		}
	}

	return result
}
