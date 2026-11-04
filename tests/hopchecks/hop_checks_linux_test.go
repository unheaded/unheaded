// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

//go:build linux

// Package hopchecks proves ADR-125's checks in hop_xdp through
// BPF_PROG_TEST_RUN: a real object, loaded by the kernel, run on a real Monad
// frame. Nothing here attaches to an interface.
//
// Run as root: go test -c, then sudo the binary from this directory.
package hopchecks

import (
	"bytes"
	"encoding/binary"
	"errors"
	"net"
	"os"
	"testing"
	"time"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/ringbuf"
	"golang.org/x/sys/unix"

	"unheaded/pkg/monadpkt"
	"unheaded/pkg/protocol/bpfschema"
	"unheaded/pkg/protocol/encoding"
)

const (
	defaultObj = "../../ebpf/target/load-gate/default/bpfel-unknown-none/release/hop-ebpf"

	xdpDrop = 1
	xdpPass = 2

	// CONFIG and STATS keys, from ADR-125 and the plan's Appendix E.
	cfgHopID         = 0
	cfgCheckMode     = 4
	statChecksRun    = 15
	statChecksFailed = 16
	statChecksEnf    = 17
	modeAbsent       = -1
	modeOff          = 0
	modeObserve      = 1
	modeEnforce      = 2

	// Monad byte offsets (foundation-06 register file).
	offHopCount = 0x03
	offRing     = 0x0A
	offChecksum = 0x12

	flagCustom = 0x02 // CUSTOM: the checksum field carries exponent keys

	// Both service ids are non-zero and different, so a key built from the
	// wrong one, or the right two swapped, misses.
	svcSrc = 0x11
	svcDst = 0x22
	flowID = uint16(svcSrc) | uint16(svcDst)<<8

	eventComputed = 0x01
	eventAnomaly  = 0x08
	offEventType  = 8 // AnamnesisEvent: u64 timestamp, then the type

	retryFlag = 1 // HopValidatorValue.Flags bit 0: a live retry entry is required
)

var (
	frameSrc = net.ParseIP("fd00::1:2:3:4")
	otherSrc = net.ParseIP("fd00::1:2:3:99")
)

// lab is one freshly loaded hop_xdp: its maps and counters start empty.
type lab struct {
	t    *testing.T
	coll *ebpf.Collection
}

func objPath() string {
	if p := os.Getenv("HOP_OBJ"); p != "" {
		return p
	}
	return defaultObj
}

func newLab(t *testing.T) *lab {
	t.Helper()
	if os.Geteuid() != 0 {
		t.Skip("needs root (CAP_BPF)")
	}
	coll, err := ebpf.LoadCollection(objPath())
	if err != nil {
		t.Fatalf("load %s: %v", objPath(), err)
	}
	t.Cleanup(coll.Close)
	return &lab{t: t, coll: coll}
}

func (l *lab) m(name string) *ebpf.Map {
	l.t.Helper()
	m := l.coll.Maps[name]
	if m == nil {
		l.t.Fatalf("object has no %s map to populate", name)
	}
	return m
}

func (l *lab) config(key uint32, v uint64) {
	l.t.Helper()
	if err := l.m("CONFIG").Put(key, v); err != nil {
		l.t.Fatalf("CONFIG[%d]: %v", key, err)
	}
}

func (l *lab) stat(key uint32) uint64 {
	l.t.Helper()
	var v uint64
	err := l.m("STATS").Lookup(key, &v)
	if errors.Is(err, ebpf.ErrKeyNotExist) {
		return 0
	}
	if err != nil {
		l.t.Fatalf("STATS[%d]: %v", key, err)
	}
	return v
}

// encode writes a bpfschema struct as the map bytes, so these tests fail if
// hop-ebpf and the Go mirror disagree.
func (l *lab) encode(v any) []byte {
	l.t.Helper()
	var b bytes.Buffer
	if err := binary.Write(&b, binary.LittleEndian, v); err != nil {
		l.t.Fatalf("encode %T: %v", v, err)
	}
	return b.Bytes()
}

func (l *lab) validator(hop, ns uint32, v bpfschema.HopValidatorValue) {
	l.t.Helper()
	key := l.encode(bpfschema.HopValidatorKey{HopID: hop, Namespace: ns})
	if err := l.m("HOP_VALIDATORS").Put(key, l.encode(v)); err != nil {
		l.t.Fatalf("HOP_VALIDATORS: %v", err)
	}
}

