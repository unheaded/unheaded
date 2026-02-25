// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

//go:build !linux
// +build !linux

package netlink

import (
	"sync"
	"testing"
)

// =============================================================================
// Stub Platform Tests (non-Linux)
// These tests verify the stub behavior on non-Linux platforms.
// =============================================================================

func TestNewSocket_Stub(t *testing.T) {
	tests := []struct {
		name   string
		family int
	}{
		{name: "NETLINK_ROUTE family", family: 0},
		{name: "NETLINK_GENERIC family", family: 16},
		{name: "arbitrary family", family: 99},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sock, err := NewSocket(tt.family)
			if sock != nil {
				t.Errorf("NewSocket(%d) returned non-nil socket on non-Linux", tt.family)
			}
			if err == nil {
				t.Errorf("NewSocket(%d) returned nil error on non-Linux", tt.family)
			}
			if err != ErrNotSupported {
				t.Errorf("NewSocket(%d) error = %v, want %v", tt.family, err, ErrNotSupported)
			}
		})
	}
}

func TestSocket_Close_Stub(t *testing.T) {
	s := &Socket{}
	err := s.Close()
	if err == nil {
		t.Error("Socket.Close() returned nil error on non-Linux")
	}
	if err != ErrNotSupported {
		t.Errorf("Socket.Close() error = %v, want %v", err, ErrNotSupported)
	}
}

func TestNewGenlSocket_Stub(t *testing.T) {
	gs, err := NewGenlSocket()
	if gs != nil {
		t.Error("NewGenlSocket() returned non-nil on non-Linux")
	}
	if err == nil {
		t.Error("NewGenlSocket() returned nil error on non-Linux")
	}
	if err != ErrNotSupported {
		t.Errorf("NewGenlSocket() error = %v, want %v", err, ErrNotSupported)
	}
}

func TestErrNotSupported(t *testing.T) {
	if ErrNotSupported == nil {
		t.Fatal("ErrNotSupported is nil")
	}
	expected := "netlink is only supported on Linux"
	if got := ErrNotSupported.Error(); got != expected {
		t.Errorf("ErrNotSupported.Error() = %q, want %q", got, expected)
	}
}

// =============================================================================
// Struct Type Tests -- verify exported struct fields exist and are correct types
// =============================================================================

func TestLinkInfo_Struct(t *testing.T) {
	link := LinkInfo{
		Index:      1,
		Name:       "eth0",
		MTU:        1500,
		Flags:      0x1,
		OperState:  6,
		TxQueueLen: 1000,
		Qdisc:      "noqueue",
		Master:     0,
	}

	if link.Index != 1 {
		t.Errorf("LinkInfo.Index = %d, want 1", link.Index)
	}
	if link.Name != "eth0" {
		t.Errorf("LinkInfo.Name = %q, want %q", link.Name, "eth0")
	}
	if link.MTU != 1500 {
		t.Errorf("LinkInfo.MTU = %d, want 1500", link.MTU)
	}
	if link.Flags != 0x1 {
		t.Errorf("LinkInfo.Flags = %d, want 1", link.Flags)
	}
	if link.OperState != 6 {
		t.Errorf("LinkInfo.OperState = %d, want 6", link.OperState)
	}
	if link.TxQueueLen != 1000 {
		t.Errorf("LinkInfo.TxQueueLen = %d, want 1000", link.TxQueueLen)
	}
	if link.Qdisc != "noqueue" {
		t.Errorf("LinkInfo.Qdisc = %q, want %q", link.Qdisc, "noqueue")
	}
	if link.Master != 0 {
		t.Errorf("LinkInfo.Master = %d, want 0", link.Master)
	}
}

func TestAddrInfo_Struct(t *testing.T) {
	addr := AddrInfo{
		Index:     1,
		Family:    2,
		PrefixLen: 24,
		Scope:     0,
		Label:     "eth0",
	}

	if addr.Index != 1 {
		t.Errorf("AddrInfo.Index = %d, want 1", addr.Index)
	}
	if addr.Family != 2 {
		t.Errorf("AddrInfo.Family = %d, want 2", addr.Family)
	}
	if addr.PrefixLen != 24 {
		t.Errorf("AddrInfo.PrefixLen = %d, want 24", addr.PrefixLen)
	}
	if addr.Scope != 0 {
		t.Errorf("AddrInfo.Scope = %d, want 0", addr.Scope)
	}
	if addr.Label != "eth0" {
		t.Errorf("AddrInfo.Label = %q, want %q", addr.Label, "eth0")
	}
}

