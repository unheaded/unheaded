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
	"unheaded/pkg/wotansig"
	"unheaded/services/wotan/internal/member"
	"unheaded/services/wotan/internal/room"
	"unheaded/services/wotan/internal/signing"
	"unheaded/services/wotan/internal/wotan"
	chatpb "unheaded/services/wotan/proto"
)

// driftFixture is a Wotan topic service trusting cfgKey for config.* and
// driftKey for drift.* (ADR-105), with "heimdall-n1" on the allowlist.
type driftFixture struct {
	svc      *TopicService
	members  *member.Manager
	rooms    *room.Manager
	cfgKey   *wotansig.TopicSigner
	driftKey *wotansig.TopicSigner
}

func newSigner(t *testing.T) *wotansig.TopicSigner {
	t.Helper()
	_, s, err := pqc.GenerateMLDSAKeyPair(pqc.MLDSA65)
	if err != nil {
		t.Fatal(err)
	}
	return wotansig.NewTopicSigner(s)
}

func newDriftFixture(t *testing.T) *driftFixture {
	t.Helper()
	f := &driftFixture{members: member.NewManager(), rooms: room.NewManager(100), cfgKey: newSigner(t), driftKey: newSigner(t)}
	f.svc = NewTopicService(f.rooms, f.members, wotan.NewWotan(), f.cfgKey.PublicKey())
	p, err := signing.NewPolicy([][]byte{f.cfgKey.PublicKey()}, signing.PrefixKeys{Prefix: "drift.", Keys: [][]byte{f.driftKey.PublicKey()}})
	if err != nil {
		t.Fatal(err)
	}
	f.svc.SetSigningPolicy(p)
	f.svc.AutoApprove = allowList{"heimdall-n1": true, "service": true}
	return f
}

// signedReq is a publish as Heimdall makes it: no sender_id, display_name
// metadata, and a signature whose sender is that display name.
func signedReq(t *testing.T, ts *wotansig.TopicSigner, topic, name, payload string) *chatpb.TopicPublishRequest {
	t.Helper()
	sg, err := ts.Sign(topic, name, []byte(payload))
	if err != nil {
		t.Fatal(err)
	}
	return &chatpb.TopicPublishRequest{
		Topic: topic, Payload: []byte(payload), Metadata: map[string]string{"display_name": name},
		Signature: sg.Signature, PublicKey: sg.PublicKey, Algorithm: sg.Algorithm, SignedAtUnixMs: sg.SignedAtUnixMs,
	}
}

func TestPublishTopic_DriftUnsignedRefused(t *testing.T) {
	f := newDriftFixture(t)
	_, err := f.svc.PublishTopic(context.Background(), &chatpb.TopicPublishRequest{
		Topic: "drift.detected.n1", Payload: []byte("x"), Metadata: map[string]string{"display_name": "heimdall-n1"},
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("unsigned drift.*: %v, want InvalidArgument", err)
	}
	if _, err := f.rooms.Get("drift.detected.n1"); err == nil {
		t.Fatal("a refused publish created the room")
	}
}

// Keys are scoped per prefix (ADR-043 #4): a config.* key cannot sign
// drift.*, and Heimdall's drift key cannot sign config.*.
func TestPublishTopic_DriftKeysAreScoped(t *testing.T) {
	f := newDriftFixture(t)
	if _, err := f.svc.PublishTopic(context.Background(), signedReq(t, f.cfgKey, "drift.detected.n1", "heimdall-n1", "x")); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("drift.* signed by the config key: %v, want PermissionDenied", err)
	}
	if _, err := f.svc.PublishTopic(context.Background(), signedReq(t, f.driftKey, "config.kingdom", "heimdall-n1", "x")); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("config.* signed by the drift key: %v, want PermissionDenied", err)
	}
	if _, err := f.svc.PublishTopic(context.Background(), signedReq(t, f.driftKey, "drift.detected.n1", "heimdall-n1", "x")); err != nil {
		t.Fatalf("drift.* signed by the drift key: %v", err)
	}
}