func (l *lab) hmacKey(flow, ns uint16, expires uint32) {
	l.t.Helper()
	key := l.encode(bpfschema.HMACKeyKey{FlowID: flow, Namespace: ns})
	val := l.encode(bpfschema.HMACKeyValue{ExpiresAt: expires})
	if err := l.m("HMAC_KEYS").Put(key, val); err != nil {
		l.t.Fatalf("HMAC_KEYS: %v", err)
	}
}

// retry writes a RETRY_TOKENS entry. There is no Go mirror until Phase 13, so
// the bytes are spelled out: the key is flow_id | src_low48 << 16, src_low48
// being the last six address bytes as one big-endian number; the value is
// token[32], issued_at u32, expires_at u32 at 36, 8 reserved.
func (l *lab) retry(flow uint16, src net.IP, expires uint32) {
	l.t.Helper()
	var low uint64
	for _, b := range src.To16()[10:16] {
		low = low<<8 | uint64(b)
	}
	key := make([]byte, 8)
	binary.LittleEndian.PutUint64(key, uint64(flow)|low<<16)
	val := make([]byte, 48)
	binary.LittleEndian.PutUint32(val[36:], expires)
	if err := l.m("RETRY_TOKENS").Put(key, val); err != nil {
		l.t.Fatalf("RETRY_TOKENS: %v", err)
	}
}

// boottimeIn returns CLOCK_BOOTTIME seconds d from now: ADR-125's clock.
func boottimeIn(t *testing.T, d time.Duration) uint32 {
	t.Helper()
	var ts unix.Timespec
	if err := unix.ClockGettime(unix.CLOCK_BOOTTIME, &ts); err != nil {
		t.Fatal(err)
	}
	return uint32(ts.Sec + int64(d/time.Second)) // #nosec G115 -- seconds since boot
}

// frame builds a Monad frame; flags go in through Build, which then computes
// the CRC, and patches go in through patchMonad, which recomputes it.
func frame(t *testing.T, flags uint8, patches map[int]byte) []byte {
	t.Helper()
	f, err := monadpkt.Build(monadpkt.Frame{
		FlowLabel: 0x5EE1, Src: frameSrc, SrcSvc: svcSrc, DstSvc: svcDst, Flags: flags,
	})
	if err != nil {
		t.Fatal(err)
	}
	for off, v := range patches {
		patchMonad(f, off, v)
	}
	return f
}

func patchMonad(f []byte, off int, v byte) {
	m := f[monadpkt.MonadOff : monadpkt.MonadOff+monadpkt.MonadLen]
	m[off] = v
	binary.BigEndian.PutUint16(m[offChecksum:], encoding.CRC16CCITT(m[:offChecksum]))
}

// events drains ANAMNESIS and returns the event types in ring order.
func (l *lab) events() []byte {
	l.t.Helper()
	rd, err := ringbuf.NewReader(l.m("ANAMNESIS"))
	if err != nil {
		l.t.Fatalf("ring reader: %v", err)
	}
	defer func() { _ = rd.Close() }()
	rd.SetDeadline(time.Now().Add(50 * time.Millisecond))
	var types []byte
	for {
		rec, err := rd.Read()
		if errors.Is(err, os.ErrDeadlineExceeded) {
			return types
		}
		if err != nil {
			l.t.Fatalf("ring read: %v", err)
		}
		types = append(types, rec.RawSample[offEventType])
	}
}

type hopCase struct {
	name                  string
	mode                  int
	setup                 func(*lab)
	flags                 uint8
	patch                 map[int]byte
	verdict               uint32
	run, failed, enforced uint64
}

