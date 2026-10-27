// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis. All rights reserved.

package wotanClient

import (
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

// refusedAddr returns a loopback address nothing listens on: bound, then
// closed, so a connect is refused rather than left hanging.
func refusedAddr(t *testing.T) string {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := lis.Addr().String()
	_ = lis.Close()
	return addr
}

// healthServerAt starts grpc.health.v1 (SERVING) on addr, as Wotan does.
func healthServerAt(t *testing.T, addr string) {
	t.Helper()
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("listen %s: %v", addr, err)
	}
	gs := grpc.NewServer()
	healthpb.RegisterHealthServer(gs, health.NewServer())
	go func() { _ = gs.Serve(lis) }()
	t.Cleanup(gs.Stop)
}

func reprobeRunning(c *Client) bool {
	c.reprobeMu.Lock()
	defer c.reprobeMu.Unlock()
	return c.reprobeOn
}

func reprobeWait(t *testing.T, within time.Duration, cond func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return cond()
}

// A client that started before Wotan's gRPC port answered comes up on HTTP
// polling and promotes itself back to gRPC once Wotan is SERVING, without
// any caller re-probing (client.go used to ask callers to, and none did).
func TestReprobe_PromotesToGRPC(t *testing.T) {
	addr := refusedAddr(t)
	c, err := newClientWithGRPC("127.0.0.1:1", addr, 20*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	if c.Transport() != TransportHTTP {
		t.Fatalf("transport with nothing listening = %v, want HTTP", c.Transport())
	}
	if !reprobeRunning(c) {
		t.Fatal("no re-probe running after a failed startup probe")
	}

	healthServerAt(t, addr)

	if !reprobeWait(t, 2*time.Second, func() bool { return c.Transport() == TransportGRPC }) {
		t.Fatalf("transport still %v 2 s after Wotan came up", c.Transport())
	}
	if !c.Health().GRPCHealthy {
		t.Error("Health().GRPCHealthy = false after promotion")
	}
	if !reprobeWait(t, time.Second, func() bool { return !reprobeRunning(c) }) {
		t.Error("re-probe still running after promotion")
	}
}

// Healthy at startup: nothing to re-probe.
func TestReprobe_NotStartedWhenHealthy(t *testing.T) {
	addr, _ := healthServer(t)
	c, err := newClientWithGRPC("127.0.0.1:1", addr, 20*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	if c.Transport() != TransportGRPC {
		t.Fatalf("transport = %v, want gRPC", c.Transport())
	}
	if reprobeRunning(c) {
		t.Error("re-probe running although the startup probe passed")
	}
}

// A runtime degradation (circuit-breaker trip) starts the loop too, and it
// promotes back while Wotan is still SERVING.
func TestReprobe_RuntimeDegradeRecovers(t *testing.T) {
	addr, _ := healthServer(t)
	c, err := newClientWithGRPC("127.0.0.1:1", addr, 20*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	c.degradeToHTTP()
	if !reprobeWait(t, 2*time.Second, func() bool { return c.Transport() == TransportGRPC }) {
		t.Fatalf("transport still %v 2 s after a runtime degrade", c.Transport())
	}
}

// Close stops the loop and returns promptly; a closed client never starts
// another one.
func TestReprobe_StopsOnClose(t *testing.T) {
	c, err := newClientWithGRPC("127.0.0.1:1", refusedAddr(t), 20*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	c.reprobeMu.Lock()
	done := c.reprobeDone
	c.reprobeMu.Unlock()
	if done == nil {
		t.Fatal("no re-probe started")
	}

	start := time.Now()
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if took := time.Since(start); took > 1500*time.Millisecond {
		t.Errorf("Close took %v", took)
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("re-probe goroutine did not exit after Close")
	}

	c.degradeToHTTP()
	if reprobeRunning(c) {
		t.Error("a closed client started a re-probe")
	}
	if err := c.Close(); err != nil { // idempotent
		t.Fatal(err)
	}
}

// The production default is 30 s.
func TestReprobe_DefaultInterval(t *testing.T) {
	c, err := NewClientWithGRPC("127.0.0.1:1", refusedAddr(t))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if got := c.reprobeEveryOr(); got != 30*time.Second {
		t.Errorf("default re-probe interval = %v, want 30s", got)
	}
}
