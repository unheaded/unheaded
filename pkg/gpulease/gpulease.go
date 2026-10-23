// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

// Package gpulease is the one GPU lease on this host (S-TWO-BRAINS Step
// 233): an flock(2) on $GPU_LOCK (default
// $XDG_RUNTIME_DIR/unheaded/gpu.lock), shared with scripts/lib/gpu-lease.sh.
//
// Inference against the loaded model (llama-server requests from the
// worker, the summariser, the eval) takes a shared lease per call; anything
// that changes what is loaded on the GPU (scripts/switch-model.sh,
// scripts/forge-train.sh) takes an exclusive lease for its whole run. The
// RX 7700 XT's 12 GB cannot hold a served model and a Gemma-4 training
// run at once: the lease is what keeps them from meeting.
//
// The lock file descriptor is close-on-exec, so a process started while
// a lease is held never inherits it; the kernel releases the lock when
// the holder exits, so a crashed holder frees the GPU by itself. Never
// delete the lock file while it is held.
package gpulease

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

// ErrTimeout: the lease was not granted in time.
var ErrTimeout = errors.New("gpu lease: not granted in time (the GPU is held by a model swap or a training run)")

// DefaultPath is $GPU_LOCK, else $XDG_RUNTIME_DIR/unheaded/gpu.lock, else
// /tmp/unheaded/gpu.lock (the same rule as gpu-lease.sh).
func DefaultPath() string {
	if p := os.Getenv("GPU_LOCK"); p != "" {
		return p
	}
	base := os.Getenv("XDG_RUNTIME_DIR")
	if base == "" {
		base = "/tmp"
	}
	return filepath.Join(base, "unheaded", "gpu.lock")
}

// Lease is a held lock; Release frees it.
type Lease struct {
	mu sync.Mutex
	f  *os.File
}

// Release frees the lease (idempotent; a nil lease is a no-op).
func (l *Lease) Release() error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.f == nil {
		return nil
	}
	err := l.f.Close() // closing the only descriptor releases the flock
	l.f = nil
	return err
}

// open creates the lock dir (0700, owned by us, not group/world
// writable) and the lock file (0600).
func open(path string) (*os.File, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("gpu lease: %w", err)
	}
	fi, err := os.Stat(dir)
	if err != nil {
		return nil, fmt.Errorf("gpu lease: %w", err)
	}
	if fi.Mode().Perm()&0o022 != 0 {
		return nil, fmt.Errorf("gpu lease: %s is group or world writable (mode %o); anyone could hold the GPU", dir, fi.Mode().Perm())
	}
	if st, ok := fi.Sys().(*syscall.Stat_t); ok && int(st.Uid) != os.Getuid() {
		return nil, fmt.Errorf("gpu lease: %s is not owned by this user", dir)
	}
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|syscall.O_NOFOLLOW, 0o600) // #nosec G304 -- the operator's lock path; O_NOFOLLOW refuses a planted symlink
	if err != nil {
		return nil, fmt.Errorf("gpu lease: %w", err)
	}
	return f, nil
}

func acquire(ctx context.Context, path string, how int, timeout time.Duration) (*Lease, error) {
	f, err := open(path)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(timeout)
	wait := 10 * time.Millisecond
	for {
		err := syscall.Flock(int(f.Fd()), how|syscall.LOCK_NB) // #nosec G115 -- a file descriptor fits an int
		if err == nil {
			return &Lease{f: f}, nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EINTR) {
			_ = f.Close()
			return nil, fmt.Errorf("gpu lease: flock: %w", err)
		}
		if !time.Now().Before(deadline) {
			_ = f.Close()
			return nil, ErrTimeout
		}
		select {
		case <-ctx.Done():
			_ = f.Close()
			return nil, ctx.Err()
		case <-time.After(min(wait, time.Until(deadline))):
		}
		wait = min(wait*2, 250*time.Millisecond)
	}
}

// Shared takes a shared lease (inference), waiting up to timeout.
func Shared(ctx context.Context, path string, timeout time.Duration) (*Lease, error) {
	return acquire(ctx, path, syscall.LOCK_SH, timeout)
}

// Exclusive takes the exclusive lease (model swap, training), waiting up
// to timeout for every shared holder to leave.
func Exclusive(ctx context.Context, path string, timeout time.Duration) (*Lease, error) {
	return acquire(ctx, path, syscall.LOCK_EX, timeout)
}

// Busy reports whether the GPU is held exclusively right now (a shared
// lease could not be taken at once). Any error opening the lock counts
// as busy: a caller deferring work is the safe side.
func Busy(path string) bool {
	l, err := acquire(context.Background(), path, syscall.LOCK_SH, 0)
	if err != nil {
		return true
	}
	_ = l.Release()
	return false
}