// The signature is verified over the stored copy on a standby (FND-020),
// and the stored copy is a string: a payload that is not valid UTF-8 would
// be rewritten by the encoding and never verify again. It is refused at
// publish, before the signature is even checked.
func TestPublishTopic_SignedNonUTF8PayloadRefused(t *testing.T) {
	f := newDriftFixture(t)
	raw := []byte{0xff, 0xfe, 'x'}
	sg, err := f.driftKey.Sign("drift.detected.n1", "heimdall-n1", raw)
	if err != nil {
		t.Fatal(err)
	}
	req := &chatpb.TopicPublishRequest{
		Topic: "drift.detected.n1", Payload: raw, Metadata: map[string]string{"display_name": "heimdall-n1"},
		Signature: sg.Signature, PublicKey: sg.PublicKey, Algorithm: sg.Algorithm, SignedAtUnixMs: sg.SignedAtUnixMs,
	}
	if _, err := f.svc.PublishTopic(context.Background(), req); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("signed drift.* with a non-UTF-8 payload: %v, want InvalidArgument", err)
	}
	if _, err := f.rooms.Get("drift.detected.n1"); err == nil {
		t.Fatal("a refused publish created the room")
	}
	// An unsigned topic keeps taking any bytes.
	if _, err := f.svc.PublishTopic(context.Background(), &chatpb.TopicPublishRequest{
		Topic: "logs.n1", Payload: raw, Metadata: map[string]string{"display_name": "heimdall-n1"},
	}); err != nil {
		t.Fatalf("unsigned logs.* with a non-UTF-8 payload: %v, want nil", err)
	}
}

func TestPublishTopic_DriftReplayRefused(t *testing.T) {
	f := newDriftFixture(t)
	req := signedReq(t, f.driftKey, "drift.detected.n1", "heimdall-n1", "x")
	if _, err := f.svc.PublishTopic(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.PublishTopic(context.Background(), req); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("replay: %v, want PermissionDenied", err)
	}
}

// With no drift. keys configured (the shipped config), every drift.*
// publish is refused, however it is signed.
func TestPublishTopic_DriftWithoutKeysFailsClosed(t *testing.T) {
	ts := newSigner(t)
	svc := NewTopicService(room.NewManager(100), member.NewManager(), wotan.NewWotan(), ts.PublicKey())
	svc.AutoApprove = allowList{"heimdall-n1": true}
	if _, err := svc.PublishTopic(context.Background(), signedReq(t, ts, "drift.detected.n1", "heimdall-n1", "x")); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("drift.* with no drift keys: %v, want PermissionDenied", err)
	}
	nilPolicy := NewTopicService(room.NewManager(100), member.NewManager(), wotan.NewWotan())
	nilPolicy.SetSigningPolicy(nil)
	if _, err := nilPolicy.PublishTopic(context.Background(), signedReq(t, ts, "drift.detected.n1", "service", "x")); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("nil policy: %v, want PermissionDenied", err)
	}
}

// The signature over a display-name publish covers that name: one made for
// another name (or for none, as before ADR-105) does not verify.
func TestPublishTopic_SignedSenderIsTheDisplayName(t *testing.T) {
	f := newDriftFixture(t)
	req := signedReq(t, f.driftKey, "drift.detected.n1", "service", "x")
	req.Metadata = map[string]string{"display_name": "heimdall-n1"}
	if _, err := f.svc.PublishTopic(context.Background(), req); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("signed as service, published as heimdall-n1: %v, want PermissionDenied", err)
	}
	req = signedReq(t, f.driftKey, "drift.detected.n1", "", "y")
	req.Metadata = map[string]string{"display_name": "heimdall-n1"}
	if _, err := f.svc.PublishTopic(context.Background(), req); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("signed for an empty sender: %v, want PermissionDenied", err)
	}
}

