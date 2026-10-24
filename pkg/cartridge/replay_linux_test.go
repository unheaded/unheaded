// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

//go:build linux

package cartridge

// S5 Phase 16: live runs traced by cartridge-ebpf (CART_TRACE), written as a
// UCTR trace, replayed off-line by `cart replay` (Rust, the same step
// function): every state hash, every verdict, the scorer's tick calls.

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// cartReplay runs `cart replay` and returns its output and exit code.
func cartReplay(t *testing.T, img []byte, trace []byte, args ...string) (string, int) {
	t.Helper()
	dir := t.TempDir()
	ip, tp := filepath.Join(dir, "x.cart"), filepath.Join(dir, "run.anam")
	if err := os.WriteFile(ip, img, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tp, trace, 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(cartBin(t), append([]string{"replay", ip, tp}, args...)...) // #nosec G204 -- test-only
	out, err := cmd.CombinedOutput()
	code := 0
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return string(out), code
}

var outcomeRE = regexp.MustCompile(`outcome=(\S+)`)

// TestReplay_LiveRunsAreBitExact (Steps 400, 406-411): trace off by default;
// a live rate limiter + sampler + scorer run recorded with tracing on
// replays bit for bit, verdict for verdict; a corrupted input byte is
// flagged at the exact step; 1-in-N sampling bounds the volume.
func TestReplay_LiveRunsAreBitExact(t *testing.T) {
	c := loadCart(t)
	rl := rlImage(t, 10, 3000)
	sp := samplerImage(t, 8)
	sc := scorerImage(t)
	mustSlot(t, c, 0, rl, 1)
	mustSlot(t, c, 1, sp, 1)
	mustSlot(t, c, 2, sc, 1)
	rd, err := c.Trace()
	if err != nil {
		t.Fatal(err)
	}
	defer rd.Close()

	send := func(n int, now *uint64, sport uint16, verdicts *[]uint32) {
		seed := uint64(sport) * 0x9E3779B97F4A7C15
		for i := 0; i < n; i++ {
			seed ^= seed << 13
			seed ^= seed >> 7
			seed ^= seed << 17
			*now += seed % 30_000
			if err := c.SetCfg(CfgClockNS, *now); err != nil {
				t.Fatal(err)
			}
			r, err := c.RunXDP(IPv4Packet(17, ipA, ipB, sport, 40000, 200+int(seed%1300)))
			if err != nil {
				t.Fatal(err)
			}
			if verdicts != nil {
				*verdicts = append(*verdicts, r)
			}
		}
	}
	now := uint64(11_000_000_000)

	// Step 400: off by default: nothing is recorded.
	send(20, &now, 1111, nil)
	if recs, _ := c.DrainTrace(rd); len(recs) != 0 {
		t.Fatalf("tracing is off by default, got %d records", len(recs))
	}

	// Trace every run.
	if err := c.SetCfg(CfgTrace, 1); err != nil {
		t.Fatal(err)
	}
	var verdicts []uint32
	var recs [][]byte
	for k := 0; k < 4; k++ {
		send(40, &now, 2222, &verdicts)
		more, err := c.DrainTrace(rd)
		if err != nil {
			t.Fatal(err)
		}
		recs = append(recs, more...)
	}
	calls, err := c.Pump(1_000)
	if err != nil {
		t.Fatal(err)
	}
	more, _ := c.DrainTrace(rd)
	recs = append(recs, more...)
	if err := c.SetCfg(CfgTrace, 0); err != nil {
		t.Fatal(err)
	}
	s, _ := c.Stats()
	if s[StatTraceDropped] != 0 {
		t.Fatalf("%d trace records dropped", s[StatTraceDropped])
	}
	var shas [Slots][32]byte
	shas[0], shas[1], shas[2] = sha256.Sum256(rl), sha256.Sum256(sp), sha256.Sum256(sc)
	trace := EncodeTrace(shas, recs)
	heads := map[int]int{}
	for _, r := range recs {
		if s := TraceRecordSlot(r); s >= 0 {
			heads[s]++
		}
	}
	t.Logf("TRACE %d records (%d bytes): heads per slot %v; %d tick calls pumped", len(recs), len(trace), heads, calls)

	// Rate limiter: every run bit-exact, outcomes == live verdicts, final
	// state hash == the live flow state.
	out, code := cartReplay(t, rl, trace)
	if code != 0 || !strings.Contains(out, "160 bit-exact, 0 diverged, 0 incomplete") {
		t.Fatalf("ratelimit replay (exit %d):\n%s", code, tail(out))
	}
	var replayed []uint32
	for _, m := range outcomeRE.FindAllStringSubmatch(out, -1) {
		switch m[1] {
		case "Verdict(0)":
			replayed = append(replayed, xdpPass)
		case "Verdict(1)":
			replayed = append(replayed, xdpDrop)
		default:
			replayed = append(replayed, 0)
		}
	}
	if fmt.Sprint(replayed) != fmt.Sprint(verdicts) {
		t.Errorf("replayed verdicts differ from live:\nlive   %v\nreplay %v", verdicts, replayed)
	}
	// Sampler (slot 1) and scorer (slot 2) runs too.
	if out, code := cartReplay(t, sp, trace); code != 0 || !strings.Contains(out, "160 bit-exact, 0 diverged") {
		t.Fatalf("sampler replay (exit %d):\n%s", code, tail(out))
	}
	// Time travel on the scorer: the MLP row accumulator r1 across the
	// second tick call of the first job, then the state just before its EMIT.
	out, code = cartReplay(t, sc, trace, "--run", "1", "--watch", "r1")
	if code != 0 || !strings.Contains(out, "1 bit-exact") || strings.Count(out, "  @") < 40 {
		t.Fatalf("scorer replay (exit %d):\n%s", code, tail(out))
	}
	t.Logf("REPLAY scorer run 1 (second tick call of job 1), row accumulator r1 (first lines):\n%s", head(out, 12))
	out, code = cartReplay(t, sc, trace, "--run", "2", "--to", "188")
	if code != 0 || !strings.Contains(out, "SYSCALL 3 (EMIT)") {
		t.Fatalf("scorer replay --to (exit %d):\n%s", code, tail(out))
	}
	all, code := cartReplay(t, sc, trace)
	if code != 0 || strings.Contains(all, "DIVERGED") {
		t.Fatalf("scorer replay (exit %d):\n%s", code, tail(all))
	}
	t.Logf("REPLAY scorer run 2 at the step before EMIT:\n%s", out)

	// Step 410-411: flip one bit of one run's pkt_len input in the file.
	bad := append([]byte(nil), trace...)
	off, run := 8+4*32, 0
	for off < len(bad) {
		n := int(binary.LittleEndian.Uint32(bad[off:]))
		rec := bad[off+4 : off+4+n]
		if TraceRecordSlot(rec) == 0 {
			if run == 9 {
				rec[72+4*4] ^= 0x10 // FIELD 4 (pkt_len)
				break
			}
			run++
		}
		off += 4 + n
	}
	out, code = cartReplay(t, rl, bad)
	if code != 3 || !strings.Contains(out, "run 9 ") || !regexp.MustCompile(`run 9 .* DIVERGED at step 2 \(hash\)`).MatchString(out) ||
		!strings.Contains(out, "159 bit-exact, 1 diverged") {
		t.Fatalf("corrupted input not flagged at step 2 (exit %d):\n%s", code, tail(out))
	}
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(l, "run 9 ") {
			t.Logf("REPLAY corrupted: %s", l)
		}
	}

	// Sampling: one run in ten.
	if err := c.SetCfg(CfgTrace, 10); err != nil {
		t.Fatal(err)
	}
	send(100, &now, 3333, nil) // 200 slot runs (rate limiter + sampler)
	_ = c.SetCfg(CfgTrace, 0)
	sampled, _ := c.DrainTrace(rd)
	h := 0
	for _, r := range sampled {
		if TraceRecordSlot(r) >= 0 {
			h++
		}
	}
	t.Logf("TRACE 1-in-10: %d traced runs of 200", h)
	if h < 10 || h > 30 {
		t.Errorf("1-in-10 sampling traced %d of 200 runs", h)
	}
}

