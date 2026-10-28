// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

//go:build linux

package cartridge

// S5 Phase 13: the token-bucket rate limiter cartridge (cartridges/ratelimit.s)
// through BPF_PROG_TEST_RUN, against the qos-ebpf token-bucket math (Go
// re-implementation below) and against the Rust reference runner.

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cilium/ebpf"

	"unheaded/pkg/protocol/encoding"
)

const rlMagic = 274_877_907 // ceil(2^35 / 125), see ratelimit.s

func TestCRC16CCITT_KnownVector(t *testing.T) {
	if got := encoding.CRC16CCITT([]byte("123456789")); got != 0x29B1 {
		t.Fatalf("crc = %#04x, want 0x29b1", got)
	}
}

// rlImage builds ratelimit.s with DATA {rate, cap, M}.
func rlImage(t *testing.T, rate, capBytes uint32) []byte {
	t.Helper()
	data := make([]byte, 12)
	binary.LittleEndian.PutUint32(data[0:], rate)
	binary.LittleEndian.PutUint32(data[4:], capBytes)
	binary.LittleEndian.PutUint32(data[8:], rlMagic)
	p := filepath.Join(t.TempDir(), "rl.data.bin")
	if err := os.WriteFile(p, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return buildCart(t, "../../cartridges/ratelimit.s", "xdp", "--data", p)
}

// qosBucket is qos-ebpf's per-flow token bucket (ebpf/qos-ebpf/src/main.rs,
// "Token Bucket Rate Limiting"; refill = monad_common::token_bucket::refill
// since 8ab534b3), in its u64 arithmetic: whole 8,000 ns quanta are credited
// and last advances by exactly those quanta (the remainder carries); a refill
// that reaches the capacity sets last = now.
type qosBucket struct {
	ok           bool
	tokens, last uint64
}

func (b *qosBucket) packet(rate, capBytes, now, pkt uint64) uint32 {
	if rate == 0 {
		return xdpPass
	}
	if !b.ok {
		b.ok, b.last = true, now
		if pkt < capBytes {
			b.tokens = capBytes - pkt
		}
		return xdpPass
	}
	elapsed := uint64(0)
	if now > b.last {
		elapsed = now - b.last
	}
	if q := elapsed / 8000; q > 0 {
		if q > capBytes/rate || b.tokens >= capBytes || q*rate >= capBytes-b.tokens {
			b.tokens, b.last = capBytes, now
		} else {
			b.tokens += q * rate
			b.last += q * 8000
		}
	}
	if b.tokens < pkt {
		return xdpDrop
	}
	b.tokens -= pkt
	return xdpPass
}

// simFlow tracks what cart_xdp's FLOW_META yields for FIELD 7-9.
type simFlow struct {
	label        string
	sport, dport uint16
	last         uint64
	pkts, bytes  uint32
	q            qosBucket
}

func (f *simFlow) fields(now uint64, n int) []uint32 {
	iat := uint32(0)
	if f.pkts > 0 {
		iat = iatBucket(now - f.last)
	}
	f.last, f.pkts, f.bytes = now, f.pkts+1, f.bytes+uint32(n)                                                // #nosec G115 -- n < 1600
	return []uint32{4, 17, uint32(f.sport), uint32(f.dport), uint32(n), 64, 0, iat, f.pkts, f.bytes, 0, 0, 0} // #nosec G115 -- small values
}

// refHashes runs `cart run` on the input log and returns final hash per label.
func refHashes(t *testing.T, raw []byte, log []byte) (map[string]uint32, []string) {
	t.Helper()
	dir := t.TempDir()
	img, lp := filepath.Join(dir, "x.cart"), filepath.Join(dir, "in.log")
	if err := os.WriteFile(img, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lp, log, 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(cartBin(t), "run", img, "--inputs", lp).CombinedOutput() // #nosec G204 -- test-only
	if err != nil {
		t.Fatalf("cart run: %v\n%s", err, out)
	}
	final := map[string]uint32{}
	var verdicts []string
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		switch f[0] {
		case "final":
			h, _ := strconv.ParseUint(strings.TrimPrefix(f[2], "hash="), 16, 32)
			final[strings.TrimPrefix(f[1], "flow=")] = uint32(h)
		case "pkt":
			verdicts = append(verdicts, strings.TrimPrefix(f[1], "flow=")+" "+strings.TrimPrefix(f[3], "verdict="))
		}
	}
	return final, verdicts
}

// TestRatelimit_BPFvsOracleAndReference (Steps 336-337, 344): two flows, a
// flood and a quiet one, on a fixed clock. Every BPF verdict equals the
// qos-ebpf math; final state hashes equal the reference runner's; the quiet
// flow is never limited by the flood.
func TestRatelimit_BPFvsOracleAndReference(t *testing.T) {
	c := loadCart(t)
	const rate, capB = 10, 3000
	raw := rlImage(t, rate, capB)
	mustSlot(t, c, 0, raw, 1)
	flood := &simFlow{label: "flood", sport: 1111, dport: 80}
	quiet := &simFlow{label: "quiet", sport: 2222, dport: 80}
	var log bytes.Buffer
	now := uint64(7_000_000_000)
	seed := uint64(0xBEEF)
	next := func() uint64 { seed ^= seed << 13; seed ^= seed >> 7; seed ^= seed << 17; return seed }
	var bpf, oracle []string
	drops := map[string]int{}
	for i := 0; i < 1000; i++ {
		f, n := flood, 200+int(next()%1300)
		if i%10 == 9 {
			f, n = quiet, 60
		}
		gap := next() % 20_000
		if next()%50 == 0 {
			gap = 4_294_967_296 + next()%1_000_000 // past the 32-bit clock wrap
		}
		now += gap
		if err := c.SetCfg(CfgClockNS, now); err != nil {
			t.Fatal(err)
		}
		r, err := c.RunXDP(IPv4Packet(17, ipA, ipB, f.sport, f.dport, n))
		if err != nil {
			t.Fatal(err)
		}
		want := f.q.packet(rate, capB, now, uint64(n)) // #nosec G115 -- n > 0
		bpf = append(bpf, fmt.Sprintf("%s %d", f.label, r))
		oracle = append(oracle, fmt.Sprintf("%s %d", f.label, want))
		if r == xdpDrop {
			drops[f.label]++
		}
		fmt.Fprintf(&log, "%s %d", f.label, now)
		for _, v := range f.fields(now, n) {
			fmt.Fprintf(&log, " %d", v)
		}
		log.WriteString("\n")
	}
	mism := 0
	for i := range bpf {
		if bpf[i] != oracle[i] {
			mism++
			if mism < 5 {
				t.Errorf("packet %d: BPF %s, qos-ebpf math %s", i, bpf[i], oracle[i])
			}
		}
	}
	final, _ := refHashes(t, raw, log.Bytes())
	for _, f := range []*simFlow{flood, quiet} {
		key := FlowKey{Sport: f.sport, Dport: f.dport, Proto: 17, L3: 4, Slot: 0}
		copy(key.Src[:], ipA[:])
		copy(key.Dst[:], ipB[:])
		st, ok, err := c.State(key)
		if err != nil || !ok {
			t.Fatalf("%s: no state (%v)", f.label, err)
		}
		if st.Hash() != final[f.label] {
			t.Errorf("%s: BPF hash %08x, reference %08x", f.label, st.Hash(), final[f.label])
		}
	}
	if drops["flood"] < 100 || drops["quiet"] != 0 {
		t.Errorf("drops %v: the flood must be limited, the quiet flow never", drops)
	}
	t.Logf("RATELIMIT BPF==qos-ebpf math on %d/%d packets; hashes == reference; drops %v", len(bpf)-mism, len(bpf), drops)
}

// TestRatelimit_EnforcedRate (Steps 341-342): offered load above the rate,
// passed bytes / time within 5 % of the configured rate on a fixed clock.
//
// Until 8ab534b3 qos-ebpf's refill (and so the cartridge's) discarded the
// remainder of elapsed / 8000 at every refill, so a 100 Mbps bucket admitted
// 80 Mbps at 20 us spacing. Both now carry the remainder: the test gates the
// rate at a quantum-aligned spacing (24 us) and at one that is not (20 us),
// and checks every verdict against the oracle. The real-clock flood
// (test_run repeat, ~0.5 us apart) adds wall-clock overhead; it is reported
// with a -10 % bound.
func TestRatelimit_EnforcedRate(t *testing.T) {
	c := loadCart(t)
	const rate, capB, L = 100, 12_500, 1000 // 100 Mbps = 12.5 bytes/us
	mustSlot(t, c, 0, rlImage(t, rate, capB), 1)

	// Fixed clock, one fresh flow per spacing: 24 us (quantum-aligned,
	// offers 333 Mbps) and 20 us (not aligned, offers 400 Mbps).
	const n = 20_000
	var got float64
	for k, spacing := range []uint64{24_000, 20_000} {
		var q qosBucket
		now, passed, oracle := uint64(1_000_000_000), 0, 0
		p := IPv4Packet(17, ipA, ipB, uint16(5000+k), 9000, L) // #nosec G115 -- k < 2
		for i := 0; i < n; i++ {
			now += spacing
			if err := c.SetCfg(CfgClockNS, now); err != nil {
				t.Fatal(err)
			}
			r, err := c.RunXDP(p)
			if err != nil {
				t.Fatal(err)
			}
			if r == xdpPass {
				passed++
			}
			if q.packet(rate, capB, now, L) == xdpPass {
				oracle++
			}
		}
		span := float64(n*spacing) / 1e9
		got = (float64(passed*L) - capB) * 8 / span / 1e6
		t.Logf("RATE fixed clock %d us: %d/%d passed (qos-ebpf math %d), enforced %.2f Mbps (configured %d)", spacing/1000, passed, n, oracle, got, rate)
		if passed != oracle {
			t.Errorf("spacing %d: %d passes, qos-ebpf math %d", spacing, passed, oracle)
		}
		if got < rate*0.95 || got > rate*1.05 {
			t.Errorf("fixed-clock enforced rate %.2f Mbps, want %d +- 5%%", got, rate)
		}
	}

	// Real clock: a flood through test_run repeat on a fresh flow.
	if err := c.SetCfg(CfgClockNS, 0); err != nil {
		t.Fatal(err)
	}
	mustSlot(t, c, 0, rlImage(t, rate, capB), 2)
	s0, _ := c.Stats()
	t0 := time.Now()
	if _, _, err := c.BenchXDP(IPv4Packet(17, ipA, ipB, 4001, 9000, L), 2_000_000); err != nil {
		t.Fatal(err)
	}
	wall := time.Since(t0).Seconds()
	s1, _ := c.Stats()
	pass := SlotStat(s1, 0, StatPass) - SlotStat(s0, 0, StatPass)
	runs := SlotStat(s1, 0, StatRuns) - SlotStat(s0, 0, StatRuns)
	got = (float64(pass*L) - capB) * 8 / wall / 1e6
	t.Logf("RATE real clock: %d/%d passed in %.3f s, enforced %.2f Mbps (configured %d), offered %.0f Mbps", pass, runs, wall, got, rate, float64(runs*L)*8/wall/1e6)
	if got < rate*0.90 || got > rate*1.05 {
		t.Errorf("real-clock enforced rate %.2f Mbps, want %d -10%%/+5%%", got, rate)
	}
}

// TestBench_InterpreterStep (Step 340 profile, CART_BENCH=1): the cost of one
// interpreted instruction, from straight-line cartridges of 60 copies of one
// instruction + VERDICT, against pass.s (VERDICT only): NOP (first in the
// dispatch chain), MOV, LD from the flow window, SYSCALL NOW_NS_LO.
func TestBench_InterpreterStep(t *testing.T) {
	benchEnabled(t)
	c := loadCart(t)
	pkt := IPv4Packet(17, ipA, ipB, 4000, 53, 128)
	xdp := func() (time.Duration, error) {
		_, d, err := c.BenchXDP(pkt, benchRepeat)
		return d, err
	}
	mustSlot(t, c, 0, buildCart(t, "../../cartridges/pass.s", "xdp"), 1)
	base := bench5(t, xdp)
	logBench(t, "step_base_pass", base)
	for i, ins := range []string{"NOP", "MOV r2, r3", "LD r2, [r15+4]", "SYSCALL 1"} {
		src := strings.Repeat("        "+ins+"\n", 60) + "        MOVI r1, 0\n        SYSCALL 2\n"
		p := filepath.Join(t.TempDir(), "s.s")
		if err := os.WriteFile(p, []byte(src), 0o600); err != nil {
			t.Fatal(err)
		}
		mustSlot(t, c, 0, buildCart(t, p, "xdp"), uint32(2+i)) // #nosec G115 -- tiny
		r := bench5(t, xdp)
		logBench(t, "step_60x_"+strings.Fields(ins)[0], r)
		t.Logf("BENCH step %s: %.1f ns/insn over pass.s", ins, float64(r.median-base.median)/60)
	}
}

// TestBench_Ratelimit (Steps 338-340, CART_BENCH=1): ns/pkt of the
// ratelimit cartridge vs b0_pass and vs native qos-ebpf's token-bucket path,
// on the same IPv6 + Monad HbH frame, in two regimes: "pass" (cap large, every
// packet admitted) and "drop" (steady state over the rate).
func TestBench_Ratelimit(t *testing.T) {
	benchEnabled(t)
	c := loadCart(t)
	pkt := IPv6MonadPacket(0x1234, 4000, 80, 3, 0, 128)
	xdp := func() (time.Duration, error) {
		_, d, err := c.BenchXDP(pkt, benchRepeat)
		return d, err
	}
	mustSlot(t, c, 0, buildCart(t, "../../cartridges/pass.s", "xdp"), 1)
	logBench(t, "b0_pass_v6", bench5(t, xdp))
	for gen, rg := range []struct {
		name       string
		rate, capB uint32
	}{{"pass", 8000, 1_000_000_000}, {"drop", 100, 12_500}} {
		// A new generation per regime: the flow's bucket must start fresh.
		mustSlot(t, c, 0, rlImage(t, rg.rate, rg.capB), uint32(10+gen)) // #nosec G115 -- tiny
		s0, _ := c.Stats()
		logBench(t, "ratelimit_"+rg.name, bench5(t, xdp))
		s1, _ := c.Stats()
		runs := SlotStat(s1, 0, StatRuns) - SlotStat(s0, 0, StatRuns)
		t.Logf("BENCH ratelimit_%s insns/pkt=%.1f pass=%d drop=%d", rg.name,
			float64(SlotStat(s1, 0, StatInsns)-SlotStat(s0, 0, StatInsns))/float64(runs),
			SlotStat(s1, 0, StatPass)-SlotStat(s0, 0, StatPass), SlotStat(s1, 0, StatDrop)-SlotStat(s0, 0, StatDrop))
	}

	obj := filepath.Join("..", "..", "ebpf", "target", "bpfel-unknown-none", "release", "qos-ebpf")
	if _, err := os.Stat(obj); err != nil {
		t.Skipf("qos-ebpf not built (cargo build --release -p qos-ebpf in ebpf/): %v", err)
	}
	for _, rg := range []struct {
		name       string
		rate, capB uint64
	}{{"pass", 8000, 1_000_000_000}, {"drop", 100, 12_500}} {
		coll, err := ebpf.LoadCollection(obj)
		if err != nil {
			t.Fatal(err)
		}
		cfg := coll.Maps["CONFIG"]
		if err := cfg.Put(uint32(1), rg.rate); err != nil { // default_rate_limit_mbps
			t.Fatal(err)
		}
		if err := cfg.Put(uint32(2), rg.capB); err != nil { // default_burst_bytes
			t.Fatal(err)
		}
		p := coll.Programs["qos_xdp"]
		logBench(t, "native_qos_bucket_"+rg.name, bench5(t, func() (time.Duration, error) {
			_, d, err := p.Benchmark(pkt, benchRepeat, nil)
			return d, err
		}))
		stat := func(k uint32) uint64 {
			var v uint64
			_ = coll.Maps["STATS"].Lookup(k, &v)
			return v
		}
		// 10 bucket created, 4 tokens consumed, 2 dropped by the rate limiter.
		t.Logf("BENCH native_qos_bucket_%s path: packets=%d created=%d consumed=%d rate_drops=%d crc_errors=%d", rg.name, stat(0), stat(10), stat(4), stat(2), stat(5))
		if stat(10) != 1 || stat(4)+stat(2) == 0 {
			t.Errorf("native qos-ebpf did not take its token-bucket path")
		}
		coll.Close()
	}
}
