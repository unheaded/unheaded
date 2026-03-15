// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package auth

import (
	"context"
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
			switch {
			case isError(err, ErrForbidden):
				return nil, status.Error(codes.PermissionDenied, err.Error())
			default:
				return nil, status.Error(codes.Unauthenticated, err.Error())
			}
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
			switch {
			case isError(err, ErrForbidden):
				return status.Error(codes.PermissionDenied, err.Error())
			default:
				return status.Error(codes.Unauthenticated, err.Error())
			}
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

// isError checks if an error matches a target error (using errors.Is semantics).
func isError(err, target error) bool {
	for err != nil {
		if err == target {
			return true
		}
		// Simple check for wrapped errors
		type causer interface {
			Cause() error
		}
		if c, ok := err.(causer); ok {
			err = c.Cause()
		} else {
			break
		}
	}
	return false
}
