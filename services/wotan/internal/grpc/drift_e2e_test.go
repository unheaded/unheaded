// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package grpc

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	wotanClient "unheaded/pkg/wotan-client"
	"unheaded/pkg/wotansig"
	"unheaded/services/wotan/internal/member"
	"unheaded/services/wotan/internal/room"
	"unheaded/services/wotan/internal/signing"
	"unheaded/services/wotan/internal/wotan"
	chatpb "unheaded/services/wotan/proto"
)

// driftWotan is a real gRPC Wotan topic service on a loopback port in this
// process, trusting driftKey for drift.* and cfgKey for config.*, with
// Heimdall's display name allowlisted. pkg/wotan-client's NewGRPCClient
// dials an address and takes no dialer, so the test listens on 127.0.0.1:0
// rather than bufconn; nothing outside the test process can reach it.
type driftWotan struct {
	addr     string
	raw      chatpb.TopicStreamClient
	driftKey *wotansig.TopicSigner
	cfgKey   *wotansig.TopicSigner
}

func startDriftWotan(t *testing.T) *driftWotan {
	t.Helper()
	d := &driftWotan{driftKey: newSigner(t), cfgKey: newSigner(t)}
	svc := NewTopicService(room.NewManager(100), member.NewManager(), wotan.NewWotan(), d.cfgKey.PublicKey())
	p, err := signing.NewPolicy([][]byte{d.cfgKey.PublicKey()}, signing.PrefixKeys{Prefix: "drift.", Keys: [][]byte{d.driftKey.PublicKey()}})
	if err != nil {
		t.Fatal(err)
	}
	svc.SetSigningPolicy(p)
	svc.AutoApprove = allowList{"heimdall": true, "trace-collector": true}

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := grpc.NewServer()
	chatpb.RegisterTopicStreamServer(srv, svc)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	d.addr = lis.Addr().String()

	conn, err := grpc.NewClient(d.addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	d.raw = chatpb.NewTopicStreamClient(conn)
	return d
}

// heimdallPublisher is a wotan-client joined to topic as Heimdall's drain
// joins it.
func (d *driftWotan) heimdallPublisher(t *testing.T, topic string) *wotanClient.GRPCClient {
	t.Helper()
	gc, err := wotanClient.NewGRPCClient(d.addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = gc.Close() })
	if _, err := gc.Subscribe(context.Background(), topic, "heimdall"); err != nil {
		t.Fatal(err)
	}
	return gc
}

