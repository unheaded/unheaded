// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package wotanClient

import (
	"context"
	"errors"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"unheaded/pkg/wotansig"
	chatpb "unheaded/services/wotan/proto"
)

// Signed topics (config.*, drift.*: ADR-043 #2, ADR-105) take only ML-DSA-65
// signed messages, published over gRPC. Wotan refuses them over HTTP, so
// these methods never fall back to HTTP and never queue a message for later:
// a refused or failed signed publish is returned to the caller.

// ErrSignedTopicNeedsGRPC: a client with no gRPC endpoint cannot publish to
// a signed topic.
var ErrSignedTopicNeedsGRPC = errors.New("signed topics are published over gRPC only; this client is in HTTP-only mode")

// ErrUnsignedMessage: VerifySignature on a message that carries no signature.
var ErrUnsignedMessage = errors.New("message carries no signature")

// PublishSigned publishes payload to topic signed by signer. The client must
// have joined topic (Subscribe); it publishes under the name it joined with
// (display_name, no sender_id) and signs for that same name, which is the
// sender Wotan verifies and hands subscribers back as sender_id.
func (gc *GRPCClient) PublishSigned(ctx context.Context, topic string, payload []byte, signer *wotansig.TopicSigner) error {
	if topic == "" {
		return errors.New("topic cannot be empty")
	}
	if len(payload) == 0 {
		return errors.New("payload cannot be empty")
	}
	if signer == nil {
		return wotansig.ErrNoSigner
	}

	gc.mu.RLock()
	sub, ok := gc.subscribers[topic]
	gc.mu.RUnlock()
	if !ok {
		return fmt.Errorf("not subscribed to topic %s", topic)
	}
	if sub.Status != "approved" {
		return ErrSubscriptionPending
	}

	sg, err := signer.Sign(topic, sub.DisplayName, payload)
	if err != nil {
		return fmt.Errorf("sign %s: %w", topic, err)
	}
	req := &chatpb.TopicPublishRequest{
		Topic:          topic,
		Payload:        payload,
		Metadata:       publishMetadata(nil, sub.DisplayName),
		Signature:      sg.Signature,
		PublicKey:      sg.PublicKey,
		Algorithm:      sg.Algorithm,
		SignedAtUnixMs: sg.SignedAtUnixMs,
	}
	var trailer metadata.MD
	if _, err := gc.client.PublishTopic(ctx, req, grpc.Trailer(&trailer)); err != nil {
		return fmt.Errorf("publish signed topic: %w", withTrailerCode(err, trailer))
	}
	return nil
}

// PublishSigned publishes payload to topic signed by signer, over gRPC
// under the name this client joined topic with (Subscribe). In HTTP-only
// mode it returns ErrSignedTopicNeedsGRPC.
func (c *Client) PublishSigned(ctx context.Context, topic string, payload []byte, signer *wotansig.TopicSigner) error {
	if c.grpcAddr == "" {
		return ErrSignedTopicNeedsGRPC
	}
	c.mu.RLock()
	sub, ok := c.subscribers[topic]
	c.mu.RUnlock()
	if !ok {
		return fmt.Errorf("not subscribed to topic %s", topic)
	}
	gc, err := c.getOrCreateGRPCClient()
	if err != nil {
		return fmt.Errorf("grpc client: %w", err)
	}
	if _, ok := gc.GetSubscriber(topic); !ok {
		if _, err := gc.Subscribe(ctx, topic, sub.DisplayName); err != nil {
			return err
		}
	}
	return gc.PublishSigned(ctx, topic, payload, signer)
}

// PublishWithSigner publishes payload to topic, signed by signer when the
// topic takes only signed messages (wotansig.RequiresSignature), as Publish
// otherwise. A signed topic with a nil signer is refused with
// wotansig.ErrSignatureRequired and nothing is sent: a producer whose
// signing key is not configured never publishes there unsigned
// (S-IRON-GATE Phase 9).
func (c *Client) PublishWithSigner(ctx context.Context, topic string, payload []byte, signer *wotansig.TopicSigner) error {
	if !wotansig.RequiresSignature(topic) {
		return c.Publish(ctx, topic, payload)
	}
	if signer == nil {
		return wotansig.RefuseUnsigned(topic)
	}
	return c.PublishSigned(ctx, topic, payload, signer)
}

// VerifySignature re-verifies a signed message against the public keys the
// caller trusts for its topic (ADR-105): ML-DSA-65, one of trusted, over
// exactly this topic, sender, signing time and payload.
func (m *Message) VerifySignature(trusted [][]byte) error {
	if m == nil || m.Signed == nil {
		return ErrUnsignedMessage
	}
	return wotansig.VerifyEvent(m.Topic, m.SenderID, []byte(m.Payload), *m.Signed, trusted)
}

// eventSignature is the signature a TopicEvent carries, or nil.
func eventSignature(event *chatpb.TopicEvent) *wotansig.Signed {
	if event == nil || len(event.Signature) == 0 {
		return nil
	}
	return &wotansig.Signed{
		Signature:      event.Signature,
		PublicKey:      event.PublicKey,
		Algorithm:      event.Algorithm,
		SignedAtUnixMs: event.SignedAtUnixMs,
	}
}
