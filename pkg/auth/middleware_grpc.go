// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package auth

import (
	"context"
	"fmt"

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

		// Create a fake request with the Authorization header for the authenticator
		// We wrap the metadata in a gRPC-compatible context for the authenticator
		identity, err := auth.Authenticate(ctx, &fakeHTTPRequest{
			authHeader: authValues[0],
		})

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
		identity, err := auth.Authenticate(ss.Context(), &fakeHTTPRequest{
			authHeader: authValues[0],
		})

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

// fakeHTTPRequest is a minimal http.Request-like object for passing through
// to Authenticator implementations that expect http.Request.
// It implements just enough to satisfy the Authenticator interface.
type fakeHTTPRequest struct {
	authHeader string
}

// Header implements a minimal header getter for the fakeHTTPRequest.
func (f *fakeHTTPRequest) Header() map[string][]string {
	return map[string][]string{
		"authorization": {f.authHeader},
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
