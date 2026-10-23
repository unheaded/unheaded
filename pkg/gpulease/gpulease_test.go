// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package gpulease

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func lockPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "unheaded", "gpu.lock")
}

func TestExclusiveBlocksShared(t *testing.T) {
	p := lockPath(t)
	ex, err := Exclusive(context.Background(), p, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t0 := time.Now()
	if _, err := Shared(context.Background(), p, 200*time.Millisecond); !errors.Is(err, ErrTimeout) {
		t.Fatalf("shared under exclusive: %v", err)
	}
	if time.Since(t0) < 150*time.Millisecond {
		t.Error("the timeout was not waited for")
	}
	if !Busy(p) {
		t.Error("Busy is false under an exclusive lease")
	}
	if err := ex.Release(); err != nil {
		t.Fatal(err)
	}
	sh, err := Shared(context.Background(), p, time.Second)
	if err != nil {
		t.Fatalf("shared after release: %v", err)
	}
	_ = sh.Release()
	if Busy(p) {
		t.Error("Busy is true with no holder")
	}
}

func TestSharedAndSharedOK(t *testing.T) {
	p := lockPath(t)
	a, err := Shared(context.Background(), p, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = a.Release() }()
	b, err := Shared(context.Background(), p, time.Second)
	if err != nil {
		t.Fatalf("second shared: %v", err)
	}
	defer func() { _ = b.Release() }()
	if Busy(p) {
		t.Error("shared holders make the GPU busy for a shared user")
	}
	if _, err := Exclusive(context.Background(), p, 100*time.Millisecond); !errors.Is(err, ErrTimeout) {
		t.Errorf("exclusive under shared: %v", err)
	}
}

func TestExclusiveWaitsForSharedToDrain(t *testing.T) {
	p := lockPath(t)
	sh, err := Shared(context.Background(), p, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	go func() { time.Sleep(100 * time.Millisecond); _ = sh.Release() }()
	ex, err := Exclusive(context.Background(), p, 2*time.Second)
	if err != nil {
		t.Fatalf("exclusive after the shared holder left: %v", err)
	}
	_ = ex.Release()
}

func TestContextCancel(t *testing.T) {
	p := lockPath(t)
	ex, _ := Exclusive(context.Background(), p, time.Second)
	defer func() { _ = ex.Release() }()
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(50 * time.Millisecond); cancel() }()
	if _, err := Shared(ctx, p, 5*time.Second); !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v", err)
	}
}

func TestLockDirIs0700AndFile0600(t *testing.T) {
	p := lockPath(t)
	l, err := Shared(context.Background(), p, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	_ = l.Release()
	for path, want := range map[string]os.FileMode{filepath.Dir(p): 0o700, p: 0o600} {
		fi, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if fi.Mode().Perm() != want {
			t.Errorf("%s mode %o, want %o", path, fi.Mode().Perm(), want)
		}
	}
}

func TestRefusesWideOpenDir(t *testing.T) {
	d := filepath.Join(t.TempDir(), "open")
	if err := os.Mkdir(d, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(d, 0o777); err != nil {
		t.Fatal(err)
	}
	if _, err := Shared(context.Background(), filepath.Join(d, "gpu.lock"), time.Second); err == nil {
		t.Error("a world-writable lock dir was accepted (anyone could hold the GPU)")
	}
}

func TestReleaseTwice(t *testing.T) {
	l, err := Shared(context.Background(), lockPath(t), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := l.Release(); err != nil {
		t.Fatal(err)
	}
	if err := l.Release(); err != nil {
		t.Errorf("second release: %v", err)
	}
	var nilLease *Lease
	if err := nilLease.Release(); err != nil {
		t.Errorf("nil release: %v", err)
	}
}

func TestDefaultPath(t *testing.T) {
	t.Setenv("GPU_LOCK", "")
	t.Setenv("XDG_RUNTIME_DIR", "/run/user/1234")
	if got := DefaultPath(); got != "/run/user/1234/unheaded/gpu.lock" {
		t.Errorf("DefaultPath = %q", got)
	}
	t.Setenv("GPU_LOCK", "/x/y.lock")
	if got := DefaultPath(); got != "/x/y.lock" {
		t.Errorf("DefaultPath with GPU_LOCK = %q", got)
	}
}
