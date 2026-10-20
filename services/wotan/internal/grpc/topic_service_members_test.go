// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package grpc

import (
	"context"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"unheaded/pkg/crypto/pqc"
	"unheaded/services/wotan/internal/member"
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
