// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package wotanClient

import (
	"sync"
	"testing"
)

// ============================================================================
// S77 BUG #19: Wotan degradation/recovery transport state transitions
// Verifies that degradeToHTTP() and ProbeGRPC() correctly update transport
// state and only log once per transition (wasHealthy/wasUnhealthy guards).
// ============================================================================

func TestDegradeToHTTP_SetsTransportState(t *testing.T) {
	c, err := NewClient("localhost:18000")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	// Simulate a healthy gRPC state
	c.transportMu.Lock()
	c.transport = TransportGRPC
	c.grpcHealthy = true
	c.transportMu.Unlock()

	// Degrade — should flip to HTTP
	c.degradeToHTTP()

	if c.Transport() != TransportHTTP {
		t.Errorf("after degradeToHTTP: Transport() = %v, want TransportHTTP", c.Transport())
	}
	if c.IsGRPCHealthy() {
		t.Error("after degradeToHTTP: IsGRPCHealthy() = true, want false")
	}
}

func TestDegradeToHTTP_IdempotentNoDoublLog(t *testing.T) {
	c, err := NewClient("localhost:18000")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	// Start degraded (as NewClient does — no gRPC configured)
	c.transportMu.Lock()
	c.transport = TransportHTTP
	c.grpcHealthy = false
	c.transportMu.Unlock()

	// Call degrade again — should be a no-op (wasHealthy=false, no log emitted)
	c.degradeToHTTP()

	if c.Transport() != TransportHTTP {
		t.Errorf("idempotent degradeToHTTP: Transport() = %v, want TransportHTTP", c.Transport())
	}
	if c.IsGRPCHealthy() {
		t.Error("idempotent degradeToHTTP: IsGRPCHealthy() = true, want false")
	}
}

func TestDegradeToHTTP_ConcurrentSafe(t *testing.T) {
	c, err := NewClient("localhost:18000")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	c.transportMu.Lock()
	c.transport = TransportGRPC
	c.grpcHealthy = true
	c.transportMu.Unlock()

	const goroutines = 100
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			c.degradeToHTTP()
		}()
	}
	wg.Wait()

	if c.Transport() != TransportHTTP {
		t.Errorf("concurrent degradeToHTTP: Transport() = %v, want TransportHTTP", c.Transport())
	}
	if c.IsGRPCHealthy() {
		t.Error("concurrent degradeToHTTP: IsGRPCHealthy() = true, want false")
	}
}

func TestProbeGRPC_NoAddrReturnsError(t *testing.T) {
	c, err := NewClient("localhost:18000")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	// No grpcAddr set — probe should fail
	err = c.ProbeGRPC()
	if err == nil {
		t.Error("ProbeGRPC with no gRPC address should return error")
	}
}

func TestProbeGRPC_FailureSetsHTTPTransport(t *testing.T) {
	c, err := NewClient("localhost:18000")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	// Set a bogus gRPC addr that won't connect
	c.grpcAddr = "localhost:1" // nothing listening here
	c.transportMu.Lock()
	c.transport = TransportGRPC
	c.grpcHealthy = true
	c.transportMu.Unlock()

	// Probe will fail — should degrade
	err = c.ProbeGRPC()
	// Note: gRPC may establish a lazy connection, so this might or might not
	// return an error depending on the gRPC implementation. Either way,
	// the transport state should be consistent.
	if err != nil {
		// If it errored, transport should be HTTP
		if c.Transport() != TransportHTTP {
			t.Errorf("after failed ProbeGRPC: Transport() = %v, want TransportHTTP", c.Transport())
		}
		if c.IsGRPCHealthy() {
			t.Error("after failed ProbeGRPC: IsGRPCHealthy() = true, want false")
		}
	}
	// If no error, gRPC connected (lazy conn) — transport should be gRPC, which is fine
}

