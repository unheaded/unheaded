// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// bpf.go — the pure half of Heimdall's BPF hooks (S5 Phase 5): ringbuf
// record decoders and the trigger policy. The loader that attaches the
// programs lives in bpf_linux.go; everything here is OS-independent and
// unit-tested.
//
// Records come from crates/heimdall-bpf:
//
//	vfs_write kprobe, execve tracepoint -> DriftEvent, 24 bytes:
//	  pid u32 @0 | pad u32 @4 | ts_ns u64 @8 | event_type u32 @16 | pad u32 @20
//	gjallarhorn XDP observer           -> HornEvent, 32 bytes:
//	  kind u8 @0 | pad [3] @1 | cluster u32 @4 | manifest_ptr u64 @8 |
//	  src_hash u32 @16 | pad u32 @20 | ts_ns u64 @24
//
// Integers are little-endian (bpfel). Every record is an untrusted input
// from the kernel boundary: the decoders accept only exact sizes and known
// values.
package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
	"time"

	"unheaded/pkg/gjallarhorn"
)

// Record sizes, fixed by the #[repr(C)] structs in crates/heimdall-bpf.
const (
	DriftEventSize = 24
	HornEventSize  = 32
)

// HookConfig says where the BPF objects are and where to attach XDP.
type HookConfig struct {
	Dir      string // directory holding the heimdall-* objects
	XDPIface string // "" = no XDP observer
	XDPMode  string // "generic" (default) or "native"
}

// hookEvent is one decoded ringbuf record: exactly one field is set.
type hookEvent struct {
	Drift *BPFDriftEvent
	Horn  *HornEvent
}

// HookType is DriftEvent.event_type.
type HookType uint32

const (
	HookVFSWrite HookType = 0
	HookExecve   HookType = 1
)

// Decoder errors.
var (
	ErrRecordSize  = errors.New("heimdall bpf: record has the wrong size")
	ErrRecordField = errors.New("heimdall bpf: record field out of range")
)

// BPFDriftEvent is one vfs_write / execve hook record.
type BPFDriftEvent struct {
	PID  uint32
	TsNS uint64
	Type HookType
}

// HornEvent is one Gjallarhorn packet the XDP observer validated.
type HornEvent struct {
	Kind        gjallarhorn.TriggerKind
	Cluster     uint32
	ManifestPtr uint64
	SrcHash     uint32
	TsNS        uint64
}

// DecodeDriftEvent decodes a vfs_write / execve ringbuf record.
func DecodeDriftEvent(b []byte) (BPFDriftEvent, error) {
	if len(b) != DriftEventSize {
		return BPFDriftEvent{}, fmt.Errorf("%w: drift %d bytes, want %d", ErrRecordSize, len(b), DriftEventSize)
	}
	ev := BPFDriftEvent{
		PID:  binary.LittleEndian.Uint32(b[0:4]),
		TsNS: binary.LittleEndian.Uint64(b[8:16]),
		Type: HookType(binary.LittleEndian.Uint32(b[16:20])),
	}
	if ev.Type != HookVFSWrite && ev.Type != HookExecve {
		return BPFDriftEvent{}, fmt.Errorf("%w: event_type %d", ErrRecordField, ev.Type)
	}
	return ev, nil
}

// DecodeHornEvent decodes a Gjallarhorn XDP ringbuf record.
func DecodeHornEvent(b []byte) (HornEvent, error) {
	if len(b) != HornEventSize {
		return HornEvent{}, fmt.Errorf("%w: horn %d bytes, want %d", ErrRecordSize, len(b), HornEventSize)
	}
	kind := gjallarhorn.TriggerKind(b[0])
	if kind != gjallarhorn.BootstrapBroadcast && kind != gjallarhorn.ReverifyUnicast {
		return HornEvent{}, fmt.Errorf("%w: kind %d", ErrRecordField, kind)
	}
	if b[1]|b[2]|b[3] != 0 || binary.LittleEndian.Uint32(b[20:24]) != 0 {
		return HornEvent{}, fmt.Errorf("%w: nonzero padding", ErrRecordField)
	}
	return HornEvent{
		Kind:        kind,
		Cluster:     binary.LittleEndian.Uint32(b[4:8]),
		ManifestPtr: binary.LittleEndian.Uint64(b[8:16]),
		SrcHash:     binary.LittleEndian.Uint32(b[16:20]),
		TsNS:        binary.LittleEndian.Uint64(b[24:32]),
	}, nil
}

// ─── trigger policy ─────────────────────────────────────────────────────────

