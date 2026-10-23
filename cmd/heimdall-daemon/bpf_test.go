// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"unheaded/pkg/enkrateia"
	"unheaded/pkg/gjallarhorn"
)

// ─── ringbuf record decoders (S5 Step 148) ──────────────────────────────────

func driftRecordBytes(pid uint32, ts uint64, typ uint32) []byte {
	b := make([]byte, DriftEventSize)
	binary.LittleEndian.PutUint32(b[0:4], pid)
	binary.LittleEndian.PutUint64(b[8:16], ts)
	binary.LittleEndian.PutUint32(b[16:20], typ)
	return b
}

func hornRecordBytes(kind byte, cluster uint32, ptr uint64, src uint32, ts uint64) []byte {
	b := make([]byte, HornEventSize)
	b[0] = kind
	binary.LittleEndian.PutUint32(b[4:8], cluster)
	binary.LittleEndian.PutUint64(b[8:16], ptr)
	binary.LittleEndian.PutUint32(b[16:20], src)
	binary.LittleEndian.PutUint64(b[24:32], ts)
	return b
}

func TestDecodeDriftEvent(t *testing.T) {
	t.Parallel()
	ev, err := DecodeDriftEvent(driftRecordBytes(4242, 99, 1))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if ev.PID != 4242 || ev.TsNS != 99 || ev.Type != HookExecve {
		t.Fatalf("got %+v", ev)
	}
	for _, n := range []int{0, 1, 23, 25, 32} {
		if _, err := DecodeDriftEvent(make([]byte, n)); !errors.Is(err, ErrRecordSize) {
			t.Errorf("len %d: err = %v, want ErrRecordSize", n, err)
		}
	}
	if _, err := DecodeDriftEvent(driftRecordBytes(1, 1, 7)); !errors.Is(err, ErrRecordField) {
		t.Errorf("unknown event_type: err = %v, want ErrRecordField", err)
	}
	if _, err := DecodeDriftEvent(nil); !errors.Is(err, ErrRecordSize) {
		t.Errorf("nil: err = %v", err)
	}
}

func TestDecodeHornEvent(t *testing.T) {
	t.Parallel()
	ev, err := DecodeHornEvent(hornRecordBytes(2, 7, 0xCAFEBABE12345678, 0xAABBCCDD, 5))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	want := HornEvent{Kind: gjallarhorn.ReverifyUnicast, Cluster: 7, ManifestPtr: 0xCAFEBABE12345678, SrcHash: 0xAABBCCDD, TsNS: 5}
	if ev != want {
		t.Fatalf("got %+v want %+v", ev, want)
	}
	for _, n := range []int{0, 20, 31, 33} {
		if _, err := DecodeHornEvent(make([]byte, n)); !errors.Is(err, ErrRecordSize) {
			t.Errorf("len %d: err = %v, want ErrRecordSize", n, err)
		}
	}
	if _, err := DecodeHornEvent(hornRecordBytes(3, 1, 1, 1, 1)); !errors.Is(err, ErrRecordField) {
		t.Errorf("kind 3: err = %v, want ErrRecordField", err)
	}
	bad := hornRecordBytes(1, 1, 1, 1, 1)
	bad[2] = 1 // padding must be zero: the BPF side writes every byte
	if _, err := DecodeHornEvent(bad); !errors.Is(err, ErrRecordField) {
		t.Errorf("nonzero pad: err = %v, want ErrRecordField", err)
	}
	bad = hornRecordBytes(1, 1, 1, 1, 1)
	bad[21] = 1
	if _, err := DecodeHornEvent(bad); !errors.Is(err, ErrRecordField) {
		t.Errorf("nonzero pad2: err = %v, want ErrRecordField", err)
	}
}

func FuzzDecodeRecords(f *testing.F) {
	f.Add(driftRecordBytes(1, 2, 0))
	f.Add(hornRecordBytes(1, 2, 3, 4, 5))
	f.Fuzz(func(t *testing.T, b []byte) {
		_, _ = DecodeDriftEvent(b) // must not panic
		_, _ = DecodeHornEvent(b)
	})
}

// ─── trigger policy (S5 Step 149) ───────────────────────────────────────────

