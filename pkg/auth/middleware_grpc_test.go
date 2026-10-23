// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package auth

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

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

// TestGRPCUnaryInterceptor_GenericErrors: the interceptor used to return
// err.Error() to the caller, which names the expected issuer and audience
// ("audience mismatch: got ..., want ...") and which check failed; and it
// matched ErrForbidden with ==, so a wrapped one came back Unauthenticated.
func TestGRPCUnaryInterceptor_GenericErrors(t *testing.T) {
	jwtAuth := NewJWTAuthenticator(JWTConfig{
		SigningKey: testSigningKey,
		Issuer:     "https://auth.unheaded.dev",
		Audience:   "secret-audience-name",
	})
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return nil, errors.New("handler should not be called")
	}
	info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/Method"}

	wrongAud, err := SignToken(testSigningKey, &JWTClaims{
		Sub: "svc", Iss: "https://auth.unheaded.dev", Aud: "other",
		Exp: time.Now().Add(time.Hour).Unix(),
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name  string
		token string
		code  codes.Code
	}{
		{"wrong audience", wrongAud, codes.PermissionDenied},
		{"bad signature", wrongAud[:len(wrongAud)-2] + "AA", codes.Unauthenticated},
	} {
		md := metadata.Pairs("authorization", "Bearer "+tc.token)
		ctx := metadata.NewIncomingContext(context.Background(), md)
		_, err := GRPCUnaryInterceptor(jwtAuth)(ctx, nil, info, handler)
		st, ok := status.FromError(err)
		if !ok {
			t.Fatalf("%s: not a gRPC status: %v", tc.name, err)
		}
		if st.Code() != tc.code {
			t.Errorf("%s: code = %s, want %s", tc.name, st.Code(), tc.code)
		}
		for _, leak := range []string{"secret-audience-name", "auth.unheaded.dev", "mismatch", "signature"} {
			if strings.Contains(st.Message(), leak) {
				t.Errorf("%s: message %q reveals %q", tc.name, st.Message(), leak)
			}
		}
	}
}

// TestGRPCInterceptors_NilIdentityRefused: (nil, nil) from Authenticate is
// not a success.
func TestGRPCInterceptors_NilIdentityRefused(t *testing.T) {
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "Bearer x"))
	called := false
	_, err := GRPCUnaryInterceptor(&mockAuthenticator{})(ctx, nil, &grpc.UnaryServerInfo{FullMethod: "/t.S/M"},
		func(context.Context, interface{}) (interface{}, error) { called = true; return nil, nil })
	if status.Code(err) != codes.Unauthenticated || called {
		t.Errorf("unary: err = %v, handler called = %v; want Unauthenticated, not called", err, called)
	}

	err = GRPCStreamInterceptor(&mockAuthenticator{})(nil, &mockServerStream{ctx: ctx}, &grpc.StreamServerInfo{},
		func(interface{}, grpc.ServerStream) error { called = true; return nil })
	if status.Code(err) != codes.Unauthenticated || called {
		t.Errorf("stream: err = %v, handler called = %v; want Unauthenticated, not called", err, called)
	}
}
