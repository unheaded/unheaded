// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package grpc

import (
	"context"
	"net"
	"testing"
	"time"

	ggrpc "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"unheaded/pkg/auth"
	"unheaded/pkg/auth/authtest"
	"unheaded/services/wotan/internal/member"
	chatpb "unheaded/services/wotan/proto"
)

// authedWotan is a TopicStream server with Wotan's auth wiring
// (AuthServerOptions + RequireIdentity), as cmd/wotan builds it with
// AUTH_ENABLED=true. AUTH_ALLOW_CALLERS: timeguru, kev-poller, captain;
// auto-approve: timeguru, captain.
type authedWotan struct {
	f    *authtest.Fixture
	svc  *TopicService
	conn *ggrpc.ClientConn
}

func newAuthedWotan(t *testing.T) *authedWotan {
	t.Helper()
	f := authtest.New(t, "wotan", "timeguru", "kev-poller", "captain", "outsider")
	a, err := auth.NewServiceAuthenticator(f.Config("wotan", "timeguru", "kev-poller", "captain"))
	if err != nil {
		t.Fatal(err)
	}
	svc := productionTopicService()
	svc.AutoApprove = allowList{"timeguru": true, "captain": true}
	svc.RequireIdentity = true

	srv := ggrpc.NewServer(AuthServerOptions(a)...)
	chatpb.RegisterTopicStreamServer(srv, svc)
	hs := health.NewServer()
	healthpb.RegisterHealthServer(srv, hs)
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	conn, err := ggrpc.NewClient(lis.Addr().String(), ggrpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return &authedWotan{f: f, svc: svc, conn: conn}
}

func withToken(ctx context.Context, authz string) context.Context {
	if authz == "" {
		return ctx
	}
	return metadata.AppendToOutgoingContext(ctx, "authorization", authz)
}

func (w *authedWotan) publish(t *testing.T, authz, topic, displayName, senderID string) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req := &chatpb.TopicPublishRequest{Topic: topic, Payload: []byte(`{"x":1}`), SenderId: senderID}
	if displayName != "" {
		req.Metadata = map[string]string{"display_name": displayName}
	}
	_, err := chatpb.NewTopicStreamClient(w.conn).PublishTopic(withToken(ctx, authz), req)
	return err
}

func TestAuthE2E_GRPCPublishWithoutTokenUnauthenticated(t *testing.T) {
	w := newAuthedWotan(t)
	if err := w.publish(t, "", "alerts.critical", "timeguru", ""); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("anonymous publish: %v, want Unauthenticated", err)
	}
}

func TestAuthE2E_GRPCStreamWithoutTokenUnauthenticated(t *testing.T) {
	w := newAuthedWotan(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	st, err := chatpb.NewTopicStreamClient(w.conn).StreamTopics(ctx, &chatpb.TopicStreamRequest{TopicPattern: "alerts.*", DisplayName: "timeguru"})
	if err == nil {
		_, err = st.Recv()
	}
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("anonymous stream: %v, want Unauthenticated", err)
	}
}

func TestAuthE2E_GRPCWrongAudienceRefused(t *testing.T) {
	w := newAuthedWotan(t)
	// pkg/auth answers a wrong audience with ErrForbidden.
	err := w.publish(t, w.f.Bearer("timeguru", "captain"), "alerts.critical", "", "")
	if c := status.Code(err); c != codes.Unauthenticated && c != codes.PermissionDenied {
		t.Fatalf("aud=captain token at Wotan: %v, want Unauthenticated or PermissionDenied", err)
	}
}

func TestAuthE2E_GRPCUntrustedKeyRefused(t *testing.T) {
	w := newAuthedWotan(t)
	if err := w.publish(t, w.f.StrangerBearer("timeguru", "wotan"), "alerts.critical", "", ""); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("untrusted kid: %v, want Unauthenticated", err)
	}
}

func TestAuthE2E_GRPCCallerNotAllowlistedForbidden(t *testing.T) {
	w := newAuthedWotan(t)
	if err := w.publish(t, w.f.Bearer("outsider", "wotan"), "alerts.critical", "timeguru", ""); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("trusted key not in AUTH_ALLOW_CALLERS: %v, want PermissionDenied", err)
	}
}

