// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

//go:build linux

package cartridge

// S5 Phase 14: the flow sampler cartridge (cartridges/sampler.s) through
// BPF_PROG_TEST_RUN, beside the rate limiter, against the reference runner.

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const samplerData = "../../cartridges/sampler.data.bin" // N = 8

func samplerImage(t *testing.T, n uint32) []byte {
	t.Helper()
	p := samplerData
	if n != 8 {
		b := make([]byte, 4)
		binary.LittleEndian.PutUint32(b, n)
		p = filepath.Join(t.TempDir(), "s.data.bin")
		if err := os.WriteFile(p, b, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return buildCart(t, "../../cartridges/sampler.s", "xdp", "--data", p)
}

// cartRunLines runs `cart run` on an XDP-stage input log; output lines.
func cartRunLines(t *testing.T, raw, log []byte) []string {
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
	var lines []string
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	return lines
}

// tcpPacket is IPv4Packet with TCP flags set.
func tcpPacket(sport, dport uint16, flags uint8, n int) []byte {
	p := IPv4Packet(6, ipA, ipB, sport, dport, n)
	p[14+20+13] = flags
	return p
}

type sampFlow struct {
	simFlow
	proto uint8
}

// TestSampler_BPFvsReference (Steps 355-357): rate limiter in slot 0, sampler
// (N = 8) in slot 1, a mixed TCP/UDP trace on a fixed clock. The sampler's
// events, queued snapshots and flow hashes equal the reference runner's; the
// rate limiter's verdicts still equal the qos-ebpf math (slots independent).
func TestSampler_BPFvsReference(t *testing.T) {
	c := loadCart(t)
	const rate, capB = 50, 6000
	mustSlot(t, c, 0, rlImage(t, rate, capB), 1)
	raw := samplerImage(t, 8)
	mustSlot(t, c, 1, raw, 1)
	rd, err := c.Events()
	if err != nil {
		t.Fatal(err)
	}
	defer rd.Close()
	flows := []*sampFlow{
		{simFlow: simFlow{label: "web", sport: 40000, dport: 443}, proto: 6},
		{simFlow: simFlow{label: "ssh", sport: 40001, dport: 22}, proto: 6},
		{simFlow: simFlow{label: "dns", sport: 40002, dport: 53}, proto: 17},
	}
	tcpFlags := []uint8{0x02, 0x10, 0x18, 0x11, 0x12, 0x04}
	var log bytes.Buffer
	now := uint64(3_000_000_000)
	seed := uint64(0xC0FFEE)
	next := func() uint64 { seed ^= seed << 13; seed ^= seed >> 7; seed ^= seed << 17; return seed }
	mism := 0
	for i := 0; i < 600; i++ {
		f := flows[next()%3]
		n := 60 + int(next()%1400)
		now += next() % 400_000
		if err := c.SetCfg(CfgClockNS, now); err != nil {
			t.Fatal(err)
		}
		var pkt []byte
		fl := uint8(0)
		if f.proto == 6 {
			fl = tcpFlags[next()%uint64(len(tcpFlags))]
			pkt = tcpPacket(f.sport, f.dport, fl, n)
		} else {
			pkt = IPv4Packet(17, ipA, ipB, f.sport, f.dport, n)
		}
		r, err := c.RunXDP(pkt)
		if err != nil {
			t.Fatal(err)
		}
		if want := f.q.packet(rate, capB, now, uint64(n)); r != want { // #nosec G115 -- n > 0
			mism++
		}
		fields := f.fields(now, n)
		fields[1], fields[6] = uint32(f.proto), uint32(fl)
		fmt.Fprintf(&log, "%s %d", f.label, now)
		for _, v := range fields {
			fmt.Fprintf(&log, " %d", v)
		}
		log.WriteString("\n")
	}
	if mism != 0 {
		t.Errorf("rate limiter beside the sampler: %d verdicts differ from the qos-ebpf math", mism)
	}
	byPort := map[uint16]string{40000: "web", 40001: "ssh", 40002: "dns"}
	var refEmits, refQueue []string
	refFinal := map[string]string{}
	for _, l := range cartRunLines(t, raw, log.Bytes()) {
		f := strings.Fields(l)
		lab := strings.TrimPrefix(f[1], "flow=")
		switch f[0] {
		case "emit":
			refEmits = append(refEmits, lab+" "+strings.Join(f[2:], " "))
		case "queue":
			refQueue = append(refQueue, lab+" "+strings.Join(f[2:], " "))
		case "final":
			refFinal[lab] = strings.TrimPrefix(f[2], "hash=")
		}
	}
	evs, err := c.DrainEvents(rd)
	if err != nil {
		t.Fatal(err)
	}
	var bpfEmits []string
	for _, e := range evs {
		if e.Slot != 1 {
			t.Errorf("event from slot %d (the rate limiter emits nothing)", e.Slot)
			continue
		}
		bpfEmits = append(bpfEmits, fmt.Sprintf("%s %d %d %d", byPort[e.Flow.Sport], e.Kind, e.A, e.B))
	}
	var bpfQueue []string
	for {
		j, ok, err := c.PopJob()
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			break
		}
		if j.SrcSlot != 1 || j.Flow.Slot != 1 {
			t.Errorf("job from slot %d/%d", j.SrcSlot, j.Flow.Slot)
		}
		fs := make([]string, len(j.Feat))
		for i, v := range j.Feat {
			fs[i] = fmt.Sprint(v)
		}
		bpfQueue = append(bpfQueue, byPort[j.Flow.Sport]+" "+strings.Join(fs, " "))
	}
	if strings.Join(bpfEmits, "|") != strings.Join(refEmits, "|") {
		t.Errorf("sample events differ\nBPF: %v\nref: %v", bpfEmits, refEmits)
	}
	if strings.Join(bpfQueue, "|") != strings.Join(refQueue, "|") {
		t.Errorf("queued snapshots differ\nBPF: %v\nref: %v", bpfQueue, refQueue)
	}
	s, _ := c.Stats()
	if got := SlotStat(s, 1, StatQueueOK); got != uint64(len(bpfQueue)) || SlotStat(s, 1, StatFaults) != 0 {
		t.Errorf("slot 1 queue_ok %d, jobs %d, faults %d", got, len(bpfQueue), SlotStat(s, 1, StatFaults))
	}
	for _, f := range flows {
		key := FlowKey{Sport: f.sport, Dport: f.dport, Proto: f.proto, L3: 4, Slot: 1}
		copy(key.Src[:], ipA[:])
		copy(key.Dst[:], ipB[:])
		st, ok, err := c.State(key)
		if err != nil || !ok {
			t.Fatalf("%s: no sampler state (%v)", f.label, err)
		}
		if fmt.Sprintf("%08x", st.Hash()) != refFinal[f.label] {
			t.Errorf("%s: BPF hash %08x, reference %s", f.label, st.Hash(), refFinal[f.label])
		}
	}
	t.Logf("SAMPLER BPF==reference: %d events, %d queued snapshots, 3/3 flow hashes; rate limiter 600/600 verdicts == qos-ebpf math", len(bpfEmits), len(bpfQueue))
}

// TestSampler_QueueOverflow (Steps 362-363): N = 1 queues every packet; past
// SCORE_QUEUE's 16,384 entries pushes are counted drops, never faults, and
// every packet still passes.
func TestSampler_QueueOverflow(t *testing.T) {
	c := loadCart(t)
	mustSlot(t, c, 0, samplerImage(t, 1), 1)
	const n = 20_000
	if _, _, err := c.BenchXDP(IPv4Packet(17, ipA, ipB, 7000, 53, 100), n); err != nil {
		t.Fatal(err)
	}
	s, _ := c.Stats()
	ok, drops := SlotStat(s, 0, StatQueueOK), SlotStat(s, 0, StatQueueDrops)
	t.Logf("OVERFLOW runs=%d pass=%d faults=%d queue_ok=%d queue_drops=%d events_dropped=%d",
		SlotStat(s, 0, StatRuns), SlotStat(s, 0, StatPass), SlotStat(s, 0, StatFaults), ok, drops, s[StatEventsDrop])
	if ok != 16_384 || drops != n-16_384 || SlotStat(s, 0, StatFaults) != 0 || SlotStat(s, 0, StatPass) != n {
		t.Fatalf("want queue_ok 16384, drops %d, no faults, all pass", n-16_384)
	}
	popped := 0
	for {
		_, more, err := c.PopJob()
		if err != nil {
			t.Fatal(err)
		}
		if !more {
			break
		}
		popped++
	}
	if popped != 16_384 {
		t.Fatalf("popped %d", popped)
	}
}

// TestBench_Sampler (Steps 358-359, CART_BENCH=1): the sampler alone, the
// rate limiter alone (pass regime), and both slots, on one IPv4 TCP frame.
// The queue and event ring fill during the run (nothing drains them): the
// steady state is the counted-drop path.
func TestBench_Sampler(t *testing.T) {
	benchEnabled(t)
	c := loadCart(t)
	pkt := tcpPacket(4000, 443, 0x18, 128)
	xdp := func() (time.Duration, error) {
		_, d, err := c.BenchXDP(pkt, benchRepeat)
		return d, err
	}
	pass := buildCart(t, "../../cartridges/pass.s", "xdp")
	rl := rlImage(t, 8000, 1_000_000_000)
	sp := samplerImage(t, 8)
	w := mapWriter{c}
	off := func(slot int) {
		if err := w.putSlot(slot, SlotCfg{}); err != nil {
			t.Fatal(err)
		}
	}
	mustSlot(t, c, 0, pass, 1)
	b0 := bench5(t, xdp)
	logBench(t, "b0_pass_tcp", b0)
	mustSlot(t, c, 0, sp, 2)
	sOnly := bench5(t, xdp)
	logBench(t, "sampler", sOnly)
	mustSlot(t, c, 0, rl, 3)
	rOnly := bench5(t, xdp)
	logBench(t, "ratelimit_pass_tcp", rOnly)
	mustSlot(t, c, 1, sp, 4)
	both := bench5(t, xdp)
	logBench(t, "ratelimit+sampler", both)
	off(0)
	off(1)
	s, _ := c.Stats()
	t.Logf("BENCH sampler insns/pkt=%.1f", float64(SlotStat(s, 1, StatInsns))/float64(SlotStat(s, 1, StatRuns)))
	ov := func(x benchResult) int64 { return x.median.Nanoseconds() - b0.median.Nanoseconds() }
	t.Logf("BENCH GATE2 sampler overhead %d ns (gate 120); both %d ns vs sum %d ns (+10%% = %d)",
		ov(sOnly), ov(both), ov(sOnly)+ov(rOnly), (ov(sOnly)+ov(rOnly))*11/10)
}