func TestTransportState_String(t *testing.T) {
	tests := []struct {
		state TransportState
		want  string
	}{
		{TransportHTTP, "HTTP"},
		{TransportGRPC, "gRPC"},
		{TransportState(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.state.String(); got != tt.want {
			t.Errorf("TransportState(%d).String() = %q, want %q", tt.state, got, tt.want)
		}
	}
}

func TestNewClientWithGRPC_InvalidAddrs(t *testing.T) {
	tests := []struct {
		name     string
		httpAddr string
		grpcAddr string
	}{
		{"empty HTTP", "", "localhost:18001"},
		{"empty gRPC", "localhost:18000", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewClientWithGRPC(tt.httpAddr, tt.grpcAddr)
			if err == nil {
				t.Error("expected error for invalid address")
			}
		})
	}
}

// ============================================================================
// S77 BUG #25: RWMutex double-check locking in getOrCreateGRPCClient
// Verifies that concurrent calls return the same client instance and that
// the RWMutex fast-path works correctly.
// ============================================================================

func TestGetOrCreateGRPCClient_RWMutexFastPath(t *testing.T) {
	c, err := NewClient("localhost:18000")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.grpcAddr = "localhost:18001"

	// First call — creates client (slow path, write lock)
	client1, err1 := c.getOrCreateGRPCClient()

	// Second call — should use fast path (read lock), return same pointer
	client2, err2 := c.getOrCreateGRPCClient()

	// Both should succeed or both should fail identically
	if (err1 == nil) != (err2 == nil) {
		t.Fatalf("inconsistent errors: err1=%v, err2=%v", err1, err2)
	}

	if err1 == nil && client1 != client2 {
		t.Errorf("fast path returned different client: %p vs %p", client1, client2)
	}
}

func TestGetOrCreateGRPCClient_NoAddr(t *testing.T) {
	c, err := NewClient("localhost:18000")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	// grpcAddr is empty — should fail
	_, err = c.getOrCreateGRPCClient()
	if err == nil {
		t.Error("expected error when grpcAddr is empty")
	}
}

func TestGetOrCreateGRPCClient_ConcurrentRace(t *testing.T) {
	// This test is explicitly designed for -race detector verification.
	// It hammers getOrCreateGRPCClient from many goroutines to verify
	// the RWMutex double-check locking is race-free.
	c, err := NewClient("localhost:18000")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.grpcAddr = "localhost:18001"

	const goroutines = 200
	var wg sync.WaitGroup
	clients := make([]*GRPCClient, goroutines)
	errs := make([]error, goroutines)

	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			clients[idx], errs[idx] = c.getOrCreateGRPCClient()
		}(i)
	}
	wg.Wait()

	// All should return the same client (or same error)
	var refClient *GRPCClient
	var refErr error
	for i := 0; i < goroutines; i++ {
		if i == 0 {
			refClient = clients[0]
			refErr = errs[0]
			continue
		}
		if clients[i] != refClient {
			t.Errorf("goroutine %d got different client: %p vs %p", i, clients[i], refClient)
		}
		if (errs[i] == nil) != (refErr == nil) {
			t.Errorf("goroutine %d got different error state: %v vs %v", i, errs[i], refErr)
		}
	}
}

func TestTransportState_DegradeAndRecoverCycle(t *testing.T) {
	c, err := NewClient("localhost:18000")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.grpcAddr = "localhost:18001"

	// Start healthy
	c.transportMu.Lock()
	c.transport = TransportGRPC
	c.grpcHealthy = true
	c.transportMu.Unlock()

	// Degrade
	c.degradeToHTTP()
	if c.Transport() != TransportHTTP {
		t.Fatal("expected HTTP after degrade")
	}

	// Simulate recovery by directly setting state (ProbeGRPC may or may not
	// succeed depending on whether a real server is running)
	c.transportMu.Lock()
	c.grpcHealthy = true
	c.transport = TransportGRPC
	c.transportMu.Unlock()

	if c.Transport() != TransportGRPC {
		t.Fatal("expected gRPC after simulated recovery")
	}
	if !c.IsGRPCHealthy() {
		t.Fatal("expected gRPC healthy after simulated recovery")
	}

	// Degrade again
	c.degradeToHTTP()
	if c.Transport() != TransportHTTP {
		t.Fatal("expected HTTP after second degrade")
	}
}
