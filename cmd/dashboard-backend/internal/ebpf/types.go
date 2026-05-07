// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package ebpf provides eBPF event ingestion for the dashboard backend.
// It parses events published by trace-collector via Wotan and maintains
// real-time flow graphs, latency histograms, and event streams.
package ebpf

import (
	"encoding/json"
	"fmt"
	"time"
)

// PacketAction represents what happened to a packet.
type PacketAction string

const (
	ActionPass      PacketAction = "pass"
	ActionDrop      PacketAction = "drop"
	ActionMarked    PacketAction = "marked"
	ActionExtracted PacketAction = "extracted"
	ActionRedirect  PacketAction = "redirect"
)

// Direction represents packet direction.
type Direction string

const (
	DirectionIngress Direction = "ingress"
	DirectionEgress  Direction = "egress"
)

// ConnectionState represents TCP connection state.
type ConnectionState string

const (
	StateUnknown     ConnectionState = "unknown"
	StateSynSent     ConnectionState = "syn_sent"
	StateSynReceived ConnectionState = "syn_received"
	StateEstablished ConnectionState = "established"
	StateFinWait1    ConnectionState = "fin_wait1"
	StateFinWait2    ConnectionState = "fin_wait2"
	StateCloseWait   ConnectionState = "close_wait"
	StateClosing     ConnectionState = "closing"
	StateLastAck     ConnectionState = "last_ack"
	StateTimeWait    ConnectionState = "time_wait"
	StateClosed      ConnectionState = "closed"
)

// FlowEventType represents what happened to a flow.
type FlowEventType string

const (
	FlowNew         FlowEventType = "new"
	FlowStateChange FlowEventType = "state_change"
	FlowExpired     FlowEventType = "expired"
	FlowUpdate      FlowEventType = "update"
)

// LatencyOperation represents the measured operation type.
type LatencyOperation string

const (
	OpTcpSend    LatencyOperation = "tcp_send"
	OpTcpRecv    LatencyOperation = "tcp_recv"
	OpTcpConnect LatencyOperation = "tcp_connect"
	OpTcpAccept  LatencyOperation = "tcp_accept"
)

// SyscallEventType represents syscall event phase.
type SyscallEventType string

const (
	SyscallEnter SyscallEventType = "enter"
	SyscallExit  SyscallEventType = "exit"
)

// TraceID is a 128-bit W3C Trace Context identifier.
type TraceID struct {
	High uint64 `json:"high"`
	Low  uint64 `json:"low"`
}

// String returns the hex representation of the trace ID.
func (t TraceID) String() string {
	return fmt.Sprintf("%016x%016x", t.High, t.Low)
}

// FlowKey identifies a network flow (5-tuple).
type FlowKey struct {
	SrcAddr  string `json:"src_addr"`
	DstAddr  string `json:"dst_addr"`
	SrcPort  uint16 `json:"src_port"`
	DstPort  uint16 `json:"dst_port"`
	Protocol uint8  `json:"protocol"`
}

// String returns a human-readable flow key.
func (k FlowKey) String() string {
	proto := "tcp"
	if k.Protocol == 17 {
		proto = "udp"
	}
	return fmt.Sprintf("%s:%d→%s:%d/%s", k.SrcAddr, k.SrcPort, k.DstAddr, k.DstPort, proto)
}

// MeshMeta contains service mesh metadata extracted from IPv4-mapped address prefix bytes.
// Present only on IPv4 packets when the eBPF collector stamps mesh context.
type MeshMeta struct {
	Version       uint8    `json:"version"`
	SrcServiceID  uint8    `json:"src_service_id"`
	DstServiceID  uint8    `json:"dst_service_id"`
	HopCount      uint8    `json:"hop_count"`
	FlowFlags     []string `json:"flow_flags"`
	TraceHash     string   `json:"trace_hash"`
	QosClass      uint16   `json:"qos_class"`
	NatType       string   `json:"nat_type"`
	LatencyHintNs uint32   `json:"latency_hint_ns"`
}

// PacketEvent represents a packet captured by the XDP packet-marker program.
type PacketEvent struct {
	TimestampNs uint64       `json:"timestamp_ns"`
	TraceID     TraceID      `json:"trace_id"`
	FlowKey     FlowKey      `json:"flow_key"`
	PacketLen   uint32       `json:"packet_len"`
	Action      PacketAction `json:"action"`
	Direction   Direction    `json:"direction"`
	Mesh        *MeshMeta    `json:"mesh,omitempty"`
}

