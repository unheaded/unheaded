// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package auth

import (
	"context"
	"errors"
	"net/http"
	"net/url"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
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
		identity, err := grpcAuthenticate(ctx, auth, info.FullMethod)
		if err != nil {
			return nil, err
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
		identity, err := grpcAuthenticate(ss.Context(), auth, info.FullMethod)
		if err != nil {
			return err
		}

		// Wrap the stream context with identity
		wrappedStream := &wrappedServerStream{
			ServerStream: ss,
			ctx:          context.WithValue(ss.Context(), identityKey, identity),
		}

		return handler(srv, wrappedStream)
	}
}

// auditMethodGRPC is the Method of an audit event from a gRPC call, whose
// Endpoint is the full method name.
const auditMethodGRPC = "GRPC"

// grpcAuthenticate verifies the call's "authorization" metadata through a
// minimal http.Request carrying what an audit event reports: the method and
// the peer. A call with no credential never reaches a, so its refusal is
// recorded here. The error returned is a gRPC status.
func grpcAuthenticate(ctx context.Context, a Authenticator, fullMethod string) (*Identity, error) {
	r := &http.Request{Method: auditMethodGRPC, URL: &url.URL{Path: fullMethod}, Header: http.Header{}}
	if p, ok := peer.FromContext(ctx); ok && p.Addr != nil {
		r.RemoteAddr = p.Addr.String()
	}
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		recordGRPCRefusal(a, r)
		return nil, status.Error(codes.Unauthenticated, "missing metadata")
	}
	authValues := md.Get("authorization")
	if len(authValues) == 0 {
		recordGRPCRefusal(a, r)
		return nil, status.Error(codes.Unauthenticated, "missing authorization header")
	}
	r.Header["Authorization"] = authValues
	identity, err := a.Authenticate(ctx, r)
	if err == nil && identity == nil {
		err = ErrUnauthenticated
	}
	if err != nil {
		return nil, grpcAuthError(err)
	}
	return identity, nil
}

// recordGRPCRefusal notes a call refused before reaching a, if a keeps a trail.
func recordGRPCRefusal(a Authenticator, r *http.Request) {
	if rec, ok := a.(refusalRecorder); ok {
		rec.recordRefusal(r, ErrUnauthenticated)
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
