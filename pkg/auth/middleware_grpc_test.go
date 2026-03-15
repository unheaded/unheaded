// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package auth

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// TestGRPCUnaryInterceptor_ValidAuth verifies valid auth passes through.
func TestGRPCUnaryInterceptor_ValidAuth(t *testing.T) {
	expectedIdentity := &Identity{
		Subject: "user@example.com",
		Roles:   []string{"admin"},
	}

	auth := &mockAuthenticator{
		identity: expectedIdentity,
		err:      nil,
	}

	interceptor := GRPCUnaryInterceptor(auth)

	// Create context with authorization metadata
	md := metadata.Pairs("authorization", "Bearer valid-token")
	ctx := metadata.NewIncomingContext(context.Background(), md)

	var capturedCtx context.Context
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		capturedCtx = ctx
		return "success", nil
	}

	info := &grpc.UnaryServerInfo{
		FullMethod: "/test.Service/Method",
	}

	result, err := interceptor(ctx, nil, info, handler)

	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
	if result != "success" {
		t.Errorf("expected 'success', got: %v", result)
	}

	// Verify identity is in context
	identity := IdentityFromContext(capturedCtx)
	if identity == nil {
		t.Fatal("expected identity in context")
	}
	if identity.Subject != expectedIdentity.Subject {
		t.Errorf("Subject = %q, want %q", identity.Subject, expectedIdentity.Subject)
	}
}

// TestGRPCUnaryInterceptor_MissingAuth returns Unauthenticated.
func TestGRPCUnaryInterceptor_MissingAuth(t *testing.T) {
	auth := &mockAuthenticator{
		identity: nil,
		err:      ErrUnauthenticated,
	}

	interceptor := GRPCUnaryInterceptor(auth)

	// Create context without authorization metadata
	ctx := metadata.NewIncomingContext(context.Background(), metadata.New(map[string]string{}))

	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return nil, errors.New("handler should not be called")
	}

	info := &grpc.UnaryServerInfo{
		FullMethod: "/test.Service/Method",
	}

	_, err := interceptor(ctx, nil, info, handler)

	if err == nil {
		t.Fatal("expected error")
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("error is not a gRPC status: %v", err)
	}

	if st.Code() != codes.Unauthenticated {
		t.Errorf("expected Unauthenticated, got %s", st.Code())
	}
}

// TestGRPCUnaryInterceptor_InvalidAuth returns Unauthenticated.
func TestGRPCUnaryInterceptor_InvalidAuth(t *testing.T) {
	auth := &mockAuthenticator{
		identity: nil,
		err:      ErrUnauthenticated,
	}

	interceptor := GRPCUnaryInterceptor(auth)

	md := metadata.Pairs("authorization", "Bearer invalid-token")
	ctx := metadata.NewIncomingContext(context.Background(), md)

	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return nil, errors.New("handler should not be called")
	}

	info := &grpc.UnaryServerInfo{
		FullMethod: "/test.Service/Method",
	}

	_, err := interceptor(ctx, nil, info, handler)

	if err == nil {
		t.Fatal("expected error")
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("error is not a gRPC status: %v", err)
	}

	if st.Code() != codes.Unauthenticated {
		t.Errorf("expected Unauthenticated, got %s", st.Code())
	}
}

// TestGRPCUnaryInterceptor_ForbiddenAuth returns PermissionDenied.
func TestGRPCUnaryInterceptor_ForbiddenAuth(t *testing.T) {
	auth := &mockAuthenticator{
		identity: nil,
		err:      ErrForbidden,
	}

	interceptor := GRPCUnaryInterceptor(auth)

	md := metadata.Pairs("authorization", "Bearer forbidden-token")
	ctx := metadata.NewIncomingContext(context.Background(), md)

	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return nil, errors.New("handler should not be called")
	}

	info := &grpc.UnaryServerInfo{
		FullMethod: "/test.Service/Method",
	}

	_, err := interceptor(ctx, nil, info, handler)

	if err == nil {
		t.Fatal("expected error")
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("error is not a gRPC status: %v", err)
	}

	if st.Code() != codes.PermissionDenied {
		t.Errorf("expected PermissionDenied, got %s", st.Code())
	}
}

