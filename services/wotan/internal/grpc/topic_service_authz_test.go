// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package grpc

import (
	"context"
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

// productionTopicService builds the service exactly as services/wotan/cmd/wotan
// builds it (NewTopicService). Authorization tests must exercise the
// constructor that actually ships, not a more complete one used only in tests.
func productionTopicService() *TopicService {
	return NewTopicService(
		room.NewManager(100),
		member.NewManager(),
		wotan.NewWotan(),
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

// TestPublishTopic_SelfSignedConfigRefused: a correctly signed config.*
// message is refused when the key is the sender's own rather than one the
// operator trusts. Before, the verifier took the key from the request, so
// this was accepted.
func TestPublishTopic_SelfSignedConfigRefused(t *testing.T) {
	_, signer, err := pqc.GenerateMLDSAKeyPair(pqc.MLDSA65)
	if err != nil {
		t.Fatal(err)
	}
	ts := signing.NewTopicSigner(signer)
	sg, err := ts.Sign("config.kingdom", "", []byte(`{"evil":true}`))
	if err != nil {
		t.Fatal(err)
	}
	pub := sg.PublicKey
	req := &chatpb.TopicPublishRequest{
		Topic: "config.kingdom", Payload: []byte(`{"evil":true}`),
		Signature: sg.Signature, PublicKey: pub, Algorithm: sg.Algorithm, SignedAtUnixMs: sg.SignedAtUnixMs,
	}
	if _, err := productionTopicService().PublishTopic(context.Background(), req); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("config.* publish with no trusted keys: %v, want PermissionDenied", err)
	}
	_, operator, err := pqc.GenerateMLDSAKeyPair(pqc.MLDSA65)
	if err != nil {
		t.Fatal(err)
	}
	other := NewTopicService(room.NewManager(100), member.NewManager(), wotan.NewWotan(), operator.PublicKey())
	if _, err := other.PublishTopic(context.Background(), req); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("self-signed config.* publish to a Wotan trusting another key: %v, want PermissionDenied", err)
	}

	// Trusted, it goes through.
	trusted := NewTopicService(room.NewManager(100), member.NewManager(), wotan.NewWotan(), pub)
	if _, err := trusted.PublishTopic(context.Background(), req); err != nil {
		t.Fatalf("publish signed by a trusted key: %v", err)
	}
	// The same signed request again is a replay.
	if _, err := trusted.PublishTopic(context.Background(), req); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("replayed config.* publish: %v, want PermissionDenied", err)
	}
}

// TestPublishTopic_RefusedSignedPublishCanBeRetried: a signed publish
// refused because its sender is not approved yet does not burn the
// signature, and a refused publish leaves no room behind.
func TestPublishTopic_RefusedSignedPublishCanBeRetried(t *testing.T) {
	_, signer, err := pqc.GenerateMLDSAKeyPair(pqc.MLDSA65)
	if err != nil {
		t.Fatal(err)
	}
	members := member.NewManager()
	rooms := room.NewManager(100)
	svc := NewTopicService(rooms, members, wotan.NewWotan(), signer.PublicKey())
	pending := members.RequestJoin("config.kingdom", "operator", "", time.Hour)

	sg, err := signing.NewTopicSigner(signer).Sign("config.kingdom", pending.ID.String(), []byte(`{"v":1}`))
	if err != nil {
		t.Fatal(err)
	}
	req := &chatpb.TopicPublishRequest{
		Topic: "config.kingdom", SenderId: pending.ID.String(), Payload: []byte(`{"v":1}`),
		Signature: sg.Signature, PublicKey: sg.PublicKey, Algorithm: sg.Algorithm, SignedAtUnixMs: sg.SignedAtUnixMs,
	}
	if _, err := svc.PublishTopic(context.Background(), req); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("unapproved sender: %v", err)
	}
	if _, err := rooms.Get("config.kingdom"); err == nil {
		t.Fatal("a refused publish created the room")
	}
	if err := members.Approve(pending.ID, "admin"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.PublishTopic(context.Background(), req); err != nil {
		t.Fatalf("same signed request after approval: %v", err)
	}
	if _, err := svc.PublishTopic(context.Background(), req); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("third time is a replay: %v", err)
	}
}
