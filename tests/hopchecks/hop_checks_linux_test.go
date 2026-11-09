// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

//go:build linux

// Package hopchecks proves ADR-125's checks in hop_xdp through
// BPF_PROG_TEST_RUN: a real object, loaded by the kernel, run on a real Monad
// frame. Nothing here attaches to an interface.
//
// Two objects: the default build, which every package ships and which refuses
// enforce (key 4 = 2 runs as observe), and the hop-enforce build, the only one
// that drops. Enforce rows run on the second; TestDefaultObjectRefusesEnforce
// runs them all again on the first and wants no drop. scripts/check-ebpf-loads.sh
// builds both (target/load-gate/default and target/load-gate/hop-enforce).
//
// Run as root: go test -c, then sudo the binary from this directory.
package hopchecks

import (
	"bytes"
	"encoding/binary"
	"errors"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/ringbuf"

	"unheaded/pkg/hoprules"
	"unheaded/pkg/monadpkt"
	"unheaded/pkg/protocol/encoding"
)

const (
	defaultObj = "../../ebpf/target/load-gate/default/bpfel-unknown-none/release/hop-ebpf"
	enforceObj = "../../ebpf/target/load-gate/hop-enforce/bpfel-unknown-none/release/hop-ebpf"

	xdpDrop = 1
	xdpPass = 2

	// CONFIG and STATS keys, from ADR-125 and the plan's Appendix E.
	cfgHopID         = 0
	cfgCheckMode     = 4
	statChecksRun    = 15
	statChecksFailed = 16
	statChecksEnf    = 17
	statChecksRefuse = 18 // enforce asked for and refused: the default object
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
)

var (
	frameSrc = net.ParseIP("fd00::1:2:3:4")
	otherSrc = net.ParseIP("fd00::1:2:3:99")
)

// lab is one freshly loaded hop_xdp: its maps and counters start empty.
type lab struct {
	t     *testing.T
	coll  *ebpf.Collection
	clock func() time.Duration
}

// objPath is the default build, the object every package ships.
func objPath() string {
	if p := os.Getenv("HOP_OBJ"); p != "" {
		return p
	}
	return defaultObj
}

// enforceObjPath is the hop-enforce build, the one object that drops.
func enforceObjPath() string {
	if p := os.Getenv("HOP_ENFORCE_OBJ"); p != "" {
		return p
	}
	return enforceObj
}