// TestGRPCUnaryInterceptor_NoMetadata returns Unauthenticated.
func TestGRPCUnaryInterceptor_NoMetadata(t *testing.T) {
	auth := &mockAuthenticator{
		identity: nil,
		err:      ErrUnauthenticated,
	}

	interceptor := GRPCUnaryInterceptor(auth)

	// Context with no metadata at all
	ctx := context.Background()

	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return nil, errors.New("handler should not be called")
	}

	info := &grpc.UnaryServerInfo{
		FullMethod: "/test.Service/Method",
	}

	_, err := interceptor(ctx, nil, info, handler)

	if err == nil {
		t.Fatal("expected error")
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("error is not a gRPC status: %v", err)
	}

	if st.Code() != codes.Unauthenticated {
		t.Errorf("expected Unauthenticated, got %s", st.Code())
	}
}

// TestGRPCStreamInterceptor_ValidAuth verifies valid auth passes through.
func TestGRPCStreamInterceptor_ValidAuth(t *testing.T) {
	expectedIdentity := &Identity{
		Subject: "service@example.com",
		Roles:   []string{"service"},
	}

	auth := &mockAuthenticator{
		identity: expectedIdentity,
		err:      nil,
	}

	interceptor := GRPCStreamInterceptor(auth)

	md := metadata.Pairs("authorization", "Bearer valid-token")
	ctx := metadata.NewIncomingContext(context.Background(), md)

	mockStream := &mockServerStream{ctx: ctx}

	var capturedCtx context.Context
	handler := func(srv interface{}, stream grpc.ServerStream) error {
		capturedCtx = stream.Context()
		return nil
	}

	err := interceptor(nil, mockStream, &grpc.StreamServerInfo{
		FullMethod: "/test.Service/Stream",
	}, handler)

	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}

	identity := IdentityFromContext(capturedCtx)
	if identity == nil {
		t.Fatal("expected identity in stream context")
	}
	if identity.Subject != expectedIdentity.Subject {
		t.Errorf("Subject = %q, want %q", identity.Subject, expectedIdentity.Subject)
	}
}

// TestGRPCStreamInterceptor_MissingAuth returns Unauthenticated.
func TestGRPCStreamInterceptor_MissingAuth(t *testing.T) {
	auth := &mockAuthenticator{
		identity: nil,
		err:      ErrUnauthenticated,
	}

	interceptor := GRPCStreamInterceptor(auth)

	ctx := metadata.NewIncomingContext(context.Background(), metadata.New(map[string]string{}))
	mockStream := &mockServerStream{ctx: ctx}

	handler := func(srv interface{}, stream grpc.ServerStream) error {
		return errors.New("handler should not be called")
	}

	err := interceptor(nil, mockStream, &grpc.StreamServerInfo{
		FullMethod: "/test.Service/Stream",
	}, handler)

	if err == nil {
		t.Fatal("expected error")
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("error is not a gRPC status: %v", err)
	}

	if st.Code() != codes.Unauthenticated {
		t.Errorf("expected Unauthenticated, got %s", st.Code())
	}
}

// mockServerStream is a minimal grpc.ServerStream for testing.
type mockServerStream struct {
	ctx context.Context
	grpc.ServerStream
}

func (m *mockServerStream) Context() context.Context {
	return m.ctx
}

func (m *mockServerStream) SendHeader(metadata.MD) error {
	return nil
}

func (m *mockServerStream) SetHeader(metadata.MD) error {
	return nil
}

func (m *mockServerStream) SetTrailer(metadata.MD) {
}

func (m *mockServerStream) SendMsg(v interface{}) error {
	return nil
}

func (m *mockServerStream) RecvMsg(v interface{}) error {
	return errors.New("EOF")
}
