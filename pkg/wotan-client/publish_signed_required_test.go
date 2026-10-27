// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package wotanClient

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"unheaded/pkg/wotansig"
)

// S-IRON-GATE Phase 9: a signed topic (wotansig.SignedTopicPrefixes) is
// never published unsigned. Every unsigned publish path refuses it before
// anything goes on the wire, so a producer without its key gets an error
// instead of a message Wotan refuses (or takes, on a Wotan never told).
var signedTopics = []string{"alerts.critical", "system.health.consensus", "system.health.remediate", "audit.login", "drift.detected.n1", "config.kingdom"}

func TestClient_UnsignedPublishRefusedOnSignedTopics(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()
	c, err := NewClient(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for _, topic := range signedTopics {
		c.subscribers[topic] = &Subscriber{Topic: topic, SubscriberID: "s1", DisplayName: "akira", Status: "approved"}
		if err := c.Publish(ctx, topic, []byte("p")); !errors.Is(err, wotansig.ErrSignatureRequired) {
			t.Errorf("Publish(%s) = %v, want ErrSignatureRequired", topic, err)
		}
		if err := c.PublishWithAck(ctx, topic, []byte("p")); !errors.Is(err, wotansig.ErrSignatureRequired) {
			t.Errorf("PublishWithAck(%s) = %v, want ErrSignatureRequired", topic, err)
		}
	}
	if n := hits.Load(); n != 0 {
		t.Fatalf("%d unsigned publishes to signed topics reached Wotan", n)
	}
	if c.FallbackQueueLen() != 0 {
		t.Fatal("an unsigned message for a signed topic was queued")
	}
	// An unsigned topic still publishes.
	c.subscribers["system.health.reports"] = &Subscriber{Topic: "system.health.reports", SubscriberID: "s1", DisplayName: "akira", Status: "approved"}
	if err := c.Publish(ctx, "system.health.reports", []byte("p")); err != nil {
		t.Fatalf("unsigned topic: %v", err)
	}
	if hits.Load() != 1 {
		t.Fatal("unsigned topic did not reach Wotan")
	}
}

func TestGRPCClient_UnsignedPublishRefusedOnSignedTopics(t *testing.T) {
	cap, gc := setupCaptureGRPC(t)
	ctx := context.Background()
	for _, topic := range signedTopics {
		gc.subscribers[topic] = &Subscriber{Topic: topic, DisplayName: "akira", Status: "approved"}
		if err := gc.Publish(ctx, topic, []byte("p")); !errors.Is(err, wotansig.ErrSignatureRequired) {
			t.Errorf("Publish(%s) = %v, want ErrSignatureRequired", topic, err)
		}
	}
	cap.mu.Lock()
	n := len(cap.reqs)
	cap.mu.Unlock()
	if n != 0 {
		t.Fatalf("%d unsigned publishes to signed topics reached Wotan", n)
	}
}

func TestTopicStreamClient_UnsignedPublishRefusedOnSignedTopics(t *testing.T) {
	c := &TopicStreamClient{subscribers: map[string]*Subscriber{}}
	for _, topic := range signedTopics {
		c.subscribers[topic] = &Subscriber{Topic: topic, DisplayName: "akira", Status: "approved"}
		if err := c.Publish(context.Background(), topic, []byte("p")); !errors.Is(err, wotansig.ErrSignatureRequired) {
			t.Errorf("Publish(%s) = %v, want ErrSignatureRequired", topic, err)
		}
	}
}

// PublishWithSigner signs what must be signed, refuses it without a
// signer, and publishes everything else as usual.
func TestClient_PublishWithSigner(t *testing.T) {
	cap, gc := setupCaptureGRPC(t)
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()
	c, err := NewClient(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	c.grpcAddr = "bufconn"
	c.grpcClient = gc
	ctx := context.Background()
	for _, topic := range []string{"system.health.consensus", "system.health.reports"} {
		c.subscribers[topic] = &Subscriber{Topic: topic, SubscriberID: "s1", DisplayName: "akira", Status: "approved"}
	}

	if err := c.PublishWithSigner(ctx, "system.health.consensus", []byte("p"), nil); !errors.Is(err, wotansig.ErrSignatureRequired) {
		t.Fatalf("signed topic, no signer: %v", err)
	}
	ts := testTopicSigner(t)
	if err := c.PublishWithSigner(ctx, "system.health.consensus", []byte(`{"band":"OK"}`), ts); err != nil {
		t.Fatalf("signed topic: %v", err)
	}
	req := cap.last(t)
	sg := wotansig.Signed{Signature: req.Signature, PublicKey: req.PublicKey, Algorithm: req.Algorithm, SignedAtUnixMs: req.SignedAtUnixMs}
	if err := wotansig.VerifyEvent(req.Topic, "akira", req.Payload, sg, [][]byte{ts.PublicKey()}); err != nil {
		t.Fatalf("signature: %v", err)
	}
	if hits.Load() != 0 {
		t.Fatal("a signed topic went over HTTP")
	}
	// Unsigned topics go the usual way, with or without a signer.
	if err := c.PublishWithSigner(ctx, "system.health.reports", []byte("p"), ts); err != nil {
		t.Fatalf("unsigned topic: %v", err)
	}
	if hits.Load() != 1 {
		t.Fatal("unsigned topic did not go over the usual path")
	}
}