// TestDriftE2E_SubscriberReverifies: Heimdall's client publishes a signed
// drift event; a subscriber on another connection receives it and verifies
// it with only the trusted drift public key, from the event's own fields
// (signed_at_unix_ms included).
func TestDriftE2E_SubscriberReverifies(t *testing.T) {
	d := startDriftWotan(t)
	topic := "drift.detected.east"
	body := []byte(`{"node_id":"east","path":"/etc/ssh/sshd_config","kind":"modified"}`)
	if err := d.heimdallPublisher(t, topic).PublishSigned(context.Background(), topic, body, d.driftKey); err != nil {
		t.Fatalf("PublishSigned: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	stream, err := d.raw.StreamTopics(ctx, &chatpb.TopicStreamRequest{TopicPattern: "drift.detected.*", DisplayName: "auditor", SinceSeq: 0})
	if err != nil {
		t.Fatal(err)
	}
	var ev *chatpb.TopicEvent
	for ev == nil {
		e, err := stream.Recv()
		if err != nil {
			t.Fatalf("recv: %v", err)
		}
		if e.Type == chatpb.TopicEventType_TOPIC_MESSAGE_PUBLISHED {
			ev = e
		}
	}
	if ev.SignedAtUnixMs == 0 || ev.Algorithm != wotansig.Algorithm || ev.SenderId != "heimdall" {
		t.Fatalf("event lacks what a subscriber needs: at=%d alg=%q sender=%q", ev.SignedAtUnixMs, ev.Algorithm, ev.SenderId)
	}
	sg := wotansig.Signed{Signature: ev.Signature, PublicKey: ev.PublicKey, Algorithm: ev.Algorithm, SignedAtUnixMs: ev.SignedAtUnixMs}
	if err := wotansig.VerifyEvent(ev.Topic, ev.SenderId, ev.Payload, sg, [][]byte{d.driftKey.PublicKey()}); err != nil {
		t.Fatalf("subscriber re-verification: %v", err)
	}
	if err := wotansig.VerifyEvent(ev.Topic, ev.SenderId, ev.Payload, sg, [][]byte{d.cfgKey.PublicKey()}); err == nil {
		t.Fatal("verified against the config key: keys are not scoped")
	}

	// The same through wotan-client, as a Go subscriber reads it.
	sub, err := wotanClient.NewGRPCClient(d.addr)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	if _, err := sub.Subscribe(ctx, topic, "auditor"); err != nil {
		t.Fatal(err)
	}
	msgs, err := sub.GetMessages(ctx, topic, 0, 1)
	if err != nil || len(msgs) != 1 {
		t.Fatalf("GetMessages: %d, %v", len(msgs), err)
	}
	if err := msgs[0].VerifySignature([][]byte{d.driftKey.PublicKey()}); err != nil {
		t.Fatalf("wotan-client Message.VerifySignature: %v", err)
	}
}

// signedDriftReq is a signed publish request as Heimdall sends it.
func signedDriftReq(t *testing.T, ts *wotansig.TopicSigner, topic, name string, payload []byte) *chatpb.TopicPublishRequest {
	t.Helper()
	sg, err := ts.Sign(topic, name, payload)
	if err != nil {
		t.Fatal(err)
	}
	return &chatpb.TopicPublishRequest{
		Topic: topic, Payload: payload, Metadata: map[string]string{"display_name": name},
		Signature: sg.Signature, PublicKey: sg.PublicKey, Algorithm: sg.Algorithm, SignedAtUnixMs: sg.SignedAtUnixMs,
	}
}

// wantCode also requires a refusal to come from the signature check (the
// error names it), so a test cannot pass because the allowlist refused.
func wantCode(t *testing.T, what string, err error, code codes.Code) {
	t.Helper()
	if status.Code(err) != code {
		t.Errorf("%s: %v, want %s", what, err, code)
		return
	}
	if !strings.Contains(status.Convert(err).Message(), "signature") {
		t.Errorf("%s: refused for another reason: %v", what, err)
	}
}

// TestLICH012_L12b_ForgedDriftRefused: LICH-012 L12b against drift.*. An
// attacker who can reach Wotan's gRPC port and is on the allowlist still
// cannot put a drift event on the bus that subscribers would believe.
func TestLICH012_L12b_ForgedDriftRefused(t *testing.T) {
	d := startDriftWotan(t)
	ctx := context.Background()
	topic := "drift.detected.east"
	body := []byte(`{"path":"/etc/shadow","kind":"modified"}`)
	attacker := newSigner(t)

	// Forged: signed by the attacker's key, attacker's key attached.
	wantCode(t, "forged (attacker key)", errOf(d.raw.PublishTopic(ctx, signedDriftReq(t, attacker, topic, "heimdall", body))), codes.PermissionDenied)

	// Forged: attacker's signature with the trusted key attached.
	forged := signedDriftReq(t, attacker, topic, "heimdall", body)
	forged.PublicKey = d.driftKey.PublicKey()
	wantCode(t, "forged (trusted key attached)", errOf(d.raw.PublishTopic(ctx, forged)), codes.PermissionDenied)

	// Unsigned.
	wantCode(t, "unsigned", errOf(d.raw.PublishTopic(ctx, &chatpb.TopicPublishRequest{
		Topic: topic, Payload: body, Metadata: map[string]string{"display_name": "heimdall"},
	})), codes.InvalidArgument)

	// Signed by the config.* key: not trusted for drift.*.
	wantCode(t, "config key on drift", errOf(d.raw.PublishTopic(ctx, signedDriftReq(t, d.cfgKey, topic, "heimdall", body))), codes.PermissionDenied)
}

// TestLICH012_L12b_ReplayedDriftRefused: a genuine signed drift event,
// captured off the wire, cannot be sent again, moved to another topic, or
// sent under another name.
func TestLICH012_L12b_ReplayedDriftRefused(t *testing.T) {
	d := startDriftWotan(t)
	ctx := context.Background()
	topic := "drift.detected.east"
	genuine := signedDriftReq(t, d.driftKey, topic, "heimdall", []byte(`{"path":"/etc/a"}`))
	wire, err := proto.Marshal(genuine)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.raw.PublishTopic(ctx, genuine); err != nil {
		t.Fatalf("genuine publish: %v", err)
	}

	// The same bytes again.
	replay := &chatpb.TopicPublishRequest{}
	if err := proto.Unmarshal(wire, replay); err != nil {
		t.Fatal(err)
	}
	wantCode(t, "replayed bytes", errOf(d.raw.PublishTopic(ctx, replay)), codes.PermissionDenied)

	// Each variant uses a fresh, never-accepted signature so the replay
	// cache cannot be what refuses it: the signature's coverage must.
	fresh := func() *chatpb.TopicPublishRequest {
		return signedDriftReq(t, d.driftKey, topic, "heimdall", []byte(`{"path":"/etc/b"}`))
	}
	// Onto config.*: the topic is covered by the signature, and the drift
	// key is not trusted there anyway.
	r := fresh()
	r.Topic = "config.kingdom"
	wantCode(t, "drift signature on config.*", errOf(d.raw.PublishTopic(ctx, r)), codes.PermissionDenied)
	// Onto another node's drift topic.
	r = fresh()
	r.Topic = "drift.detected.west"
	wantCode(t, "drift signature on another node", errOf(d.raw.PublishTopic(ctx, r)), codes.PermissionDenied)
	// Under another allowlisted name.
	r = fresh()
	r.Metadata = map[string]string{"display_name": "trace-collector"}
	wantCode(t, "drift signature under another name", errOf(d.raw.PublishTopic(ctx, r)), codes.PermissionDenied)
	// Re-dated to dodge the replay window.
	r = fresh()
	r.SignedAtUnixMs++
	wantCode(t, "re-dated", errOf(d.raw.PublishTopic(ctx, r)), codes.PermissionDenied)
	// A fresh genuine publish still goes through: the refusals above were
	// the signature checks, not a dead server.
	if _, err := d.raw.PublishTopic(ctx, fresh()); err != nil {
		t.Fatalf("fresh genuine publish after the attacks: %v", err)
	}
}

func errOf(_ *chatpb.TopicPublishResponse, err error) error { return err }
