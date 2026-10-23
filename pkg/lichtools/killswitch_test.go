// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package lichtools

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// fakeSandbox counts Destroy calls; it needs no real processes.
type fakeSandbox struct{ destroyed int32 }

func (f *fakeSandbox) Spawn(context.Context, Target, Limits) (*Handle, error) { return &Handle{}, nil }
func (f *fakeSandbox) Exec(context.Context, *Handle, []string) (ExecResult, error) {
	return ExecResult{}, nil
}
func (f *fakeSandbox) Destroy(*Handle) error { atomic.AddInt32(&f.destroyed, 1); return nil }

func TestKillSwitch_TripReapsAndRefuses(t *testing.T) {
	fs := &fakeSandbox{}
	var halts int32
	ks := NewKillSwitch("", 10*time.Millisecond, fs, func(string) { atomic.AddInt32(&halts, 1) })
	ks.Register(&Handle{ID: "a"})
	ks.Register(&Handle{ID: "b"})

	if ks.Tripped() {
		t.Fatal("should not be tripped yet")
	}
	ks.Trip("test")
	if !ks.Tripped() {
		t.Fatal("should be tripped")
	}
	if got := atomic.LoadInt32(&fs.destroyed); got != 2 {
		t.Fatalf("expected 2 handles destroyed, got %d", got)
	}
	if got := atomic.LoadInt32(&halts); got != 1 {
		t.Fatalf("onHalt should fire exactly once, got %d", got)
	}
}

func TestKillSwitch_TripIdempotent(t *testing.T) {
	fs := &fakeSandbox{}
	var halts int32
	ks := NewKillSwitch("", time.Millisecond, fs, func(string) { atomic.AddInt32(&halts, 1) })
	ks.Register(&Handle{ID: "a"})
	ks.Trip("first")
	ks.Trip("second")
	ks.Trip("third")
	if got := atomic.LoadInt32(&fs.destroyed); got != 1 {
		t.Fatalf("handle destroyed more than once: %d", got)
	}
	if got := atomic.LoadInt32(&halts); got != 1 {
		t.Fatalf("onHalt fired more than once: %d", got)
	}
}

func TestKillSwitch_CheckTripsOnHaltFile(t *testing.T) {
	dir := t.TempDir()
	halt := filepath.Join(dir, "lich.halt")
	ks := NewKillSwitch(halt, time.Millisecond, &fakeSandbox{}, nil)
	if ks.Check() {
		t.Fatal("should not trip without halt file")
	}
	if err := os.WriteFile(halt, []byte("x"), 0o600); err != nil {
		t.Fatalf("touch halt: %v", err)
	}
	if !ks.Check() {
		t.Fatal("should trip once halt file exists")
	}
	if !ks.Tripped() {
		t.Fatal("tripped should be sticky")
	}
}

func TestKillSwitch_WatchStopsOnContext(t *testing.T) {
	ks := NewKillSwitch("", time.Millisecond, &fakeSandbox{}, nil)
	ks.poll = time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	done := make(chan struct{})
	go func() { ks.Watch(ctx); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Watch did not return on context cancel")
	}
}
