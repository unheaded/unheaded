// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package wotanClient

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	"unheaded/pkg/crypto/pqc"
	"unheaded/pkg/wotansig"
	chatpb "unheaded/services/wotan/proto"
)

// capturePublishServer records every PublishTopic request it receives.
type capturePublishServer struct {
	chatpb.UnimplementedTopicStreamServer
	mu   sync.Mutex
	reqs []*chatpb.TopicPublishRequest
	err  error
}

func (s *capturePublishServer) PublishTopic(_ context.Context, req *chatpb.TopicPublishRequest) (*chatpb.TopicPublishResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return &chatpb.TopicPublishResponse{MessageId: "m1", Seq: int64(len(s.reqs))}, nil
}

func (s *capturePublishServer) last(t *testing.T) *chatpb.TopicPublishRequest {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.reqs) == 0 {
		t.Fatal("no PublishTopic request reached the server")
	}
	return s.reqs[len(s.reqs)-1]
}

func setupCaptureGRPC(t *testing.T) (*capturePublishServer, *GRPCClient) {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer()
	cap := &capturePublishServer{}
	chatpb.RegisterTopicStreamServer(srv, cap)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	conn, err := grpc.NewClient("passthrough:///bufconn",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	gc := &GRPCClient{conn: conn, client: chatpb.NewTopicStreamClient(conn), subscribers: map[string]*Subscriber{}, channels: map[string]*safeChannel{}}
	t.Cleanup(func() { _ = gc.Close() })
	return cap, gc
}

func testTopicSigner(t *testing.T) *wotansig.TopicSigner {
	t.Helper()
	_, s, err := pqc.GenerateMLDSAKeyPair(pqc.MLDSA65)
	if err != nil {
		t.Fatal(err)
	}
	return wotansig.NewTopicSigner(s)
}

// PublishSigned sends the four signature fields over gRPC, publishes under
// the name it joined with (display_name, no sender_id), and signs for that
// same name, so Wotan's check and a subscriber's re-check both pass.
func TestGRPCClient_PublishSigned(t *testing.T) {
	cap, gc := setupCaptureGRPC(t)
	ts := testTopicSigner(t)
	ctx := context.Background()
	if _, err := gc.Subscribe(ctx, "drift.detected.n1", "heimdall"); err != nil {
		t.Fatal(err)
	}
	before := time.Now().UnixMilli()
	if err := gc.PublishSigned(ctx, "drift.detected.n1", []byte(`{"path":"/etc/x"}`), ts); err != nil {
		t.Fatalf("PublishSigned: %v", err)
	}
	req := cap.last(t)
	if req.SenderId != "" || req.Metadata["display_name"] != "heimdall" {
		t.Fatalf("sender_id=%q display_name=%q, want \"\" and heimdall", req.SenderId, req.Metadata["display_name"])
	}
	if req.Algorithm != wotansig.Algorithm || len(req.Signature) == 0 || len(req.PublicKey) == 0 || req.SignedAtUnixMs < before {
		t.Fatalf("signature fields missing: alg=%q at=%d", req.Algorithm, req.SignedAtUnixMs)
	}
	sg := wotansig.Signed{Signature: req.Signature, PublicKey: req.PublicKey, Algorithm: req.Algorithm, SignedAtUnixMs: req.SignedAtUnixMs}
	if err := wotansig.VerifyEvent(req.Topic, "heimdall", req.Payload, sg, [][]byte{ts.PublicKey()}); err != nil {
		t.Fatalf("the signature does not cover what was sent: %v", err)
	}
}

func TestGRPCClient_PublishSigned_Refusals(t *testing.T) {
	cap, gc := setupCaptureGRPC(t)
	ts := testTopicSigner(t)
	ctx := context.Background()
	if err := gc.PublishSigned(ctx, "drift.x", []byte("p"), ts); err == nil {
		t.Error("published without joining the topic")
	}
	_, _ = gc.Subscribe(ctx, "drift.x", "heimdall")
	if err := gc.PublishSigned(ctx, "drift.x", []byte("p"), nil); !errors.Is(err, wotansig.ErrNoSigner) {
		t.Errorf("nil signer: %v", err)
	}
	if err := gc.PublishSigned(ctx, "", []byte("p"), ts); err == nil {
		t.Error("empty topic accepted")
	}
	if err := gc.PublishSigned(ctx, "drift.x", nil, ts); err == nil {
		t.Error("empty payload accepted")
	}
	cap.mu.Lock()
	n := len(cap.reqs)
	cap.err = status.Error(codes.PermissionDenied, "signature verification failed")
	cap.mu.Unlock()
	if n != 0 {
		t.Fatalf("%d refused publishes reached the server", n)
	}
	err := gc.PublishSigned(ctx, "drift.x", []byte("p"), ts)
	if status.Code(errors.Unwrap(err)) != codes.PermissionDenied {
		t.Fatalf("server refusal not returned: %v", err)
	}
}

// Signed topics are gRPC-only: a client without gRPC says so instead of
// trying HTTP (which Wotan refuses) or queueing the message.
func TestClient_PublishSigned_HTTPOnlyMode(t *testing.T) {
	c, err := NewClient("http://127.0.0.1:1")
	if err != nil {
		t.Fatal(err)
	}
	c.subscribers["drift.x"] = &Subscriber{Topic: "drift.x", DisplayName: "heimdall", Status: "approved"}
	if err := c.PublishSigned(context.Background(), "drift.x", []byte("p"), testTopicSigner(t)); !errors.Is(err, ErrSignedTopicNeedsGRPC) {
		t.Fatalf("HTTP-only PublishSigned: %v, want ErrSignedTopicNeedsGRPC", err)
	}
	if c.FallbackQueueLen() != 0 {
		t.Fatal("a signed message was queued for HTTP")
	}
}

func TestClient_PublishSigned_UsesGRPC(t *testing.T) {
	cap, gc := setupCaptureGRPC(t)
	c, err := NewClient("http://127.0.0.1:1")
	if err != nil {
		t.Fatal(err)
	}
	c.grpcAddr = "bufconn"
	c.grpcClient = gc
	if err := c.PublishSigned(context.Background(), "drift.x", []byte("p"), testTopicSigner(t)); err == nil {
		t.Fatal("published without joining the topic")
	}
	c.subscribers["drift.x"] = &Subscriber{Topic: "drift.x", DisplayName: "heimdall", Status: "approved"}
	if err := c.PublishSigned(context.Background(), "drift.x", []byte("p"), testTopicSigner(t)); err != nil {
		t.Fatalf("PublishSigned: %v", err)
	}
	if req := cap.last(t); req.Metadata["display_name"] != "heimdall" || len(req.Signature) == 0 {
		t.Fatalf("request %+v", req)
	}
}

// A subscriber reading through wotan-client gets the signature with the
// message and can check it against the keys it trusts.
func TestMessage_VerifySignature(t *testing.T) {
	ts := testTopicSigner(t)
	sg, err := ts.Sign("drift.detected.n1", "heimdall", []byte("body"))
	if err != nil {
		t.Fatal(err)
	}
	m := topicEventToMessage(&chatpb.TopicEvent{
		Topic: "drift.detected.n1", SenderId: "heimdall", Payload: []byte("body"),
		Signature: sg.Signature, PublicKey: sg.PublicKey, Algorithm: sg.Algorithm, SignedAtUnixMs: sg.SignedAtUnixMs,
	})
	if m.Signed == nil {
		t.Fatal("signature dropped converting the event")
	}
	if err := m.VerifySignature([][]byte{ts.PublicKey()}); err != nil {
		t.Fatalf("VerifySignature: %v", err)
	}
	if err := m.VerifySignature([][]byte{testTopicSigner(t).PublicKey()}); !errors.Is(err, wotansig.ErrUntrustedKey) {
		t.Fatalf("other key: %v", err)
	}
	m.Payload = "bodY"
	if err := m.VerifySignature([][]byte{ts.PublicKey()}); !errors.Is(err, wotansig.ErrInvalidSignature) {
		t.Fatalf("edited payload: %v", err)
	}
	unsigned := topicEventToMessage(&chatpb.TopicEvent{Topic: "logs.x", Payload: []byte("x")})
	if unsigned.Signed != nil {
		t.Fatal("unsigned event has a signature")
	}
	if err := unsigned.VerifySignature([][]byte{ts.PublicKey()}); !errors.Is(err, ErrUnsignedMessage) {
		t.Fatalf("unsigned: %v", err)
	}
	var nilMsg *Message
	if err := nilMsg.VerifySignature(nil); !errors.Is(err, ErrUnsignedMessage) {
		t.Fatalf("nil message: %v", err)
	}
}
