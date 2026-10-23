// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package lichtools

import (
	"context"
	"os"
	"sync"
	"time"
)

// DefaultHaltGrace is how long the kill switch waits after SIGTERM before it
// SIGKILLs a sandbox (ADR-063 Marshal HARD-HALT discipline).
const DefaultHaltGrace = 10 * time.Second

// KillSwitch halts the Lich surface: on trip it stops accepting new spawns,
// SIGTERMs every live sandbox, and after a grace window SIGKILLs and destroys
// them. It trips on a halt file appearing or on an explicit Trip (wired to
// SIGTERM/SIGINT by the server). Trip is idempotent.
type KillSwitch struct {
	haltFile string
	grace    time.Duration
	poll     time.Duration
	sandbox  Sandbox
	onHalt   func(reason string) // audit hook; may be nil

	mu      sync.Mutex
	tripped bool
	handles map[string]*Handle
}

// NewKillSwitch builds a kill switch over sandbox sb. onHalt (optional) is
// called once with the trip reason for the audit trail.
func NewKillSwitch(haltFile string, grace time.Duration, sb Sandbox, onHalt func(string)) *KillSwitch {
	if grace <= 0 {
		grace = DefaultHaltGrace
	}
	return &KillSwitch{
		haltFile: haltFile,
		grace:    grace,
		poll:     time.Second,
		sandbox:  sb,
		onHalt:   onHalt,
		handles:  make(map[string]*Handle),
	}
}

// Register tracks a live sandbox handle so a trip can reap it.
func (k *KillSwitch) Register(h *Handle) {
	if h == nil {
		return
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	k.handles[h.ID] = h
}

// Unregister drops a handle once its run completes normally.
func (k *KillSwitch) Unregister(h *Handle) {
	if h == nil {
		return
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	delete(k.handles, h.ID)
}

// Tripped reports whether the kill switch has fired. Callers refuse new spawns
// while it is true.
func (k *KillSwitch) Tripped() bool {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.tripped
}

// Trip halts the surface: mark tripped, SIGTERM every live handle, wait the
// grace window, then SIGKILL+Destroy. Idempotent — a second call is a no-op.
func (k *KillSwitch) Trip(reason string) {
	k.mu.Lock()
	if k.tripped {
		k.mu.Unlock()
		return
	}
	k.tripped = true
	live := make([]*Handle, 0, len(k.handles))
	for _, h := range k.handles {
		live = append(live, h)
	}
	k.mu.Unlock()

	for _, h := range live {
		h.terminate()
	}
	if len(live) > 0 {
		time.Sleep(k.grace)
	}
	for _, h := range live {
		if k.sandbox != nil {
			_ = k.sandbox.Destroy(h)
		}
	}
	k.mu.Lock()
	for _, h := range live {
		delete(k.handles, h.ID)
	}
	k.mu.Unlock()

	if k.onHalt != nil {
		k.onHalt(reason)
	}
}

// Check trips the switch if the halt file exists. Returns the tripped state.
func (k *KillSwitch) Check() bool {
	if k.haltFile != "" {
		if _, err := os.Stat(k.haltFile); err == nil {
			k.Trip("halt file present: " + k.haltFile)
		}
	}
	return k.Tripped()
}

// Watch polls for the halt file until ctx is done or the switch trips. The
// server also wires SIGTERM/SIGINT directly to Trip.
func (k *KillSwitch) Watch(ctx context.Context) {
	t := time.NewTicker(k.poll)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if k.Check() {
				return
			}
		}
	}
}
