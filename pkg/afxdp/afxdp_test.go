// SPDX-License-Identifier: GPL-2.0-only
// AF_XDP Bridge — Go wrapper tests
//
// All tests are marked [STUCK] because AF_XDP requires:
// - Linux kernel 5.8+ with AF_XDP support
// - NIC driver with XDP support
// - CAP_SYS_ADMIN or root privileges
// - libaf_xdp.a linked (Rust library must be built first)

//go:build linux && cgo && afxdp

package afxdp

import (
	"testing"
)

func TestNewEngine(t *testing.T) {
	t.Skip("AF_XDP requires Linux NIC support and CAP_SYS_ADMIN")

	engine, err := NewEngine("lo", 0)
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}
	defer engine.Close()
}

func TestNewEngineWithOptions(t *testing.T) {
	t.Skip("AF_XDP requires Linux NIC support and CAP_SYS_ADMIN")

	engine, err := NewEngine("lo", 0,
		WithFrameCount(8192),
		WithBatchSize(64),
	)
	if err != nil {
		t.Fatalf("NewEngine with options failed: %v", err)
	}
	defer engine.Close()
}

func TestStats(t *testing.T) {
	t.Skip("AF_XDP requires Linux NIC support and CAP_SYS_ADMIN")

	engine, err := NewEngine("lo", 0)
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}
	defer engine.Close()

	stats, err := engine.Stats()
	if err != nil {
		t.Fatalf("Stats failed: %v", err)
	}

	if stats.RxPackets != 0 {
		t.Errorf("expected 0 rx_packets, got %d", stats.RxPackets)
	}
}

func TestClose(t *testing.T) {
	t.Skip("AF_XDP requires Linux NIC support and CAP_SYS_ADMIN")

	engine, err := NewEngine("lo", 0)
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	err = engine.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Double close should error
	err = engine.Close()
	if err == nil {
		t.Fatal("expected error on double close")
	}
}