// collect streams pattern until want events arrive (or 3 s pass).
func collect(t *testing.T, svc *TopicService, pattern string, want int, during func()) []*chatpb.TopicEvent {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	stream := newMockTopicStream(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = svc.StreamTopics(&chatpb.TopicStreamRequest{TopicPattern: pattern, SinceSeq: 0}, stream)
	}()
	waitFor := func(n int) {
		for ctx.Err() == nil {
			got := 0
			for _, e := range stream.getEvents() {
				if e.Type == chatpb.TopicEventType_TOPIC_MESSAGE_PUBLISHED {
					got++
				}
			}
			if got >= n {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	waitFor(want - 1)
	if during != nil {
		during()
	}
	waitFor(want)
	cancel()
	<-done
	var out []*chatpb.TopicEvent
	for _, e := range stream.getEvents() {
		if e.Type == chatpb.TopicEventType_TOPIC_MESSAGE_PUBLISHED {
			out = append(out, e)
		}
	}
	if len(out) < want {
		t.Fatalf("got %d published events, want %d", len(out), want)
	}
	return out
}

// eventSigned is what a subscriber reads off a TopicEvent to re-verify it.
func eventSigned(ev *chatpb.TopicEvent) wotansig.Signed {
	return wotansig.Signed{Signature: ev.Signature, PublicKey: ev.PublicKey, Algorithm: ev.Algorithm, SignedAtUnixMs: ev.SignedAtUnixMs}
}

// A subscriber gets the signature, key, algorithm and signing time with
// every signed message, replayed or live, and can verify it with nothing
// but the trusted drift key: Wotan did not have to be trusted to relay it
// faithfully.
func TestStreamTopics_DriftEventsCarryVerifiableSignatures(t *testing.T) {
	f := newDriftFixture(t)
	first := signedReq(t, f.driftKey, "drift.detected.n1", "heimdall-n1", `{"path":"/etc/a"}`)
	if _, err := f.svc.PublishTopic(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	second := signedReq(t, f.driftKey, "drift.detected.n1", "heimdall-n1", `{"path":"/etc/b"}`)
	events := collect(t, f.svc, "drift.#", 2, func() {
		if _, err := f.svc.PublishTopic(context.Background(), second); err != nil {
			t.Error(err)
		}
	})
	trusted := [][]byte{f.driftKey.PublicKey()}
	for i, ev := range events[:2] {
		if ev.SenderId != "heimdall-n1" {
			t.Errorf("event %d sender_id = %q, want the signed sender heimdall-n1", i, ev.SenderId)
		}
		if ev.Algorithm != wotansig.Algorithm || ev.SignedAtUnixMs == 0 || len(ev.Signature) == 0 || len(ev.PublicKey) == 0 {
			t.Fatalf("event %d missing signature fields: alg=%q at=%d", i, ev.Algorithm, ev.SignedAtUnixMs)
		}
		if err := wotansig.VerifyEvent(ev.Topic, ev.SenderId, ev.Payload, eventSigned(ev), trusted); err != nil {
			t.Errorf("event %d does not verify: %v", i, err)
		}
		// A relay that edits the payload is caught.
		if err := wotansig.VerifyEvent(ev.Topic, ev.SenderId, append(ev.Payload, ' '), eventSigned(ev), trusted); err == nil {
			t.Errorf("event %d: edited payload verified", i)
		}
	}
	if events[0].SignedAtUnixMs != first.SignedAtUnixMs || events[1].SignedAtUnixMs != second.SignedAtUnixMs {
		t.Error("signed_at on the events is not the publisher's")
	}
}

// config.* published with an explicit sender_id: the event's sender_id is
// exactly the string that was signed, so it verifies too.
func TestStreamTopics_ConfigEventsCarryVerifiableSignatures(t *testing.T) {
	f := newDriftFixture(t)
	m, _ := f.members.RequestJoin("config.kingdom", "operator", "", time.Hour)
	if err := f.members.Approve(m.ID, "admin"); err != nil {
		t.Fatal(err)
	}
	sg, err := f.cfgKey.Sign("config.kingdom", m.ID.String(), []byte(`{"v":1}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.PublishTopic(context.Background(), &chatpb.TopicPublishRequest{
		Topic: "config.kingdom", SenderId: m.ID.String(), Payload: []byte(`{"v":1}`),
		Signature: sg.Signature, PublicKey: sg.PublicKey, Algorithm: sg.Algorithm, SignedAtUnixMs: sg.SignedAtUnixMs,
	}); err != nil {
		t.Fatal(err)
	}
	ev := collect(t, f.svc, "config.*", 1, nil)[0]
	if ev.SenderId != m.ID.String() {
		t.Fatalf("sender_id = %q, want %q", ev.SenderId, m.ID.String())
	}
	if err := wotansig.VerifyEvent(ev.Topic, ev.SenderId, ev.Payload, eventSigned(ev), [][]byte{f.cfgKey.PublicKey()}); err != nil {
		t.Fatalf("config event does not verify: %v", err)
	}
}

// Unsigned topics are delivered as before: no signature fields.
func TestStreamTopics_UnsignedEventsHaveNoSignature(t *testing.T) {
	f := newDriftFixture(t)
	if _, err := f.svc.PublishTopic(context.Background(), &chatpb.TopicPublishRequest{
		Topic: "logs.n1", Payload: []byte("x"), Metadata: map[string]string{"display_name": "service"},
	}); err != nil {
		t.Fatal(err)
	}
	ev := collect(t, f.svc, "logs.*", 1, nil)[0]
	if len(ev.Signature) != 0 || len(ev.PublicKey) != 0 || ev.Algorithm != "" || ev.SignedAtUnixMs != 0 {
		t.Fatalf("unsigned event carries signature fields: %+v", ev)
	}
	if ev.SenderId == "" || ev.SenderId == "service" {
		t.Fatalf("unsigned event sender_id = %q, want the member id", ev.SenderId)
	}
}