func TestTokenBucket_BurstThenRate(t *testing.T) {
	t.Parallel()
	t0 := time.Unix(1000, 0)
	b := newTokenBucket(1, 3, t0)
	got := 0
	for i := 0; i < 50; i++ { // 50 requests inside the same instant
		if b.Allow(t0) {
			got++
		}
	}
	if got != 3 {
		t.Fatalf("burst allowed %d, want 3", got)
	}
	if b.Allow(t0.Add(500 * time.Millisecond)) {
		t.Fatal("half a second refilled a whole token")
	}
	if !b.Allow(t0.Add(1 * time.Second)) {
		t.Fatal("one second did not refill a token")
	}
	// A long gap refills to burst, never past it.
	n := 0
	for i := 0; i < 10; i++ {
		if b.Allow(t0.Add(time.Hour)) {
			n++
		}
	}
	if n != 3 {
		t.Fatalf("after a long gap allowed %d, want burst 3", n)
	}
	// A clock that steps backwards never mints tokens.
	if b.Allow(t0) {
		t.Fatal("backwards clock minted a token")
	}
}

func TestHornPolicy(t *testing.T) {
	t.Parallel()
	t0 := time.Unix(2000, 0)
	p := newHornPolicy(7, t0)
	rv := HornEvent{Kind: gjallarhorn.ReverifyUnicast, Cluster: 7}
	acts := map[HornAction]int{}
	// 50 reverifies inside one second (S5 Step 157/158): burst 3, plus at
	// most 1 more refilled within the second.
	for i := 0; i < 50; i++ {
		acts[p.Decide(rv, t0.Add(time.Duration(i)*20*time.Millisecond))]++
	}
	if acts[HornReverify] < 3 || acts[HornReverify] > 4 {
		t.Fatalf("reverifies in 1 s = %d, want 3..4 (%v)", acts[HornReverify], acts)
	}
	if acts[HornRateLimited] != 50-acts[HornReverify] {
		t.Fatalf("rate-limited = %d (%v)", acts[HornRateLimited], acts)
	}
	if got := p.Decide(HornEvent{Kind: gjallarhorn.ReverifyUnicast, Cluster: 8}, t0.Add(time.Hour)); got != HornOtherCluster {
		t.Fatalf("other cluster: %v", got)
	}
	if got := p.Decide(HornEvent{Kind: gjallarhorn.BootstrapBroadcast, Cluster: 7}, t0.Add(time.Hour)); got != HornBootstrap {
		t.Fatalf("bootstrap: %v", got)
	}
	// The other-cluster horn spent no token: a fresh hour still has burst.
	n := 0
	for i := 0; i < 5; i++ {
		if p.Decide(rv, t0.Add(2*time.Hour)) == HornReverify {
			n++
		}
	}
	if n != 3 {
		t.Fatalf("burst after idle = %d, want 3", n)
	}
	s := p.Stats()
	if s.OtherCluster != 1 || s.Bootstrap != 1 || s.Reverify != uint64(acts[HornReverify]+3) {
		t.Fatalf("stats %+v", s)
	}
}

func TestDebouncer_BoundedLatencyUnderNoise(t *testing.T) {
	t.Parallel()
	t0 := time.Unix(3000, 0)
	d := newDebouncer(250 * time.Millisecond)
	if d.Due(t0) {
		t.Fatal("due before any kick")
	}
	d.Kick(t0)
	// Continuous events every 10 ms must not push the scan out forever: it
	// fires 250 ms after the FIRST event of a burst.
	for i := 1; i < 25; i++ {
		now := t0.Add(time.Duration(i) * 10 * time.Millisecond)
		d.Kick(now)
		if d.Due(now) {
			t.Fatalf("fired early at +%dms", i*10)
		}
	}
	if w := d.Wait(t0.Add(240 * time.Millisecond)); w != 10*time.Millisecond {
		t.Fatalf("Wait = %v, want 10ms", w)
	}
	if !d.Due(t0.Add(250 * time.Millisecond)) {
		t.Fatal("not due at +250ms")
	}
	if d.Due(t0.Add(260 * time.Millisecond)) {
		t.Fatal("due twice for one burst")
	}
	if d.Armed() {
		t.Fatal("still armed after firing")
	}
}

func TestHookFilter_SkipsOwnPID(t *testing.T) {
	t.Parallel()
	f := hookFilter{selfPID: 10}
	if f.Wanted(BPFDriftEvent{PID: 10, Type: HookVFSWrite}) {
		t.Fatal("own write would re-trigger a scan forever")
	}
	if !f.Wanted(BPFDriftEvent{PID: 11, Type: HookVFSWrite}) {
		t.Fatal("other pid filtered")
	}
}

// ─── Run with hook events (S5 Step 150) ─────────────────────────────────────

// hookHarness runs a Heimdall over a lab manifest that watches one file
// with its correct hash, fed by a fake hook channel.
type hookHarness struct {
	h      *Heimdall
	file   string
	events chan hookEvent
	lines  chan string
	alerts chan string
	done   chan error
}