func TestRouteInfo_Struct(t *testing.T) {
	route := RouteInfo{
		Family:   2,
		DstLen:   24,
		SrcLen:   0,
		Table:    254,
		Protocol: 2,
		Scope:    0,
		Type:     1,
		OifIndex: 2,
		IifIndex: 0,
		Priority: 100,
	}

	if route.Family != 2 {
		t.Errorf("RouteInfo.Family = %d, want 2", route.Family)
	}
	if route.DstLen != 24 {
		t.Errorf("RouteInfo.DstLen = %d, want 24", route.DstLen)
	}
	if route.SrcLen != 0 {
		t.Errorf("RouteInfo.SrcLen = %d, want 0", route.SrcLen)
	}
	if route.Table != 254 {
		t.Errorf("RouteInfo.Table = %d, want 254", route.Table)
	}
	if route.Protocol != 2 {
		t.Errorf("RouteInfo.Protocol = %d, want 2", route.Protocol)
	}
	if route.Scope != 0 {
		t.Errorf("RouteInfo.Scope = %d, want 0", route.Scope)
	}
	if route.Type != 1 {
		t.Errorf("RouteInfo.Type = %d, want 1", route.Type)
	}
	if route.OifIndex != 2 {
		t.Errorf("RouteInfo.OifIndex = %d, want 2", route.OifIndex)
	}
	if route.IifIndex != 0 {
		t.Errorf("RouteInfo.IifIndex = %d, want 0", route.IifIndex)
	}
	if route.Priority != 100 {
		t.Errorf("RouteInfo.Priority = %d, want 100", route.Priority)
	}
}

func TestGenlSocket_Struct(t *testing.T) {
	// Verify GenlSocket embeds *Socket
	gs := &GenlSocket{Socket: &Socket{}}
	if gs.Socket == nil {
		t.Error("GenlSocket.Socket is nil")
	}
}

// =============================================================================
// Concurrency Tests -- verify stub functions are safe under concurrent access
// =============================================================================

func TestNewSocket_Concurrent(t *testing.T) {
	const goroutines = 100
	var wg sync.WaitGroup
	wg.Add(goroutines)

	errors := make(chan error, goroutines)

	for i := 0; i < goroutines; i++ {
		go func(family int) {
			defer wg.Done()
			sock, err := NewSocket(family)
			if sock != nil {
				errors <- err
				return
			}
			if err != ErrNotSupported {
				errors <- err
				return
			}
		}(i % 17)
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("unexpected result in concurrent NewSocket: %v", err)
	}
}

func TestNewGenlSocket_Concurrent(t *testing.T) {
	const goroutines = 100
	var wg sync.WaitGroup
	wg.Add(goroutines)

	errors := make(chan error, goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			gs, err := NewGenlSocket()
			if gs != nil {
				errors <- err
				return
			}
			if err != ErrNotSupported {
				errors <- err
				return
			}
		}()
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("unexpected result in concurrent NewGenlSocket: %v", err)
	}
}

func TestSocket_Close_Concurrent(t *testing.T) {
	const goroutines = 100
	s := &Socket{}
	var wg sync.WaitGroup
	wg.Add(goroutines)

	errors := make(chan error, goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			err := s.Close()
			if err != ErrNotSupported {
				errors <- err
			}
		}()
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("unexpected error in concurrent Close: %v", err)
	}
}

// =============================================================================
// Zero Value Tests
// =============================================================================

func TestLinkInfo_ZeroValue(t *testing.T) {
	var link LinkInfo
	if link.Index != 0 {
		t.Errorf("zero LinkInfo.Index = %d, want 0", link.Index)
	}
	if link.Name != "" {
		t.Errorf("zero LinkInfo.Name = %q, want empty", link.Name)
	}
	if link.MTU != 0 {
		t.Errorf("zero LinkInfo.MTU = %d, want 0", link.MTU)
	}
	if link.Flags != 0 {
		t.Errorf("zero LinkInfo.Flags = %d, want 0", link.Flags)
	}
	if link.OperState != 0 {
		t.Errorf("zero LinkInfo.OperState = %d, want 0", link.OperState)
	}
	if link.TxQueueLen != 0 {
		t.Errorf("zero LinkInfo.TxQueueLen = %d, want 0", link.TxQueueLen)
	}
	if link.Qdisc != "" {
		t.Errorf("zero LinkInfo.Qdisc = %q, want empty", link.Qdisc)
	}
	if link.Master != 0 {
		t.Errorf("zero LinkInfo.Master = %d, want 0", link.Master)
	}
}

func TestAddrInfo_ZeroValue(t *testing.T) {
	var addr AddrInfo
	if addr.Index != 0 {
		t.Errorf("zero AddrInfo.Index = %d, want 0", addr.Index)
	}
	if addr.Family != 0 {
		t.Errorf("zero AddrInfo.Family = %d, want 0", addr.Family)
	}
	if addr.PrefixLen != 0 {
		t.Errorf("zero AddrInfo.PrefixLen = %d, want 0", addr.PrefixLen)
	}
	if addr.Label != "" {
		t.Errorf("zero AddrInfo.Label = %q, want empty", addr.Label)
	}
}

func TestRouteInfo_ZeroValue(t *testing.T) {
	var route RouteInfo
	if route.Family != 0 {
		t.Errorf("zero RouteInfo.Family = %d, want 0", route.Family)
	}
	if route.DstLen != 0 {
		t.Errorf("zero RouteInfo.DstLen = %d, want 0", route.DstLen)
	}
	if route.OifIndex != 0 {
		t.Errorf("zero RouteInfo.OifIndex = %d, want 0", route.OifIndex)
	}
	if route.Priority != 0 {
		t.Errorf("zero RouteInfo.Priority = %d, want 0", route.Priority)
	}
}