func head(s string, n int) string {
	l := strings.Split(strings.TrimSpace(s), "\n")
	if len(l) > n {
		l = l[:n]
	}
	return strings.Join(l, "\n")
}

func tail(s string) string {
	l := strings.Split(strings.TrimSpace(s), "\n")
	if len(l) > 12 {
		l = l[len(l)-12:]
	}
	return strings.Join(l, "\n")
}

func TestEncodeTrace(t *testing.T) {
	head := make([]byte, TraceHeadSize)
	head[0], head[1], head[2], head[4] = TraceEventComputed, TraceSubkind, 0, 2
	step := make([]byte, TraceStepSize)
	step[0], step[1], step[2] = TraceEventComputed, TraceSubkind, 1
	junk := bytes.Repeat([]byte{7}, 32)
	var shas [Slots][32]byte
	shas[2][0] = 0xAB
	b := EncodeTrace(shas, [][]byte{head, junk, step})
	if string(b[:4]) != "UCTR" || binary.LittleEndian.Uint32(b[4:]) != 1 || b[8+64] != 0xAB {
		t.Fatalf("header %x", b[:16])
	}
	if want := 8 + 128 + 4 + TraceHeadSize + 4 + TraceStepSize; len(b) != want {
		t.Fatalf("len %d, want %d (junk skipped)", len(b), want)
	}
	if TraceRecordSlot(head) != 2 || TraceRecordSlot(step) != -1 || IsTraceRecord(junk) {
		t.Fatal("record classification")
	}
}
