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
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"unheaded/pkg/auth"
	"unheaded/pkg/auth/authtest"
	chatpb "unheaded/services/wotan/proto"
)

// M2: ChatStream holds a member to its identity as the HTTP room routes do.
// A member id is no secret: every message carries its creator's.
func TestAuthE2E_ChatStreamBoundToCallerMember(t *testing.T) {
	f := authtest.New(t, "wotan", "timeguru", "captain")
	a, err := auth.NewServiceAuthenticator(f.Config("wotan", "timeguru", "captain"))
	if err != nil {
		t.Fatal(err)
	}
	svc := setupTestService()
	srv := ggrpc.NewServer(AuthServerOptions(a)...)
	chatpb.RegisterChatStreamServer(srv, svc)
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

	rm, err := svc.roomManager.Create("ops", "ops")
	if err != nil {
		t.Fatal(err)
	}
	captain, err := svc.memberManager.EnsureApproved("ops", "captain", "test")
	if err != nil {
		t.Fatal(err)
	}
	rm.SendMessage(captain.ID, "held for the captain")

	first := func(authz string) (*chatpb.MessageEvent, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		st, err := chatpb.NewChatStreamClient(conn).StreamMessages(withToken(ctx, authz), &chatpb.StreamRequest{
			MemberId: captain.ID.String(),
			RoomId:   "ops",
			Since:    timestamppb.New(time.Now().Add(-time.Hour)),
		})
		if err != nil {
			return nil, err
		}
		return st.Recv()
	}

	if ev, err := first(f.Bearer("timeguru", "wotan")); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("timeguru streaming as captain: event %v, err %v, want PermissionDenied", ev, err)
	}
	ev, err := first(f.Bearer("captain", "wotan"))
	if err != nil {
		t.Fatalf("captain streaming as itself: %v", err)
	}
	if ev.GetContent() != "held for the captain" {
		t.Fatalf("captain's first event = %q", ev.GetContent())
	}
}
