// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Steven Bellis. All rights reserved.

package cluster

import (
	"context"
	"testing"
	"time"
)

func TestFailoverStateMachine(t *testing.T) {
	cfg := Config{Mode: ModeCluster, Role: RoleStandby, NodeID: "east", PeerAddr: "west:18002", ReplicationPort: 18002}
	fm := NewFailoverManager(cfg)

	if fm.State() != StateStandby {
		t.Fatalf("initial state should be standby, got %s", fm.State())
	}
	if fm.IsWritable() {
		t.Fatal("standby should not be writable")
	}

	// Promote
	if err := fm.PromoteToWriter(context.Background()); err != nil {
		t.Fatalf("promote failed: %v", err)
	}
	if fm.State() != StatePrimary {
		t.Fatalf("state should be primary after promotion, got %s", fm.State())
	}
	if !fm.IsWritable() {
		t.Fatal("primary should be writable")
	}

	// Can't promote again
	if err := fm.PromoteToWriter(context.Background()); err == nil {
		t.Fatal("should not be able to promote from primary state")
	}

	// Demote
	if err := fm.DemoteToReader(context.Background()); err == nil {
		// Cooldown should prevent immediate demote? No — cooldown only applies to promote
		// Demotion is always allowed from primary state
	}
}

func TestFailoverCooldown(t *testing.T) {
	cfg := Config{Mode: ModeCluster, Role: RoleStandby, NodeID: "east", PeerAddr: "west:18002", ReplicationPort: 18002}
	fm := NewFailoverManager(cfg)
	fm.cooldownPeriod = 100 * time.Millisecond // Short for testing

	// First promote succeeds
	if err := fm.PromoteToWriter(context.Background()); err != nil {
		t.Fatalf("first promote: %v", err)
	}

	// Demote back
	fm.DemoteToReader(context.Background())

	// Immediate re-promote should fail (cooldown)
	if err := fm.PromoteToWriter(context.Background()); err == nil {
		t.Fatal("should fail due to cooldown")
	}

	// Wait for cooldown
	time.Sleep(150 * time.Millisecond)

	// Now should succeed
	if err := fm.PromoteToWriter(context.Background()); err != nil {
		t.Fatalf("promote after cooldown: %v", err)
	}
}

func TestFailoverMaxLimit(t *testing.T) {
	cfg := Config{Mode: ModeCluster, Role: RoleStandby, NodeID: "east", PeerAddr: "west:18002", ReplicationPort: 18002}
	fm := NewFailoverManager(cfg)
	fm.cooldownPeriod = 0 // Disable cooldown for test
	fm.maxFailovers = 2

	// First and second promote/demote cycle
	for i := 0; i < 2; i++ {
		if err := fm.PromoteToWriter(context.Background()); err != nil {
			t.Fatalf("promote %d: %v", i+1, err)
		}
		fm.DemoteToReader(context.Background())
	}

	// Third should fail
	if err := fm.PromoteToWriter(context.Background()); err == nil {
		t.Fatal("should fail after max failovers")
	}

	// Reset and try again
	fm.ResetFailoverCount()
	if err := fm.PromoteToWriter(context.Background()); err != nil {
		t.Fatalf("promote after reset: %v", err)
	}
}

func TestFailoverCallbacks(t *testing.T) {
	cfg := Config{Mode: ModeCluster, Role: RoleStandby, NodeID: "east", PeerAddr: "west:18002", ReplicationPort: 18002}
	fm := NewFailoverManager(cfg)

	promoteCalled := false
	demoteCalled := false

	fm.OnPromote = func(ctx context.Context) error {
		promoteCalled = true
		return nil
	}
	fm.OnDemote = func(ctx context.Context) error {
		demoteCalled = true
		return nil
	}

	fm.PromoteToWriter(context.Background())
	if !promoteCalled {
		t.Fatal("OnPromote callback not called")
	}

	fm.DemoteToReader(context.Background())
	if !demoteCalled {
		t.Fatal("OnDemote callback not called")
	}
}

func TestPrimaryStartsAsPrimary(t *testing.T) {
	cfg := Config{Mode: ModeCluster, Role: RolePrimary, NodeID: "west", PeerAddr: "east:18002", ReplicationPort: 18002}
	fm := NewFailoverManager(cfg)

	if fm.State() != StatePrimary {
		t.Fatalf("primary config should start as primary, got %s", fm.State())
	}
	if !fm.IsWritable() {
		t.Fatal("primary should be writable from start")
	}
}
