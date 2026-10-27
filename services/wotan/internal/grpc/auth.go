// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package grpc

import (
	"context"
	"strings"

	ggrpc "google.golang.org/grpc"

	"unheaded/pkg/auth"
)

// healthMethodPrefix is grpc.health.v1: Check and Watch stay open, as
// /health does over HTTP (pkg/wotan-client probes it before it has
// anything to authenticate).
const healthMethodPrefix = "/grpc.health.v1.Health/"

// AuthServerOptions are the server options that make every gRPC call to
// Wotan (TopicStream, ChatStream) carry a service token a verifies: auth's
// unary and stream interceptors, which put the verified identity in the
// call's context (TopicService reads it) and answer Unauthenticated or
// PermissionDenied, with grpc.health.v1 exempt (S-IRON-GATE Phase 7,
// ADR-103 §6).
func AuthServerOptions(a auth.Authenticator) []ggrpc.ServerOption {
	unary := auth.GRPCUnaryInterceptor(a)
	stream := auth.GRPCStreamInterceptor(a)
	return []ggrpc.ServerOption{
		ggrpc.ChainUnaryInterceptor(func(ctx context.Context, req any, info *ggrpc.UnaryServerInfo, h ggrpc.UnaryHandler) (any, error) {
			if strings.HasPrefix(info.FullMethod, healthMethodPrefix) {
				return h(ctx, req)
			}
			return unary(ctx, req, info, h)
		}),
		ggrpc.ChainStreamInterceptor(func(srv any, ss ggrpc.ServerStream, info *ggrpc.StreamServerInfo, h ggrpc.StreamHandler) error {
			if strings.HasPrefix(info.FullMethod, healthMethodPrefix) {
				return h(srv, ss)
			}
			return stream(srv, ss, info, h)
		}),
	}
}
