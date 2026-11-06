// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"context"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"unheaded/pkg/auth"
)

func panicUnary(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(emptypb.Empty)
	if err := dec(in); err != nil {
		return nil, err
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/test.Panics/Unary"}
	return interceptor(ctx, in, info, func(context.Context, interface{}) (interface{}, error) {
		panic("handler panic")
	})
}

var panicService = grpc.ServiceDesc{
	ServiceName: "test.Panics",
	HandlerType: (*interface{})(nil),
	Methods:     []grpc.MethodDesc{{MethodName: "Unary", Handler: panicUnary}},
	Streams: []grpc.StreamDesc{{
		StreamName:    "Stream",
		ServerStreams: true,
		Handler:       func(interface{}, grpc.ServerStream) error { panic("handler panic") },
	}},
}

// A handler's panic ends that call with Internal and leaves the listener up. The
// server's own wiring is under test, so a dropped interceptor ends this test
// binary rather than failing an assertion.
func TestGRPCHandlerPanicEndsTheCallNotTheProcess(t *testing.T) {
	ensureMockMode(t)
	srv, err := newGRPCServer(auth.ServiceAuthConfig{}, grpc.WaitForHandlers(true))
	if err != nil {
		t.Fatal(err)
	}
	srv.RegisterService(&panicService, struct{}{})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(srv.Stop)
	conn, err := grpc.NewClient(ln.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for i := 0; i < 2; i++ {
		err := conn.Invoke(ctx, "/test.Panics/Unary", &emptypb.Empty{}, &emptypb.Empty{})
		if status.Code(err) != codes.Internal {
			t.Fatalf("unary call %d: %v, want Internal", i, err)
		}
	}

	st, err := conn.NewStream(ctx, &grpc.StreamDesc{StreamName: "Stream", ServerStreams: true}, "/test.Panics/Stream")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SendMsg(&emptypb.Empty{}); err != nil {
		t.Fatal(err)
	}
	if err := st.CloseSend(); err != nil {
		t.Fatal(err)
	}
	if err := st.RecvMsg(&emptypb.Empty{}); status.Code(err) != codes.Internal {
		t.Fatalf("stream: %v, want Internal", err)
	}
}
