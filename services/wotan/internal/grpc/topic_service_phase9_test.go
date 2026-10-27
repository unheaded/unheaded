// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package grpc

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"unheaded/services/wotan/internal/member"
	"unheaded/services/wotan/internal/room"
	"unheaded/services/wotan/internal/signing"
	"unheaded/services/wotan/internal/wotan"
	chatpb "unheaded/services/wotan/proto"
)

// S-IRON-GATE Phase 9 over gRPC: Akira's signed topics take only a
// signature by the health key, alerts.critical (the prefix's stem) is
// signed, and system.health.reports stays open to unsigned votes.
func TestPublishTopic_Phase9SignedTopics(t *testing.T) {
	healthKey, auditKey := newSigner(t), newSigner(t)
	rooms := room.NewManager(100)
	svc := NewTopicService(rooms, member.NewManager(), wotan.NewWotan())
	p, err := signing.NewPolicy(nil,
		signing.PrefixKeys{Prefix: "system.health.consensus.", Keys: [][]byte{healthKey.PublicKey()}},
		signing.PrefixKeys{Prefix: "audit.", Keys: [][]byte{auditKey.PublicKey()}},
		signing.PrefixKeys{Prefix: "alerts.critical."},
	)
	if err != nil {
		t.Fatal(err)
	}
	svc.SetSigningPolicy(p)
	svc.AutoApprove = allowList{"akira": true}
	ctx := context.Background()

	unsigned := func(topic string) *chatpb.TopicPublishRequest {
		return &chatpb.TopicPublishRequest{Topic: topic, Payload: []byte("x"), Metadata: map[string]string{"display_name": "akira"}}
	}
	if _, err := svc.PublishTopic(ctx, unsigned("system.health.consensus")); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("unsigned consensus: %v, want InvalidArgument", err)
	}
	if _, err := svc.PublishTopic(ctx, unsigned("alerts.critical")); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("unsigned alerts.critical: %v, want InvalidArgument", err)
	}
	if _, err := svc.PublishTopic(ctx, signedReq(t, auditKey, "system.health.consensus", "akira", `{"band":"OK"}`)); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("audit key on consensus: %v, want PermissionDenied", err)
	}
	if _, err := svc.PublishTopic(ctx, signedReq(t, healthKey, "alerts.critical", "akira", "x")); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("alerts.critical with no enrolled key: %v, want PermissionDenied", err)
	}
	if _, err := svc.PublishTopic(ctx, signedReq(t, healthKey, "system.health.consensus", "akira", `{"band":"OK"}`)); err != nil {
		t.Fatalf("health key on consensus: %v", err)
	}
	if _, err := svc.PublishTopic(ctx, unsigned("system.health.reports")); err != nil {
		t.Fatalf("unsigned vote on system.health.reports: %v", err)
	}
}
