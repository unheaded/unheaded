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
	"unheaded/services/wotan/internal/wotan"
	chatpb "unheaded/services/wotan/proto"
)

// productionTopicService builds the service exactly as services/wotan/cmd/wotan
// builds it (NewTopicServiceWithCounter). Authorization tests must exercise the
// constructor that actually ships, not a more complete one used only in tests.
func productionTopicService() *TopicService {
	return NewTopicServiceWithCounter(
		room.NewManager(100),
		member.NewManager(),
		wotan.NewWotan(),
		NewTopicSequenceCounter(),
	)
}

// ADR-043 hard condition #2 says config.* publishes require a valid ML-DSA-65
// signature. PublishTopic only verifies when s.topicVerifier != nil, and the
// production constructor never sets it — so on the shipping path any non-empty
// signature and public key are accepted without being checked.
func TestPublishTopic_ConfigTopicSignatureIsActuallyVerified(t *testing.T) {
	svc := productionTopicService()

	if svc.topicVerifier == nil {
		t.Error("production constructor leaves topicVerifier nil: config.* signatures are never verified")
	}

	_, err := svc.PublishTopic(context.Background(), &chatpb.TopicPublishRequest{
		Topic:     "config.kingdom",
		Payload:   []byte(`{"evil":true}`),
		Signature: []byte("not-a-signature"),
		PublicKey: []byte("not-a-key"),
		Algorithm: "ML-DSA-65",
	})
	if err == nil {
		t.Fatal("garbage signature on a config.* topic was accepted")
	}
	if got := status.Code(err); got != codes.PermissionDenied {
		t.Errorf("code = %s, want PermissionDenied (got error: %v)", got, err)
	}
}

// allowList is a test AutoApprover.
type allowList map[string]bool

func (a allowList) IsAutoApproved(displayName string) bool { return a[displayName] }

// PublishTopic checks IsApproved only when sender_id is supplied. When it is
// omitted the else-branch creates a member and approves it, so the check was
// bypassed by leaving the field empty. With an allowlist wired (as cmd/wotan
// does) the anonymous branch must be gated the same way the HTTP path is.
func TestPublishTopic_EmptySenderIsGatedByAllowlist(t *testing.T) {
	svc := productionTopicService()
	svc.AutoApprove = allowList{"service": true}

	// Unnamed caller -> "service", which this allowlist approves.
	if _, err := svc.PublishTopic(context.Background(), &chatpb.TopicPublishRequest{
		Topic:   "alerts.critical",
		Payload: []byte(`{"x":1}`),
	}); err != nil {
		t.Fatalf("allowlisted default publisher rejected: %v", err)
	}

	// A name that is not on the allowlist must be refused, not self-approved.
	_, err := svc.PublishTopic(context.Background(), &chatpb.TopicPublishRequest{
		Topic:    "alerts.critical",
		Payload:  []byte(`{"x":1}`),
		Metadata: map[string]string{"display_name": "intruder"},
	})
	if err == nil {
		t.Fatal("publish from a non-allowlisted name was accepted")
	}
	if got := status.Code(err); got != codes.PermissionDenied {
		t.Errorf("code = %s, want PermissionDenied", got)
	}
}

// With no allowlist configured the gate is open — embedding callers and the
// existing test suite depend on that. cmd/wotan always wires one.
func TestPublishTopic_NoAllowlistKeepsPriorBehaviour(t *testing.T) {
	svc := productionTopicService()
	if _, err := svc.PublishTopic(context.Background(), &chatpb.TopicPublishRequest{
		Topic:    "alerts.critical",
		Payload:  []byte(`{"x":1}`),
		Metadata: map[string]string{"display_name": "anyone"},
	}); err != nil {
		t.Fatalf("nil AutoApprove should not reject: %v", err)
	}
}
