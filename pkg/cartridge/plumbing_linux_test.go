// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

//go:build linux

package cartridge

// Root-gated cartridge runs through BPF_PROG_TEST_RUN (S5 Phase 11 Steps
// 293-299). Nothing is attached. Images are built by the real toolchain
// (`cart build`, crates/upc-cartridge; CART_BIN overrides the path).

import (
	"bufio"
	"bytes"
	"fmt"
	"math/bits"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const (
	xdpDrop = 1
	xdpPass = 2
)

func cartBin(t *testing.T) string {
	t.Helper()
	p := os.Getenv("CART_BIN")
	if p == "" {
		p = filepath.Join("..", "..", "crates", "upc-cartridge", "target", "release", "cart")
	}
	if _, err := os.Stat(p); err != nil {
		t.Skipf("cart CLI not built (cargo build --release in crates/upc-cartridge): %v", err)
	}
	return p
}

// buildCart assembles asm with `cart build` and returns the image bytes.
func buildCart(t *testing.T, asm, stage string) []byte {
	t.Helper()
	out := filepath.Join(t.TempDir(), "x.cart")
	cmd := exec.Command(cartBin(t), "build", "--stage", stage, "--asm", asm, "-o", out) // #nosec G204 -- test-only, fixed binary
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("cart build %s: %v\n%s", asm, err, b)
	}
	b, err := os.ReadFile(out) // #nosec G304 -- test temp file
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func loadCart(t *testing.T) *Cart {
	t.Helper()
	c, err := Load(objPath(t), Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	return c
}

func mustSlot(t *testing.T, c *Cart, slot int, raw []byte, gen uint32) *Image {
	t.Helper()
	img, err := DecodeImage(raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.WriteSlot(slot, img, gen); err != nil {
		t.Fatal(err)
	}
	return img
}

var (
	ipA = [4]byte{10, 0, 0, 1}
	ipB = [4]byte{10, 0, 0, 2}
)

// TestPlumbing_PassAndDropUDP9 (Steps 294-295): pass.s passes everything;
// drop-udp-9.s drops only UDP to port 9; counters move per slot.
func TestPlumbing_PassAndDropUDP9(t *testing.T) {
	c := loadCart(t)
	pkts := map[string][]byte{
		"udp/9":  IPv4Packet(17, ipA, ipB, 4000, 9, 80),
		"udp/53": IPv4Packet(17, ipA, ipB, 4000, 53, 80),
		"tcp/22": IPv4Packet(6, ipA, ipB, 4000, 22, 80),
	}
	run := func(name string) uint32 {
		r, err := c.RunXDP(pkts[name])
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	mustSlot(t, c, 0, buildCart(t, "../../cartridges/pass.s", "xdp"), 1)
	for name := range pkts {
		if r := run(name); r != xdpPass {
			t.Errorf("pass.s %s: ret %d, want PASS", name, r)
		}
	}
	mustSlot(t, c, 0, buildCart(t, "../../cartridges/drop-udp-9.s", "xdp"), 2)
	want := map[string]uint32{"udp/9": xdpDrop, "udp/53": xdpPass, "tcp/22": xdpPass}
	for name, w := range want {
		if r := run(name); r != w {
			t.Errorf("drop-udp-9.s %s: ret %d, want %d", name, r, w)
		}
	}
	s, err := c.Stats()
	if err != nil {
		t.Fatal(err)
	}
	if SlotStat(s, 0, StatRuns) != 6 || SlotStat(s, 0, StatDrop) != 1 || SlotStat(s, 0, StatPass) != 5 || SlotStat(s, 0, StatFaults) != 0 {
		t.Fatalf("slot 0 runs=%d pass=%d drop=%d faults=%d", SlotStat(s, 0, StatRuns), SlotStat(s, 0, StatPass), SlotStat(s, 0, StatDrop), SlotStat(s, 0, StatFaults))
	}
	if SlotStat(s, 1, StatRuns) != 0 {
		t.Fatal("an inactive slot ran")
	}
	t.Logf("slot0 runs=%d pass=%d drop=%d insns=%d", SlotStat(s, 0, StatRuns), SlotStat(s, 0, StatPass), SlotStat(s, 0, StatDrop), SlotStat(s, 0, StatInsns))
}

// TestFault_PassesAndTripsBreaker (Steps 296-297): a fault passes the packet
// and counts; 64 faults inside one second disable the slot.
func TestFault_PassesAndTripsBreaker(t *testing.T) {
	c := loadCart(t)
	mustSlot(t, c, 1, buildCart(t, "testdata/fault.s", "xdp"), 1)
	if err := c.SetCfg(CfgClockNS, 10_000_000_000); err != nil { // fixed clock: one window
		t.Fatal(err)
	}
	pkt := IPv4Packet(17, ipA, ipB, 1, 2, 64)
	for i := 1; i <= 70; i++ {
		r, err := c.RunXDP(pkt)
		if err != nil {
			t.Fatal(err)
		}
		if r != xdpPass {
			t.Fatalf("packet %d: ret %d, a fault must pass", i, r)
		}
	}
	s, _ := c.Stats()
	cfg, err := c.SlotConfig(1)
	if err != nil {
		t.Fatal(err)
	}
	if got := SlotStat(s, 1, StatFaults); got != 64 {
		t.Errorf("faults = %d, want 64 (the slot stops running at the 64th)", got)
	}
	if SlotStat(s, 1, StatRuns) != 64 || cfg.Active != 0 || s[StatBreakerTrips] != 1 {
		t.Fatalf("runs=%d active=%d trips=%d", SlotStat(s, 1, StatRuns), cfg.Active, s[StatBreakerTrips])
	}
}

func iatBucket(ns uint64) uint32 {
	b := uint32(bits.Len64(ns/1000+1) - 1)
	if b > 31 {
		b = 31
	}
	return b
}

// TestBitExact_BPFvsReference (Steps 298-299): the same packets through
// cart_xdp and through upc-cartridge's reference runner reach the same state
// hash for every flow, and emit the same events in the same order.
func TestBitExact_BPFvsReference(t *testing.T) {
	c := loadCart(t)
	raw := buildCart(t, "testdata/counter.s", "xdp")
	mustSlot(t, c, 2, raw, 9)
	rd, err := c.Events()
	if err != nil {
		t.Fatal(err)
	}
	defer rd.Close()

	type flow struct {
		label        string
		sport, dport uint16
		last         uint64
		pkts, bytes  uint32
	}
	flows := []*flow{{label: "a", sport: 1000, dport: 53}, {label: "b", sport: 2000, dport: 443}, {label: "c", sport: 3000, dport: 9}}
	var log bytes.Buffer
	now := uint64(5_000_000_000)
	seed := uint64(0x5EED)
	next := func() uint64 { seed ^= seed << 13; seed ^= seed >> 7; seed ^= seed << 17; return seed }
	for i := 0; i < 90; i++ {
		f := flows[next()%3]
		n := 64 + int(next()%1200)
		now += next() % 3_000_000 // up to 3 ms apart
		if err := c.SetCfg(CfgClockNS, now); err != nil {
			t.Fatal(err)
		}
		if r, err := c.RunXDP(IPv4Packet(17, ipA, ipB, f.sport, f.dport, n)); err != nil || r != xdpPass {
			t.Fatalf("packet %d: ret %d err %v", i, r, err)
		}
		iat := uint32(0)
		if f.pkts > 0 {
			iat = iatBucket(now - f.last)
		}
		f.last, f.pkts, f.bytes = now, f.pkts+1, f.bytes+uint32(n)                                                   // #nosec G115 -- n < 1300
		fields := []uint32{4, 17, uint32(f.sport), uint32(f.dport), uint32(n), 64, 0, iat, f.pkts, f.bytes, 0, 0, 0} // #nosec G115 -- small test values
		fmt.Fprintf(&log, "%s %d", f.label, now)
		for _, v := range fields {
			fmt.Fprintf(&log, " %d", v)
		}
		log.WriteString("\n")
	}
	imgPath := filepath.Join(t.TempDir(), "counter.cart")
	logPath := filepath.Join(t.TempDir(), "inputs.log")
	if err := os.WriteFile(imgPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(logPath, log.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(cartBin(t), "run", imgPath, "--inputs", logPath).CombinedOutput() // #nosec G204 -- test-only
	if err != nil {
		t.Fatalf("cart run: %v\n%s", err, out)
	}
	refFinal := map[string]uint32{}
	var refEmits []string
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		switch f[0] {
		case "final":
			h, _ := strconv.ParseUint(strings.TrimPrefix(f[2], "hash="), 16, 32)
			refFinal[strings.TrimPrefix(f[1], "flow=")] = uint32(h)
		case "emit":
			refEmits = append(refEmits, strings.TrimPrefix(f[1], "flow=")+" "+strings.Join(f[2:], " "))
		}
	}
	agree := 0
	for _, f := range flows {
		key := FlowKey{Sport: f.sport, Dport: f.dport, Proto: 17, L3: 4, Slot: 2}
		copy(key.Src[:], ipA[:])
		copy(key.Dst[:], ipB[:])
		st, ok, err := c.State(key)
		if err != nil || !ok {
			t.Fatalf("flow %s: no BPF state (%v)", f.label, err)
		}
		if st.Hash() != refFinal[f.label] {
			t.Errorf("flow %s: BPF hash %08x, reference %08x (mem %v)", f.label, st.Hash(), refFinal[f.label], st.Mem[:6])
		} else {
			agree++
		}
		if st.Gen != 9 {
			t.Errorf("flow %s: gen %d", f.label, st.Gen)
		}
	}
	evs, err := c.DrainEvents(rd)
	if err != nil {
		t.Fatal(err)
	}
	label := map[uint16]string{1000: "a", 2000: "b", 3000: "c"}
	var bpfEmits []string
	for _, e := range evs {
		bpfEmits = append(bpfEmits, fmt.Sprintf("%s %d %d %d", label[e.Flow.Sport], e.Kind, e.A, e.B))
	}
	if strings.Join(bpfEmits, "|") != strings.Join(refEmits, "|") {
		t.Errorf("events differ\nBPF: %v\nref: %v", bpfEmits, refEmits)
	}
	t.Logf("BITEXACT %d/%d flows agree, %d events identical", agree, len(flows), len(bpfEmits))
}