// tokenBucket is a classic token bucket on an injected clock.
type tokenBucket struct {
	rate   float64 // tokens per second
	burst  float64
	tokens float64
	last   time.Time
}

func newTokenBucket(rate, burst float64, now time.Time) *tokenBucket {
	return &tokenBucket{rate: rate, burst: burst, tokens: burst, last: now}
}

// Allow spends one token if one is available at now.
func (b *tokenBucket) Allow(now time.Time) bool {
	if now.After(b.last) {
		b.tokens += now.Sub(b.last).Seconds() * b.rate
		if b.tokens > b.burst {
			b.tokens = b.burst
		}
		b.last = now
	}
	if b.tokens >= 1 {
		b.tokens--
		return true
	}
	return false
}

// HornAction is what Heimdall does with one validated horn.
type HornAction int

const (
	// HornReverify: reload + re-verify the manifest and scan now.
	HornReverify HornAction = iota
	// HornRateLimited: a reverify the token bucket refused (flood guard).
	HornRateLimited
	// HornBootstrap: a BootstrapBroadcast; handled by the bootstrap flow
	// (S5 Phase 6, deferred), logged and otherwise ignored here.
	HornBootstrap
	// HornOtherCluster: addressed to a different cluster.
	HornOtherCluster
)

func (a HornAction) String() string {
	switch a {
	case HornReverify:
		return "reverify"
	case HornRateLimited:
		return "rate-limited"
	case HornBootstrap:
		return "bootstrap"
	case HornOtherCluster:
		return "other-cluster"
	}
	return fmt.Sprintf("HornAction(%d)", int(a))
}

// HornStats counts policy decisions.
type HornStats struct {
	Reverify, RateLimited, Bootstrap, OtherCluster uint64
}

// hornPolicy decides what a horn does. The horn is unauthenticated by design
// (ADR-043): it can only ask Heimdall to re-check a manifest it already
// trusts, and the bucket (1/s, burst 3) keeps forged horns from turning that
// into a scanner DoS.
type hornPolicy struct {
	mu      sync.Mutex
	cluster uint32
	bucket  *tokenBucket
	stats   HornStats
}

func newHornPolicy(cluster uint32, now time.Time) *hornPolicy {
	return &hornPolicy{cluster: cluster, bucket: newTokenBucket(1, 3, now)}
}

// Decide classifies one horn. Only reverifies for this node's cluster spend
// tokens.
func (p *hornPolicy) Decide(ev HornEvent, now time.Time) HornAction {
	p.mu.Lock()
	defer p.mu.Unlock()
	if ev.Cluster != p.cluster {
		p.stats.OtherCluster++
		return HornOtherCluster
	}
	if ev.Kind == gjallarhorn.BootstrapBroadcast {
		p.stats.Bootstrap++
		return HornBootstrap
	}
	if !p.bucket.Allow(now) {
		p.stats.RateLimited++
		return HornRateLimited
	}
	p.stats.Reverify++
	return HornReverify
}

// Stats returns a copy of the decision counters.
func (p *hornPolicy) Stats() HornStats {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.stats
}

// debouncer coalesces bursts of file events into one scan. It fires a fixed
// delay after the FIRST event of a burst, not after the last: vfs_write fires
// on every write on the host, so a trailing debounce would never fire on a
// busy machine. Worst-case detection latency is the delay.
type debouncer struct {
	delay    time.Duration
	deadline time.Time
	armed    bool
}

func newDebouncer(delay time.Duration) *debouncer { return &debouncer{delay: delay} }

// Kick records an event at now.
func (d *debouncer) Kick(now time.Time) {
	if !d.armed {
		d.armed = true
		d.deadline = now.Add(d.delay)
	}
}

// Armed reports whether a scan is pending.
func (d *debouncer) Armed() bool { return d.armed }

// Wait is how long until the pending scan is due (0 if due or not armed).
func (d *debouncer) Wait(now time.Time) time.Duration {
	if !d.armed || !now.Before(d.deadline) {
		return 0
	}
	return d.deadline.Sub(now)
}

// Due reports, once per burst, that the scan should run now.
func (d *debouncer) Due(now time.Time) bool {
	if d.armed && !now.Before(d.deadline) {
		d.armed = false
		return true
	}
	return false
}

// hookFilter drops file events that would make Heimdall chase its own tail.
type hookFilter struct {
	selfPID uint32
}

// Wanted reports whether ev should trigger a scan. The daemon's own writes
// (alerts to stdout, logs) would otherwise re-trigger a scan forever.
func (f hookFilter) Wanted(ev BPFDriftEvent) bool {
	return ev.PID != f.selfPID
}
