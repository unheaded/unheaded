// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package ebpf

import (
	"sync"
	"time"
)

// FlowTTL is the default time-to-live for active flows, matching FLOW_TIMEOUT_NS in kernel.
const FlowTTL = 30 * time.Second

// ActiveFlow represents a tracked network flow with dashboard-visible state.
type ActiveFlow struct {
	Key        FlowKey         `json:"key"`
	TraceID    string          `json:"trace_id"`
	State      ConnectionState `json:"state"`
	PacketsIn  uint64          `json:"packets_in"`
	PacketsOut uint64          `json:"packets_out"`
	BytesIn    uint64          `json:"bytes_in"`
	BytesOut   uint64          `json:"bytes_out"`
	FirstSeen  time.Time       `json:"first_seen"`
	LastSeen   time.Time       `json:"last_seen"`
}

// FlowGraphConfig configures the flow graph tracker.
type FlowGraphConfig struct {
	TTL      time.Duration // How long flows stay active without updates
	MaxFlows int           // Maximum tracked flows (LRU eviction)
}

// FlowGraph tracks active network flows and provides graph data for the dashboard.
type FlowGraph struct {
	config FlowGraphConfig
	flows  map[string]*ActiveFlow // key: FlowKey.String()
	mu     sync.RWMutex

	// Stats
	totalFlowsSeen uint64
	totalPackets   uint64
	totalBytes     uint64
}

// NewFlowGraph creates a new flow graph tracker.
func NewFlowGraph(config FlowGraphConfig) *FlowGraph {
	if config.TTL == 0 {
		config.TTL = FlowTTL
	}
	if config.MaxFlows == 0 {
		config.MaxFlows = 65536
	}
	return &FlowGraph{
		config: config,
		flows:  make(map[string]*ActiveFlow),
	}
}

// IngestPacket processes a PacketEvent and updates the flow graph.
func (fg *FlowGraph) IngestPacket(e *PacketEvent) {
	key := e.FlowKey.String()
	now := e.Time()

	fg.mu.Lock()
	defer fg.mu.Unlock()

	fg.totalPackets++
	fg.totalBytes += uint64(e.PacketLen)

	flow, exists := fg.flows[key]
	if !exists {
		fg.totalFlowsSeen++
		flow = &ActiveFlow{
			Key:       e.FlowKey,
			TraceID:   e.TraceID.String(),
			State:     StateUnknown,
			FirstSeen: now,
		}
		fg.flows[key] = flow

		// LRU eviction if at capacity
		if len(fg.flows) > fg.config.MaxFlows {
			fg.evictOldestLocked()
		}
	}

	flow.LastSeen = now
	if e.Direction == DirectionIngress {
		flow.PacketsIn++
		flow.BytesIn += uint64(e.PacketLen)
	} else {
		flow.PacketsOut++
		flow.BytesOut += uint64(e.PacketLen)
	}
}

// IngestFlow processes a FlowEvent and updates the flow graph.
func (fg *FlowGraph) IngestFlow(e *FlowEvent) {
	key := e.FlowKey.String()
	now := e.Time()

	fg.mu.Lock()
	defer fg.mu.Unlock()

	flow, exists := fg.flows[key]
	if !exists {
		fg.totalFlowsSeen++
		flow = &ActiveFlow{
			Key:       e.FlowKey,
			TraceID:   e.FlowState.TraceID.String(),
			FirstSeen: now,
		}
		fg.flows[key] = flow
	}

	flow.LastSeen = now
	flow.State = e.FlowState.State
	flow.PacketsIn = e.FlowState.PacketsIn
	flow.PacketsOut = e.FlowState.PacketsOut
	flow.BytesIn = e.FlowState.BytesIn
	flow.BytesOut = e.FlowState.BytesOut

	// Remove closed flows; expired flows keep their last state for TTL display
	if flow.State == StateClosed {
		delete(fg.flows, key)
	}
}

// evictOldestLocked removes the least recently seen flow. Caller must hold fg.mu.
func (fg *FlowGraph) evictOldestLocked() {
	var oldestKey string
	var oldestTime time.Time
	first := true

	for k, f := range fg.flows {
		if first || f.LastSeen.Before(oldestTime) {
			oldestKey = k
			oldestTime = f.LastSeen
			first = false
		}
	}

	if oldestKey != "" {
		delete(fg.flows, oldestKey)
	}
}

// Expire removes flows that haven't been seen within the TTL.
func (fg *FlowGraph) Expire() int {
	cutoff := time.Now().Add(-fg.config.TTL)

	fg.mu.Lock()
	defer fg.mu.Unlock()

	expired := 0
	for k, f := range fg.flows {
		if f.LastSeen.Before(cutoff) {
			delete(fg.flows, k)
			expired++
		}
	}
	return expired
}

// GetActiveFlows returns a snapshot of all active flows.
func (fg *FlowGraph) GetActiveFlows() []ActiveFlow {
	fg.mu.RLock()
	defer fg.mu.RUnlock()

	flows := make([]ActiveFlow, 0, len(fg.flows))
	for _, f := range fg.flows {
		flows = append(flows, *f)
	}
	return flows
}

// ActiveFlowCount returns the number of currently tracked flows.
func (fg *FlowGraph) ActiveFlowCount() int {
	fg.mu.RLock()
	defer fg.mu.RUnlock()
	return len(fg.flows)
}

// Stats returns flow graph statistics.
func (fg *FlowGraph) Stats() FlowGraphStats {
	fg.mu.RLock()
	defer fg.mu.RUnlock()

	return FlowGraphStats{
		ActiveFlows:    len(fg.flows),
		TotalFlowsSeen: fg.totalFlowsSeen,
		TotalPackets:   fg.totalPackets,
		TotalBytes:     fg.totalBytes,
	}
}

// FlowGraphStats holds flow graph statistics.
type FlowGraphStats struct {
	ActiveFlows    int    `json:"active_flows"`
	TotalFlowsSeen uint64 `json:"total_flows_seen"`
	TotalPackets   uint64 `json:"total_packets"`
	TotalBytes     uint64 `json:"total_bytes"`
}
