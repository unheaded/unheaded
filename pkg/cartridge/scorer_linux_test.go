// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

//go:build linux

package cartridge

// S5 Phase 15: the int8 scorer (cartridges/scorer.s, tick stage) fed by the
// sampler through SCORE_QUEUE, against the Rust reference (`cart tick`).

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// ScoreKind is the scorer's EMIT kind ('A'); a = score 0..100, b = model id.
const ScoreKind = 0x41

func scorerImage(t *testing.T) []byte {
	t.Helper()
	return buildCart(t, "../../cartridges/scorer.s", "tick", "--loop-bound", "16", "--data", "../../cartridges/scorer.data.bin")
}

// cartTickLines runs `cart tick` on jobs ("label f0..f7" per line).
func cartTickLines(t *testing.T, raw []byte, jobs string) []string {
	t.Helper()
	dir := t.TempDir()
	img, lp := filepath.Join(dir, "s.cart"), filepath.Join(dir, "jobs.log")
	if err := os.WriteFile(img, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lp, []byte(jobs), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(cartBin(t), "tick", img, "--jobs", lp).CombinedOutput() // #nosec G204 -- test-only
	if err != nil {
		t.Fatalf("cart tick: %v\n%s", err, out)
	}
	return strings.Split(strings.TrimSpace(string(out)), "\n")
}

// synthPkt is one packet of a test flow.
type synthPkt struct {
	gap   uint64
	n     int
	flags uint8
}

// TestScorer_EndToEndBPF (Steps 375-376): rate limiter (slot 0), sampler
// (slot 1, N = 8) and scorer (slot 2, tick). Three flows through cart_xdp,
// then the pump drains SCORE_QUEUE through cart_tick. Every queued job gets
// exactly one score event; scores, their order and the last job's state hash
// equal the reference.
func TestScorer_EndToEndBPF(t *testing.T) {
	c := loadCart(t)
	mustSlot(t, c, 0, rlImage(t, 8000, 1_000_000_000), 1)
	samp := samplerImage(t, 8)
	mustSlot(t, c, 1, samp, 1)
	sc := scorerImage(t)
	mustSlot(t, c, 2, sc, 1)
	rd, err := c.Events()
	if err != nil {
		t.Fatal(err)
	}
	defer rd.Close()

	seed := uint64(0xA11CE)
	next := func() uint64 { seed ^= seed << 13; seed ^= seed >> 7; seed ^= seed << 17; return seed }
	type tflow struct {
		label string
		proto uint8
		sport uint16
		dport uint16
		pkts  []synthPkt
	}
	mk := func(n int, f func(i int) synthPkt) []synthPkt {
		out := make([]synthPkt, n)
		for i := range out {
			out[i] = f(i)
		}
		return out
	}
	flows := []*tflow{
		{"web", 6, 41000, 443, mk(48, func(i int) synthPkt {
			if i == 0 {
				return synthPkt{1_000_000, 60, 0x02}
			}
			if next()%3 == 0 {
				return synthPkt{200_000 + next()%5_000_000, 200 + int(next()%700), 0x18}
			}
			return synthPkt{200_000 + next()%5_000_000, 60, 0x10}
		})},
		{"synflood", 6, 41001, 80, mk(48, func(int) synthPkt { return synthPkt{500 + next()%20_000, 60, 0x02} })},
		{"udpflood", 17, 41002, 40000, mk(48, func(int) synthPkt { return synthPkt{500 + next()%10_000, 1000 + int(next()%470), 0} })},
	}
	// Interleave the three flows on one clock.
	type ev struct {
		at uint64
		f  *tflow
		p  synthPkt
	}
	var evs []ev
	for _, f := range flows {
		at := uint64(9_000_000_000)
		for _, p := range f.pkts {
			at += p.gap
			evs = append(evs, ev{at, f, p})
		}
	}
	sort.SliceStable(evs, func(i, j int) bool { return evs[i].at < evs[j].at })
	sims := map[string]*simFlow{}
	for _, f := range flows {
		sims[f.label] = &simFlow{label: f.label, sport: f.sport, dport: f.dport}
	}
	var log bytes.Buffer
	for _, e := range evs {
		if err := c.SetCfg(CfgClockNS, e.at); err != nil {
			t.Fatal(err)
		}
		var pkt []byte
		if e.f.proto == 6 {
			pkt = tcpPacket(e.f.sport, e.f.dport, e.p.flags, e.p.n)
		} else {
			pkt = IPv4Packet(17, ipA, ipB, e.f.sport, e.f.dport, e.p.n)
		}
		if _, err := c.RunXDP(pkt); err != nil {
			t.Fatal(err)
		}
		fields := sims[e.f.label].fields(e.at, e.p.n)
		fields[1], fields[6] = uint32(e.f.proto), uint32(e.p.flags)
		if e.f.proto == 17 {
			fields[6] = 0
		}
		fmt.Fprintf(&log, "%s %d", e.f.label, e.at)
		for _, v := range fields {
			fmt.Fprintf(&log, " %d", v)
		}
		log.WriteString("\n")
	}
	// Reference: the sampler's queued snapshots (in queue order), then the
	// scorer on each.
	var jobs strings.Builder
	for _, l := range cartRunLines(t, samp, log.Bytes()) {
		f := strings.Fields(l)
		if f[0] == "queue" {
			jobs.WriteString(strings.TrimPrefix(f[1], "flow=") + " " + strings.Join(f[2:], " ") + "\n")
		}
	}
	var refScores []string
	lastHash := ""
	for _, l := range cartTickLines(t, sc, jobs.String()) {
		f := strings.Fields(l)
		switch f[0] {
		case "emit":
			refScores = append(refScores, fmt.Sprintf("%s %s %s %s", strings.TrimPrefix(f[1], "job="), f[2], f[3], f[4]))
		case "job":
			lastHash = strings.TrimPrefix(f[4], "hash=")
		}
	}
	s0, _ := c.Stats()
	queued := SlotStat(s0, 1, StatQueueOK)
	calls, err := c.Pump(100_000)
	if err != nil {
		t.Fatal(err)
	}
	s1, _ := c.Stats()
	got, err := c.DrainEvents(rd)
	if err != nil {
		t.Fatal(err)
	}
	byPort := map[uint16]string{41000: "web", 41001: "synflood", 41002: "udpflood"}
	var bpfScores []string
	perFlow := map[string][]uint32{}
	for _, e := range got {
		if e.Stage != StageTick {
			continue
		}
		if e.Kind != ScoreKind || e.Slot != 2 {
			t.Errorf("tick event kind %#x slot %d", e.Kind, e.Slot)
		}
		lab := byPort[e.Flow.Sport]
		bpfScores = append(bpfScores, fmt.Sprintf("%s %d %d %d", lab, e.Kind, e.A, e.B))
		perFlow[lab] = append(perFlow[lab], e.A)
	}
	if uint64(len(bpfScores)) != queued || s1[StatTickJobsDone]-s0[StatTickJobsDone] != queued {
		t.Errorf("queued %d jobs, %d score events, %d jobs done", queued, len(bpfScores), s1[StatTickJobsDone]-s0[StatTickJobsDone])
	}
	if strings.Join(bpfScores, "|") != strings.Join(refScores, "|") {
		t.Errorf("scores differ\nBPF: %v\nref: %v", bpfScores, refScores)
	}
	tj, err := c.TickJob()
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprintf("%08x", tj.State.Hash()) != lastHash || tj.Active != 0 {
		t.Errorf("last job: BPF hash %08x active %d, reference %s", tj.State.Hash(), tj.Active, lastHash)
	}
	if SlotStat(s1, 2, StatFaults) != 0 {
		t.Errorf("scorer faults: %d", SlotStat(s1, 2, StatFaults))
	}
	t.Logf("SCORER BPF: %d jobs queued, %d score events in %d cart_tick calls, all == reference; last-job hash %s; scores %v", queued, len(bpfScores), calls, lastHash, perFlow)
}

// TestBench_Scorer (Steps 377-378, CART_BENCH=1): tick-stage throughput
// (flows scored per second on one core) with SCORE_QUEUE prefilled, and the
// sampler's XDP cost with and without the scorer slot loaded.
func TestBench_Scorer(t *testing.T) {
	benchEnabled(t)
	c := loadCart(t)
	mustSlot(t, c, 2, scorerImage(t), 1)
	job := ScoreJob{Feat: [FeatWords]uint32{1709, 60, 1, 656, 8, 6, 443, 26}, SrcSlot: 1}
	const jobs = 16_000
	var rates []float64
	for run := 0; run < 5; run++ {
		for i := 0; i < jobs; i++ {
			job.Flow.Sport = uint16(i) // #nosec G115 -- i < 16,000
			if err := c.PushJob(job); err != nil {
				t.Fatal(err)
			}
		}
		s0, _ := c.Stats()
		d, err := c.BenchTick(3 * jobs) // 3 calls per job (scorer: <= 3 tick calls)
		if err != nil {
			t.Fatal(err)
		}
		s1, _ := c.Stats()
		done := s1[StatTickJobsDone] - s0[StatTickJobsDone]
		total := float64(d.Nanoseconds()) * 3 * jobs / 1e9
		rates = append(rates, float64(done)/total)
		t.Logf("BENCH scorer run %d: %d jobs in %d calls, %d ns/call, %.0f flows/s", run, done, 3*jobs, d.Nanoseconds(), float64(done)/total)
		for {
			if _, ok, _ := c.PopJob(); !ok {
				break
			}
		}
	}
	sort.Float64s(rates)
	t.Logf("BENCH scorer median %.0f flows/s (min %.0f, max %.0f; gate 100,000)", rates[2], rates[0], rates[4])

	// XDP path: the sampler alone vs the sampler with the scorer slot active.
	pkt := tcpPacket(4000, 443, 0x18, 128)
	xdp := func() (time.Duration, error) {
		_, d, err := c.BenchXDP(pkt, benchRepeat)
		return d, err
	}
	mustSlot(t, c, 0, samplerImage(t, 8), 1)
	with := bench5(t, xdp)
	logBench(t, "sampler_with_scorer_slot", with)
	if err := c.SetSlotActive(2, false); err != nil {
		t.Fatal(err)
	}
	without := bench5(t, xdp)
	logBench(t, "sampler_without_scorer_slot", without)
}