// The verified sub decides auto-approval; the self-reported display_name
// is a label only.
func TestAuthE2E_GRPCAutoApproveKeysOnVerifiedSub(t *testing.T) {
	w := newAuthedWotan(t)
	// timeguru is auto-approved; its display_name claims to be someone else.
	if err := w.publish(t, w.f.Bearer("timeguru", "wotan"), "alerts.critical", "not-on-any-list", ""); err != nil {
		t.Fatalf("allowlisted sub with an odd display_name refused: %v", err)
	}
	names := map[string]bool{}
	for _, m := range w.svc.memberManager.GetByRoom("alerts.critical", member.StatusApproved) {
		names[m.Name] = true
	}
	if !names["timeguru"] || names["not-on-any-list"] {
		t.Fatalf("publisher members %v: the member must be the verified sub, not the label", names)
	}
	// kev-poller may call Wotan but is not auto-approved: claiming an
	// auto-approved display_name must not get it in.
	if err := w.publish(t, w.f.Bearer("kev-poller", "wotan"), "alerts.critical", "timeguru", ""); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("kev-poller posing as timeguru: %v, want PermissionDenied", err)
	}
}

// A sender_id is a member id; every subscriber sees member ids as messages'
// sender_id. With auth on, a caller publishes only through its own members.
func TestAuthE2E_GRPCSenderIDBoundToCaller(t *testing.T) {
	w := newAuthedWotan(t)
	captains, err := w.svc.memberManager.EnsureApproved("alerts.critical", "captain", "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := w.publish(t, w.f.Bearer("timeguru", "wotan"), "alerts.critical", "", captains.ID.String()); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("timeguru publishing as captain's member: %v, want PermissionDenied", err)
	}
	if err := w.publish(t, w.f.Bearer("captain", "wotan"), "alerts.critical", "", captains.ID.String()); err != nil {
		t.Fatalf("captain through its own member: %v", err)
	}
}

func TestAuthE2E_GRPCStreamWithTokenDelivers(t *testing.T) {
	w := newAuthedWotan(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	st, err := chatpb.NewTopicStreamClient(w.conn).StreamTopics(withToken(ctx, w.f.Bearer("captain", "wotan")),
		&chatpb.TopicStreamRequest{TopicPattern: "alerts.*", DisplayName: "captain", SinceSeq: -1})
	if err != nil {
		t.Fatal(err)
	}
	// Publish until the stream (which starts asynchronously) sees one.
	got := make(chan error, 1)
	go func() {
		for {
			ev, err := st.Recv()
			if err != nil {
				got <- err
				return
			}
			if ev.GetType() == chatpb.TopicEventType_TOPIC_MESSAGE_PUBLISHED {
				got <- nil
				return
			}
		}
	}()
	for {
		if err := w.publish(t, w.f.Bearer("timeguru", "wotan"), "alerts.critical", "", ""); err != nil {
			t.Fatal(err)
		}
		select {
		case err := <-got:
			if err != nil {
				t.Fatalf("stream: %v", err)
			}
			return
		case <-time.After(100 * time.Millisecond):
		case <-ctx.Done():
			t.Fatal("authenticated stream received nothing")
		}
	}
}

func TestAuthE2E_GRPCHealthOpen(t *testing.T) {
	w := newAuthedWotan(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	resp, err := healthpb.NewHealthClient(w.conn).Check(ctx, &healthpb.HealthCheckRequest{})
	if err != nil || resp.GetStatus() != healthpb.HealthCheckResponse_SERVING {
		t.Fatalf("grpc.health.v1 without a token: %v %v, want SERVING", resp, err)
	}
}

// RequireIdentity: a call that reaches the service with no verified
// identity (an embedding without the interceptors) is refused, never
// approved by display_name.
func TestAuthE2E_RequireIdentityRefusesBareCall(t *testing.T) {
	svc := productionTopicService()
	svc.AutoApprove = allowList{"timeguru": true}
	svc.RequireIdentity = true
	_, err := svc.PublishTopic(context.Background(), &chatpb.TopicPublishRequest{
		Topic: "alerts.critical", Payload: []byte("x"), Metadata: map[string]string{"display_name": "timeguru"}})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("bare call with auth required: %v, want Unauthenticated", err)
	}
}