// check runs one frame through hop_xdp and asserts everything ADR-125 says
// about it: the verdict, the three counters, the ANOMALY, and that a check
// wrote no wire field.
func check(t *testing.T, c hopCase) {
	l := newLab(t)
	if c.mode != modeAbsent {
		l.config(cfgCheckMode, uint64(c.mode)) // #nosec G115 -- 0, 1 or 2
	}
	if c.setup != nil {
		c.setup(l)
	}
	in := frame(t, c.flags, c.patch)
	out := make([]byte, len(in))
	verdict, err := l.coll.Programs["hop_xdp"].Run(&ebpf.RunOptions{Data: in, DataOut: out})
	if err != nil {
		t.Fatalf("test run: %v", err)
	}
	if verdict != c.verdict {
		t.Errorf("verdict = %d, want %d", verdict, c.verdict)
	}
	got := [3]uint64{l.stat(statChecksRun), l.stat(statChecksFailed), l.stat(statChecksEnf)}
	if want := [3]uint64{c.run, c.failed, c.enforced}; got != want {
		t.Errorf("run/failed/enforced = %v, want %v", got, want)
	}

	// A failed check emits ANOMALY in observe and in enforce. A packet that
	// passes goes on to the Computed event; one that drops does not.
	var want []byte
	if c.failed > 0 {
		want = append(want, eventAnomaly)
	}
	if c.verdict == xdpPass {
		want = append(want, eventComputed)
	}
	if evs := l.events(); !bytes.Equal(evs, want) {
		t.Errorf("events = %x, want %x", evs, want)
	}

	if c.verdict == xdpPass {
		wantOut := bytes.Clone(in)
		if c.flags&flagCustom != 0 {
			wantOut[monadpkt.MonadOff+offHopCount]++ // no CRC to recompute
		} else {
			patchMonad(wantOut, offHopCount, wantOut[monadpkt.MonadOff+offHopCount]+1)
		}
		if !bytes.Equal(out, wantOut) {
			t.Errorf("frame out differs from the hop count increment alone:\n got %x\nwant %x", out, wantOut)
		}
	}
}

func hmacOnly() bpfschema.HopValidatorValue { return bpfschema.HopValidatorValue{RequireHMAC: 1} }

func denyAll() bpfschema.HopValidatorValue {
	return bpfschema.HopValidatorValue{AllowedVersionMin: 2, AllowedVersionMax: 3, RequireHMAC: 1, RequireCRC: 1, MaxHopCount: 1}
}

// TestHopChecks is the plan's fourteen-row table (Step 85).
func TestHopChecks(t *testing.T) {
	cases := []hopCase{
		{name: "01_mode_absent_deny_validator_passes", mode: modeAbsent,
			setup:   func(l *lab) { l.validator(0, 0, denyAll()) },
			verdict: xdpPass},
		{name: "02_observe_no_validator", mode: modeObserve, verdict: xdpPass},
		{name: "03_observe_hmac_no_key", mode: modeObserve,
			setup:   func(l *lab) { l.validator(0, 0, hmacOnly()) },
			verdict: xdpPass, run: 1, failed: 1},
		{name: "04_observe_hmac_live_key", mode: modeObserve,
			setup: func(l *lab) {
				l.validator(0, 0, hmacOnly())
				l.hmacKey(flowID, 0, boottimeIn(t, time.Hour))
			},
			verdict: xdpPass, run: 1},
		{name: "05_observe_hmac_expired_key", mode: modeObserve,
			setup: func(l *lab) {
				l.validator(0, 0, hmacOnly())
				l.hmacKey(flowID, 0, 1)
			},
			verdict: xdpPass, run: 1, failed: 1},
		{name: "06_enforce_hmac_no_key", mode: modeEnforce,
			setup:   func(l *lab) { l.validator(0, 0, hmacOnly()) },
			verdict: xdpDrop, run: 1, failed: 1, enforced: 1},
		{name: "07_enforce_retry_entry_present", mode: modeEnforce,
			setup: func(l *lab) {
				l.validator(0, 0, bpfschema.HopValidatorValue{Flags: retryFlag})
				l.retry(flowID, frameSrc, boottimeIn(t, time.Hour))
			},
			verdict: xdpPass, run: 1},
		{name: "08_enforce_retry_wrong_source", mode: modeEnforce,
			setup: func(l *lab) {
				l.validator(0, 0, bpfschema.HopValidatorValue{Flags: retryFlag})
				l.retry(flowID, otherSrc, boottimeIn(t, time.Hour))
			},
			verdict: xdpDrop, run: 1, failed: 1, enforced: 1},
		{name: "09_enforce_hop_count_over_cap", mode: modeEnforce,
			setup:   func(l *lab) { l.validator(0, 0, bpfschema.HopValidatorValue{MaxHopCount: 1}) },
			patch:   map[int]byte{offHopCount: 3},
			verdict: xdpDrop, run: 1, failed: 1, enforced: 1},
		{name: "10_enforce_version_out_of_range", mode: modeEnforce,
			setup: func(l *lab) {
				l.validator(0, 0, bpfschema.HopValidatorValue{AllowedVersionMin: 2, AllowedVersionMax: 3})
			},
			verdict: xdpDrop, run: 1, failed: 1, enforced: 1},
		{name: "11_enforce_require_crc_custom", mode: modeEnforce,
			setup:   func(l *lab) { l.validator(0, 0, bpfschema.HopValidatorValue{RequireCRC: 1}) },
			flags:   flagCustom,
			verdict: xdpDrop, run: 1, failed: 1, enforced: 1},
		{name: "12_mode_zero_case_6_setup", mode: modeOff,
			setup:   func(l *lab) { l.validator(0, 0, hmacOnly()) },
			verdict: xdpPass},
		{name: "13_enforce_no_rules", mode: modeEnforce,
			setup:   func(l *lab) { l.validator(0, 0, bpfschema.HopValidatorValue{}) },
			verdict: xdpPass, run: 1},
		{name: "14_enforce_validator_on_other_ring", mode: modeEnforce,
			setup:   func(l *lab) { l.validator(0, 2, denyAll()) },
			verdict: xdpPass},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) { check(t, c) })
	}
}

