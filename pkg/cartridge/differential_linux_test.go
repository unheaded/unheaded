// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

//go:build linux

package cartridge

// TestDifferential_ThreeEngines (root-gated, S5 Phase 17 Steps 424-426): the
// three-engine oracle the Lich sprint drives. `cart diff --out` generates
// random validated XDP-stage cartridges and runs each in lockstep on the
// shared step (monad_common::cart) and the host MBC emulator (monad-mbc
// execute.rs); it refuses to write a case the two disagree on. This test
// runs every case through the third engine, cart_xdp via BPF_PROG_TEST_RUN
// (nothing attached), and checks the verdict, the FLOW_STATE hash and the
// EMIT records against what the two host engines agreed on.
//
// CART_DIFF_COUNT (default 1000) and CART_DIFF_SEED (default 0x17d1ff) set
// the campaign.

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

type diffCase struct {
	file    string
	sport   uint16
	now     uint64
	end     string
	verdict uint32
	hash    uint32
	emits   string
}

func parseDiffManifest(t *testing.T, b []byte) []diffCase {
	t.Helper()
	var out []diffCase
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) != 7 {
			t.Fatalf("manifest line %q", sc.Text())
		}
		kv := map[string]string{}
		for _, x := range f[1:] {
			k, v, ok := strings.Cut(x, "=")
			if !ok {
				t.Fatalf("manifest field %q", x)
			}
			kv[k] = v
		}
		sport, err1 := strconv.ParseUint(kv["sport"], 10, 16)
		now, err2 := strconv.ParseUint(kv["now"], 10, 64)
		v, err3 := strconv.ParseUint(kv["verdict"], 10, 32)
		h, err4 := strconv.ParseUint(kv["hash"], 16, 32)
		for _, err := range []error{err1, err2, err3, err4} {
			if err != nil {
				t.Fatalf("manifest line %q: %v", sc.Text(), err)
			}
		}
		out = append(out, diffCase{f[0], uint16(sport), now, kv["end"], uint32(v), uint32(h), kv["emits"]})
	}
	return out
}

func eventsString(evs []Event) string {
	if len(evs) == 0 {
		return "-"
	}
	parts := make([]string, len(evs))
	for i, e := range evs {
		parts[i] = fmt.Sprintf("%d:%d:%d", e.Kind, e.A, e.B)
	}
	return strings.Join(parts, ",")
}

func TestDifferential_ThreeEngines(t *testing.T) {
	c := loadCart(t)
	n, sd := uint64(1000), uint64(0x17d1ff)
	if v := os.Getenv("CART_DIFF_COUNT"); v != "" {
		p, err := strconv.ParseUint(v, 0, 32)
		if err != nil || p == 0 || p > 50_000 {
			t.Fatalf("CART_DIFF_COUNT=%q: want 1..50000", v)
		}
		n = p
	}
	if v := os.Getenv("CART_DIFF_SEED"); v != "" {
		p, err := strconv.ParseUint(v, 0, 64)
		if err != nil {
			t.Fatalf("CART_DIFF_SEED=%q: %v", v, err)
		}
		sd = p
	}
	count, seed := strconv.FormatUint(n, 10), "0x"+strconv.FormatUint(sd, 16)
	dir := t.TempDir()
	out, err := exec.Command(cartBin(t), "diff", "--seed", seed, "--count", count, "--out", dir).CombinedOutput() // #nosec G204 G702 -- test-only, fixed binary, numeric args
	if err != nil {
		t.Fatalf("cart diff (shared step vs host emulator): %v\n%s", err, out)
	}
	t.Logf("%s", bytes.TrimSpace(out))
	man, err := os.ReadFile(filepath.Join(dir, "manifest.txt")) // #nosec G304 -- test temp file
	if err != nil {
		t.Fatal(err)
	}
	cases := parseDiffManifest(t, man)
	if n, _ := strconv.Atoi(count); len(cases) != n {
		t.Fatalf("manifest has %d cases, want %s", len(cases), count)
	}
	rd, err := c.Events()
	if err != nil {
		t.Fatal(err)
	}
	defer rd.Close()

	agree, ends := 0, map[string]int{}
	for i, dc := range cases {
		raw, err := os.ReadFile(filepath.Join(dir, dc.file)) // #nosec G304 -- test temp file
		if err != nil {
			t.Fatal(err)
		}
		gen := uint32(i + 1) // #nosec G115 -- i < 100,000
		mustSlot(t, c, 0, raw, gen)
		if err := c.SetCfg(CfgClockNS, dc.now); err != nil {
			t.Fatal(err)
		}
		r, err := c.RunXDP(IPv4Packet(17, ipA, ipB, dc.sport, 53, 80))
		if err != nil {
			t.Fatal(err)
		}
		want := uint32(xdpPass)
		if dc.verdict == 1 {
			want = xdpDrop
		}
		key := FlowKey{Sport: dc.sport, Dport: 53, Proto: 17, L3: 4, Slot: 0}
		copy(key.Src[:], ipA[:])
		copy(key.Dst[:], ipB[:])
		st, ok, err := c.State(key)
		if err != nil || !ok {
			t.Fatalf("%s: no BPF flow state (%v)", dc.file, err)
		}
		evs, err := c.DrainEvents(rd)
		if err != nil {
			t.Fatal(err)
		}
		bad := []string{}
		if r != want {
			bad = append(bad, fmt.Sprintf("xdp ret %d, want %d", r, want))
		}
		if st.Hash() != dc.hash {
			bad = append(bad, fmt.Sprintf("state hash %08x, host engines %08x (pc %d regs %x)", st.Hash(), dc.hash, st.PC, st.Regs))
		}
		if st.Gen != gen {
			bad = append(bad, fmt.Sprintf("gen %d, want %d", st.Gen, gen))
		}
		if got := eventsString(evs); got != dc.emits {
			bad = append(bad, fmt.Sprintf("emits %s, host engines %s", got, dc.emits))
		}
		if len(bad) > 0 {
			t.Errorf("%s (%s, image %x): %s", dc.file, dc.end, raw, strings.Join(bad, "; "))
			continue
		}
		agree++
		ends[strings.SplitN(dc.end, ":", 2)[0]]++
	}
	s, err := c.Stats()
	if err != nil {
		t.Fatal(err)
	}
	if s[StatBreakerTrips] != 0 {
		t.Errorf("breaker tripped %d times (cases are spaced 2 s apart, one fault window each)", s[StatBreakerTrips])
	}
	t.Logf("three engines (shared step, host emulator, BPF cart_xdp) agree on %d/%d cartridges; ends %v; faults %d",
		agree, len(cases), ends, SlotStat(s, 0, StatFaults))
}
