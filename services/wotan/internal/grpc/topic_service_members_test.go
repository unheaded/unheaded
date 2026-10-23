// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package grpc

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"unheaded/pkg/crypto/pqc"
	"unheaded/services/wotan/internal/member"
	"unheaded/services/wotan/internal/metrics"
	"unheaded/services/wotan/internal/ringbuffer"
	"unheaded/services/wotan/internal/room"
	"unheaded/services/wotan/internal/signing"
	"unheaded/services/wotan/internal/wotan"
	chatpb "unheaded/services/wotan/proto"
)

// Both Wotan gRPC clients publish without a sender_id, so every log line and
// trace batch takes the unnamed-publisher path. That path created and
// approved a new member per publish, and only pending members are ever
// cleaned up: members grew by one per message for as long as Wotan ran.
// One member per (topic, publisher name) is enough.
func TestPublishTopic_UnnamedPublisherReusesItsMember(t *testing.T) {
	members := member.NewManager()
	svc := NewTopicService(room.NewManager(100), members, wotan.NewWotan())
	svc.AutoApprove = allowList{"service": true, "trace-collector": true}

	publish := func(topic, name string) {
		t.Helper()
		req := &chatpb.TopicPublishRequest{Topic: topic, Payload: []byte(`{"x":1}`)}
		if name != "" {
			req.Metadata = map[string]string{"display_name": name}
		}
		if _, err := svc.PublishTopic(context.Background(), req); err != nil {
			t.Fatalf("publish %s as %q: %v", topic, name, err)
		}
	}
	for i := 0; i < 1000; i++ {
		publish("logs.wotan.info", "")
	}
	if got := members.Count(); got != 1 {
		t.Fatalf("1000 unnamed publishes to one topic left %d members, want 1", got)
	}
	publish("logs.wotan.info", "trace-collector")
	publish("traces.flow", "trace-collector")
	publish("traces.flow", "trace-collector")
	if got := members.Count(); got != 3 {
		t.Fatalf("members = %d, want 3: one per (topic, publisher name)", got)
	}
}

// LICH-012 L12b (verifier flooding): trusted public keys are public, so a
// caller can pair one with a fresh timestamp and garbage bytes and reach the
// ML-DSA verify every time. A publisher the allowlist refuses must be turned
// away before that work, not after it.
func TestPublishTopic_UnapprovedPublisherRefusedBeforeSignatureCheck(t *testing.T) {
	_, trustedKey, err := pqc.GenerateMLDSAKeyPair(pqc.MLDSA65)
	if err != nil {
		t.Fatal(err)
	}
	svc := NewTopicService(room.NewManager(100), member.NewManager(), wotan.NewWotan(), trustedKey.PublicKey())
	svc.AutoApprove = allowList{"service": true}

	_, err = svc.PublishTopic(context.Background(), &chatpb.TopicPublishRequest{
		Topic:          "config.kingdom",
		Payload:        []byte(`{"evil":true}`),
		Signature:      make([]byte, 3309), // ML-DSA-65 signature length, all zero
		PublicKey:      trustedKey.PublicKey(),
		Algorithm:      signing.Algorithm,
		SignedAtUnixMs: time.Now().UnixMilli(),
		Metadata:       map[string]string{"display_name": "intruder"},
	})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("err = %v, want PermissionDenied", err)
	}
	if strings.Contains(err.Error(), "signature") {
		t.Fatalf("refused by the signature check (%v): the ML-DSA verify ran for a publisher that was never allowed", err)
	}
}

// Each StreamTopics call made an approved member under a room name unique to
// that stream and never removed it, so every reconnect (the clients retry in
// a loop) left a member and a byRoom key behind for good. The member exists
// for the stream and must go with it.
func TestStreamTopics_MemberLeavesWithTheStream(t *testing.T) {
	members := member.NewManager()
	svc := NewTopicService(room.NewManager(100), members, wotan.NewWotan())
	for i := 0; i < 50; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() {
			done <- svc.StreamTopics(&chatpb.TopicStreamRequest{TopicPattern: "logs.*", DisplayName: "dashboard"}, newMockTopicStream(ctx))
		}()
		deadline := time.Now().Add(2 * time.Second)
		for members.Count() == 0 && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
		if got := members.Count(); got != 1 {
			t.Fatalf("stream %d open: members = %d, want 1", i, got)
		}
		cancel()
		<-done
		if got := members.Count(); got != 0 {
			t.Fatalf("stream %d closed: members = %d, want 0", i, got)
		}
	}
}

