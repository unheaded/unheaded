// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package wotanClient

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"unheaded/pkg/wotanerr"
	chatpb "unheaded/services/wotan/proto"
)

// S-TRUE-WIRE Step 145: an HTTP refusal's X-Wotan-Error decodes into a
// *wotanerr.Code, and the sentinel the client returned before still matches.
func TestHTTPRefusalDecodesWotanErrorCode(t *testing.T) {
	_, c := newTestServer(t, map[string]http.HandlerFunc{
		"/api/v1/topics/alerts/publish": func(w http.ResponseWriter, r *http.Request) {
			wotanerr.SetHeader(w.Header(), wotanerr.TopicAccessDenied)
			errorResponse(w, http.StatusForbidden, "Forbidden", "subscriber not approved")
		},
	})
	_, appErr := c.doPublish(context.Background(), "alerts", "00000000-0000-0000-0000-000000000001", []byte("x"))
	if appErr == nil {
		t.Fatal("publish accepted")
	}
	var code *wotanerr.Code
	if !errors.As(appErr, &code) || *code != wotanerr.TopicAccessDenied {
		t.Fatalf("errors.As = %v (%v)", code, appErr)
	}
	if !errors.Is(appErr, ErrSubscriberNotApproved) {
		t.Fatalf("lost ErrSubscriberNotApproved: %v", appErr)
	}
}

// Without the header (an older Wotan) the error is exactly what it was.
func TestHTTPRefusalWithoutCodeUnchanged(t *testing.T) {
	_, c := newTestServer(t, map[string]http.HandlerFunc{
		"/api/v1/topics/alerts/publish": func(w http.ResponseWriter, r *http.Request) {
			errorResponse(w, http.StatusNotFound, "INVALID_TOPIC", "no")
		},
	})
	_, appErr := c.doPublish(context.Background(), "alerts", "00000000-0000-0000-0000-000000000001", []byte("x"))
	if appErr != ErrTopicNotFound {
		t.Fatalf("appErr = %v, want ErrTopicNotFound itself", appErr)
	}
	var code *wotanerr.Code
	if errors.As(appErr, &code) {
		t.Fatalf("a code appeared from nowhere: %v", code)
	}
}

// codedPublishServer refuses every publish the way Wotan does.
type codedPublishServer struct {
	chatpb.UnimplementedTopicStreamServer
	code wotanerr.Code
}

func (s *codedPublishServer) PublishTopic(ctx context.Context, _ *chatpb.TopicPublishRequest) (*chatpb.TopicPublishResponse, error) {
	_ = grpc.SetTrailer(ctx, metadata.Pairs(wotanerr.TrailerKey, s.code.Hex()))
	return nil, status.Error(codes.PermissionDenied, "publisher not approved")
}

func TestGRPCRefusalDecodesWotanErrorCode(t *testing.T) {
	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer()
	chatpb.RegisterTopicStreamServer(srv, &codedPublishServer{code: wotanerr.UnauthorizedPublish})
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	conn, err := grpc.NewClient("passthrough:///bufconn",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	gc := &GRPCClient{
		conn:        conn,
		client:      chatpb.NewTopicStreamClient(conn),
		subscribers: map[string]*Subscriber{"alerts": {Topic: "alerts", Status: "approved", DisplayName: "svc"}},
		channels:    make(map[string]*safeChannel),
		maxRetries:  1,
		retryDelay:  time.Millisecond,
	}
	t.Cleanup(func() { gc.Close() })

	err = gc.Publish(context.Background(), "alerts", []byte("x"))
	var code *wotanerr.Code
	if !errors.As(err, &code) || *code != wotanerr.UnauthorizedPublish {
		t.Fatalf("errors.As = %v (%v)", code, err)
	}
	if status.Code(errors.Unwrap(err)) != codes.PermissionDenied && status.Code(err) != codes.PermissionDenied {
		t.Fatalf("gRPC status lost: %v", err)
	}
}