func newHookHarness(t *testing.T) *hookHarness {
	t.Helper()
	dir := t.TempDir()
	file := filepath.Join(dir, "watched.conf")
	if err := os.WriteFile(file, []byte("good\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte("good\n"))
	man := filepath.Join(dir, "m.yaml")
	body := "apiVersion: v1\nkind: BaselineManifest\nmetadata:\n  name: lab\n  version: \"1\"\nspec:\n  files:\n    - path: " +
		file + "\n      sha256: " + hex.EncodeToString(sum[:]) + "\n"
	if err := os.WriteFile(man, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	trust := Trust{AllowUnsigned: true}
	m, err := LoadManifest(man, trust)
	if err != nil {
		t.Fatal(err)
	}
	hh := &hookHarness{file: file, events: make(chan hookEvent, 64), lines: make(chan string, 256), alerts: make(chan string, 64), done: make(chan error, 1)}
	hh.h = &Heimdall{
		nodeID: "t", manifestPath: man, trust: trust, manifest: m,
		aggregator: enkrateia.NewAggregator(64),
		printAlert: func(s string) { hh.alerts <- s },
		logf: func(f string, a ...any) {
			select {
			case hh.lines <- fmt.Sprintf(f, a...):
			default:
			}
		},
		hornPolicy: newHornPolicy(1, time.Now()),
		hookEvents: hh.events,
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() { hh.done <- hh.h.Run(ctx, time.Hour) }()
	t.Cleanup(func() { cancel(); <-hh.done })
	return hh
}

func (hh *hookHarness) waitLine(t *testing.T, sub string, d time.Duration) {
	t.Helper()
	deadline := time.After(d)
	for {
		select {
		case l := <-hh.lines:
			if strings.Contains(l, sub) {
				return
			}
		case <-deadline:
			t.Fatalf("no log line containing %q within %v", sub, d)
		}
	}
}

func TestRun_FileEventScansWithinDebounce(t *testing.T) {
	hh := newHookHarness(t)
	if err := os.WriteFile(hh.file, []byte("tampered\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	hh.events <- hookEvent{Drift: &BPFDriftEvent{PID: uint32(os.Getpid()) + 1, Type: HookVFSWrite}}
	select {
	case a := <-hh.alerts:
		if !strings.Contains(a, hh.file) {
			t.Fatalf("alert %q does not name the file", a)
		}
		if el := time.Since(start); el > time.Second {
			t.Fatalf("alert after %v, want <= 1s", el)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no alert after a file event (the 1h poll must not be what catches it)")
	}
	// The same unchanged drift seen again by an event-driven scan is not
	// re-alerted: noise on a busy host must not become an alert storm.
	hh.events <- hookEvent{Drift: &BPFDriftEvent{PID: uint32(os.Getpid()) + 1, Type: HookExecve}}
	select {
	case a := <-hh.alerts:
		t.Fatalf("unchanged drift re-alerted: %q", a)
	case <-time.After(600 * time.Millisecond):
	}
}

func TestRun_OwnWritesIgnored(t *testing.T) {
	hh := newHookHarness(t)
	if err := os.WriteFile(hh.file, []byte("tampered\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	hh.events <- hookEvent{Drift: &BPFDriftEvent{PID: uint32(os.Getpid()), Type: HookVFSWrite}}
	select {
	case a := <-hh.alerts:
		t.Fatalf("own write triggered a scan: %q", a)
	case <-time.After(600 * time.Millisecond):
	}
}

func TestRun_HornReverifyIsImmediateAndRateLimited(t *testing.T) {
	hh := newHookHarness(t)
	if err := os.WriteFile(hh.file, []byte("tampered\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	hh.events <- hookEvent{Horn: &HornEvent{Kind: gjallarhorn.ReverifyUnicast, Cluster: 1, ManifestPtr: 0xCAFE}}
	hh.waitLine(t, "action=reverify", time.Second)
	select {
	case <-hh.alerts:
	case <-time.After(time.Second):
		t.Fatal("reverify did not scan")
	}
	// A flood: at most burst-1 more reverifies (one token already spent),
	// plus at most one refill.
	for i := 0; i < 50; i++ {
		hh.events <- hookEvent{Horn: &HornEvent{Kind: gjallarhorn.ReverifyUnicast, Cluster: 1}}
	}
	for i := 0; i < 5; i++ {
		hh.events <- hookEvent{Horn: &HornEvent{Kind: gjallarhorn.ReverifyUnicast, Cluster: 99}}
	}
	time.Sleep(300 * time.Millisecond)
	s := hh.h.hornPolicy.Stats()
	if s.Reverify > 4 {
		t.Fatalf("flood ran %d reverifies, want <= 4", s.Reverify)
	}
	if s.RateLimited < 46 || s.OtherCluster != 5 {
		t.Fatalf("stats %+v", s)
	}
}