func newLab(t *testing.T, obj string) *lab {
	t.Helper()
	if os.Geteuid() != 0 {
		t.Skip("needs root (CAP_BPF)")
	}
	coll, err := ebpf.LoadCollection(obj)
	if err != nil {
		t.Fatalf("load %s: %v", obj, err)
	}
	t.Cleanup(coll.Close)
	return &lab{t: t, coll: coll, clock: func() time.Duration {
		now, err := hoprules.Boottime()
		if err != nil {
			t.Fatal(err)
		}
		return now
	}}
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

// Update makes the lab a hoprules.MapWriter: every entry below reaches the
// object through the writer `unheaded hop rules apply` uses, so a key or value
// byte the writer gets wrong is a check that misses here.
func (l *lab) Update(mapName string, key, value []byte) error {
	return l.m(mapName).Put(key, value)
}

func (l *lab) apply(f *hoprules.File) {
	l.t.Helper()
	if err := f.Load(); err != nil {
		l.t.Fatalf("load: %v", err)
	}
	if _, err := hoprules.Apply(l, f, l.clock); err != nil {
		l.t.Fatalf("apply: %v", err)
	}
}

func (l *lab) validator(hop uint32, ns uint8, v hoprules.Validator) {
	l.t.Helper()
	v.Hop, v.Namespace = hop, ns
	l.apply(&hoprules.File{Validators: []hoprules.Validator{v}})
}

// hmacKey writes a key good for ttl; a ttl of zero never expires.
func (l *lab) hmacKey(flow uint16, ns uint8, ttl time.Duration) {
	l.t.Helper()
	path := filepath.Join(l.t.TempDir(), "hop.key")
	if err := os.WriteFile(path, make([]byte, hoprules.KeySize), 0o400); err != nil {
		l.t.Fatal(err)
	}
	l.apply(&hoprules.File{HMACKeys: []hoprules.HMACKey{{Flow: flow, Namespace: ns, KeyFile: path, TTL: ttl}}})
}

// retry writes a validated source good for ttl.
func (l *lab) retry(flow uint16, src net.IP, ttl time.Duration) {
	l.t.Helper()
	addr, ok := netip.AddrFromSlice(src.To16())
	if !ok {
		l.t.Fatalf("source %v", src)
	}
	l.apply(&hoprules.File{Retry: []hoprules.Retry{{Flow: flow, Source: addr, TTL: ttl}}})
}

// sinceBoot sets the writer's clock to zero, so a one-second ttl lands on
// expires_at 1: long past on the clock hop-ebpf reads.
func (l *lab) sinceBoot() {
	l.clock = func() time.Duration { return 0 }
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
	refused               uint64
}

// caseObj picks the object a row runs on: enforce rows need the hop-enforce
// build, every other row runs on the default build.
func caseObj(c hopCase) string {
	if c.mode == modeEnforce {
		return enforceObjPath()
	}
	return objPath()
}

// check runs one frame through hop_xdp in obj and asserts everything ADR-125
// says about it: the verdict, the four counters, the ANOMALY, and that a check
// wrote no wire field.
func check(t *testing.T, obj string, c hopCase) {
	l := newLab(t, obj)
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
	got := [4]uint64{l.stat(statChecksRun), l.stat(statChecksFailed), l.stat(statChecksEnf), l.stat(statChecksRefuse)}
	if want := [4]uint64{c.run, c.failed, c.enforced, c.refused}; got != want {
		t.Errorf("run/failed/enforced/refused = %v, want %v", got, want)
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

func hmacOnly() hoprules.Validator { return hoprules.Validator{RequireHMAC: true} }

func denyAll() hoprules.Validator {
	return hoprules.Validator{VersionMin: 2, VersionMax: 3, RequireHMAC: true, RequireCRC: true, MaxHopCount: 1}
}

// hopTable is the plan's fourteen-row table (Step 85).
func hopTable() []hopCase {
	return []hopCase{
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
				l.hmacKey(flowID, 0, time.Hour)
			},
			verdict: xdpPass, run: 1},
		{name: "05_observe_hmac_expired_key", mode: modeObserve,
			setup: func(l *lab) {
				l.validator(0, 0, hmacOnly())
				l.sinceBoot()
				l.hmacKey(flowID, 0, time.Second)
			},
			verdict: xdpPass, run: 1, failed: 1},
		{name: "06_enforce_hmac_no_key", mode: modeEnforce,
			setup:   func(l *lab) { l.validator(0, 0, hmacOnly()) },
			verdict: xdpDrop, run: 1, failed: 1, enforced: 1},
		{name: "07_enforce_retry_entry_present", mode: modeEnforce,
			setup: func(l *lab) {
				l.validator(0, 0, hoprules.Validator{RequireRetry: true})
				l.retry(flowID, frameSrc, time.Hour)
			},
			verdict: xdpPass, run: 1},
		{name: "08_enforce_retry_wrong_source", mode: modeEnforce,
			setup: func(l *lab) {
				l.validator(0, 0, hoprules.Validator{RequireRetry: true})
				l.retry(flowID, otherSrc, time.Hour)
			},
			verdict: xdpDrop, run: 1, failed: 1, enforced: 1},
		{name: "09_enforce_hop_count_over_cap", mode: modeEnforce,
			setup:   func(l *lab) { l.validator(0, 0, hoprules.Validator{MaxHopCount: 1}) },
			patch:   map[int]byte{offHopCount: 3},
			verdict: xdpDrop, run: 1, failed: 1, enforced: 1},
		{name: "10_enforce_version_out_of_range", mode: modeEnforce,
			setup: func(l *lab) {
				l.validator(0, 0, hoprules.Validator{VersionMin: 2, VersionMax: 3})
			},
			verdict: xdpDrop, run: 1, failed: 1, enforced: 1},
		{name: "11_enforce_require_crc_custom", mode: modeEnforce,
			setup:   func(l *lab) { l.validator(0, 0, hoprules.Validator{RequireCRC: true}) },
			flags:   flagCustom,
			verdict: xdpDrop, run: 1, failed: 1, enforced: 1},
		{name: "12_mode_zero_case_6_setup", mode: modeOff,
			setup:   func(l *lab) { l.validator(0, 0, hmacOnly()) },
			verdict: xdpPass},
		{name: "13_enforce_no_rules", mode: modeEnforce,
			setup:   func(l *lab) { l.validator(0, 0, hoprules.Validator{}) },
			verdict: xdpPass, run: 1},
		{name: "14_enforce_validator_on_other_ring", mode: modeEnforce,
			setup:   func(l *lab) { l.validator(0, 2, denyAll()) },
			verdict: xdpPass},
	}
}

func TestHopChecks(t *testing.T) {
	for _, c := range hopTable() {
		t.Run(c.name, func(t *testing.T) { check(t, caseObj(c), c) })
	}
}

// edgeTable holds what the table cannot catch: case 14 passes with a
// hard-coded namespace, and every row runs at hop 0 with one boundary each.
func edgeTable() []hopCase {
	return []hopCase{
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
				l.validator(0, 0, hoprules.Validator{RequireRetry: true})
				l.sinceBoot()
				l.retry(flowID, frameSrc, time.Second)
			},
			verdict: xdpDrop, run: 1, failed: 1, enforced: 1},
		{name: "hop_count_at_cap_passes", mode: modeEnforce,
			setup:   func(l *lab) { l.validator(0, 0, hoprules.Validator{MaxHopCount: 3}) },
			patch:   map[int]byte{offHopCount: 3},
			verdict: xdpPass, run: 1},
		{name: "version_in_range_passes", mode: modeEnforce,
			setup: func(l *lab) {
				l.validator(0, 0, hoprules.Validator{VersionMin: 1, VersionMax: 1})
			},
			verdict: xdpPass, run: 1},
		{name: "unknown_mode_is_off", mode: 3,
			setup:   func(l *lab) { l.validator(0, 0, denyAll()) },
			verdict: xdpPass},
	}
}

func TestHopCheckEdges(t *testing.T) {
	for _, c := range edgeTable() {
		t.Run(c.name, func(t *testing.T) { check(t, caseObj(c), c) })
	}
}

// TestDefaultObjectRefusesEnforce runs every enforce row of both tables on the
// default build, where key 4 = 2 is observe: each check still runs, counts and
// emits ANOMALY, nothing drops, checks_enforced stays 0, and a failed check
// counts in STATS 18 instead (ADR-125, 2026-11-09; FND-031, FND-033, FND-035).
func TestDefaultObjectRefusesEnforce(t *testing.T) {
	n := 0
	for _, c := range append(hopTable(), edgeTable()...) {
		if c.mode != modeEnforce {
			continue
		}
		n++
		c.verdict, c.enforced, c.refused = xdpPass, 0, c.failed
		t.Run(c.name, func(t *testing.T) { check(t, objPath(), c) })
	}
	if n == 0 {
		t.Fatal("no enforce rows to run on the default object")
	}
}
