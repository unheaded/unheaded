// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package wotanClient

import (
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

// healthServer starts a gRPC server on 127.0.0.1:0 with grpc.health.v1, as
// Wotan registers it, and returns its address and the health service.
func healthServer(t *testing.T) (string, *health.Server) {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	gs := grpc.NewServer()
	hs := health.NewServer()
	healthpb.RegisterHealthServer(gs, hs)
	go func() { _ = gs.Serve(lis) }()
	t.Cleanup(gs.Stop)
	return lis.Addr().String(), hs
}

func probeClient(t *testing.T, grpcAddr string) *Client {
	t.Helper()
	c, err := NewClient("127.0.0.1:1")
	if err != nil {
		t.Fatal(err)
	}
	c.grpcAddr = grpcAddr
	t.Cleanup(func() { _ = c.Close() })
	return c
}

// A Wotan answering SERVING is healthy and gRPC becomes the transport.
func TestProbeGRPC_ServingIsHealthy(t *testing.T) {
	addr, _ := healthServer(t)
	c := probeClient(t, addr)
	if err := c.ProbeGRPC(); err != nil {
		t.Fatalf("ProbeGRPC against a SERVING Wotan: %v", err)
	}
	if !c.IsGRPCHealthy() || c.Transport() != TransportGRPC {
		t.Errorf("healthy=%v transport=%v, want true and gRPC", c.IsGRPCHealthy(), c.Transport())
	}
}

// NOT_SERVING is unhealthy. The probe used to declare health as soon as the
// (lazy) client object existed, without asking anything.
func TestProbeGRPC_NotServingIsUnhealthy(t *testing.T) {
	addr, hs := healthServer(t)
	hs.SetServingStatus("", healthpb.HealthCheckResponse_NOT_SERVING)
	c := probeClient(t, addr)
	c.transport, c.grpcHealthy = TransportGRPC, true
	if err := c.ProbeGRPC(); err == nil {
		t.Fatal("ProbeGRPC against a NOT_SERVING Wotan returned nil")
	}
	if c.IsGRPCHealthy() || c.Transport() != TransportHTTP {
		t.Errorf("healthy=%v transport=%v, want false and HTTP", c.IsGRPCHealthy(), c.Transport())
	}
}

// Nothing listening fails, and within the probe's deadline.
func TestProbeGRPC_NothingListeningFailsWithinDeadline(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := lis.Addr().String()
	_ = lis.Close()
	c := probeClient(t, addr)
	c.transport, c.grpcHealthy = TransportGRPC, true

	start := time.Now()
	err = c.ProbeGRPC()
	took := time.Since(start)
	if err == nil {
		t.Fatal("ProbeGRPC with nothing listening returned nil")
	}
	if took >= 3*time.Second {
		t.Errorf("probe took %v, want under 3 s", took)
	}
	if c.IsGRPCHealthy() || c.Transport() != TransportHTTP {
		t.Errorf("healthy=%v transport=%v, want false and HTTP", c.IsGRPCHealthy(), c.Transport())
	}
}

// A server that accepts the connection but never answers is cut off by the
// 2 s deadline rather than hanging the caller.
func TestProbeGRPC_SilentServerTimesOut(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lis.Close() })
	go func() {
		for {
			conn, err := lis.Accept()
			if err != nil {
				return
			}
			t.Cleanup(func() { _ = conn.Close() }) // hold it open, say nothing
		}
	}()
	c := probeClient(t, lis.Addr().String())
	start := time.Now()
	if err := c.ProbeGRPC(); err == nil {
		t.Fatal("ProbeGRPC against a silent server returned nil")
	}
	if took := time.Since(start); took >= 3*time.Second {
		t.Errorf("probe took %v, want the 2 s deadline", took)
	}
}
