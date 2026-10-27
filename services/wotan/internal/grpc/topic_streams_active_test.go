// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis. All rights reserved.

package grpc

import (
	"context"
	"testing"
	"time"

	"unheaded/services/wotan/internal/member"
	"unheaded/services/wotan/internal/metrics"
	"unheaded/services/wotan/internal/room"
	"unheaded/services/wotan/internal/wotan"
	chatpb "unheaded/services/wotan/proto"
)

// wotan_streams_active counted only the legacy room streams, while every
// service streams topics (StreamTopics), so the gauge read 0 with the whole
// kingdom subscribed. A topic stream counts under one fixed label (the
// pattern is client-chosen, so it is not a label value) while it is open.
func TestStreamTopics_CountsInStreamsActive(t *testing.T) {
	gauge := metrics.StreamsActive.WithLabelValues(TopicStreamsLabel)
	base := gauge.Value()
	svc := NewTopicService(room.NewManager(100), member.NewManager(), wotan.NewWotan())

	waitGauge := func(want float64) bool {
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			if gauge.Value() == want {
				return true
			}
			time.Sleep(time.Millisecond)
		}
		return gauge.Value() == want
	}

	ctx1, cancel1 := context.WithCancel(context.Background())
	ctx2, cancel2 := context.WithCancel(context.Background())
	done := make(chan error, 2)
	go func() {
		done <- svc.StreamTopics(&chatpb.TopicStreamRequest{TopicPattern: "alerts.critical", DisplayName: "svc-a"}, newMockTopicStream(ctx1))
	}()
	go func() {
		done <- svc.StreamTopics(&chatpb.TopicStreamRequest{TopicPattern: "logs.*", DisplayName: "svc-b"}, newMockTopicStream(ctx2))
	}()
	if !waitGauge(base + 2) {
		t.Fatalf("streams_active{%s} = %v with two topic streams open, want %v", TopicStreamsLabel, gauge.Value(), base+2)
	}

	cancel1()
	<-done
	if !waitGauge(base + 1) {
		t.Errorf("after one stream closed: %v, want %v", gauge.Value(), base+1)
	}
	cancel2()
	<-done
	if !waitGauge(base) {
		t.Errorf("after both closed: %v, want %v", gauge.Value(), base)
	}
}
