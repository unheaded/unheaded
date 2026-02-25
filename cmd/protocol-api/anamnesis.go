// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	pb "unheaded/proto/unheaded/v1"
	"google.golang.org/grpc"
)

// anamnesisServer implements event streaming from eBPF
type anamnesisServer struct {
	pb.UnimplementedAnamnesisServiceServer
	mockEventLog []*pb.Event
	mu           sync.RWMutex
	subscribers  []chan *pb.Event
	subMu        sync.RWMutex
}

var (
	// Global Anamnesis instance
	anamnesisInstance *anamnesisServer
	anamnesisOnce     sync.Once
)

// getAnamnesisServer returns the singleton Anamnesis server instance
func getAnamnesisServer() *anamnesisServer {
	anamnesisOnce.Do(func() {
		anamnesisInstance = &anamnesisServer{
			mockEventLog: make([]*pb.Event, 0),
			subscribers:  make([]chan *pb.Event, 0),
		}
		// Initialize with some example events
		anamnesisInstance.initializeExampleEvents()
	})
	return anamnesisInstance
}

// initializeExampleEvents creates some example events for testing
func (s *anamnesisServer) initializeExampleEvents() {
	now := uint64(time.Now().UnixMilli())

	events := []*pb.Event{
		{
			Type:      pb.Event_EVENT_TYPE_FLOW_CREATE,
			FlowLabel: 0x12345,
			Timestamp: now - 5000,
			Payload:   []byte("Flow created"),
			Source:    "kernel",
			Metadata: map[string]string{
				"kingdom_mode": "IDLE",
				"source_addr":  "2001:db8::1",
			},
		},
		{
			Type:      pb.Event_EVENT_TYPE_FLOW_UPDATE,
			FlowLabel: 0x12345,
			Timestamp: now - 3000,
			Payload:   []byte("Kingdom mode changed to ACTIVE"),
			Source:    "kernel",
			Metadata: map[string]string{
				"old_state": "IDLE",
				"new_state": "ACTIVE",
			},
		},
		{
			Type:      pb.Event_EVENT_TYPE_PACKET_RECEIVED,
			FlowLabel: 0x12345,
			Timestamp: now - 1000,
			Payload:   []byte("Packet received: 64 bytes"),
			Source:    "kernel",
			Metadata: map[string]string{
				"packet_size": "64",
				"header_size": "40",
			},
		},
		{
			Type:      pb.Event_EVENT_TYPE_PACKET_SENT,
			FlowLabel: 0x12345,
			Timestamp: now,
			Payload:   []byte("Packet sent: 128 bytes"),
			Source:    "userspace",
			Metadata: map[string]string{
				"packet_size": "128",
			},
		},
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.mockEventLog = append(s.mockEventLog, events...)
}

// Query returns events matching the specified criteria
func (s *anamnesisServer) Query(ctx context.Context, req *pb.EventQuery) (*pb.EventQueryResponse, error) {
	if globalMockMode {
		return s.queryMock(req)
	}

	return s.queryBPF(req)
}

// queryMock searches mock event log
func (s *anamnesisServer) queryMock(req *pb.EventQuery) (*pb.EventQueryResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var filteredEvents []*pb.Event

	for _, event := range s.mockEventLog {
		// Filter by time range
		if req.StartTime > 0 && event.Timestamp < uint64(req.StartTime) {
			continue
		}
		if req.EndTime > 0 && event.Timestamp > uint64(req.EndTime) {
			continue
		}

		// Filter by flow labels
		if len(req.FlowLabels) > 0 {
			found := false
			for _, label := range req.FlowLabels {
				if event.FlowLabel == label {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}

		// Filter by event types
		if len(req.EventTypes) > 0 {
			found := false
			for _, et := range req.EventTypes {
				if event.Type == et {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}

		filteredEvents = append(filteredEvents, event)
	}

	// Apply pagination
	limit := req.Limit
	if limit == 0 {
		limit = 100
	}
	offset := req.Offset

	if offset > int32(len(filteredEvents)) {
		offset = int32(len(filteredEvents))
	}

	end := offset + limit
	if end > int32(len(filteredEvents)) {
		end = int32(len(filteredEvents))
	}

	slicedEvents := filteredEvents[offset:end]

	return &pb.EventQueryResponse{
		Events:     slicedEvents,
		TotalCount: int64(len(filteredEvents)),
	}, nil
}

// queryBPF searches eBPF event ring buffer
func (s *anamnesisServer) queryBPF(req *pb.EventQuery) (*pb.EventQueryResponse, error) {
	// In a real implementation, this would:
	// 1. Open the eBPF perf/ring buffer at /sys/kernel/debug/tracing/trace_pipe
	// 2. Parse protobuf-encoded events
	// 3. Apply filters (time range, flow labels, event types)
	// 4. Return paginated results

	// For now, fall back to mock
	return s.queryMock(req)
}

// Stream returns a continuous stream of events
func (s *anamnesisServer) Stream(req *pb.StreamRequest, stream grpc.ServerStreamingServer[pb.Event]) error {
	if globalMockMode {
		return s.streamMock(req, stream)
	}

	return s.streamBPF(req, stream)
}

// streamMock sends mock events from the event log
func (s *anamnesisServer) streamMock(req *pb.StreamRequest, stream grpc.ServerStreamingServer[pb.Event]) error {
	// Send historical events if requested
	if req.IncludeHistorical {
		s.mu.RLock()
		events := s.mockEventLog
		s.mu.RUnlock()

		for _, event := range events {
			// Filter by flow labels if specified
			if len(req.FlowLabels) > 0 {
				found := false
				for _, label := range req.FlowLabels {
					if event.FlowLabel == label {
						found = true
						break
					}
				}
				if !found {
					continue
				}
			}

			// Filter by event types if specified
			if len(req.EventTypes) > 0 {
				found := false
				for _, et := range req.EventTypes {
					if event.Type == et {
						found = true
						break
					}
				}
				if !found {
					continue
				}
			}

			if err := stream.Send(event); err != nil {
				return err
			}
		}
	}

	// Subscribe to new events
	eventChan := make(chan *pb.Event, 100)
	s.subMu.Lock()
	s.subscribers = append(s.subscribers, eventChan)
	s.subMu.Unlock()

	// Wait for context cancellation or events
	for {
		select {
		case <-stream.Context().Done():
			return stream.Context().Err()
		case event := <-eventChan:
			// Filter by flow labels
			if len(req.FlowLabels) > 0 {
				found := false
				for _, label := range req.FlowLabels {
					if event.FlowLabel == label {
						found = true
						break
					}
				}
				if !found {
					continue
				}
			}

			// Filter by event types
			if len(req.EventTypes) > 0 {
				found := false
				for _, et := range req.EventTypes {
					if event.Type == et {
						found = true
						break
					}
				}
				if !found {
					continue
				}
			}

			if err := stream.Send(event); err != nil {
				return err
			}
		}
	}
}

// streamBPF streams events from eBPF ring buffer
func (s *anamnesisServer) streamBPF(req *pb.StreamRequest, stream grpc.ServerStreamingServer[pb.Event]) error {
	// In a real implementation, this would:
	// 1. Attach to eBPF ring buffer
	// 2. Read protobuf-encoded events as they arrive
	// 3. Apply filters
	// 4. Send via gRPC stream

	// For now, fall back to mock
	return s.streamMock(req, stream)
}

// PublishEvent adds a new event to the log and broadcasts to subscribers (development)
func (s *anamnesisServer) PublishEvent(event *pb.Event) {
	if event.Timestamp == 0 {
		event.Timestamp = uint64(time.Now().UnixMilli())
	}

	s.mu.Lock()
	s.mockEventLog = append(s.mockEventLog, event)
	s.mu.Unlock()

	// Broadcast to subscribers
	s.subMu.RLock()
	subscribers := s.subscribers
	s.subMu.RUnlock()

	for _, ch := range subscribers {
		select {
		case ch <- event:
		default:
			// Channel full, skip
		}
	}
}

// GetEventCount returns the total number of events in the log
func (s *anamnesisServer) GetEventCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return len(s.mockEventLog)
}

// ClearEventLog removes all events (development/testing only)
func (s *anamnesisServer) ClearEventLog() {
	s.mu.Lock()
	defer s.mu.Lock()

	s.mockEventLog = make([]*pb.Event, 0)
}

// GetEventsByFlowLabel returns all events for a specific flow
func (s *anamnesisServer) GetEventsByFlowLabel(flowLabel uint32) []*pb.Event {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*pb.Event
	for _, event := range s.mockEventLog {
		if event.FlowLabel == flowLabel {
			result = append(result, event)
		}
	}

	return result
}

// GetEventsByType returns all events of a specific type
func (s *anamnesisServer) GetEventsByType(eventType pb.Event_EventType) []*pb.Event {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*pb.Event
	for _, event := range s.mockEventLog {
		if event.Type == eventType {
			result = append(result, event)
		}
	}

	return result
}

// EventStatistics provides summary statistics about events
type EventStatistics struct {
	TotalEvents     int
	FlowCount       int
	EventTypeCount  map[string]int
	OldestTimestamp uint64
	NewestTimestamp uint64
}

// GetStatistics returns event log statistics
func (s *anamnesisServer) GetStatistics() *EventStatistics {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := &EventStatistics{
		TotalEvents:    len(s.mockEventLog),
		EventTypeCount: make(map[string]int),
	}

	flowLabels := make(map[uint32]bool)

	for _, event := range s.mockEventLog {
		flowLabels[event.FlowLabel] = true
		eventTypeName := event.Type.String()
		stats.EventTypeCount[eventTypeName]++

		if stats.OldestTimestamp == 0 || event.Timestamp < stats.OldestTimestamp {
			stats.OldestTimestamp = event.Timestamp
		}
		if event.Timestamp > stats.NewestTimestamp {
			stats.NewestTimestamp = event.Timestamp
		}
	}

	stats.FlowCount = len(flowLabels)
	return stats
}

// CreateEventFromFlow generates a FLOW_CREATE event
func CreateFlowEvent(flowLabel uint32, kingdomMode string, sourceAddr, destAddr string) *pb.Event {
	return &pb.Event{
		Type:      pb.Event_EVENT_TYPE_FLOW_CREATE,
		FlowLabel: flowLabel,
		Timestamp: uint64(time.Now().UnixMilli()),
		Source:    "userspace",
		Metadata: map[string]string{
			"kingdom_mode": kingdomMode,
			"source_addr":  sourceAddr,
			"dest_addr":    destAddr,
		},
	}
}

// CreatePacketEvent generates a PACKET_RECEIVED or PACKET_SENT event
func CreatePacketEvent(
	flowLabel uint32,
	eventType pb.Event_EventType,
	packetSize int,
	metadata map[string]string,
) *pb.Event {
	if metadata == nil {
		metadata = make(map[string]string)
	}

	metadata["packet_size"] = fmt.Sprintf("%d", packetSize)

	return &pb.Event{
		Type:      eventType,
		FlowLabel: flowLabel,
		Timestamp: uint64(time.Now().UnixMilli()),
		Source:    "kernel",
		Metadata:  metadata,
	}
}

// CreateErrorEvent generates an ERROR event
func CreateErrorEvent(flowLabel uint32, errMsg string) *pb.Event {
	return &pb.Event{
		Type:      pb.Event_EVENT_TYPE_ERROR,
		FlowLabel: flowLabel,
		Timestamp: uint64(time.Now().UnixMilli()),
		Payload:   []byte(errMsg),
		Source:    "system",
	}
}
