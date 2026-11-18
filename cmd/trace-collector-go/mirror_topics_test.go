// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"context"
	"encoding/json"
	"net"
	"regexp"
	"sync"
	"testing"
)

// recordingWotan is a topicPublisher that keeps every publish.
type recordingWotan struct {
	mu       sync.Mutex
	topics   []string
	payloads map[string][][]byte
}

func (r *recordingWotan) PublishPreferGRPC(_ context.Context, topic, _ string, payload []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.payloads == nil {
		r.payloads = map[string][][]byte{}
	}
	r.topics = append(r.topics, topic)
	r.payloads[topic] = append(r.payloads[topic], append([]byte(nil), payload...))
	return nil
}

// ADR-147 Phase 10: each trace event goes out once, on traces.*. The
// ebpf.{packet,flow,latency}.events mirrors carried the same events a
// second time for the dashboard (Developer D7: six trace rooms, 39 of 53 MB
// of Wotan's buffers).
func TestNoEBPFMirrorTopics(t *testing.T) {
	cfg := DefaultTracePublisherConfig()
	cfg.BatchSize = 1000
	pub := NewTracePublisher(cfg)
	rec := &recordingWotan{}
	if err := pub.Connect(rec); err != nil {
		t.Fatal(err)
	}
	pub.PublishPacketEvent(makeTraceEntry(net.ParseIP("10.0.0.1"), net.ParseIP("10.0.0.2"), 8080, 443, 6, 1000))
	pub.PublishFlowEvent(json.RawMessage(`{"timestamp_ns":1}`))
	pub.PublishLatencyEvent(json.RawMessage(`{"latency_ns":1}`))
	pub.Flush(context.Background())

	mirror := regexp.MustCompile(`^ebpf\.(packet|flow|latency)\.events$`)
	count := map[string]int{}
	for _, topic := range rec.topics {
		if mirror.MatchString(topic) {
			t.Errorf("published on mirror topic %q", topic)
		}
		count[topic]++
	}
	for _, want := range []string{TopicTracesPacket, TopicTracesFlow, TopicTracesLatency} {
		if count[want] != 1 {
			t.Errorf("%s: %d batches, want 1", want, count[want])
		}
	}
	if len(rec.topics) != 3 {
		t.Errorf("published %v, want exactly the three traces.* topics", rec.topics)
	}
}

// The one packet payload is the dashboard ingestor's PacketEvent shape
// (trace_id {high, low}, a nested flow_key), and it still carries the trace
// ID: the trace view reads it from the same topic.
func TestPacketPayloadIsTheDashboardShape(t *testing.T) {
	pub := NewTracePublisher(DefaultTracePublisherConfig())
	rec := &recordingWotan{}
	if err := pub.Connect(rec); err != nil {
		t.Fatal(err)
	}
	te, err := DecodeKernelPacketEvent(kernelPacketEvent())
	if err != nil {
		t.Fatal(err)
	}
	if err := pub.PublishPacketEvent(te); err != nil {
		t.Fatal(err)
	}
	pub.Flush(context.Background())

	batches := rec.payloads[TopicTracesPacket]
	if len(batches) != 1 {
		t.Fatalf("traces.packet batches = %d, want 1", len(batches))
	}
	var got []struct {
		TimestampNs uint64 `json:"timestamp_ns"`
		TraceID     *struct {
			High uint64 `json:"high"`
			Low  uint64 `json:"low"`
		} `json:"trace_id"`
		FlowKey *struct {
			SrcAddr  string `json:"src_addr"`
			DstAddr  string `json:"dst_addr"`
			SrcPort  uint16 `json:"src_port"`
			DstPort  uint16 `json:"dst_port"`
			Protocol uint8  `json:"protocol"`
		} `json:"flow_key"`
		PacketLen uint32 `json:"packet_len"`
		Action    string `json:"action"`
		Direction string `json:"direction"`
		SrcIP     string `json:"src_ip"`
	}
	if err := json.Unmarshal(batches[0], &got); err != nil {
		t.Fatalf("traces.packet batch is not an array of the dashboard shape: %v (%s)", err, batches[0])
	}
	if len(got) != 1 {
		t.Fatalf("records = %d, want 1", len(got))
	}
	p := got[0]
	if p.TraceID == nil || p.TraceID.High != 0x1111222233334444 || p.TraceID.Low != 0x5555666677778888 {
		t.Errorf("trace_id = %+v, want {0x1111222233334444 0x5555666677778888}", p.TraceID)
	}
	if p.FlowKey == nil || p.FlowKey.SrcAddr != "172.28.1.5" || p.FlowKey.DstPort != 18000 || p.FlowKey.Protocol != 6 {
		t.Errorf("flow_key = %+v", p.FlowKey)
	}
	if p.PacketLen != 1514 || p.Action != "marked" || p.Direction != "egress" || p.TimestampNs != 123456789 {
		t.Errorf("packet = %+v", p)
	}
	if p.SrcIP != "" {
		t.Errorf("src_ip = %q: the flat traces.packet shape is gone", p.SrcIP)
	}
}