// Past the room cap, a publish naming a new topic is refused with
// ResourceExhausted; existing topics keep working.
func TestPublishTopic_RoomCap(t *testing.T) {
	rooms := room.NewManager(100)
	rooms.SetMaxRooms(3)
	svc := NewTopicService(rooms, member.NewManager(), wotan.NewWotan())
	pub := func(topic string) error {
		_, err := svc.PublishTopic(context.Background(), &chatpb.TopicPublishRequest{Topic: topic, Payload: []byte("x")})
		return err
	}
	for _, topic := range []string{"t1", "t2", "t3"} {
		if err := pub(topic); err != nil {
			t.Fatalf("%s: %v", topic, err)
		}
	}
	if err := pub("t4"); status.Code(err) != codes.ResourceExhausted {
		t.Fatalf("fourth topic: %v, want ResourceExhausted", err)
	}
	if err := pub("t1"); err != nil {
		t.Fatalf("existing topic at the cap: %v", err)
	}
}

// A forwarder drops an event when the stream's fan-in buffer is full (the
// client reads slower than the topic is written). Those drops were not
// counted anywhere.
func TestForwardTopicEvents_CountsDropsWhenTheStreamIsFull(t *testing.T) {
	svc := NewTopicService(room.NewManager(100), member.NewManager(), wotan.NewWotan())
	dropped := metrics.Get().StreamErrors.WithLabelValues("fwd.drop", "stream_buffer_full")
	before := dropped.Value()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sub := svc.wotan.Subscribe("fwd.drop", uuid.New(), 10)
	defer svc.wotan.Unsubscribe(sub)
	full := make(chan *topicEvent) // nobody reads: every send finds it full
	done := make(chan struct{})
	go func() { defer close(done); svc.forwardTopicEvents(ctx, "fwd.drop", sub, full) }()

	for i := 0; i < 3; i++ {
		svc.wotan.PublishMessageCreated(&ringbuffer.Message{ID: uuid.New(), RoomID: "fwd.drop", Timestamp: time.Now()})
	}
	deadline := time.Now().Add(2 * time.Second)
	for dropped.Value()-before < 3 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := dropped.Value() - before; got != 3 {
		t.Fatalf(`stream_errors_total{error_type="stream_buffer_full"} rose by %v, want 3`, got)
	}
	cancel()
	<-done
}

// StreamTopics closed its fan-in channel (the last defer, so the first to
// run) while the per-topic forwarders were still running: ctx was cancelled
// only by the first defer, which runs last. A forwarder holding an event then
// sent on the closed channel and panicked, which no gRPC recovery catches in
// a goroutine: the whole of Wotan went down. A stream that ends on a failed
// Send (the client went away mid-write) on a busy topic was enough. Teardown
// must stop every goroutine before it unsubscribes, and leave no
// subscription behind.
func TestStreamTopics_TeardownWhileTopicIsBusy(t *testing.T) {
	svc := NewTopicService(room.NewManager(100), member.NewManager(), wotan.NewWotan())
	if _, err := svc.roomManager.Create("busy.topic", "busy.topic"); err != nil {
		t.Fatal(err)
	}

	stop := make(chan struct{})
	publishing := make(chan struct{})
	go func() {
		defer close(publishing)
		for {
			select {
			case <-stop:
				return
			default:
			}
			svc.wotan.PublishMessageCreated(&ringbuffer.Message{
				ID: uuid.New(), RoomID: "busy.topic", Content: "x", Timestamp: time.Now(),
			})
		}
	}()
	defer func() { close(stop); <-publishing }()

	for i := 0; i < 200; i++ {
		stream := newMockTopicStream(context.Background())
		stream.sendFunc = func(*chatpb.TopicEvent) error { return errors.New("client went away") }
		err := svc.StreamTopics(&chatpb.TopicStreamRequest{TopicPattern: "busy.*", SinceSeq: -1}, stream)
		if status.Code(err) != codes.Aborted {
			t.Fatalf("stream %d: %v, want Aborted", i, err)
		}
		if got := svc.wotan.GetTotalSubscriptions(); got != 0 {
			t.Fatalf("stream %d ended: %d subscriptions left, want 0", i, got)
		}
	}
	// Give any forwarder still running time to touch the closed channel.
	time.Sleep(50 * time.Millisecond)
}
