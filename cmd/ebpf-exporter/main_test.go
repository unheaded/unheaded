// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"testing"
	"unsafe"
)

// ─── MonadEvent layout pin ──────────────────────────────────────────────────

func TestMonadEvent_LayoutMatchesKernelStruct(t *testing.T) {
	t.Parallel()
	// Per source comment: kernel-side struct monad_event is
	//   u64 ts_ns; u32 flow_id; u16 action; u8 hop; u8 pad; u32 latency_ns;
	// Sum of fields = 8+4+2+1+1+4 = 20 bytes. Go's #[repr(C)]-equivalent
	// for plain struct is sequential fields with natural alignment, so
	// total = 24 (the u64 forces 8-byte struct alignment, padding the
	// trailing u32 up to 24). KEEP THIS IN SYNC with monad.bpf.c.
	const want = 24
	got := int(unsafe.Sizeof(MonadEvent{}))
	if got != want {
		t.Errorf("sizeof(MonadEvent) = %d, want %d — wire-format mismatch with monad.bpf.c?", got, want)
	}
	if monandEventSize != got {
		t.Errorf("monandEventSize const (%d) != sizeof (%d) — recompute the const",
			monandEventSize, got)
	}
}

func TestMonadEvent_FieldOffsets(t *testing.T) {
	t.Parallel()
	// Pin field offsets so any field reorder surfaces as a wire-format
	// change. The exporter reads ringbuf bytes byte-for-byte; offsets
	// MUST match the kernel-side layout.
	var ev MonadEvent
	base := uintptr(unsafe.Pointer(&ev))
	cases := []struct {
		name string
		off  uintptr
		want uintptr
	}{
		{"TsNs", uintptr(unsafe.Pointer(&ev.TsNs)) - base, 0},
		{"FlowID", uintptr(unsafe.Pointer(&ev.FlowID)) - base, 8},
		{"Action", uintptr(unsafe.Pointer(&ev.Action)) - base, 12},
		{"Hop", uintptr(unsafe.Pointer(&ev.Hop)) - base, 14},
		{"Pad", uintptr(unsafe.Pointer(&ev.Pad)) - base, 15},
		{"LatencyNs", uintptr(unsafe.Pointer(&ev.LatencyNs)) - base, 16},
	}
	for _, c := range cases {
		if c.off != c.want {
			t.Errorf("offsetof(%s) = %d, want %d", c.name, c.off, c.want)
		}
	}
}

// ─── xdpActions enum ─────────────────────────────────────────────────────────

func TestXdpActions_AllStandardCodesMapped(t *testing.T) {
	t.Parallel()
	// Linux kernel's standard XDP return codes per uapi/linux/bpf.h:
	// XDP_ABORTED=0, XDP_DROP=1, XDP_PASS=2, XDP_TX=3, XDP_REDIRECT=4
	want := map[uint32]string{
		0: "aborted",
		1: "drop",
		2: "pass",
		3: "tx",
		4: "redirect",
	}
	for code, label := range want {
		got, ok := xdpActions[code]
		if !ok {
			t.Errorf("XDP code %d (%s) missing from xdpActions map", code, label)
			continue
		}
		if got != label {
			t.Errorf("xdpActions[%d] = %q, want %q", code, got, label)
		}
	}
	if len(xdpActions) != 5 {
		t.Errorf("xdpActions has %d entries, expected exactly 5 (XDP_ABORTED..XDP_REDIRECT)", len(xdpActions))
	}
}

// ─── BPF map name pins ──────────────────────────────────────────────────────

func TestBPFMapNames_PinnedToKernelDefinitions(t *testing.T) {
	t.Parallel()
	// These constants MUST match the kernel-side C definitions per the
	// source comment. Hardcoded test values catch accidental rename.
	cases := map[string]string{
		"mapXDPLatency": mapXDPLatency,
		"mapPktCount":   mapPktCount,
		"mapRingEvents": mapRingEvents,
		"mapDropCount":  mapDropCount,
	}
	expected := map[string]string{
		"mapXDPLatency": "unheaded_xdp_latency_ns",
		"mapPktCount":   "unheaded_pkt_count",
		"mapRingEvents": "unheaded_ring_events",
		"mapDropCount":  "unheaded_drop_count",
	}
	for name, got := range cases {
		if got != expected[name] {
			t.Errorf("%s = %q, want %q (match kernel-side BPF map name)", name, got, expected[name])
		}
	}
}

func TestBPFMapNames_AllUseUnheadedPrefix(t *testing.T) {
	t.Parallel()
	for _, name := range []string{mapXDPLatency, mapPktCount, mapRingEvents, mapDropCount} {
		if len(name) < 9 || name[:9] != "unheaded_" {
			t.Errorf("BPF map name %q does not use kingdom convention 'unheaded_' prefix", name)
		}
	}
}