// TestHopCheckEdges holds what the table cannot catch: case 14 passes with a
// hard-coded namespace, and every row runs at hop 0 with one boundary each.
func TestHopCheckEdges(t *testing.T) {
	cases := []hopCase{
		{name: "ring_keys_the_validator", mode: modeEnforce,
			setup:   func(l *lab) { l.validator(0, 2, denyAll()) },
			patch:   map[int]byte{offRing: 2},
			verdict: xdpDrop, run: 1, failed: 1, enforced: 1},
		{name: "hop_id_keys_the_validator", mode: modeEnforce,
			setup: func(l *lab) {
				l.config(cfgHopID, 7)
				l.validator(7, 0, denyAll())
			},
			verdict: xdpDrop, run: 1, failed: 1, enforced: 1},
		{name: "other_hop_id_misses", mode: modeEnforce,
			setup: func(l *lab) {
				l.config(cfgHopID, 7)
				l.validator(0, 0, denyAll())
			},
			verdict: xdpPass},
		{name: "ring_keys_the_hmac_entry", mode: modeEnforce,
			setup: func(l *lab) {
				l.validator(0, 2, hmacOnly())
				l.hmacKey(flowID, 2, 0)
			},
			patch:   map[int]byte{offRing: 2},
			verdict: xdpPass, run: 1},
		{name: "hmac_entry_on_other_ring_misses", mode: modeEnforce,
			setup: func(l *lab) {
				l.validator(0, 0, hmacOnly())
				l.hmacKey(flowID, 2, 0)
			},
			verdict: xdpDrop, run: 1, failed: 1, enforced: 1},
		{name: "hmac_key_swapped_pair_misses", mode: modeEnforce,
			setup: func(l *lab) {
				l.validator(0, 0, hmacOnly())
				l.hmacKey(uint16(svcDst)|uint16(svcSrc)<<8, 0, 0)
			},
			verdict: xdpDrop, run: 1, failed: 1, enforced: 1},
		{name: "hmac_expires_zero_is_never", mode: modeEnforce,
			setup: func(l *lab) {
				l.validator(0, 0, hmacOnly())
				l.hmacKey(flowID, 0, 0)
			},
			verdict: xdpPass, run: 1},
		{name: "retry_expired", mode: modeEnforce,
			setup: func(l *lab) {
				l.validator(0, 0, bpfschema.HopValidatorValue{Flags: retryFlag})
				l.retry(flowID, frameSrc, 1)
			},
			verdict: xdpDrop, run: 1, failed: 1, enforced: 1},
		{name: "hop_count_at_cap_passes", mode: modeEnforce,
			setup:   func(l *lab) { l.validator(0, 0, bpfschema.HopValidatorValue{MaxHopCount: 3}) },
			patch:   map[int]byte{offHopCount: 3},
			verdict: xdpPass, run: 1},
		{name: "version_in_range_passes", mode: modeEnforce,
			setup: func(l *lab) {
				l.validator(0, 0, bpfschema.HopValidatorValue{AllowedVersionMin: 1, AllowedVersionMax: 1})
			},
			verdict: xdpPass, run: 1},
		{name: "unknown_mode_is_off", mode: 3,
			setup:   func(l *lab) { l.validator(0, 0, denyAll()) },
			verdict: xdpPass},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) { check(t, c) })
	}
}
