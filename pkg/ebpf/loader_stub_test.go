// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

//go:build !linux
// +build !linux

package ebpf

import (
	"context"
	"testing"
)

func TestStubLoaderMethods(t *testing.T) {
	stub := &StubLoader{}
	ctx := context.Background()

	// Test Load
	err := stub.Load(ctx, &ProgramSpec{Name: "test"})
	if err != ErrNotSupported {
		t.Errorf("expected ErrNotSupported, got %v", err)
	}

	// Test Unload
	err = stub.Unload(ctx, "test")
	if err != ErrNotSupported {
		t.Errorf("expected ErrNotSupported, got %v", err)
	}

	// Test Attach
	err = stub.Attach(ctx, "test")
	if err != ErrNotSupported {
		t.Errorf("expected ErrNotSupported, got %v", err)
	}

	// Test Detach
	err = stub.Detach(ctx, "test")
	if err != ErrNotSupported {
		t.Errorf("expected ErrNotSupported, got %v", err)
	}

	// Test GetProgram
	_, err = stub.GetProgram(ctx, "test")
	if err != ErrNotSupported {
		t.Errorf("expected ErrNotSupported, got %v", err)
	}

	// Test ListPrograms
	_, err = stub.ListPrograms(ctx)
	if err != ErrNotSupported {
		t.Errorf("expected ErrNotSupported, got %v", err)
	}

	// Test ProgramExists
	if stub.ProgramExists(ctx, "test") {
		t.Error("expected ProgramExists to return false on stub")
	}

	// Test GetMap
	_, err = stub.GetMap(ctx, "prog", "map")
	if err != ErrNotSupported {
		t.Errorf("expected ErrNotSupported, got %v", err)
	}

	// Test ListMaps
	_, err = stub.ListMaps(ctx, "prog")
	if err != ErrNotSupported {
		t.Errorf("expected ErrNotSupported, got %v", err)
	}

	// Test ReadMap
	_, err = stub.ReadMap(ctx, "prog", "map", []byte{0x01})
	if err != ErrNotSupported {
		t.Errorf("expected ErrNotSupported, got %v", err)
	}

	// Test WriteMap
	err = stub.WriteMap(ctx, "prog", "map", []byte{0x01}, []byte{0x02})
	if err != ErrNotSupported {
		t.Errorf("expected ErrNotSupported, got %v", err)
	}

	// Test DeleteMapEntry
	err = stub.DeleteMapEntry(ctx, "prog", "map", []byte{0x01})
	if err != ErrNotSupported {
		t.Errorf("expected ErrNotSupported, got %v", err)
	}

	// Test IterateMap
	err = stub.IterateMap(ctx, "prog", "map", func(key, value []byte) error { return nil })
	if err != ErrNotSupported {
		t.Errorf("expected ErrNotSupported, got %v", err)
	}

	// Test ReadRingbuf
	_, err = stub.ReadRingbuf(ctx, "prog", "map")
	if err != ErrNotSupported {
		t.Errorf("expected ErrNotSupported, got %v", err)
	}

	// Test ReadPerfEvents
	_, err = stub.ReadPerfEvents(ctx, "prog", "map")
	if err != ErrNotSupported {
		t.Errorf("expected ErrNotSupported, got %v", err)
	}

	// Test GetMetrics
	_, err = stub.GetMetrics(ctx, "test")
	if err != ErrNotSupported {
		t.Errorf("expected ErrNotSupported, got %v", err)
	}

	// Test Close
	err = stub.Close()
	if err != nil {
		t.Errorf("expected Close to return nil, got %v", err)
	}
}

func TestLoaderInterface(t *testing.T) {
	// Verify StubLoader implements Loader interface
	var _ Loader = (*StubLoader)(nil)
}
