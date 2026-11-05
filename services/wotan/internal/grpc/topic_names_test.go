// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package grpc

import (
	"context"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	chatpb "unheaded/services/wotan/proto"
)

// L15: gRPC holds a topic name to the rule HTTP does: it could otherwise
// make rooms no HTTP client can name.
func TestPublishTopic_NameRule(t *testing.T) {
	f := newDriftFixture(t)
	for _, topic := range []string{"has space", "semi;colon", "slash/ed", "alerts.*", "alerts.#", strings.Repeat("a", 256)} {
		_, err := f.svc.PublishTopic(context.Background(), &chatpb.TopicPublishRequest{Topic: topic, Payload: []byte("p")})
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("topic %.20q = %v, want InvalidArgument", topic, err)
		}
	}
	if _, err := f.svc.PublishTopic(context.Background(), &chatpb.TopicPublishRequest{Topic: "alerts.critical", Payload: []byte("p")}); err != nil {
		t.Errorf("a good topic: %v", err)
	}
}
