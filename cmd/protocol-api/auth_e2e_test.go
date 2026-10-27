// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package main

import (
	"context"
	"net/http"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"unheaded/pkg/auth/authtest"
)

// S-IRON-GATE Phase 6 (Steps 179-182, protocol-api): writes and reads need a
// service token from an allowlisted caller (unheaded-cli); /health stays open.
func TestAuthE2E_ProtocolAPI(t *testing.T) {
	f := authtest.New(t, "protocol-api", "unheaded-cli", "kanban-app")
	h, err := newRESTHandler(f.Config("protocol-api", "unheaded-cli"))
	if err != nil {
		t.Fatal(err)
	}
	const write, body = "/api/v1/monad/validate", `{"header":"AA=="}`

	if c := authtest.Code(h, http.MethodGet, "/health", "", ""); c != http.StatusOK {
		t.Errorf("GET /health anonymous: %d, want 200", c)
	}
	if c := authtest.Code(h, http.MethodPost, write, body, ""); c != http.StatusUnauthorized {
		t.Errorf("POST %s anonymous: %d, want 401", write, c)
	}
	if c := authtest.Code(h, http.MethodGet, "/api/v1/flows", "", ""); c != http.StatusUnauthorized {
		t.Errorf("GET /api/v1/flows anonymous: %d, want 401", c)
	}
	if c := authtest.Code(h, http.MethodPost, write, body, f.Bearer("unheaded-cli", "protocol-api")); !authtest.Authorized(c) {
		t.Errorf("POST %s as unheaded-cli: %d, want past auth", write, c)
	}
	if c := authtest.Code(h, http.MethodPost, write, body, f.StrangerBearer("unheaded-cli", "protocol-api")); c != http.StatusUnauthorized {
		t.Errorf("POST %s with a key trust.d lacks: %d, want 401", write, c)
	}
	if c := authtest.Code(h, http.MethodPost, write, body, f.Bearer("kanban-app", "protocol-api")); c != http.StatusForbidden {
		t.Errorf("POST %s as trusted, not allowlisted kanban-app: %d, want 403", write, c)
	}
	if c := authtest.Code(h, http.MethodPost, write, body, f.Bearer("unheaded-cli", "monad")); c != http.StatusForbidden {
		t.Errorf("POST %s with a token for another audience: %d, want 403", write, c)
	}
}

// The gRPC listener takes the same tokens in "authorization" metadata.
func TestAuthE2E_ProtocolAPIGRPC(t *testing.T) {
	f := authtest.New(t, "protocol-api", "unheaded-cli", "kanban-app")
	ic, err := grpcInterceptor(f.Config("protocol-api", "unheaded-cli"))
	if err != nil {
		t.Fatal(err)
	}
	call := func(authz string) codes.Code {
		ctx := context.Background()
		if authz != "" {
			ctx = metadata.NewIncomingContext(ctx, metadata.Pairs("authorization", authz))
		}
		_, err := ic(ctx, nil, &grpc.UnaryServerInfo{FullMethod: "/unheaded.v1.WotanService/Write"},
			func(context.Context, interface{}) (interface{}, error) { return "ok", nil })
		return status.Code(err)
	}
	if c := call(""); c != codes.Unauthenticated {
		t.Errorf("gRPC anonymous: %v, want Unauthenticated", c)
	}
	if c := call(f.Bearer("unheaded-cli", "protocol-api")); c != codes.OK {
		t.Errorf("gRPC as unheaded-cli: %v, want OK", c)
	}
	if c := call(f.Bearer("kanban-app", "protocol-api")); c != codes.PermissionDenied {
		t.Errorf("gRPC as not allowlisted kanban-app: %v, want PermissionDenied", c)
	}
}

func TestAuthE2E_ProtocolAPIRefusesBadConfig(t *testing.T) {
	f := authtest.New(t, "protocol-api")
	cfg := f.Config("protocol-api")
	cfg.TrustDir = ""
	if _, err := newRESTHandler(cfg); err == nil {
		t.Error("auth on with nothing to verify against: no error, want refusal to start")
	}
	if _, err := grpcInterceptor(cfg); err == nil {
		t.Error("gRPC: auth on with nothing to verify against: no error, want refusal to start")
	}
}