// Time returns the event timestamp as time.Time.
func (e *PacketEvent) Time() time.Time {
	return time.Unix(0, int64(e.TimestampNs))
}

// FlowState represents the state of a tracked flow.
type FlowState struct {
	TraceID    TraceID         `json:"trace_id"`
	StartNs    uint64          `json:"start_ns"`
	LastSeenNs uint64          `json:"last_seen_ns"`
	PacketsIn  uint64          `json:"packets_in"`
	PacketsOut uint64          `json:"packets_out"`
	BytesIn    uint64          `json:"bytes_in"`
	BytesOut   uint64          `json:"bytes_out"`
	State      ConnectionState `json:"state"`
}

// FlowEvent represents a flow state change from the flow-tracker program.
type FlowEvent struct {
	TimestampNs uint64        `json:"timestamp_ns"`
	FlowKey     FlowKey       `json:"flow_key"`
	FlowState   FlowState     `json:"flow_state"`
	EventType   FlowEventType `json:"event_type"`
}

// Time returns the event timestamp as time.Time.
func (e *FlowEvent) Time() time.Time {
	return time.Unix(0, int64(e.TimestampNs))
}

// LatencyEvent represents a latency measurement from the latency-probe program.
type LatencyEvent struct {
	TimestampNs uint64           `json:"timestamp_ns"`
	TraceID     TraceID          `json:"trace_id"`
	PID         uint32           `json:"pid"`
	TID         uint32           `json:"tid"`
	LatencyNs   uint64           `json:"latency_ns"`
	Operation   LatencyOperation `json:"operation"`
}

// Time returns the event timestamp as time.Time.
func (e *LatencyEvent) Time() time.Time {
	return time.Unix(0, int64(e.TimestampNs))
}

// LatencyDuration returns the latency as a time.Duration.
func (e *LatencyEvent) LatencyDuration() time.Duration {
	return time.Duration(e.LatencyNs)
}

// SyscallEvent represents a syscall audit event from the syscall-tracer program.
type SyscallEvent struct {
	TimestampNs uint64           `json:"timestamp_ns"`
	PID         uint32           `json:"pid"`
	TID         uint32           `json:"tid"`
	UID         uint32           `json:"uid"`
	GID         uint32           `json:"gid"`
	SyscallNr   int64            `json:"syscall_nr"`
	Args        [6]uint64        `json:"args"`
	Ret         int64            `json:"ret"`
	Comm        string           `json:"comm"`
	EventType   SyscallEventType `json:"event_type"`
}

// Time returns the event timestamp as time.Time.
func (e *SyscallEvent) Time() time.Time {
	return time.Unix(0, int64(e.TimestampNs))
}

// ParsePacketEvent parses a JSON payload into a PacketEvent.
func ParsePacketEvent(data []byte) (*PacketEvent, error) {
	var e PacketEvent
	if err := json.Unmarshal(data, &e); err != nil {
		return nil, fmt.Errorf("parse packet event: %w", err)
	}
	return &e, nil
}

// ParseFlowEvent parses a JSON payload into a FlowEvent.
func ParseFlowEvent(data []byte) (*FlowEvent, error) {
	var e FlowEvent
	if err := json.Unmarshal(data, &e); err != nil {
		return nil, fmt.Errorf("parse flow event: %w", err)
	}
	return &e, nil
}

// ParseLatencyEvent parses a JSON payload into a LatencyEvent.
func ParseLatencyEvent(data []byte) (*LatencyEvent, error) {
	var e LatencyEvent
	if err := json.Unmarshal(data, &e); err != nil {
		return nil, fmt.Errorf("parse latency event: %w", err)
	}
	return &e, nil
}

// ParseSyscallEvent parses a JSON payload into a SyscallEvent.
func ParseSyscallEvent(data []byte) (*SyscallEvent, error) {
	var e SyscallEvent
	if err := json.Unmarshal(data, &e); err != nil {
		return nil, fmt.Errorf("parse syscall event: %w", err)
	}
	return &e, nil
}
