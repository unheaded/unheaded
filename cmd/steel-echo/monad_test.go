// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"unheaded/pkg/monadpkt"
)

func mustIP(s string) net.IP { return net.ParseIP(s) }

func TestParseSendFlags(t *testing.T) {
	c, err := parseSendFlags([]string{"--iface", "steel0-h", "--dst", "fd00:5ee1::2", "--flow-label", "0x5EE1", "--count", "1000"})
	if err != nil {
		t.Fatal(err)
	}
	if c.iface != "steel0-h" || c.flowLabel != 0x5EE1 || c.count != 1000 || c.interval != time.Millisecond || !c.dst.Equal(mustIP("fd00:5ee1::2")) || !c.src.Equal(mustIP("fd00:5ee1::1")) {
		t.Fatalf("cfg = %+v", c)
	}
	frame, err := c.frame([6]byte{2, 0, 0, 0, 0, 7})
	if err != nil {
		t.Fatal(err)
	}
	in, err := monadpkt.Parse(frame)
	if err != nil || in.FlowLabel != 0x5EE1 || in.Chaos || !in.CRCOK {
		t.Fatalf("frame = %+v %v", in, err)
	}
	if !bytes.Equal(frame[6:12], []byte{2, 0, 0, 0, 0, 7}) || !bytes.Equal(frame[0:6], monadpkt.DefaultDstMAC[:]) {
		t.Fatalf("macs = % x", frame[:12])
	}
	c, err = parseSendFlags([]string{"--iface", "steel1-n", "--dst", "fd00::2", "--flow-label", "7", "--dst-mac", "aa:bb:cc:dd:ee:ff", "--size", "200", "--interval", "0"})
	if err != nil || c.dstMAC != [6]byte{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff} || c.size != 200 {
		t.Fatalf("cfg = %+v %v", c, err)
	}
	bad := map[string][]string{
		"host nic":      {"--iface", "enp16s0", "--dst", "fd00::2", "--flow-label", "1"},
		"no iface":      {"--dst", "fd00::2", "--flow-label", "1"},
		"no dst":        {"--iface", "steel0-h", "--flow-label", "1"},
		"ipv4 dst":      {"--iface", "steel0-h", "--dst", "10.0.0.1", "--flow-label", "1"},
		"no label":      {"--iface", "steel0-h", "--dst", "fd00::2"},
		"label 0":       {"--iface", "steel0-h", "--dst", "fd00::2", "--flow-label", "0"},
		"label wide":    {"--iface", "steel0-h", "--dst", "fd00::2", "--flow-label", "0x100000"},
		"count 0":       {"--iface", "steel0-h", "--dst", "fd00::2", "--flow-label", "1", "--count", "0"},
		"count huge":    {"--iface", "steel0-h", "--dst", "fd00::2", "--flow-label", "1", "--count", "1000001"},
		"interval long": {"--iface", "steel0-h", "--dst", "fd00::2", "--flow-label", "1", "--interval", "2s"},
		"bad mac":       {"--iface", "steel0-h", "--dst", "fd00::2", "--flow-label", "1", "--dst-mac", "zz"},
		"svc 256":       {"--iface", "steel0-h", "--dst", "fd00::2", "--flow-label", "1", "--src-svc", "256"},
		"size small":    {"--iface", "steel0-h", "--dst", "fd00::2", "--flow-label", "1", "--size", "60"},
		"size jumbo":    {"--iface", "steel0-h", "--dst", "fd00::2", "--flow-label", "1", "--size", "1501"},
		"stray arg":     {"--iface", "steel0-h", "--dst", "fd00::2", "--flow-label", "1", "x"},
	}
	for name, args := range bad {
		if _, err := parseSendFlags(args); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestParseCountFlags(t *testing.T) {
	c, err := parseCountFlags([]string{"--iface", "steel0-n", "--seconds", "30"})
	if err != nil || c.iface != "steel0-n" || c.seconds != 30 {
		t.Fatalf("cfg = %+v %v", c, err)
	}
	for name, args := range map[string][]string{
		"host nic":  {"--iface", "lo"},
		"no iface":  {"--seconds", "1"},
		"seconds 0": {"--iface", "steel0-n", "--seconds", "0"},
		"too long":  {"--iface", "steel0-n", "--seconds", "3601"},
		"stray":     {"--iface", "steel0-n", "x"},
	} {
		if _, err := parseCountFlags(args); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

type sink struct {
	frames [][]byte
	fail   error
	n      atomic.Int64
}

func (s *sink) Write(b []byte) error {
	if s.fail != nil {
		return s.fail
	}
	s.frames = append(s.frames, append([]byte{}, b...))
	s.n.Add(1)
	return nil
}

func TestSendFrames(t *testing.T) {
	f, _ := monadpkt.Build(monadpkt.Frame{FlowLabel: 0x5EE1})
	s := &sink{}
	n, err := sendFrames(context.Background(), s, f, 5, 0)
	if err != nil || n != 5 || len(s.frames) != 5 || !bytes.Equal(s.frames[4], f) {
		t.Fatalf("sent %d %v", n, err)
	}
	// Spacing is honoured.
	s = &sink{}
	start := time.Now()
	if n, err := sendFrames(context.Background(), s, f, 4, 20*time.Millisecond); err != nil || n != 4 {
		t.Fatalf("sent %d %v", n, err)
	}
	if d := time.Since(start); d < 55*time.Millisecond {
		t.Fatalf("4 frames 20ms apart took %v", d)
	}
	// A cancelled context stops before the next write and reports what went out.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if n, err := sendFrames(ctx, &sink{}, f, 5, time.Millisecond); !errors.Is(err, context.Canceled) || n != 0 {
		t.Fatalf("cancelled: sent %d %v", n, err)
	}
	ctx, cancel = context.WithCancel(context.Background())
	s = &sink{}
	go func() {
		for s.n.Load() == 0 {
			time.Sleep(time.Millisecond)
		}
		cancel()
	}()
	if n, err := sendFrames(ctx, s, f, 1000, 10*time.Millisecond); !errors.Is(err, context.Canceled) || n == 0 || n == 1000 {
		t.Fatalf("cancelled mid-run: sent %d %v", n, err)
	}
	// A write error is returned.
	if _, err := sendFrames(context.Background(), &sink{fail: errors.New("ENETDOWN")}, f, 5, 0); err == nil {
		t.Fatal("write error swallowed")
	}
}

type source struct {
	frames [][]byte
}

func (s *source) Read(ctx context.Context) ([]byte, error) {
	if len(s.frames) == 0 {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	b := s.frames[0]
	s.frames = s.frames[1:]
	return b, nil
}

func TestCountFrames_PerFlowChaosAndCRC(t *testing.T) {
	enrolled, _ := monadpkt.Build(monadpkt.Frame{FlowLabel: 0x5EE1, Flags: monadpkt.FlagChaos})
	plain, _ := monadpkt.Build(monadpkt.Frame{FlowLabel: 0x5EE2})
	stale := append([]byte{}, enrolled...)
	stale[monadpkt.MonadOff+3] ^= 1 // bitflip mode: CHAOS set, CRC stale
	nd := make([]byte, 86)
	binary.BigEndian.PutUint16(nd[12:], 0x86DD)
	nd[14], nd[14+6] = 0x60, 58 // ICMPv6, no HbH
	short := []byte{1, 2, 3}
	// The counter gets header bytes only, as the socket delivers them.
	src := &source{frames: [][]byte{enrolled[:monadpkt.HeaderLen], plain[:monadpkt.HeaderLen], enrolled, stale, nd, short, plain}}
	r := newCountReport("steel0-n", 1)
	if err := countFrames(context.Background(), src, r, 200*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if r.Frames != 7 || r.NonMonad != 2 {
		t.Fatalf("frames %d non_monad %d", r.Frames, r.NonMonad)
	}
	e := r.Flows["0x5ee1"]
	if e == nil || e.Received != 3 || e.ChaosSet != 3 || e.ChaosClear != 0 || e.CRCOK != 2 || e.CRCBad != 1 {
		t.Fatalf("0x5ee1 = %+v", e)
	}
	p := r.Flows["0x5ee2"]
	if p == nil || p.Received != 2 || p.ChaosSet != 0 || p.ChaosClear != 2 || p.CRCOK != 2 {
		t.Fatalf("0x5ee2 = %+v", p)
	}
	var out bytes.Buffer
	if err := r.write(&out); err != nil {
		t.Fatal(err)
	}
	var back countReport
	if err := json.Unmarshal(out.Bytes(), &back); err != nil || back.Iface != "steel0-n" || back.Flows["0x5ee1"].ChaosSet != 3 {
		t.Fatalf("json = %s (%v)", out.String(), err)
	}
	if s := r.summary(); s != "0x5ee1 received=3 chaos_set=3 chaos_clear=0 crc_bad=1; 0x5ee2 received=2 chaos_set=0 chaos_clear=2 crc_bad=0; non_monad=2" {
		t.Fatalf("summary = %q", s)
	}
}

func TestCountFrames_StopsAtTheDeadline(t *testing.T) {
	start := time.Now()
	r := newCountReport("steel0-n", 1)
	if err := countFrames(context.Background(), &source{}, r, 50*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d < 40*time.Millisecond || d > 2*time.Second {
		t.Fatalf("ran %v", d)
	}
}
