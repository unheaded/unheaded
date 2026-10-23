// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package auth

import (
	"context"
	"errors"
	"net/http"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// GRPCUnaryInterceptor returns a gRPC UnaryServerInterceptor that validates
// authorization headers from the request metadata.
//
// It extracts the "authorization" metadata value (Bearer token or Service-Token)
// and validates it using the provided Authenticator.
// On success, the Identity is stored in the context.
// On failure, returns codes.Unauthenticated or codes.PermissionDenied.
func GRPCUnaryInterceptor(auth Authenticator) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		// Extract authorization metadata
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, status.Error(codes.Unauthenticated, "missing metadata")
		}

		authValues := md.Get("authorization")
		if len(authValues) == 0 {
			return nil, status.Error(codes.Unauthenticated, "missing authorization header")
		}

		// Create a minimal http.Request with the Authorization header for the authenticator
		fakeReq := &http.Request{Header: http.Header{"Authorization": authValues}}
		identity, err := auth.Authenticate(ctx, fakeReq)

		if err != nil {
			return nil, grpcAuthError(err)
		}

		// Store identity in context for the handler
		ctxWithIdentity := context.WithValue(ctx, identityKey, identity)
		return handler(ctxWithIdentity, req)
	}
}

// GRPCStreamInterceptor returns a gRPC StreamServerInterceptor that validates
// authorization headers from the request metadata.
func GRPCStreamInterceptor(auth Authenticator) grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		// Extract authorization metadata
		md, ok := metadata.FromIncomingContext(ss.Context())
		if !ok {
			return status.Error(codes.Unauthenticated, "missing metadata")
		}

		authValues := md.Get("authorization")
		if len(authValues) == 0 {
			return status.Error(codes.Unauthenticated, "missing authorization header")
		}

		// Authenticate
		fakeReq := &http.Request{Header: http.Header{"Authorization": authValues}}
		identity, err := auth.Authenticate(ss.Context(), fakeReq)

		if err != nil {
			return grpcAuthError(err)
		}

		// Wrap the stream context with identity
		wrappedStream := &wrappedServerStream{
			ServerStream: ss,
			ctx:          context.WithValue(ss.Context(), identityKey, identity),
		}

		return handler(srv, wrappedStream)
	}
}

// wrappedServerStream wraps a grpc.ServerStream with a modified context.
type wrappedServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

// Context returns the wrapped context with identity information.
func (w *wrappedServerStream) Context() context.Context {
	return w.ctx
}

// grpcAuthError maps an authentication failure to a gRPC status, like
// Middleware does for HTTP: the code and a fixed message, never err's text,
// which names the expected issuer or audience and the check that failed.
// Authenticators wrap the sentinels with %w, so errors.Is is required.
func grpcAuthError(err error) error {
	switch {
	case errors.Is(err, ErrRateLimited):
		return status.Error(codes.ResourceExhausted, "rate limited")
	case errors.Is(err, ErrForbidden):
		return status.Error(codes.PermissionDenied, "forbidden")
	default:
		return status.Error(codes.Unauthenticated, "unauthenticated")
	}
}
